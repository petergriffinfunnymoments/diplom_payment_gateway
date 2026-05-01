package simple

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
	"payment-gateway/internal/subsystems/antifraud"
	"payment-gateway/internal/subsystems/adapter"
	"payment-gateway/internal/subsystems/notifications"
	"payment-gateway/internal/subsystems/storage"
	"payment-gateway/internal/subsystems/tokenizer"
	"payment-gateway/internal/subsystems/validator"
)

const (
	statusSuccess = "SUCCESS"
	statusFailed  = "FAILED"
	statusPending = "PENDING"
)

type noOpLogger struct{}

func (l noOpLogger) Log(ctx context.Context, event contracts.PaymentEvent) error {
	_ = ctx
	_ = event
	return nil
}

type SimpleOrchestrator struct {
	validator     contracts.PaymentValidator
	antiFraud     contracts.AntiFraud
	tokenizer     contracts.Tokenizer
	adapter       contracts.PaymentAdapter
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

// NewSimpleOrchestrator создаёт “склеенный” оркестратор на dummy-подсистемах и in-memory TransactionStore.
// NewSimpleOrchestrator создаёт “склеенный” оркестратор на dummy-подсистемах.
// Если TransactionStore не передан, используется in-memory хранилище для локального запуска.
func NewSimpleOrchestrator(stores ...contracts.TransactionStore) *SimpleOrchestrator {
	store := contracts.TransactionStore(storage.NewInMemoryTransactionStore())

	if len(stores) > 0 && stores[0] != nil {
		store = stores[0]
	}

	// Containers
	sm := newInMemoryStateManager()
	rt := newSimplePaymentRouter()
	wf := newSimpleWorkflowEngine()
	retry := newSimpleRetryHandler()
	cb := newSimpleCallbackHandler()

	// Subsystems (dummy)
	return &SimpleOrchestrator{
		validator:     validator.NewDummyValidator(),
		antiFraud:     antifraud.NewDummyAntiFraud(),
		tokenizer:     tokenizer.NewDummyTokenizer(),
		adapter:       adapter.NewDummyAdapter("DUMMY"),
		notifications: notifications.NewDummyNotifications(),

		store:  store,
		logger: noOpLogger{},

		stateManager: sm,
		router:       rt,
		workflow:     wf,
		retry:        retry,
		callback:     cb,
	}
}

func (o *SimpleOrchestrator) CreatePayment(ctx context.Context, req dto.CreatePaymentRequest) (dto.PaymentResponse, error) {
	if req.MerchantID == "" || req.IdempotencyKey == "" || req.PaymentID == "" {
		return dto.PaymentResponse{}, errors.New("merchant_id, idempotency_key и payment_id обязательны")
	}

	// 1) Idempotency: если этот idempotency_key уже был обработан — отдаем сохранённый response.
	if status, payloadJSON, found, err := o.store.GetByIdempotencyKey(ctx, req.MerchantID, req.IdempotencyKey); err != nil {
		return dto.PaymentResponse{}, err
	} else if found {
		if payloadJSON == "" {
			// на случай “поломанных” записей
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
					UpdatedAt:        time.Now().UTC(),
				},
				TransactionDetails: dto.TransactionDetails{
					RetryCount: 0,
				},
				Error: nil,
			}, nil
		}

		var cached dto.PaymentResponse
		if err := json.Unmarshal([]byte(payloadJSON), &cached); err != nil {
			return dto.PaymentResponse{}, fmt.Errorf("failed to unmarshal cached response: %w", err)
		}
		return cached, nil
	}

	// 2) Workflow containers: start session / set status
	sessionID, err := o.workflow.StartSession(ctx, req)
	if err != nil {
		return dto.PaymentResponse{}, err
	}
	_ = sessionID

	_ = o.stateManager.SetStatus(ctx, req.MerchantID, req.PaymentID, statusPending)

	_ = o.logger.Log(ctx, contracts.PaymentEvent{
		Type:       contracts.EventPaymentReceived,
		MerchantID: req.MerchantID,
		PaymentID:  req.PaymentID,
		Timestamp:  time.Now().UTC(),
		Details:    "received",
	})

	// 3) Validator
	validatedReq, err := o.validator.Validate(ctx, req)
	if err != nil {
		_ = o.stateManager.SetStatus(ctx, req.MerchantID, req.PaymentID, statusFailed)
		return buildErrorResponse(req, statusFailed, "VALIDATION_ERROR", err.Error()), o.store.Save(
			ctx,
			req.MerchantID,
			req.PaymentID,
			req.IdempotencyKey,
			statusFailed,
			mustMarshalPaymentResponse(buildErrorResponse(req, statusFailed, "VALIDATION_ERROR", err.Error())),
			time.Now().UTC(),
		)
	}
	_ = o.logger.Log(ctx, contracts.PaymentEvent{
		Type:       contracts.EventPaymentValidated,
		MerchantID: validatedReq.MerchantID,
		PaymentID:  validatedReq.PaymentID,
		Timestamp:  time.Now().UTC(),
		Details:    "validated",
	})

	// 4) AntiFraud
	fraudResult, err := o.antiFraud.Check(ctx, validatedReq)
	if err != nil {
		_ = o.stateManager.SetStatus(ctx, req.MerchantID, req.PaymentID, statusFailed)
		resp := buildErrorResponse(req, statusFailed, "ANTIFRAUD_ERROR", err.Error())
		_ = o.store.Save(ctx, req.MerchantID, req.PaymentID, req.IdempotencyKey, statusFailed, mustMarshalPaymentResponse(resp), time.Now().UTC())
		return resp, nil
	}
	_ = o.logger.Log(ctx, contracts.PaymentEvent{
		Type:       contracts.EventFraudChecked,
		MerchantID: validatedReq.MerchantID,
		PaymentID:  validatedReq.PaymentID,
		Timestamp:  time.Now().UTC(),
		Details:    "antifraud=" + fraudResult.Result,
	})

	// 5) Router (choose paymentSystem + adapterKey)
	paymentSystem, adapterKey, err := o.router.Route(ctx, validatedReq, fraudResult)
	if err != nil {
		_ = o.stateManager.SetStatus(ctx, req.MerchantID, req.PaymentID, statusFailed)
		resp := buildErrorResponse(req, statusFailed, "ROUTING_ERROR", err.Error())
		_ = o.store.Save(ctx, req.MerchantID, req.PaymentID, req.IdempotencyKey, statusFailed, mustMarshalPaymentResponse(resp), time.Now().UTC())
		return resp, nil
	}
	_ = paymentSystem
	_ = adapterKey

	// Для каркаса: если router поменял paymentSystem — обновим адаптер “на лету”.
	// В реальном проекте это делается через фабрику адаптеров по adapterKey/paymentSystem.
	o.adapter = adapter.NewDummyAdapter(paymentSystem)

	// 6) Tokenization
	tok, err := o.tokenizer.Tokenize(ctx, validatedReq)
	if err != nil {
		_ = o.stateManager.SetStatus(ctx, req.MerchantID, req.PaymentID, statusFailed)
		resp := buildErrorResponse(req, statusFailed, "TOKENIZATION_ERROR", err.Error())
		_ = o.store.Save(ctx, req.MerchantID, req.PaymentID, req.IdempotencyKey, statusFailed, mustMarshalPaymentResponse(resp), time.Now().UTC())
		return resp, nil
	}
	_ = o.logger.Log(ctx, contracts.PaymentEvent{
		Type:       contracts.EventTokenized,
		MerchantID: validatedReq.MerchantID,
		PaymentID:  validatedReq.PaymentID,
		Timestamp:  time.Now().UTC(),
		Details:    "tokenized",
	})

	// 7) Adapter + retry
	var lastAdapterResult contracts.AdapterResult
	retryCount := 0
	for {
		_ = o.logger.Log(ctx, contracts.PaymentEvent{
			Type:       contracts.EventAdapterCalled,
			MerchantID: validatedReq.MerchantID,
			PaymentID:  validatedReq.PaymentID,
			Timestamp:  time.Now().UTC(),
			Details:    fmt.Sprintf("attempt=%d", retryCount),
		})

		lastAdapterResult, err = o.adapter.Send(ctx, tok, validatedReq)
		if err != nil {
			// treat transport error as FAILED adapter result
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
			Timestamp:  time.Now().UTC(),
			Details:    "status=" + lastAdapterResult.Status,
		})

		if lastAdapterResult.Status == statusSuccess || !o.retry.ShouldRetry(ctx, lastAdapterResult, retryCount) {
			break
		}

		retryCount = o.retry.NextRetryCount(retryCount)
		time.Sleep(o.retry.RetryAfter(retryCount))
	}

	// 8) Callback handler: build gateway response
	resp, err := o.callback.HandleCallback(ctx, lastAdapterResult, validatedReq, tok)
	if err != nil {
		_ = o.stateManager.SetStatus(ctx, req.MerchantID, req.PaymentID, statusFailed)
		resp = buildErrorResponse(req, statusFailed, "CALLBACK_ERROR", err.Error())
	}

	finalStatus := resp.CurrentStatus
	_ = o.stateManager.SetStatus(ctx, req.MerchantID, req.PaymentID, finalStatus)
	_ = o.workflow.CompleteSession(ctx, sessionID, finalStatus)

	// 9) Notifications (for каркаса — no-op)
	_ = o.notifications.Notify(ctx, resp)

	// 10) Save for idempotency
	_ = o.store.Save(
		ctx,
		req.MerchantID,
		req.PaymentID,
		req.IdempotencyKey,
		resp.CurrentStatus,
		mustMarshalPaymentResponse(resp),
		time.Now().UTC(),
	)

	return resp, nil
}

// ---- helpers ----

func mustMarshalPaymentResponse(resp dto.PaymentResponse) string {
	b, err := json.Marshal(resp)
	if err != nil {
		// для каркаса не падать из-за marshal
		return ""
	}
	return string(b)
}

func buildErrorResponse(req dto.CreatePaymentRequest, status string, code string, msg string) dto.PaymentResponse {
	now := time.Now().UTC()
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
		TransactionDetails: dto.TransactionDetails{
			RetryCount: 0,
		},
		Error: &dto.GatewayError{
			Code:    code,
			Message: msg,
		},
	}
}

// ---- Containers implementations (simple, in-memory) ----

type inMemoryStateManager struct {
	byPayment map[string]string // merchant:paymentID -> status
}

func newInMemoryStateManager() contracts.TransactionStateManager {
	return &inMemoryStateManager{byPayment: make(map[string]string)}
}

func (s *inMemoryStateManager) GetStatus(ctx context.Context, merchantID, paymentID string) (string, error) {
	_ = ctx
	return s.byPayment[merchantID+":"+paymentID], nil
}
func (s *inMemoryStateManager) SetStatus(ctx context.Context, merchantID, paymentID, status string) error {
	_ = ctx
	s.byPayment[merchantID+":"+paymentID] = status
	return nil
}

type simplePaymentRouter struct{}

func newSimplePaymentRouter() contracts.PaymentRouter {
	return &simplePaymentRouter{}
}

func (r *simplePaymentRouter) Route(ctx context.Context, req dto.CreatePaymentRequest, fraud contracts.AntiFraudResult) (paymentSystem string, adapterKey string, err error) {
	_ = ctx

	// Простейшее правило: на основе типа метода.
	switch req.PaymentInfo.PaymentMethodData.Type {
	case dto.PaymentMethodSBP:
		return "SBP", "sbp_adapter", nil
	case dto.PaymentMethodCard:
		_ = fraud
		return "CARD", "card_adapter", nil
	case dto.PaymentMethodDigitalWallet:
		_ = fraud
		return "DIGITAL_WALLET", "wallet_adapter", nil
	default:
		return "UNKNOWN", "unknown_adapter", nil
	}
}

type simpleWorkflowEngine struct{}

func newSimpleWorkflowEngine() contracts.WorkflowEngine {
	return &simpleWorkflowEngine{}
}

func (w *simpleWorkflowEngine) StartSession(ctx context.Context, req dto.CreatePaymentRequest) (string, error) {
	_ = ctx
	// В каркасе сессия = paymentID + timestamp.
	return fmt.Sprintf("sess_%s_%d", req.PaymentID, time.Now().UnixNano()), nil
}
func (w *simpleWorkflowEngine) CompleteSession(ctx context.Context, sessionID string, finalStatus string) error {
	_ = ctx
	_ = sessionID
	_ = finalStatus
	return nil
}

type simpleRetryHandler struct {
	maxAttempts int
}

func newSimpleRetryHandler() contracts.RetryHandler {
	return &simpleRetryHandler{maxAttempts: 3}
}

func (h *simpleRetryHandler) ShouldRetry(ctx context.Context, adapterResult contracts.AdapterResult, retryCount int) bool {
	_ = ctx
	if retryCount >= h.maxAttempts-1 {
		return false
	}
	return adapterResult.Status != statusSuccess
}

func (h *simpleRetryHandler) NextRetryCount(current int) int { return current + 1 }
func (h *simpleRetryHandler) RetryAfter(attempt int) time.Duration {
	_ = attempt
	return 50 * time.Millisecond
}

type simpleCallbackHandler struct{}

func newSimpleCallbackHandler() contracts.CallbackHandler {
	return &simpleCallbackHandler{}
}

func (c *simpleCallbackHandler) HandleCallback(
	ctx context.Context,
	adapterResult contracts.AdapterResult,
	req dto.CreatePaymentRequest,
	token string,
) (dto.PaymentResponse, error) {
	_ = ctx

	now := time.Now().UTC()
	retryCount := 0 // в каркасе можно потом пробросить реальное значение из RetryHandler/цикла

	fraudCheck := adapterResult.Status
	if adapterResult.Status == statusSuccess {
		fraudCheck = "PASSED"
	} else {
		fraudCheck = "FAILED"
	}

	status := statusSuccess
	errObj := (*dto.GatewayError)(nil)
	if adapterResult.Status != statusSuccess {
		status = statusFailed
		errObj = &dto.GatewayError{
			Code:    "ADAPTER_FAILED",
			Message: adapterResult.ErrorMessage,
		}
	}

	resp := dto.PaymentResponse{
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
		TransactionDetails: dto.TransactionDetails{
			ExternalTransactionID: adapterResult.ExternalTransactionID,
			PaymentSystem:         adapterResult.PaymentSystem,
			Token:                 token,
			FraudCheckResult:     fraudCheck,
			RetryCount:           retryCount,
		},
		Error: errObj,
	}

	return resp, nil
}
