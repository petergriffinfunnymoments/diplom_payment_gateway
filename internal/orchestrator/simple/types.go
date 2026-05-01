package simple

import (
	"context"
	"time"

	"payment-gateway/internal/contracts"
)

const (
	statusCreated = "CREATED"
	statusSuccess = "SUCCESS"
	statusFailed  = "FAILED"
	statusPending = "PENDING"

	statusValidated           = "VALIDATED"
	statusFraudChecked        = "FRAUD_CHECKED"
	statusTokenized           = "TOKENIZED"
	statusSentToPaymentSystem = "SENT_TO_PAYMENT_SYSTEM"
)

type noOpLogger struct{}

func (l noOpLogger) Log(ctx context.Context, event contracts.PaymentEvent) error {
	_ = ctx
	_ = event
	return nil
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
