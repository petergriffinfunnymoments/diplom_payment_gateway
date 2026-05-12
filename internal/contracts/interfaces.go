package contracts

import (
	"context"
	"time"

	"payment-gateway/internal/dto"
)

// -------------------- Shared / common types --------------------

type PaymentEventType string

const (
	EventPaymentReceived         PaymentEventType = "payment_received"
	EventPaymentValidated        PaymentEventType = "payment_validated"
	EventFraudChecked            PaymentEventType = "fraud_checked"
	EventTokenized               PaymentEventType = "tokenized"
	EventAdapterCalled           PaymentEventType = "adapter_called"
	EventAdapterResultReceived   PaymentEventType = "adapter_result_received"
	EventPaymentResponseSent     PaymentEventType = "payment_response_sent"
	EventPaymentFailed           PaymentEventType = "payment_failed"
	EventNotificationSent        PaymentEventType = "notification_sent"
	EventNotificationFailed      PaymentEventType = "notification_failed"
	EventMerchantWebhookReceived PaymentEventType = "merchant_webhook_received"
	EventAuthorizationFailed     PaymentEventType = "authorization_failed"
	EventRefundRequested         PaymentEventType = "refund_requested"
	EventRefundAdapterCalled     PaymentEventType = "refund_adapter_called"
	EventRefundAdapterResult     PaymentEventType = "refund_adapter_result_received"
	EventRefundResponseSent      PaymentEventType = "refund_response_sent"
	EventRefundFailed            PaymentEventType = "refund_failed"
)

type LogLevel string

const (
	LogLevelInfo  LogLevel = "INFO"
	LogLevelWarn  LogLevel = "WARN"
	LogLevelError LogLevel = "ERROR"
)

type PaymentEvent struct {
	Type           PaymentEventType
	Level          LogLevel
	Service        string
	MerchantID     string
	PaymentID      string
	IdempotencyKey string
	CorrelationID  string
	CurrentStatus  string
	Timestamp      time.Time
	Message        string
	Details        string
	Context        map[string]string
}

// -------------------- Subsystems (9) --------------------

type PaymentValidator interface {
	// Validate проверяет корректность входных данных и при необходимости нормализует.
	// Возвращает валидные данные, готовые к антифроду/оркестратору.
	Validate(ctx context.Context, req dto.CreatePaymentRequest) (dto.CreatePaymentRequest, error)
}

type AntiFraud interface {
	// Check выполняет антифрод-проверки и возвращает результат.
	Check(ctx context.Context, req dto.CreatePaymentRequest) (AntiFraudResult, error)
}

type AntiFraudResult struct {
	Result string `json:"result"` // например: PASSED / BLOCKED
	Reason string `json:"reason,omitempty"`
}

type Tokenizer interface {
	// Tokenize преобразует платежные данные в токен для передачи в платежную систему.
	Tokenize(ctx context.Context, req dto.CreatePaymentRequest) (token string, err error)
}

type PaymentAdapter interface {
	// Send отправляет токен (или пакет данных) в ЭПС.
	// Возвращает статус транзакции, внешний id, payment_system и причину ошибки при необходимости.
	Send(ctx context.Context, token string, req dto.CreatePaymentRequest) (AdapterResult, error)
}

type AdapterResult struct {
	ExternalTransactionID string
	PaymentSystem         string
	Status                string // например: CAPTURED / FAILED / PENDING
	ProviderStatus        string // исходный статус внешней платежной системы
	PaymentURL            string // ссылка на платежную форму, если провайдер требует подтверждение пользователя
	QRID                  string
	QRPayload             string
	QRExpiresAt           time.Time
	ParticipantBank       string
	SchemaVersion         string
	SettlementHint        string
	ErrorMessage          string
}

type RefundAdapter interface {
	Refund(ctx context.Context, req RefundRequest) (RefundResult, error)
}

type RefundRequest struct {
	RefundID          string
	MerchantID        string
	PaymentID         string
	IdempotencyKey    string
	ExternalPaymentID string
	Amount            dto.AmountMoney
	Reason            string
	Full              bool
	Payment           dto.PaymentResponse
}

type RefundResult struct {
	ExternalRefundID string
	PaymentSystem    string
	Status           string
	ProviderStatus   string
	ErrorMessage     string
}

type Notifications interface {
	// Notify отправляет результат в API-шлюз (для передачи интернет-магазину).
	Notify(ctx context.Context, resp dto.PaymentResponse) error
}

type EventLogger interface {
	// Log сохраняет событие в логирование/хранилище (модуль логирования).
	Log(ctx context.Context, event PaymentEvent) error
}

type TransactionStore interface {
	// Save создаёт/обновляет запись транзакции в БД.
	// Для каркаса: храним итог и текущее состояние.
	Save(ctx context.Context, merchantID string, paymentID string, idempotencyKey string, status string, payloadJSON string, updatedAt time.Time) error

	// GetByIdempotencyKey возвращает текущую транзакцию, если idempotencyKey уже был использован.
	GetByIdempotencyKey(ctx context.Context, merchantID string, idempotencyKey string) (status string, payloadJSON string, found bool, err error)

	// GetByPaymentID возвращает последнее сохранённое состояние платежа по payment_id.
	GetByPaymentID(ctx context.Context, merchantID string, paymentID string) (status string, payloadJSON string, found bool, err error)
}

type TransactionReportStore interface {
	BuildTransactionReport(ctx context.Context, filter dto.TransactionReportFilter) (dto.TransactionReport, error)
}

type RefundStore interface {
	SaveRefund(ctx context.Context, refund dto.Refund) error
	GetRefundByID(ctx context.Context, merchantID string, refundID string) (refund dto.Refund, found bool, err error)
	GetRefundByIdempotencyKey(ctx context.Context, merchantID string, idempotencyKey string) (refund dto.Refund, found bool, err error)
	ListRefundsByPaymentID(ctx context.Context, merchantID string, paymentID string) ([]dto.Refund, error)
}

// PaymentRoute описывает правило выбора внешнего платежного провайдера.
type PaymentRoute struct {
	MerchantID    string
	PaymentMethod dto.PaymentMethodType
	Provider      string
	PaymentSystem string
	Priority      int
}

// PaymentRouteStore хранит правила маршрутизации платежей для мерчантов.
type PaymentRouteStore interface {
	GetActiveRoute(ctx context.Context, merchantID string, paymentMethod dto.PaymentMethodType) (route PaymentRoute, found bool, err error)
}

// -------------------- Orchestrator "containers" (5) --------------------

type TransactionStateManager interface {
	GetStatus(ctx context.Context, merchantID, paymentID string) (status string, err error)
	SetStatus(ctx context.Context, merchantID, paymentID, status string) error
}

type PaymentRouter interface {
	// Route определяет, какой адаптер/ЭПС использовать (по антифроду, типу оплаты, политике и т.д.)
	// Для каркаса возвращаем paymentSystem и "key" адаптера.
	Route(ctx context.Context, req dto.CreatePaymentRequest, fraud AntiFraudResult) (paymentSystem string, adapterKey string, err error)
}

type WorkflowEngine interface {
	// StartSession создаёт сессию транзакции (жизненный цикл).
	StartSession(ctx context.Context, req dto.CreatePaymentRequest) (sessionID string, err error)

	// CompleteSession завершает сессию.
	CompleteSession(ctx context.Context, sessionID string, finalStatus string) error
}

type RetryHandler interface {
	// ShouldRetry отвечает на вопрос, нужно ли делать повтор.
	ShouldRetry(ctx context.Context, adapterResult AdapterResult, retryCount int) bool

	// NextRetryCount возвращает новое значение счетчика.
	NextRetryCount(current int) int

	// RetryAfter задаёт (ориентировочно) задержку между повторами.
	RetryAfter(attempt int) time.Duration
}

type CallbackHandler interface {
	// HandleCallback преобразует результат адаптера в response для API.
	HandleCallback(ctx context.Context, adapterResult AdapterResult, req dto.CreatePaymentRequest, token string) (dto.PaymentResponse, error)
}

// -------------------- Orchestrator main facade --------------------

type PaymentOrchestrator interface {
	CreatePayment(ctx context.Context, req dto.CreatePaymentRequest) (dto.PaymentResponse, error)
}
