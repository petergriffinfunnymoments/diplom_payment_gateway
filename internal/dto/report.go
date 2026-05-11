package dto

import "time"

type TransactionReportFilter struct {
	MerchantID    string            `json:"merchant_id"`
	DateFrom      *time.Time        `json:"date_from,omitempty"`
	DateTo        *time.Time        `json:"date_to,omitempty"`
	Status        string            `json:"status,omitempty"`
	PaymentSystem string            `json:"payment_system,omitempty"`
	PaymentMethod PaymentMethodType `json:"payment_method,omitempty"`
	Limit         int               `json:"limit"`
}

type TransactionReportFilterResponse struct {
	MerchantID    string `json:"merchant_id"`
	DateFrom      string `json:"date_from,omitempty"`
	DateTo        string `json:"date_to,omitempty"`
	Status        string `json:"status,omitempty"`
	PaymentSystem string `json:"payment_system,omitempty"`
	PaymentMethod string `json:"payment_method,omitempty"`
	Limit         int    `json:"limit"`
}

type TransactionReportBucket struct {
	Count  int     `json:"count"`
	Amount float64 `json:"amount"`
}

type TransactionReportSummary struct {
	TotalCount      int                                `json:"total_count"`
	TotalAmount     float64                            `json:"total_amount"`
	CapturedCount   int                                `json:"captured_count"`
	CapturedAmount  float64                            `json:"captured_amount"`
	PendingCount    int                                `json:"pending_count"`
	DeclinedCount   int                                `json:"declined_count"`
	FailedCount     int                                `json:"failed_count"`
	AverageAmount   float64                            `json:"average_amount"`
	ByStatus        map[string]TransactionReportBucket `json:"by_status"`
	ByPaymentSystem map[string]TransactionReportBucket `json:"by_payment_system"`
	ByPaymentMethod map[string]TransactionReportBucket `json:"by_payment_method"`
}

type TransactionReportItem struct {
	PaymentID             string            `json:"payment_id"`
	IdempotencyKey        string            `json:"idempotency_key"`
	Status                string            `json:"status"`
	Amount                AmountMoney       `json:"amount"`
	PaymentMethod         PaymentMethodType `json:"payment_method"`
	PaymentSystem         string            `json:"payment_system,omitempty"`
	ProviderStatus        string            `json:"provider_status,omitempty"`
	ExternalTransactionID string            `json:"external_transaction_id,omitempty"`
	FraudCheckResult      string            `json:"fraud_check_result,omitempty"`
	CreatedAt             time.Time         `json:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at"`
}

type TransactionReport struct {
	MerchantID   string                          `json:"merchant_id"`
	GeneratedAt  time.Time                       `json:"generated_at"`
	Filter       TransactionReportFilterResponse `json:"filter"`
	Summary      TransactionReportSummary        `json:"summary"`
	Transactions []TransactionReportItem         `json:"transactions"`
}

type TransactionReportResponse struct {
	Data    *TransactionReport `json:"data,omitempty"`
	Success bool               `json:"success"`
	Error   *GatewayError      `json:"error,omitempty"`
}
