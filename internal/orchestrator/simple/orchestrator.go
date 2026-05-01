package simple

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
	"payment-gateway/internal/subsystems/adapter"
	"payment-gateway/internal/subsystems/antifraud"
	"payment-gateway/internal/subsystems/notifications"
	"payment-gateway/internal/subsystems/storage"
	"payment-gateway/internal/subsystems/tokenizer"
	"payment-gateway/internal/subsystems/validator"
)

type SimpleOrchestrator struct {
	validator     contracts.PaymentValidator
	antiFraud     contracts.AntiFraud
	tokenizer     contracts.Tokenizer
	notifications contracts.Notifications

	store  contracts.TransactionStore
	logger contracts.EventLogger

	// containers (5)
	stateManager contracts.TransactionStateManager
	router       contracts.PaymentRouter
	workflow     contracts.WorkflowEngine
	retry        contracts.RetryHandler
	callback     contracts.CallbackHandler
}

// NewSimpleOrchestrator создаёт оркестратор платежей.
// Если TransactionStore не передан, используется in-memory хранилище.
func NewSimpleOrchestrator(stores ...contracts.TransactionStore) *SimpleOrchestrator {
	store := storage.NewInMemoryTransactionStore()
	if len(stores) > 0 && stores[0] != nil {
		store = stores[0]
	}

	return &SimpleOrchestrator{
		validator:     validator.NewDummyValidator(),
		antiFraud:     antifraud.NewDummyAntiFraud(),
		tokenizer:     tokenizer.NewDummyTokenizer(),
		notifications: notifications.NewDummyNotifications(),

		store:  store,
		logger: noOpLogger{},

		stateManager: newInMemoryStateManager(),
		router:       newSimplePaymentRouter(),
		workflow:     newSimpleWorkflowEngine(),
		retry:        newSimpleRetryHandler(),
		callback:     newSimpleCallbackHandler(),
	}
}

func (o *SimpleOrchestrator) CreatePayment(ctx context.Context, req dto.CreatePaymentRequest) (dto.PaymentResponse, error) {
	if req.MerchantID == "" || req.IdempotencyKey == "" || req.PaymentID == "" {
		return dto.PaymentResponse{}, errors.New("merchant_id, idempotency_key и payment_id обязательны")
	}

	// 1) Idempotency: если этот idempotency_key уже был обработан — отдаём сохранённый response.
	if status, payloadJSON, found, err := o.store.GetByIdempotencyKey(ctx, req.MerchantID, req.IdempotencyKey); err != nil {
		return dto.PaymentResponse{}, err
	} else if found {
		if payloadJSON == "" {
			return dto.PaymentResponse{
				ID:             req.PaymentID,
				MerchantID:     req.MerchantID,
				IdempotencyKey: req.IdempotencyKey,
				CurrentStatus:  status,
				PaymentInfo: dto.PaymentInfoResponse{
					Amount:            req.PaymentInfo.Amount,
					PaymentMethodData: req.PaymentInfo.PaymentMethodData,
					CustomerData:      req.PaymentInfo.CustomerData,
					Description:       req.PaymentInfo.Description,
					CreatedAt:         req.PaymentInfo.CreatedAt,
					UpdatedAt:         nowUTC(),
				},
				TransactionDetails: dto.TransactionDetails{RetryCount: 0},
				Error:              nil,
			}, nil
		}

		var cached dto.PaymentResponse
		if err := json.Unmarshal([]byte(payloadJSON), &cached); err != nil {
			return dto.PaymentResponse{}, fmt.Errorf("failed to unmarshal cached response: %w", err)
		}
		return cached, nil
	}

	// 2) Workflow containers: start session / set status.
	sessionID, err := o.workflow.StartSession(ctx, req)
	if err != nil {
		return dto.PaymentResponse{}, err
	}

	_ = o.stateManager.SetStatus(ctx, req.MerchantID, req.PaymentID, statusPending)
	_ = o.logger.Log(ctx, contracts.PaymentEvent{
		Type:       contracts.EventPaymentReceived,
		MerchantID: req.MerchantID,
		PaymentID:  req.PaymentID,
		Timestamp:  nowUTC(),
		Details:    "received",
	})

	// 3) Validator.
	validatedReq, err := o.validator.Validate(ctx, req)
	if err != nil {
		return o.failAndSave(ctx, req, statusFailed, "VALIDATION_ERROR", err.Error())
	}
	_ = o.logger.Log(ctx, contracts.PaymentEvent{
		Type:       contracts.EventPaymentValidated,
		MerchantID: validatedReq.MerchantID,
		PaymentID:  validatedReq.PaymentID,
		Timestamp:  nowUTC(),
		Details:    "validated",
	})

	// 4) AntiFraud.
	fraudResult, err := o.antiFraud.Check(ctx, validatedReq)
	if err != nil {
		return o.failAndSave(ctx, req, statusFailed, "ANTIFRAUD_ERROR", err.Error())
	}
	if fraudResult.Result == "BLOCKED" {
		msg := fraudResult.Reason
		if msg == "" {
			msg = "payment blocked by antifraud"
		}
		return o.failAndSave(ctx, req, statusDeclined, "ANTIFRAUD_DECLINED", msg)
	}
	_ = o.logger.Log(ctx, contracts.PaymentEvent{
		Type:       contracts.EventFraudChecked,
		MerchantID: validatedReq.MerchantID,
		PaymentID:  validatedReq.PaymentID,
		Timestamp:  nowUTC(),
		Details:    "antifraud=" + fraudResult.Result,
	})

	// 5) Router.
	paymentSystem, adapterKey, err := o.router.Route(ctx, validatedReq, fraudResult)
	if err != nil {
		return o.failAndSave(ctx, req, statusFailed, "ROUTING_ERROR", err.Error())
	}
	_ = adapterKey

	// 6) Tokenization.
	tok, err := o.tokenizer.Tokenize(ctx, validatedReq)
	if err != nil {
		return o.failAndSave(ctx, req, statusFailed, "TOKENIZATION_ERROR", err.Error())
	}
	_ = o.logger.Log(ctx, contracts.PaymentEvent{
		Type:       contracts.EventTokenized,
		MerchantID: validatedReq.MerchantID,
		PaymentID:  validatedReq.PaymentID,
		Timestamp:  nowUTC(),
		Details:    "tokenized",
	})

	// 7) Adapter + retry.
	paymentAdapter := adapter.NewDummyAdapter(paymentSystem)
	var lastAdapterResult contracts.AdapterResult
	retryCount := 0

	for {
		_ = o.logger.Log(ctx, contracts.PaymentEvent{
			Type:       contracts.EventAdapterCalled,
			MerchantID: validatedReq.MerchantID,
			PaymentID:  validatedReq.PaymentID,
			Timestamp:  nowUTC(),
			Details:    fmt.Sprintf("adapter=%s attempt=%d", paymentSystem, retryCount),
		})

		lastAdapterResult, err = paymentAdapter.Send(ctx, tok, validatedReq)
		if err != nil {
			lastAdapterResult = contracts.AdapterResult{
				ExternalTransactionID: "",
				PaymentSystem:         paymentSystem,
				Status:                statusFailed,
				ErrorMessage:          err.Error(),
			}
		}
		lastAdapterResult.PaymentSystem = paymentSystem

		_ = o.logger.Log(ctx, contracts.PaymentEvent{
			Type:       contracts.EventAdapterResultReceived,
			MerchantID: validatedReq.MerchantID,
			PaymentID:  validatedReq.PaymentID,
			Timestamp:  nowUTC(),
			Details:    "status=" + lastAdapterResult.Status,
		})

		if lastAdapterResult.Status == statusCaptured || !o.retry.ShouldRetry(ctx, lastAdapterResult, retryCount) {
			break
		}

		retryCount = o.retry.NextRetryCount(retryCount)
		time.Sleep(o.retry.RetryAfter(retryCount))
	}

	// 8) Callback handler: build gateway response.
	resp, err := o.callback.HandleCallback(ctx, lastAdapterResult, validatedReq, tok)
	if err != nil {
		resp = buildErrorResponse(req, statusFailed, "CALLBACK_ERROR", err.Error())
	}
	resp.TransactionDetails.RetryCount = retryCount

	finalStatus := resp.CurrentStatus
	_ = o.stateManager.SetStatus(ctx, req.MerchantID, req.PaymentID, finalStatus)
	_ = o.workflow.CompleteSession(ctx, sessionID, finalStatus)

	// 9) Notifications.
	_ = o.notifications.Notify(ctx, resp)
	_ = o.logger.Log(ctx, contracts.PaymentEvent{
		Type:       contracts.EventPaymentResponseSent,
		MerchantID: validatedReq.MerchantID,
		PaymentID:  validatedReq.PaymentID,
		Timestamp:  nowUTC(),
		Details:    "status=" + resp.CurrentStatus,
	})

	// 10) Save for idempotency.
	_ = o.store.Save(ctx, req.MerchantID, req.PaymentID, req.IdempotencyKey, resp.CurrentStatus, mustMarshalPaymentResponse(resp), nowUTC())

	return resp, nil
}

func (o *SimpleOrchestrator) failAndSave(ctx context.Context, req dto.CreatePaymentRequest, status string, code string, msg string) (dto.PaymentResponse, error) {
	_ = o.stateManager.SetStatus(ctx, req.MerchantID, req.PaymentID, status)
	resp := buildErrorResponse(req, status, code, msg)
	_ = o.logger.Log(ctx, contracts.PaymentEvent{
		Type:       contracts.EventPaymentFailed,
		MerchantID: req.MerchantID,
		PaymentID:  req.PaymentID,
		Timestamp:  nowUTC(),
		Details:    code + ": " + msg,
	})
	return resp, o.store.Save(ctx, req.MerchantID, req.PaymentID, req.IdempotencyKey, status, mustMarshalPaymentResponse(resp), nowUTC())
}

func mustMarshalPaymentResponse(resp dto.PaymentResponse) string {
	b, err := json.Marshal(resp)
	if err != nil {
		return ""
	}
	return string(b)
}

func buildErrorResponse(req dto.CreatePaymentRequest, status string, code string, msg string) dto.PaymentResponse {
	return dto.PaymentResponse{
		ID:             req.PaymentID,
		MerchantID:     req.MerchantID,
		IdempotencyKey: req.IdempotencyKey,
		CurrentStatus:  status,
		PaymentInfo: dto.PaymentInfoResponse{
			Amount:            req.PaymentInfo.Amount,
			PaymentMethodData: req.PaymentInfo.PaymentMethodData,
			CustomerData:      req.PaymentInfo.CustomerData,
			Description:       req.PaymentInfo.Description,
			CreatedAt:         req.PaymentInfo.CreatedAt,
			UpdatedAt:         nowUTC(),
		},
		TransactionDetails: dto.TransactionDetails{RetryCount: 0},
		Error: &dto.GatewayError{
			Code:    code,
			Message: msg,
		},
	}
}
