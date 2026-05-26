package dto

import (
	"encoding/json"
	"strings"
	"time"
)

// PaymentMethodType определяет тип способа оплаты.
type PaymentMethodType string

const (
	PaymentMethodSBP           PaymentMethodType = "СБП"
	PaymentMethodCard          PaymentMethodType = "Банковская карта"
	PaymentMethodDigitalWallet PaymentMethodType = "Цифровой кошелек"
	PaymentMethodDigitalRuble  PaymentMethodType = "Цифровой рубль"
)

// PaymentCurrency — валютный код (для примера используется RUB).
type PaymentCurrency string

// AmountMoney — сумма платежа.
type AmountMoney struct {
	Value    float64         `json:"value"`
	Currency PaymentCurrency `json:"currency"`
}

// PaymentItem описывает позицию заказа для платежных систем, которым нужна номенклатура.
type PaymentItem struct {
	Name          string  `json:"name"`
	Price         float64 `json:"price"`
	Quantity      float64 `json:"quantity"`
	Category      string  `json:"category,omitempty"`
	VATTag        string  `json:"vat_tag,omitempty"`
	PaymentMethod string  `json:"payment_method,omitempty"`
	PaymentObject string  `json:"payment_object,omitempty"`
	IDInternal    string  `json:"id_internal,omitempty"`
}

// DigitalRubleData содержит дополнительные параметры сценария цифрового рубля.
// В прототипе они используются эмулятором платформы для проверки маркировки денег
// и учебного смарт-контракта целевого расходования.
type DigitalRubleData struct {
	SmartContractID    string `json:"smart_contract_id,omitempty"`
	RequireMarkedMoney bool   `json:"require_marked_money,omitempty"`
}

// CustomerData — данные клиента (в зависимости от типа оплаты набор полей может отличаться).
type CustomerData struct {
	Email                  string `json:"email,omitempty"`
	Phone                  string `json:"phone,omitempty"`
	CardNumber             string `json:"card_number,omitempty"`
	CardDate               string `json:"card_date,omitempty"`
	CvvCode                string `json:"CVV_code,omitempty"`
	DigitalWalletID        string `json:"digital_wallet_id,omitempty"`
	DigitalRubleWalletID   string `json:"digital_ruble_wallet_id,omitempty"`
	DigitalRubleAccount    string `json:"digital_ruble_account,omitempty"`
	DigitalRubleIdentifier string `json:"digital_ruble_identifier,omitempty"`
}

// PaymentMethodData — данные способа оплаты.
type PaymentMethodData struct {
	Type PaymentMethodType `json:"type"`
}

// CreatePaymentRequest — запрос на создание платежа.
type CreatePaymentRequest struct {
	MerchantID     string `json:"merchant_id"`
	IdempotencyKey string `json:"idempotency_key"`
	PaymentID      string `json:"payment_id"`
	CurrentStatus  string `json:"current_status"`

	PaymentInfo PaymentInfo `json:"payment_info"`
}

type PaymentInfo struct {
	Amount            AmountMoney       `json:"amount"`
	PaymentMethodData PaymentMethodData `json:"payment_method_data"`
	CustomerData      CustomerData      `json:"customer_data"`
	Items             []PaymentItem     `json:"items,omitempty"`
	DigitalRubleData  DigitalRubleData  `json:"digital_ruble_data,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	Description       string            `json:"description"`
}

// PaymentResponse — ответ платежного шлюза.
type PaymentResponse struct {
	ID             string `json:"id"`
	MerchantID     string `json:"merchant_id"`
	IdempotencyKey string `json:"idempotency_key"`
	CurrentStatus  string `json:"current_status"`

	PaymentInfo PaymentInfoResponse `json:"payment_info"`

	TransactionDetails TransactionDetails `json:"transaction_details"`
	Error              *GatewayError      `json:"error"`
}

type PaymentInfoResponse struct {
	Amount            AmountMoney       `json:"amount"`
	PaymentMethodData PaymentMethodData `json:"payment_method_data"`
	CustomerData      CustomerData      `json:"customer_data"`
	Items             []PaymentItem     `json:"items,omitempty"`
	DigitalRubleData  DigitalRubleData  `json:"digital_ruble_data,omitempty"`
	Description       string            `json:"description"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

type TransactionDetails struct {
	ExternalTransactionID string `json:"external_transaction_id"`
	PaymentSystem         string `json:"payment_system"`
	ProviderStatus        string `json:"provider_status,omitempty"`
	ProviderErrorCode     string `json:"provider_error_code,omitempty"`
	ProviderErrorMessage  string `json:"provider_error_message,omitempty"`
	CancellationParty     string `json:"cancellation_party,omitempty"`
	CancellationReason    string `json:"cancellation_reason,omitempty"`
	PaymentURL            string `json:"payment_url,omitempty"`
	QRID                  string `json:"qr_id,omitempty"`
	QRPayload             string `json:"qr_payload,omitempty"`
	QRImageDataURI        string `json:"qr_image_data_uri,omitempty"`
	QRExpiresAt           string `json:"qr_expires_at,omitempty"`
	ParticipantBank       string `json:"participant_bank,omitempty"`
	SchemaVersion         string `json:"schema_version,omitempty"`
	SettlementHint        string `json:"settlement_hint,omitempty"`
	MoneyMark             string `json:"money_mark,omitempty"`
	SmartContractID       string `json:"smart_contract_id,omitempty"`
	SmartContractResult   string `json:"smart_contract_result,omitempty"`
	SmartContractReason   string `json:"smart_contract_reason,omitempty"`
	PlatformMessageID     string `json:"platform_message_id,omitempty"`
	PlatformTransport     string `json:"platform_transport,omitempty"`
	PlatformSignatureType string `json:"platform_signature_type,omitempty"`
	Token                 string `json:"token,omitempty"`
	TokenPreview          string `json:"token_preview,omitempty"`
	FraudCheckResult      string `json:"fraud_check_result"`
	RetryCount            int    `json:"retry_count"`
}

type GatewayError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

func (r PaymentResponse) Sanitized() PaymentResponse {
	r.PaymentInfo.CustomerData = r.PaymentInfo.CustomerData.Sanitized()
	if r.TransactionDetails.TokenPreview == "" {
		r.TransactionDetails.TokenPreview = TokenPreview(r.TransactionDetails.Token)
	}
	r.TransactionDetails.Token = ""
	return r
}

func (r CreatePaymentRequest) WithoutSensitiveAuthenticationData() CreatePaymentRequest {
	r.PaymentInfo.CustomerData = r.PaymentInfo.CustomerData.WithoutSensitiveAuthenticationData()
	return r
}

func (c CustomerData) WithoutSensitiveAuthenticationData() CustomerData {
	c.CvvCode = ""
	return c
}

func (c CustomerData) Sanitized() CustomerData {
	c.CardNumber = MaskCardNumber(c.CardNumber)
	return c.WithoutSensitiveAuthenticationData()
}

func SanitizePaymentPayloadJSON(payloadJSON string) string {
	if strings.TrimSpace(payloadJSON) == "" {
		return payloadJSON
	}

	var resp PaymentResponse
	if err := json.Unmarshal([]byte(payloadJSON), &resp); err != nil {
		return payloadJSON
	}
	if resp.ID == "" && resp.MerchantID == "" && resp.IdempotencyKey == "" {
		return sanitizeGenericPaymentJSON(payloadJSON)
	}

	b, err := json.Marshal(resp.Sanitized())
	if err != nil {
		return sanitizeGenericPaymentJSON(payloadJSON)
	}
	return string(b)
}

func sanitizeGenericPaymentJSON(payloadJSON string) string {
	var value any
	if err := json.Unmarshal([]byte(payloadJSON), &value); err != nil {
		return payloadJSON
	}
	if !removeSensitiveAuthenticationData(value) {
		return payloadJSON
	}
	b, err := json.Marshal(value)
	if err != nil {
		return payloadJSON
	}
	return string(b)
}

func removeSensitiveAuthenticationData(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		changed := false
		for key, nested := range v {
			if isSensitiveAuthenticationKey(key) {
				delete(v, key)
				changed = true
				continue
			}
			if removeSensitiveAuthenticationData(nested) {
				changed = true
			}
		}
		return changed
	case []any:
		changed := false
		for _, nested := range v {
			if removeSensitiveAuthenticationData(nested) {
				changed = true
			}
		}
		return changed
	default:
		return false
	}
}

func isSensitiveAuthenticationKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "cvv_code", "cvv", "cvc", "cid":
		return true
	default:
		return false
	}
}

func MaskCardNumber(card string) string {
	digits := digitsOnly(card)
	if len(digits) < 10 {
		return ""
	}
	return digits[:6] + strings.Repeat("*", len(digits)-10) + digits[len(digits)-4:]
}

func TokenPreview(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if len(token) <= 12 {
		return token
	}
	return token[:8] + "..." + token[len(token)-4:]
}

func digitsOnly(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
