package simple

import (
	"context"
	"fmt"
	"strings"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

type simplePaymentRouter struct {
	routes contracts.PaymentRouteStore
}

func newSimplePaymentRouter() contracts.PaymentRouter {
	return newSimplePaymentRouterWithStore(nil)
}

func newSimplePaymentRouterWithStore(routes contracts.PaymentRouteStore) contracts.PaymentRouter {
	return &simplePaymentRouter{routes: routes}
}

func (r *simplePaymentRouter) Route(
	ctx context.Context,
	req dto.CreatePaymentRequest,
	fraud contracts.AntiFraudResult,
) (paymentSystem string, adapterKey string, err error) {
	_ = fraud

	if r.routes != nil {
		route, found, err := r.routes.GetActiveRoute(ctx, req.MerchantID, req.PaymentInfo.PaymentMethodData.Type)
		if err != nil {
			return "", "", fmt.Errorf("failed to load payment route: %w", err)
		}
		if found {
			// Здесь маршрутизатор сам выбирает внешний provider для конкретного мерчанта и способа оплаты.
			// adapterKey = provider: yookassa / stripe / dummy / mock и т.д.
			return route.PaymentSystem, strings.ToLower(strings.TrimSpace(route.Provider)), nil
		}
	}

	// Fallback для разработки: если маршрута в БД нет, используем старую логику.
	// В этом режиме Factory может взять provider из переменных окружения или dummy.
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
