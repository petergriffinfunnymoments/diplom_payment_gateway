package contracts

import (
	"context"
	"time"

	"payment-gateway/internal/dto"
)

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
	EventNetworkAccessDenied     PaymentEventType = "network_access_denied"
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

type PaymentValidator interface {
	Validate(ctx context.Context, req dto.CreatePaymentRequest) (dto.CreatePaymentRequest, error)
}

type AntiFraud interface {
	Check(ctx context.Context, req dto.CreatePaymentRequest) (AntiFraudResult, error)
}

type AntiFraudResult struct {
	Result string `json:"result"`
	Reason string `json:"reason,omitempty"`
}

type Tokenizer interface {
	Tokenize(ctx context.Context, req dto.CreatePaymentRequest) (token string, err error)
}

type PaymentAdapter interface {
	Send(ctx context.Context, token string, req dto.CreatePaymentRequest) (AdapterResult, error)
}

type AdapterResult struct {
	ExternalTransactionID string
	PaymentSystem         string
	Status                string
	ProviderStatus        string
	ErrorCode             string
	PaymentURL            string
	QRID                  string
	QRPayload             string
	QRImageDataURI        string
	QRExpiresAt           time.Time
	ParticipantBank       string
	SchemaVersion         string
	SettlementHint        string
	MoneyMark             string
	SmartContractID       string
	SmartContractResult   string
	SmartContractReason   string
	PlatformMessageID     string
	PlatformTransport     string
	PlatformSignatureType string
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
	Notify(ctx context.Context, resp dto.PaymentResponse) error
}

type EventLogger interface {
	Log(ctx context.Context, event PaymentEvent) error
}

type TransactionStore interface {
	Save(ctx context.Context, merchantID string, paymentID string, idempotencyKey string, status string, payloadJSON string, updatedAt time.Time) error

	GetByIdempotencyKey(ctx context.Context, merchantID string, idempotencyKey string) (status string, payloadJSON string, found bool, err error)

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

type PaymentRoute struct {
	MerchantID    string
	PaymentMethod dto.PaymentMethodType
	Provider      string
	PaymentSystem string
	Priority      int
}

type PaymentRouteStore interface {
	GetActiveRoute(ctx context.Context, merchantID string, paymentMethod dto.PaymentMethodType) (route PaymentRoute, found bool, err error)
}

type TransactionStateManager interface {
	GetStatus(ctx context.Context, merchantID, paymentID string) (status string, err error)
	SetStatus(ctx context.Context, merchantID, paymentID, status string) error
}

type PaymentRouter interface {
	Route(ctx context.Context, req dto.CreatePaymentRequest, fraud AntiFraudResult) (paymentSystem string, adapterKey string, err error)
}

type WorkflowEngine interface {
	StartSession(ctx context.Context, req dto.CreatePaymentRequest) (sessionID string, err error)

	CompleteSession(ctx context.Context, sessionID string, finalStatus string) error
}

type RetryHandler interface {
	ShouldRetry(ctx context.Context, adapterResult AdapterResult, retryCount int) bool

	NextRetryCount(current int) int

	RetryAfter(attempt int) time.Duration
}

type CallbackHandler interface {
	HandleCallback(ctx context.Context, adapterResult AdapterResult, req dto.CreatePaymentRequest, token string) (dto.PaymentResponse, error)
}

type PaymentOrchestrator interface {
	CreatePayment(ctx context.Context, req dto.CreatePaymentRequest) (dto.PaymentResponse, error)
}
