package logging

import (
	"context"

	"github.com/go-kit/kit/log"
	"payment-gateway/internal/contracts"
)

type DummyEventLogger struct {
	logger log.Logger
}

func NewDummyEventLogger(logger log.Logger) contracts.EventLogger {
	return &DummyEventLogger{logger: logger}
}

func (l *DummyEventLogger) Log(ctx context.Context, event contracts.PaymentEvent) error {
	_ = ctx
	// Для диплома события можно структурировать, но сейчас достаточно вывести тип и идентификаторы.
	return l.logger.Log(
		"type", event.Type,
		"merchant_id", event.MerchantID,
		"payment_id", event.PaymentID,
		"ts", event.Timestamp.Format("2006-01-02T15:04:05.000Z07:00"),
		"details", event.Details,
	)
}
