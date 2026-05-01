package dto

import "time"

// PaymentMethodType определяет тип способа оплаты.
type PaymentMethodType string

const (
	PaymentMethodSBP            PaymentMethodType = "СБП"
	PaymentMethodCard          PaymentMethodType = "Банковская карта"
	PaymentMethodDigitalWallet PaymentMethodType = "Цифровой кошелек"
)

// PaymentCurrency — валютный код (для примера используется RUB).
type PaymentCurrency string

// AmountMoney — сумма платежа.
type AmountMoney struct {
	Value    float64          `json:"value"`
	Currency PaymentCurrency `json:"currency"`
}

// CustomerData — данные клиента (в зависимости от типа оплаты набор полей может отличаться).
type CustomerData struct {
	Email           string `json:"email,omitempty"`
	Phone           string `json:"phone,omitempty"`
	CardNumber      string `json:"card_number,omitempty"`
	CardDate        string `json:"card_date,omitempty"`
	CvvCode         string `json:"CVV_code,omitempty"`
	DigitalWalletID string `json:"digital_wallet_id,omitempty"`
}

// PaymentMethodData — данные способа оплаты.
type PaymentMethodData struct {
	Type PaymentMethodType `json:"type"`
}

// CreatePaymentRequest — запрос на создание платежа.
type CreatePaymentRequest struct {
	MerchantID      string `json:"merchant_id"`
	IdempotencyKey string `json:"idempotency_key"`
	PaymentID       string `json:"payment_id"`
	CurrentStatus   string `json:"current_status"`

	PaymentInfo PaymentInfo `json:"payment_info"`
}

type PaymentInfo struct {
	Amount            AmountMoney        `json:"amount"`
	PaymentMethodData PaymentMethodData `json:"payment_method_data"`
	CustomerData      CustomerData      `json:"customer_data"`
	CreatedAt         time.Time         `json:"created_at"`
	Description       string            `json:"description"`
}

// PaymentResponse — ответ платежного шлюза.
type PaymentResponse struct {
	ID              string `json:"id"`
	MerchantID      string `json:"merchant_id"`
	IdempotencyKey string `json:"idempotency_key"`
	CurrentStatus  string `json:"current_status"`

	PaymentInfo PaymentInfoResponse `json:"payment_info"`

	TransactionDetails TransactionDetails `json:"transaction_details"`
	Error              *GatewayError      `json:"error"`
}

type PaymentInfoResponse struct {
	Amount            AmountMoney        `json:"amount"`
	PaymentMethodData PaymentMethodData `json:"payment_method_data"`
	CustomerData      CustomerData      `json:"customer_data"`
	Description       string            `json:"description"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

type TransactionDetails struct {
	ExternalTransactionID string `json:"external_transaction_id"`
	PaymentSystem        string `json:"payment_system"`
	Token                string `json:"token"`
	FraudCheckResult     string `json:"fraud_check_result"`
	RetryCount           int    `json:"retry_count"`
}

type GatewayError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}
