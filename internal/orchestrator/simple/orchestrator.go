package simple

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
	"payment-gateway/internal/subsystems/adapter"
	"payment-gateway/internal/subsystems/antifraud"
	paymentlogging "payment-gateway/internal/subsystems/logging"
	"payment-gateway/internal/subsystems/notifications"
	"payment-gateway/internal/subsystems/storage"
	"payment-gateway/internal/subsystems/tokenizer"
	"payment-gateway/internal/subsystems/validator"
)

type SimpleOrchestrator struct {
	validator      contracts.PaymentValidator
	antiFraud      contracts.AntiFraud
	tokenizer      contracts.Tokenizer
	adapterFactory *adapter.Factory
	notifications  contracts.Notifications

	store  contracts.TransactionStore
	logger contracts.EventLogger

	stateManager contracts.TransactionStateManager
	router       contracts.PaymentRouter
	workflow     contracts.WorkflowEngine
	retry        contracts.RetryHandler
	callback     contracts.CallbackHandler
}

func NewSimpleOrchestrator(stores ...contracts.TransactionStore) *SimpleOrchestrator {
	store := storage.NewInMemoryTransactionStore()
	if len(stores) > 0 && stores[0] != nil {
		store = stores[0]
	}

	return newSimpleOrchestrator(store, noOpLogger{}, nil, notifications.NewNoOpNotifications(), nil)
}

func NewSimpleOrchestratorWithLogger(store contracts.TransactionStore, eventLogger contracts.EventLogger) *SimpleOrchestrator {
	return NewSimpleOrchestratorWithDependencies(store, eventLogger, nil)
}

func NewSimpleOrchestratorWithDependencies(
	store contracts.TransactionStore,
	eventLogger contracts.EventLogger,
	tokenizerService contracts.Tokenizer,
	adapterFactories ...*adapter.Factory,
) *SimpleOrchestrator {
	return NewSimpleOrchestratorWithServices(store, eventLogger, tokenizerService, nil, adapterFactories...)
}

func NewSimpleOrchestratorWithServices(
	store contracts.TransactionStore,
	eventLogger contracts.EventLogger,
	tokenizerService contracts.Tokenizer,
	notificationService contracts.Notifications,
	adapterFactories ...*adapter.Factory,
) *SimpleOrchestrator {
	return NewSimpleOrchestratorWithRouting(store, eventLogger, tokenizerService, notificationService, nil, adapterFactories...)
}

func NewSimpleOrchestratorWithRouting(
	store contracts.TransactionStore,
	eventLogger contracts.EventLogger,
	tokenizerService contracts.Tokenizer,
	notificationService contracts.Notifications,
	routeStore contracts.PaymentRouteStore,
	adapterFactories ...*adapter.Factory,
) *SimpleOrchestrator {
	if store == nil {
		store = storage.NewInMemoryTransactionStore()
	}
	if eventLogger == nil {
		eventLogger = noOpLogger{}
	}
	if notificationService == nil {
		notificationService = notifications.NewNoOpNotifications()
	}

	return newSimpleOrchestrator(store, eventLogger, tokenizerService, notificationService, routeStore, adapterFactories...)
}

func newSimpleOrchestrator(
	store contracts.TransactionStore,
	eventLogger contracts.EventLogger,
	tokenizerService contracts.Tokenizer,
	notificationService contracts.Notifications,
	routeStore contracts.PaymentRouteStore,
	adapterFactories ...*adapter.Factory,
) *SimpleOrchestrator {
	if tokenizerService == nil {
		tokenizerService = tokenizer.NewEphemeralTokenizer()
	}
	adapterFactory := adapter.NewFactoryFromEnv()
	if len(adapterFactories) > 0 && adapterFactories[0] != nil {
		adapterFactory = adapterFactories[0]
	}

	return &SimpleOrchestrator{
		validator:      validator.NewPaymentDataValidator(),
		antiFraud:      antifraud.NewRuleBasedAntiFraud(),
		tokenizer:      tokenizerService,
		adapterFactory: adapterFactory,
		notifications:  notificationService,

		store:  store,
		logger: eventLogger,

		stateManager: newInMemoryStateManager(),
		router:       newSimplePaymentRouterWithStore(routeStore),
		workflow:     newSimpleWorkflowEngine(),
		retry:        newSimpleRetryHandler(),
		callback:     newSimpleCallbackHandler(),
	}
}

func (o *SimpleOrchestrator) CreatePayment(ctx context.Context, req dto.CreatePaymentRequest) (dto.PaymentResponse, error) {
	if req.MerchantID == "" || req.IdempotencyKey == "" || req.PaymentID == "" {
		return dto.PaymentResponse{}, errors.New("merchant_id, idempotency_key и payment_id обязательны")
	}
	safeReq := req.WithoutSensitiveAuthenticationData()

	if status, payloadJSON, found, err := o.store.GetByIdempotencyKey(ctx, req.MerchantID, req.IdempotencyKey); err != nil {
		return dto.PaymentResponse{}, err
	} else if found {
		_ = o.logEvent(ctx, safeReq, contracts.EventPaymentResponseSent, contracts.LogLevelInfo, "orchestrator", status, "Cached payment response returned by idempotency key", map[string]string{
			"idempotency_hit": "true",
		})

		if payloadJSON == "" {
			return dto.PaymentResponse{
				ID:             safeReq.PaymentID,
				MerchantID:     safeReq.MerchantID,
				IdempotencyKey: safeReq.IdempotencyKey,
				CurrentStatus:  status,
				PaymentInfo: dto.PaymentInfoResponse{
					Amount:            safeReq.PaymentInfo.Amount,
					PaymentMethodData: safeReq.PaymentInfo.PaymentMethodData,
					CustomerData:      safeReq.PaymentInfo.CustomerData.Sanitized(),
					Items:             safeReq.PaymentInfo.Items,
					Description:       safeReq.PaymentInfo.Description,
					CreatedAt:         safeReq.PaymentInfo.CreatedAt,
					UpdatedAt:         nowUTC(),
				},
				TransactionDetails: dto.TransactionDetails{RetryCount: 0},
				Error:              nil,
			}.Sanitized(), nil
		}

		var cached dto.PaymentResponse
		if err := json.Unmarshal([]byte(payloadJSON), &cached); err != nil {
			return dto.PaymentResponse{}, fmt.Errorf("failed to unmarshal cached response: %w", err)
		}
		return cached.Sanitized(), nil
	}

	sessionID, err := o.workflow.StartSession(ctx, safeReq)
	if err != nil {
		return dto.PaymentResponse{}, err
	}

	_ = o.stateManager.SetStatus(ctx, safeReq.MerchantID, safeReq.PaymentID, statusCreated)
	_ = o.logEvent(ctx, safeReq, contracts.EventPaymentReceived, contracts.LogLevelInfo, "orchestrator", statusCreated, "Payment request received by orchestrator", map[string]string{
		"session_id":      sessionID,
		"amount":          strconv.FormatFloat(safeReq.PaymentInfo.Amount.Value, 'f', 2, 64),
		"currency":        string(safeReq.PaymentInfo.Amount.Currency),
		"payment_method":  string(safeReq.PaymentInfo.PaymentMethodData.Type),
		"phone_mask":      paymentlogging.MaskPhone(safeReq.PaymentInfo.CustomerData.Phone),
		"email_mask":      paymentlogging.MaskEmail(safeReq.PaymentInfo.CustomerData.Email),
		"card_mask":       paymentlogging.MaskCardNumber(safeReq.PaymentInfo.CustomerData.CardNumber),
		"has_wallet_id":   strconv.FormatBool(safeReq.PaymentInfo.CustomerData.DigitalWalletID != ""),
		"description_len": strconv.Itoa(len(safeReq.PaymentInfo.Description)),
	})

	validatedReq, err := o.validator.Validate(ctx, req)
	if err != nil {
		return o.failAndSave(ctx, safeReq, statusFailed, dto.ErrorValidation, err.Error(), "validator")
	}
	validatedReq = validatedReq.WithoutSensitiveAuthenticationData()
	_ = o.stateManager.SetStatus(ctx, validatedReq.MerchantID, validatedReq.PaymentID, statusPending)
	_ = o.logEvent(ctx, validatedReq, contracts.EventPaymentValidated, contracts.LogLevelInfo, "validator", statusPending, "Payment request validated", nil)

	fraudResult, err := o.antiFraud.Check(ctx, validatedReq)
	if err != nil {
		return o.failAndSave(ctx, validatedReq, statusFailed, dto.ErrorAntifraud, err.Error(), "antifraud")
	}
	_ = o.logEvent(ctx, validatedReq, contracts.EventFraudChecked, contracts.LogLevelInfo, "antifraud", statusPending, "Antifraud check completed", map[string]string{
		"fraud_result": fraudResult.Result,
		"fraud_reason": fraudResult.Reason,
	})
	if fraudResult.Result == "BLOCKED" {
		msg := fraudResult.Reason
		if msg == "" {
			msg = "payment blocked by antifraud"
		}
		return o.failAndSave(ctx, validatedReq, statusDeclined, dto.ErrorAntifraudDeclined, msg, "antifraud")
	}

	paymentSystem, adapterKey, err := o.router.Route(ctx, validatedReq, fraudResult)
	if err != nil {
		return o.failAndSave(ctx, validatedReq, statusFailed, dto.ErrorRouting, err.Error(), "orchestrator")
	}

	tok, err := o.tokenizer.Tokenize(ctx, validatedReq)
	if err != nil {
		return o.failAndSave(ctx, validatedReq, statusFailed, dto.ErrorTokenization, err.Error(), "tokenizer")
	}
	_ = o.logEvent(ctx, validatedReq, contracts.EventTokenized, contracts.LogLevelInfo, "tokenizer", statusPending, "Payment data tokenized", map[string]string{
		"token_preview": paymentlogging.TokenPreview(tok),
	})

	paymentAdapter, selectedProvider, err := o.adapterFactory.Resolve(adapterKey, paymentSystem)
	if err != nil {
		return o.failAndSave(ctx, validatedReq, statusFailed, dto.ErrorAdapterFactory, err.Error(), "adapter")
	}
	var lastAdapterResult contracts.AdapterResult
	retryCount := 0

	for {
		_ = o.stateManager.SetStatus(ctx, req.MerchantID, req.PaymentID, statusCaptureRequested)
		_ = o.logEvent(ctx, validatedReq, contracts.EventAdapterCalled, contracts.LogLevelInfo, "adapter", statusCaptureRequested, "Payment adapter call started", map[string]string{
			"payment_system": paymentSystem,
			"adapter_key":    adapterKey,
			"provider":       selectedProvider,
			"attempt":        strconv.Itoa(retryCount),
		})

		lastAdapterResult, err = paymentAdapter.Send(ctx, tok, validatedReq)
		if err != nil {
			lastAdapterResult = contracts.AdapterResult{
				ExternalTransactionID: "",
				PaymentSystem:         paymentSystem,
				Status:                statusFailed,
				ErrorCode:             dto.ErrorProviderUnavailable,
				ErrorMessage:          err.Error(),
			}
		}
		if lastAdapterResult.PaymentSystem == "" {
			lastAdapterResult.PaymentSystem = paymentSystem
		}

		_ = o.logEvent(ctx, validatedReq, contracts.EventAdapterResultReceived, contracts.LogLevelInfo, "adapter", lastAdapterResult.Status, "Payment adapter returned result", map[string]string{
			"payment_system":          paymentSystem,
			"external_transaction_id": lastAdapterResult.ExternalTransactionID,
			"adapter_status":          lastAdapterResult.Status,
			"provider_status":         lastAdapterResult.ProviderStatus,
			"payment_url":             lastAdapterResult.PaymentURL,
			"error_message":           lastAdapterResult.ErrorMessage,
		})

		if lastAdapterResult.Status == statusCaptured || !o.retry.ShouldRetry(ctx, lastAdapterResult, retryCount) {
			break
		}

		retryCount = o.retry.NextRetryCount(retryCount)
		time.Sleep(o.retry.RetryAfter(retryCount))
	}

	resp, err := o.callback.HandleCallback(ctx, lastAdapterResult, validatedReq, tok)
	if err != nil {
		resp = buildErrorResponse(validatedReq, statusFailed, dto.ErrorCallback, err.Error())
	}
	resp = resp.Sanitized()
	resp.TransactionDetails.RetryCount = retryCount

	finalStatus := resp.CurrentStatus
	_ = o.stateManager.SetStatus(ctx, req.MerchantID, req.PaymentID, finalStatus)
	_ = o.workflow.CompleteSession(ctx, sessionID, finalStatus)

	_ = o.logEvent(ctx, validatedReq, contracts.EventPaymentResponseSent, contracts.LogLevelInfo, "orchestrator", finalStatus, "Payment response sent to merchant", map[string]string{
		"retry_count": strconv.Itoa(retryCount),
	})
	if err := o.notifications.Notify(ctx, resp); err != nil {
		_ = o.logEvent(ctx, validatedReq, contracts.EventNotificationFailed, contracts.LogLevelWarn, "notifications", finalStatus, "Merchant notification failed", map[string]string{
			"error_message": err.Error(),
		})
	}

	_ = o.store.Save(ctx, validatedReq.MerchantID, validatedReq.PaymentID, validatedReq.IdempotencyKey, resp.CurrentStatus, mustMarshalPaymentResponse(resp), nowUTC())

	return resp, nil
}

func (o *SimpleOrchestrator) failAndSave(ctx context.Context, req dto.CreatePaymentRequest, status string, code string, msg string, service string) (dto.PaymentResponse, error) {
	req = req.WithoutSensitiveAuthenticationData()
	_ = o.stateManager.SetStatus(ctx, req.MerchantID, req.PaymentID, status)

	resp := buildErrorResponse(req, status, code, msg)
	if code == dto.ErrorAntifraudDeclined {
		resp.TransactionDetails.FraudCheckResult = "BLOCKED"
	} else if service == "antifraud" {
		resp.TransactionDetails.FraudCheckResult = "ERROR"
	}

	_ = o.logEvent(ctx, req, contracts.EventPaymentFailed, contracts.LogLevelError, service, status, "Payment processing failed", map[string]string{
		"error_code":    code,
		"error_message": msg,
	})

	if err := o.notifications.Notify(ctx, resp); err != nil {
		_ = o.logEvent(ctx, req, contracts.EventNotificationFailed, contracts.LogLevelWarn, "notifications", status, "Merchant notification failed", map[string]string{
			"error_code":    code,
			"error_message": err.Error(),
		})
	}

	return resp, o.store.Save(ctx, req.MerchantID, req.PaymentID, req.IdempotencyKey, status, mustMarshalPaymentResponse(resp), nowUTC())
}

func (o *SimpleOrchestrator) logEvent(
	ctx context.Context,
	req dto.CreatePaymentRequest,
	eventType contracts.PaymentEventType,
	level contracts.LogLevel,
	service string,
	status string,
	message string,
	contextMap map[string]string,
) error {
	return o.logger.Log(ctx, contracts.PaymentEvent{
		Type:           eventType,
		Level:          level,
		Service:        service,
		MerchantID:     req.MerchantID,
		PaymentID:      req.PaymentID,
		IdempotencyKey: req.IdempotencyKey,
		CorrelationID:  req.PaymentID,
		CurrentStatus:  status,
		Timestamp:      nowUTC(),
		Message:        message,
		Details:        message,
		Context:        contextMap,
	})
}

func mustMarshalPaymentResponse(resp dto.PaymentResponse) string {
	b, err := json.Marshal(resp.Sanitized())
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
			CustomerData:      req.PaymentInfo.CustomerData.Sanitized(),
			Items:             req.PaymentInfo.Items,
			Description:       req.PaymentInfo.Description,
			CreatedAt:         req.PaymentInfo.CreatedAt,
			UpdatedAt:         nowUTC(),
		},
		TransactionDetails: dto.TransactionDetails{RetryCount: 0},
		Error:              dto.NewGatewayError(code, msg),
	}.Sanitized()
}
