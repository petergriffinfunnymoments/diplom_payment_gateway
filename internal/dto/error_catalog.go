package dto

import "strings"

const (
	ErrorBadRequest          = "BAD_REQUEST"
	ErrorMethodNotAllowed    = "METHOD_NOT_ALLOWED"
	ErrorNotFound            = "NOT_FOUND"
	ErrorNotImplemented      = "NOT_IMPLEMENTED"
	ErrorForbidden           = "FORBIDDEN"
	ErrorAuthContextMissing  = "AUTH_CONTEXT_MISSING"
	ErrorAuthentication      = "AUTHENTICATION_ERROR"
	ErrorNetworkAccessDenied = "NETWORK_ACCESS_DENIED"
	ErrorHTTPSRequired       = "HTTPS_REQUIRED"

	ErrorValidation            = "VALIDATION_ERROR"
	ErrorStorage               = "STORAGE_ERROR"
	ErrorPaymentStorage        = "PAYMENT_STORAGE_ERROR"
	ErrorPaymentNotFound       = "PAYMENT_NOT_FOUND"
	ErrorPaymentNotCaptured    = "PAYMENT_NOT_CAPTURED"
	ErrorInvalidStoredResponse = "INVALID_STORED_RESPONSE"
	ErrorInvalidStoredPayment  = "INVALID_STORED_PAYMENT"

	ErrorAntifraud         = "ANTIFRAUD_ERROR"
	ErrorAntifraudDeclined = "ANTIFRAUD_DECLINED"
	ErrorRouting           = "ROUTING_ERROR"
	ErrorTokenization      = "TOKENIZATION_ERROR"
	ErrorAdapterFactory    = "ADAPTER_FACTORY_ERROR"
	ErrorAdapterFailed     = "ADAPTER_FAILED"
	ErrorPaymentDeclined   = "PAYMENT_DECLINED"
	ErrorCallback          = "CALLBACK_ERROR"

	ErrorProviderUnavailable     = "PROVIDER_UNAVAILABLE"
	ErrorWebhookSignatureInvalid = "WEBHOOK_SIGNATURE_INVALID"
	ErrorWebhookPayloadInvalid   = "WEBHOOK_PAYLOAD_INVALID"
	ErrorYooKassaPaymentDeclined = "YOOKASSA_PAYMENT_DECLINED"
	ErrorYooKassaFraudSuspected  = "YOOKASSA_FRAUD_SUSPECTED"
	ErrorStripePaymentDeclined   = "STRIPE_PAYMENT_DECLINED"
	ErrorDigitalRubleDeclined    = "DIGITAL_RUBLE_PAYMENT_DECLINED"
	ErrorDigitalRubleTechnical   = "DIGITAL_RUBLE_TECHNICAL_ERROR"
	ErrorBlockedByProviderFraud  = "BLOCKED_BY_PROVIDER_FRAUD"
	ErrorDeclinedByProvider      = "DECLINED_BY_PROVIDER"

	ErrorRefundStoreUnavailable = "REFUND_STORE_UNAVAILABLE"
	ErrorRefundStorage          = "REFUND_STORAGE_ERROR"
	ErrorRefundNotSupported     = "REFUND_NOT_SUPPORTED"
	ErrorRefundNotFound         = "REFUND_NOT_FOUND"
	ErrorRefundFailed           = "REFUND_FAILED"

	ErrorReportStoreUnavailable = "REPORT_STORE_UNAVAILABLE"
	ErrorReportStorage          = "REPORT_STORAGE_ERROR"
	ErrorMerchantScopeMismatch  = "MERCHANT_SCOPE_MISMATCH"
)

type GatewayErrorDefinition struct {
	Code           string `json:"code"`
	Category       string `json:"category"`
	PaymentStatus  string `json:"payment_status,omitempty"`
	Retryable      bool   `json:"retryable"`
	DefaultMessage string `json:"default_message"`
	Description    string `json:"description"`
}

var gatewayErrorCatalog = []GatewayErrorDefinition{
	{Code: ErrorBadRequest, Category: "request", DefaultMessage: "request is invalid", Description: "Некорректный HTTP-запрос или JSON payload."},
	{Code: ErrorMethodNotAllowed, Category: "request", DefaultMessage: "method is not allowed", Description: "HTTP-метод не поддерживается endpoint-ом."},
	{Code: ErrorNotFound, Category: "request", DefaultMessage: "endpoint not found", Description: "Запрошенный endpoint не найден."},
	{Code: ErrorNotImplemented, Category: "gateway", PaymentStatus: string(StatusFailed), Retryable: false, DefaultMessage: "operation is not implemented", Description: "Операция не реализована или подсистема не подключена."},
	{Code: ErrorForbidden, Category: "access", DefaultMessage: "access is forbidden", Description: "Роль или границы мерчанта не позволяют выполнить операцию."},
	{Code: ErrorAuthContextMissing, Category: "access", DefaultMessage: "authenticated merchant context is required", Description: "Запрос дошёл до handler-а без контекста аутентифицированного мерчанта."},
	{Code: ErrorAuthentication, Category: "authentication", DefaultMessage: "merchant authentication failed", Description: "Ошибка merchant authentication: отсутствуют заголовки, неверный ключ, подпись или timestamp."},
	{Code: ErrorNetworkAccessDenied, Category: "network", DefaultMessage: "request source IP is not allowed", Description: "Источник запроса не входит в разрешённый IP/CIDR allowlist."},
	{Code: ErrorHTTPSRequired, Category: "transport", DefaultMessage: "HTTPS is required", Description: "Запрос отклонён, потому что включено требование HTTPS."},

	{Code: ErrorValidation, Category: "validation", PaymentStatus: string(StatusFailed), DefaultMessage: "payment data validation failed", Description: "Платёжные данные не прошли бизнес-валидацию."},
	{Code: ErrorStorage, Category: "storage", PaymentStatus: string(StatusFailed), Retryable: true, DefaultMessage: "storage operation failed", Description: "Общая ошибка чтения или записи состояния платежа."},
	{Code: ErrorPaymentStorage, Category: "storage", PaymentStatus: string(StatusFailed), Retryable: true, DefaultMessage: "payment storage operation failed", Description: "Ошибка чтения платежной транзакции из хранилища."},
	{Code: ErrorPaymentNotFound, Category: "storage", DefaultMessage: "payment not found", Description: "Платёж не найден для указанного мерчанта."},
	{Code: ErrorPaymentNotCaptured, Category: "business", DefaultMessage: "payment is not captured", Description: "Операция разрешена только для платежа в статусе CAPTURED."},
	{Code: ErrorInvalidStoredResponse, Category: "storage", PaymentStatus: string(StatusFailed), Retryable: true, DefaultMessage: "stored payment response is invalid", Description: "Сохранённый PaymentResponse не удалось разобрать."},
	{Code: ErrorInvalidStoredPayment, Category: "storage", PaymentStatus: string(StatusFailed), Retryable: true, DefaultMessage: "stored payment is invalid", Description: "Сохранённый платеж не удалось разобрать для дальнейшей операции."},

	{Code: ErrorAntifraud, Category: "antifraud", PaymentStatus: string(StatusFailed), Retryable: true, DefaultMessage: "antifraud check failed", Description: "Антифрод-модуль завершился технической ошибкой."},
	{Code: ErrorAntifraudDeclined, Category: "antifraud", PaymentStatus: string(StatusDeclined), DefaultMessage: "payment blocked by antifraud", Description: "Антифрод-модуль заблокировал платёж."},
	{Code: ErrorRouting, Category: "routing", PaymentStatus: string(StatusFailed), DefaultMessage: "payment routing failed", Description: "Маршрутизатор не смог выбрать платежного провайдера."},
	{Code: ErrorTokenization, Category: "tokenization", PaymentStatus: string(StatusFailed), Retryable: true, DefaultMessage: "payment tokenization failed", Description: "Модуль токенизации не смог создать внутренний токен."},
	{Code: ErrorAdapterFactory, Category: "adapter", PaymentStatus: string(StatusFailed), DefaultMessage: "payment adapter is not configured", Description: "Фабрика адаптеров не смогла вернуть провайдера по выбранному ключу."},
	{Code: ErrorAdapterFailed, Category: "adapter", PaymentStatus: string(StatusFailed), Retryable: true, DefaultMessage: "payment adapter failed", Description: "Адаптер провайдера вернул технический сбой или неизвестный статус."},
	{Code: ErrorPaymentDeclined, Category: "provider", PaymentStatus: string(StatusDeclined), DefaultMessage: "payment was declined by provider", Description: "Внешний провайдер отклонил платёж."},
	{Code: ErrorCallback, Category: "gateway", PaymentStatus: string(StatusFailed), Retryable: true, DefaultMessage: "payment callback handling failed", Description: "Шлюз не смог преобразовать результат адаптера в PaymentResponse."},

	{Code: ErrorProviderUnavailable, Category: "provider", PaymentStatus: string(StatusFailed), Retryable: true, DefaultMessage: "payment provider is unavailable", Description: "Внешний провайдер недоступен или вернул сетевую ошибку."},
	{Code: ErrorWebhookSignatureInvalid, Category: "webhook", DefaultMessage: "webhook signature is invalid", Description: "Подпись webhook-а внешней платёжной системы не прошла проверку."},
	{Code: ErrorWebhookPayloadInvalid, Category: "webhook", DefaultMessage: "webhook payload is invalid", Description: "Webhook внешней платёжной системы имеет некорректное тело."},
	{Code: ErrorYooKassaPaymentDeclined, Category: "provider", PaymentStatus: string(StatusDeclined), DefaultMessage: "YooKassa payment was declined", Description: "YooKassa отменила или отклонила платёж."},
	{Code: ErrorYooKassaFraudSuspected, Category: "provider", PaymentStatus: string(StatusDeclined), DefaultMessage: "YooKassa declined payment because fraud was suspected", Description: "YooKassa вернула cancellation_reason=fraud_suspected."},
	{Code: ErrorStripePaymentDeclined, Category: "provider", PaymentStatus: string(StatusDeclined), DefaultMessage: "Stripe payment was declined", Description: "Stripe Checkout сообщил об отказе или истечении сессии."},
	{Code: ErrorDigitalRubleDeclined, Category: "provider", PaymentStatus: string(StatusDeclined), DefaultMessage: "digital ruble payment was declined", Description: "Эмулятор цифрового рубля отклонил платёж."},
	{Code: ErrorDigitalRubleTechnical, Category: "provider", PaymentStatus: string(StatusFailed), Retryable: true, DefaultMessage: "digital ruble technical error", Description: "Эмулятор цифрового рубля вернул техническую ошибку."},
	{Code: ErrorBlockedByProviderFraud, Category: "provider", PaymentStatus: string(StatusDeclined), DefaultMessage: "payment blocked by provider antifraud", Description: "Внешний провайдер отклонил платёж по антифрод-причине."},
	{Code: ErrorDeclinedByProvider, Category: "provider", PaymentStatus: string(StatusDeclined), DefaultMessage: "payment declined by provider", Description: "Общий provider-level отказ без антифрод-признака."},

	{Code: ErrorRefundStoreUnavailable, Category: "refund", DefaultMessage: "refund store is not configured", Description: "Хранилище возвратов недоступно."},
	{Code: ErrorRefundStorage, Category: "refund", Retryable: true, DefaultMessage: "refund storage operation failed", Description: "Ошибка чтения или записи возврата."},
	{Code: ErrorRefundNotSupported, Category: "refund", DefaultMessage: "provider does not support refunds", Description: "Выбранный провайдер не реализует интерфейс возвратов."},
	{Code: ErrorRefundNotFound, Category: "refund", DefaultMessage: "refund not found", Description: "Возврат не найден."},
	{Code: ErrorRefundFailed, Category: "refund", Retryable: true, DefaultMessage: "refund failed", Description: "Провайдер вернул ошибку возврата."},

	{Code: ErrorReportStoreUnavailable, Category: "report", DefaultMessage: "report store is not configured", Description: "Хранилище отчётов недоступно."},
	{Code: ErrorReportStorage, Category: "report", Retryable: true, DefaultMessage: "report storage operation failed", Description: "Ошибка построения отчёта по транзакциям."},
	{Code: ErrorMerchantScopeMismatch, Category: "access", DefaultMessage: "merchant scope mismatch", Description: "Мерчант пытается получить данные другого мерчанта."},
}

func GatewayErrorCatalog() []GatewayErrorDefinition {
	out := make([]GatewayErrorDefinition, len(gatewayErrorCatalog))
	copy(out, gatewayErrorCatalog)
	return out
}

func GatewayErrorDefinitionByCode(code string) (GatewayErrorDefinition, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	for _, def := range gatewayErrorCatalog {
		if def.Code == code {
			return def, true
		}
	}
	return GatewayErrorDefinition{}, false
}

func NewGatewayError(code string, message string) *GatewayError {
	code = strings.ToUpper(strings.TrimSpace(code))
	message = strings.TrimSpace(message)
	if message == "" {
		if def, ok := GatewayErrorDefinitionByCode(code); ok {
			message = def.DefaultMessage
		}
	}
	return &GatewayError{Code: code, Message: message}
}
