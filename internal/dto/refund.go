package dto

import (
	"strings"
	"time"
)

type RefundStatus string

const (
	RefundStatusProcess RefundStatus = "PROCESS_REFUND"
	RefundStatusSuccess RefundStatus = "SUCCESS_REFUND"
	RefundStatusFail    RefundStatus = "FAIL_REFUND"
)

func NormalizeRefundStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case string(RefundStatusProcess), "PROCESS", "PROCESSING", "PENDING":
		return string(RefundStatusProcess)
	case string(RefundStatusSuccess), "SUCCESS", "SUCCEEDED", "SUCCEEDED_REFUND":
		return string(RefundStatusSuccess)
	case string(RefundStatusFail), "FAIL", "FAILED", "CANCELED", "CANCELLED":
		return string(RefundStatusFail)
	default:
		return string(RefundStatusProcess)
	}
}

type Refund struct {
	ID                string          `json:"id"`
	MerchantID        string          `json:"merchant_id,omitempty"`
	Status            string          `json:"status"`
	Amount            float64         `json:"amount"`
	Currency          PaymentCurrency `json:"currency"`
	EntityType        string          `json:"entity_type"`
	EntityID          string          `json:"entity_id"`
	PaymentID         string          `json:"payment_id"`
	IdempotencyKey    string          `json:"idempotency_key,omitempty"`
	Provider          string          `json:"provider,omitempty"`
	PaymentSystem     string          `json:"payment_system,omitempty"`
	ExternalRefundID  string          `json:"external_refund_id,omitempty"`
	ProviderStatus    string          `json:"provider_status,omitempty"`
	ProviderErrorCode string          `json:"provider_error_code,omitempty"`
	ProviderErrorMsg  string          `json:"provider_error_message,omitempty"`
	RefundType        string          `json:"refund_type"`
	Reason            string          `json:"reason,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type CreateRefundRequest struct {
	MerchantID     string       `json:"merchant_id"`
	PaymentID      string       `json:"payment_id"`
	IdempotencyKey string       `json:"idempotency_key"`
	Amount         *AmountMoney `json:"amount,omitempty"`
	Reason         string       `json:"reason,omitempty"`
}

type RefundAPIResponse struct {
	Data    *Refund       `json:"data,omitempty"`
	Success bool          `json:"success"`
	Error   *GatewayError `json:"error,omitempty"`
}

type RefundSearchResponse struct {
	Data    []Refund      `json:"data"`
	Success bool          `json:"success"`
	Error   *GatewayError `json:"error,omitempty"`
}
