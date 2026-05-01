package adapter

import (
	"context"
	"fmt"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

type DummyAdapter struct {
	paymentSystem string
}

func NewDummyAdapter(paymentSystem string) contracts.PaymentAdapter {
	if paymentSystem == "" {
		paymentSystem = "DUMMY"
	}
	return &DummyAdapter{paymentSystem: paymentSystem}
}

func (a *DummyAdapter) Send(ctx context.Context, token string, req dto.CreatePaymentRequest) (contracts.AdapterResult, error) {
	_ = ctx
	_ = req

	// Каркас: эмулируем задержку сети.
	_ = time.Now()

	extID := fmt.Sprintf("ext_%d", time.Now().UnixNano())

	return contracts.AdapterResult{
		ExternalTransactionID: extID,
		PaymentSystem:         a.paymentSystem,
		Status:                "SUCCESS",
		ErrorMessage:          "",
	}, nil
}
