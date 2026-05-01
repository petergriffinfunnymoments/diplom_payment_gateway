package simple

import (
	"context"
	"fmt"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

type simplePaymentRouter struct{}

func newSimplePaymentRouter() contracts.PaymentRouter {
	return &simplePaymentRouter{}
}

func (r *simplePaymentRouter) Route(
	ctx context.Context,
	req dto.CreatePaymentRequest,
	fraud contracts.AntiFraudResult,
) (paymentSystem string, adapterKey string, err error) {
	_ = ctx
	_ = fraud

	switch req.PaymentInfo.PaymentMethodData.Type {
	case dto.PaymentMethodSBP:
		return "SBP", "sbp_adapter", nil
	case dto.PaymentMethodCard:
		return "CARD", "card_adapter", nil
	case dto.PaymentMethodDigitalWallet:
		return "DIGITAL_WALLET", "wallet_adapter", nil
	default:
		return "", "", fmt.Errorf("unsupported payment method: %s", req.PaymentInfo.PaymentMethodData.Type)
	}
}
