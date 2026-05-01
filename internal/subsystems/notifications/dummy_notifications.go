package notifications

import (
	"context"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

type DummyNotifications struct{}

// NewDummyNotifications создаёт заглушку сервиса уведомлений.
func NewDummyNotifications() contracts.Notifications {
	return &DummyNotifications{}
}

func (n *DummyNotifications) Notify(ctx context.Context, resp dto.PaymentResponse) error {
	_ = ctx
	_ = resp

	// Каркас: реальная реализация позже отправит данные в API-шлюз/внешним контрагентам.
	return nil
}
