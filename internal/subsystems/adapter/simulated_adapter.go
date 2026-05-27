package adapter

import (
	"context"
	"fmt"
	"strings"
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

func (a *SimulatedAdapter) Send(ctx context.Context, token string, req dto.CreatePaymentRequest) (contracts.AdapterResult, error) {
	_ = ctx
	_ = req

	_ = time.Now()

	extID := fmt.Sprintf("ext_%d", time.Now().UnixNano())

	return contracts.AdapterResult{
		ExternalTransactionID: extID,
		PaymentSystem:         a.paymentSystem,
		Status:                string(dto.StatusCaptured),
		ErrorMessage:          "",
	}, nil
}

func (a *SimulatedAdapter) Refund(ctx context.Context, req contracts.RefundRequest) (contracts.RefundResult, error) {
	_ = ctx

	status := string(dto.RefundStatusSuccess)
	providerStatus := "succeeded"
	errorMessage := ""
	if req.Reason == "simulate_failed" || req.PaymentID == "pay_refund_failed" {
		status = string(dto.RefundStatusFail)
		providerStatus = "failed"
		errorMessage = "simulated refund failure"
	}

	return contracts.RefundResult{
		ExternalRefundID: "rfnd_" + strings.TrimPrefix(req.RefundID, "ref_"),
		PaymentSystem:    a.paymentSystem,
		Status:           status,
		ProviderStatus:   providerStatus,
		ErrorMessage:     errorMessage,
	}, nil
}
