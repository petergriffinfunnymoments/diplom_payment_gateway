package adapter

import (
	"context"
	"fmt"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

type SimulatedAdapter struct {
	paymentSystem string
}

func NewSimulatedAdapter(paymentSystem string) contracts.PaymentAdapter {
	if paymentSystem == "" {
		paymentSystem = "SIMULATED"
	}
	return &SimulatedAdapter{paymentSystem: paymentSystem}
}

// NewDummyAdapter сохраняет совместимость со старым provider key dummy.
//
// Deprecated: use NewSimulatedAdapter.
func NewDummyAdapter(paymentSystem string) contracts.PaymentAdapter {
	if paymentSystem == "" {
		paymentSystem = "DUMMY"
	}
	return NewSimulatedAdapter(paymentSystem)
}

func (a *SimulatedAdapter) Send(ctx context.Context, token string, req dto.CreatePaymentRequest) (contracts.AdapterResult, error) {
	_ = ctx
	_ = req

	// Локальная эмуляция успешного ответа платежного провайдера.
	_ = time.Now()

	extID := fmt.Sprintf("ext_%d", time.Now().UnixNano())

	return contracts.AdapterResult{
		ExternalTransactionID: extID,
		PaymentSystem:         a.paymentSystem,
		Status:                string(dto.StatusCaptured),
		ErrorMessage:          "",
	}, nil
}
