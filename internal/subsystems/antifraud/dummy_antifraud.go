package antifraud

import (
	"context"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

type DummyAntiFraud struct{}

func NewDummyAntiFraud() contracts.AntiFraud {
	return &DummyAntiFraud{}
}

func (a *DummyAntiFraud) Check(ctx context.Context, req dto.CreatePaymentRequest) (contracts.AntiFraudResult, error) {
	_ = ctx

	// Каркас: считаем, что антифрод всегда PASSED.
	// Позже заменим на реальные эвристики/правила, и учтём отличия для типов оплат (СБП/карта/кошелек).
	_ = req

	return contracts.AntiFraudResult{
		Result: "PASSED",
		Reason: "",
	}, nil
}
