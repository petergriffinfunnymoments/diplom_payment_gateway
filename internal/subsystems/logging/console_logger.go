package logging

import (
	"context"
	"time"

	"github.com/go-kit/kit/log"
	"payment-gateway/internal/contracts"
)

type ConsoleEventLogger struct {
	logger log.Logger
}

func NewConsoleEventLogger(logger log.Logger) contracts.EventLogger {
	return &ConsoleEventLogger{logger: logger}
}

// NewDummyEventLogger оставлен для совместимости со старым кодом.
//
// Deprecated: use NewConsoleEventLogger.
func NewDummyEventLogger(logger log.Logger) contracts.EventLogger {
	return NewConsoleEventLogger(logger)
}

func (l *ConsoleEventLogger) Log(ctx context.Context, event contracts.PaymentEvent) error {
	_ = ctx

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.Level == "" {
		event.Level = contracts.LogLevelInfo
	}
	if event.Service == "" {
		event.Service = "orchestrator"
	}
	if event.Message == "" {
		event.Message = event.Details
	}
	event.Message = MaskSensitive(event.Message)

	return l.logger.Log(
		"level", event.Level,
		"service", event.Service,
		"event", event.Type,
		"merchant_id", event.MerchantID,
		"payment_id", event.PaymentID,
		"status", event.CurrentStatus,
		"ts", event.Timestamp.Format("2006-01-02T15:04:05.000Z07:00"),
		"message", event.Message,
	)
}
