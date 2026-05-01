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
	adapter       contracts.PaymentAdapter
	notifications contracts.Notifications

	store  contracts.TransactionStore
	logger contracts.EventLogger

	// 5 внутренних контейнеров оркестратора
	stateManager contracts.TransactionStateManager
	router       contracts.PaymentRouter
	workflow     contracts.WorkflowEngine
	retry        contracts.RetryHandler
	callback     contracts.CallbackHandler
}

// NewSimpleOrchestrator создаёт оркестратор с dummy-подсистемами.
// Если передать TransactionStore, он будет использоваться вместо in-memory хранилища.
func NewSimpleOrchestrator(stores ...contracts.TransactionStore) *SimpleOrchestrator {
	store := storage.NewInMemoryTransactionStore()
	if len(stores) > 0 && stores[0] != nil {
		store = stores[0]
	}

	return NewSimpleOrchestratorWithDependencies(store, noOpLogger{})
}

// NewSimpleOrchestratorWithDependencies нужен для подключения реального хранилища и логгера.
func NewSimpleOrchestratorWithDependencies(
	store contracts.TransactionStore,
	logger contracts.EventLogger,
) *SimpleOrchestrator {
	if store == nil {
		store = storage.NewInMemoryTransactionStore()
	}
	if logger == nil {
		logger = noOpLogger{}
	}

	return &SimpleOrchestrator{
		validator:     validator.NewDummyValidator(),
		antiFraud:     antifraud.NewDummyAntiFraud(),
		tokenizer:     tokenizer.NewDummyTokenizer(),
		adapter:       adapter.NewDummyAdapter("DUMMY"),
		notifications: notifications.NewDummyNotifications(),

		store:  store,
		logger: logger,

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

	// 1. Идемпотентность: если такой ключ уже был обработан, возвращаем сохранённый ответ.
	cached, found, err := o.getCachedResponse(ctx, req)
	if err != nil {
		return dto.PaymentResponse{}, err
	}
	if found {
		return cached, nil
	}

	// 2. Старт workflow-сессии.
	sessionID, err := o.workflow.StartSession(ctx, req)
	if err != nil {
		return dto.PaymentResponse{}, err
	}

	o.setStatus(ctx, req, statusPending)
	o.logEvent(ctx, contracts.EventPaymentReceived, req.MerchantID, req.PaymentID, "payment request received")

	// 3. Валидация входных данных.
	validatedReq, err := o.validator.Validate(ctx, req)
	if err != nil {
		resp := buildErrorResponse(req, statusFailed, "VALIDATION_ERROR", err.Error())
		o.failSession(ctx, sessionID, req, resp, contracts.EventPaymentFailed, "validation failed")
		return resp, nil
	}

	o.setStatus(ctx, validatedReq, statusValidated)
	o.logEvent(ctx, contracts.EventPaymentValidated, validatedReq.MerchantID, validatedReq.PaymentID, "payment request validated")

	// 4. Антифрод.
	fraudResult, err := o.antiFraud.Check(ctx, validatedReq)
	if err != nil {
		resp := buildErrorResponse(validatedReq, statusFailed, "ANTIFRAUD_ERROR", err.Error())
		o.failSession(ctx, sessionID, validatedReq, resp, contracts.EventPaymentFailed, "antifraud error")
		return resp, nil
	}
	if fraudResult.Result == "BLOCKED" {
		reason := fraudResult.Reason
		if reason == "" {
			reason = "payment blocked by antifraud"
		}
		resp := buildErrorResponse(validatedReq, statusFailed, "ANTIFRAUD_BLOCKED", reason)
		o.failSession(ctx, sessionID, validatedReq, resp, contracts.EventPaymentFailed, "antifraud blocked")
		return resp, nil
	}

	o.setStatus(ctx, validatedReq, statusFraudChecked)
	o.logEvent(ctx, contracts.EventFraudChecked, validatedReq.MerchantID, validatedReq.PaymentID, "antifraud="+fraudResult.Result)

	// 5. Маршрутизация платежа.
	paymentSystem, adapterKey, err := o.router.Route(ctx, validatedReq, fraudResult)
	if err != nil {
		resp := buildErrorResponse(validatedReq, statusFailed, "ROUTING_ERROR", err.Error())
		o.failSession(ctx, sessionID, validatedReq, resp, contracts.EventPaymentFailed, "routing failed")
		return resp, nil
	}
	_ = adapterKey

	// Важно: не изменяем o.adapter глобально, чтобы параллельные платежи не мешали друг другу.
	selectedAdapter := adapter.NewDummyAdapter(paymentSystem)
	if selectedAdapter == nil {
		selectedAdapter = o.adapter
	}

	// 6. Токенизация.
	tok, err := o.tokenizer.Tokenize(ctx, validatedReq)
	if err != nil {
		resp := buildErrorResponse(validatedReq, statusFailed, "TOKENIZATION_ERROR", err.Error())
		o.failSession(ctx, sessionID, validatedReq, resp, contracts.EventPaymentFailed, "tokenization failed")
		return resp, nil
	}

	o.setStatus(ctx, validatedReq, statusTokenized)
	o.logEvent(ctx, contracts.EventTokenized, validatedReq.MerchantID, validatedReq.PaymentID, "payment data tokenized")

	// 7. Вызов адаптера с retry.
	adapterResult, retryCount := o.callAdapterWithRetry(ctx, selectedAdapter, paymentSystem, tok, validatedReq)

	// 8. Обработка результата адаптера и формирование ответа шлюза.
	resp, err := o.callback.HandleCallback(ctx, adapterResult, validatedReq, tok)
	if err != nil {
		resp = buildErrorResponse(validatedReq, statusFailed, "CALLBACK_ERROR", err.Error())
	}
	resp.TransactionDetails.RetryCount = retryCount

	finalStatus := resp.CurrentStatus
	if finalStatus == "" {
		finalStatus = statusFailed
		resp.CurrentStatus = finalStatus
	}

	o.setStatus(ctx, validatedReq, finalStatus)
	_ = o.workflow.CompleteSession(ctx, sessionID, finalStatus)

	// 9. Уведомление и сохранение ответа для идемпотентности.
	_ = o.notifications.Notify(ctx, resp)
	o.saveResponse(ctx, validatedReq, resp)
	o.logEvent(ctx, contracts.EventPaymentResponseSent, validatedReq.MerchantID, validatedReq.PaymentID, "response sent with status="+finalStatus)

	return resp, nil
}

func (o *SimpleOrchestrator) getCachedResponse(ctx context.Context, req dto.CreatePaymentRequest) (dto.PaymentResponse, bool, error) {
	status, payloadJSON, found, err := o.store.GetByIdempotencyKey(ctx, req.MerchantID, req.IdempotencyKey)
	if err != nil {
		return dto.PaymentResponse{}, false, err
	}
	if !found {
		return dto.PaymentResponse{}, false, nil
	}
	if payloadJSON == "" {
		return dto.PaymentResponse{
			ID:              req.PaymentID,
			MerchantID:      req.MerchantID,
			IdempotencyKey: req.IdempotencyKey,
			CurrentStatus:  status,
			PaymentInfo: dto.PaymentInfoResponse{
				Amount:            req.PaymentInfo.Amount,
				PaymentMethodData: req.PaymentInfo.PaymentMethodData,
				CustomerData:      req.PaymentInfo.CustomerData,
				Description:       req.PaymentInfo.Description,
				CreatedAt:         req.PaymentInfo.CreatedAt,
				UpdatedAt:        nowUTC(),
			},
			TransactionDetails: dto.TransactionDetails{},
			Error:              nil,
		}, true, nil
	}

	var cached dto.PaymentResponse
	if err := json.Unmarshal([]byte(payloadJSON), &cached); err != nil {
		return dto.PaymentResponse{}, false, fmt.Errorf("failed to unmarshal cached response: %w", err)
	}
	return cached, true, nil
}

func (o *SimpleOrchestrator) callAdapterWithRetry(
	ctx context.Context,
	paymentAdapter contracts.PaymentAdapter,
	paymentSystem string,
	tok string,
	req dto.CreatePaymentRequest,
) (contracts.AdapterResult, int) {
	var lastResult contracts.AdapterResult
	retryCount := 0

	for {
		o.setStatus(ctx, req, statusSentToPaymentSystem)
		o.logEvent(ctx, contracts.EventAdapterCalled, req.MerchantID, req.PaymentID, fmt.Sprintf("payment_system=%s attempt=%d", paymentSystem, retryCount))

		adapterResult, err := paymentAdapter.Send(ctx, tok, req)
		if err != nil {
			adapterResult = contracts.AdapterResult{
				ExternalTransactionID: "",
				PaymentSystem:         paymentSystem,
				Status:                statusFailed,
				ErrorMessage:          err.Error(),
			}
		}
		if adapterResult.PaymentSystem == "" {
			adapterResult.PaymentSystem = paymentSystem
		}
		if adapterResult.Status == "" {
			adapterResult.Status = statusFailed
		}

		lastResult = adapterResult
		o.logEvent(ctx, contracts.EventAdapterResultReceived, req.MerchantID, req.PaymentID, "adapter status="+lastResult.Status)

		if lastResult.Status == statusSuccess || !o.retry.ShouldRetry(ctx, lastResult, retryCount) {
			break
		}

		retryCount = o.retry.NextRetryCount(retryCount)
		select {
		case <-ctx.Done():
			lastResult.Status = statusFailed
			lastResult.ErrorMessage = ctx.Err().Error()
			return lastResult, retryCount
		case <-timeAfter(o.retry.RetryAfter(retryCount)):
		}
	}

	return lastResult, retryCount
}

var timeAfter = func(d time.Duration) <-chan time.Time {
	return time.After(d)
}

func (o *SimpleOrchestrator) setStatus(ctx context.Context, req dto.CreatePaymentRequest, status string) {
	_ = o.stateManager.SetStatus(ctx, req.MerchantID, req.PaymentID, status)
}

func (o *SimpleOrchestrator) failSession(
	ctx context.Context,
	sessionID string,
	req dto.CreatePaymentRequest,
	resp dto.PaymentResponse,
	eventType contracts.PaymentEventType,
	details string,
) {
	o.setStatus(ctx, req, statusFailed)
	_ = o.workflow.CompleteSession(ctx, sessionID, statusFailed)
	o.saveResponse(ctx, req, resp)
	o.logEvent(ctx, eventType, req.MerchantID, req.PaymentID, details)
}

func (o *SimpleOrchestrator) saveResponse(ctx context.Context, req dto.CreatePaymentRequest, resp dto.PaymentResponse) {
	payloadJSON := mustMarshalPaymentResponse(resp)
	_ = o.store.Save(ctx, req.MerchantID, req.PaymentID, req.IdempotencyKey, resp.CurrentStatus, payloadJSON, nowUTC())
}

func (o *SimpleOrchestrator) logEvent(ctx context.Context, eventType contracts.PaymentEventType, merchantID, paymentID, details string) {
	_ = o.logger.Log(ctx, contracts.PaymentEvent{
		Type:       eventType,
		MerchantID: merchantID,
		PaymentID:  paymentID,
		Timestamp:  nowUTC(),
		Details:    details,
	})
}

func mustMarshalPaymentResponse(resp dto.PaymentResponse) string {
	b, err := json.Marshal(resp)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func buildErrorResponse(req dto.CreatePaymentRequest, status string, code string, msg string) dto.PaymentResponse {
	now := nowUTC()
	return dto.PaymentResponse{
		ID:              req.PaymentID,
		MerchantID:      req.MerchantID,
		IdempotencyKey: req.IdempotencyKey,
		CurrentStatus:  status,
		PaymentInfo: dto.PaymentInfoResponse{
			Amount:            req.PaymentInfo.Amount,
			PaymentMethodData: req.PaymentInfo.PaymentMethodData,
			CustomerData:      req.PaymentInfo.CustomerData,
			Description:       req.PaymentInfo.Description,
			CreatedAt:         req.PaymentInfo.CreatedAt,
			UpdatedAt:        now,
		},
		TransactionDetails: dto.TransactionDetails{},
		Error: &dto.GatewayError{
			Code:    code,
			Message: msg,
		},
	}
}
