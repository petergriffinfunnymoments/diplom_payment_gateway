package validator

import (
	"context"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

// DummyValidator — временная заглушка валидатора.
// В дипломе позже заменим на реальную валидацию схемы/полей и нормализацию.
type DummyValidator struct{}

func NewDummyValidator() contracts.PaymentValidator {
	return &DummyValidator{}
}

func (v *DummyValidator) Validate(ctx context.Context, req dto.CreatePaymentRequest) (dto.CreatePaymentRequest, error) {
	_ = ctx
	// Минимальная “проверка существования” для каркаса:
	// (дальше добавим валидацию сумм, валюты, форматов карт/телефонов и т.д.)
	return req, nil
}
