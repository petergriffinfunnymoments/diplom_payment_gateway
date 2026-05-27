package notifications

import (
	"context"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

type NoOpNotifications struct{}

func NewNoOpNotifications() contracts.Notifications {
	return &NoOpNotifications{}
}

func (n *NoOpNotifications) Notify(ctx context.Context, resp dto.PaymentResponse) error {
	_ = ctx
	_ = resp

	return nil
}
