package simple

import (
	"context"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

const (
	statusCreated          = string(dto.StatusCreated)
	statusPending          = string(dto.StatusPending)
	statusCaptureRequested = string(dto.StatusCaptureRequested)
	statusCaptured         = string(dto.StatusCaptured)
	statusDeclined         = string(dto.StatusDeclined)
	statusCancelled        = string(dto.StatusCancelled)
	statusFailed           = string(dto.StatusFailed)
)

type noOpLogger struct{}

func (l noOpLogger) Log(ctx context.Context, event contracts.PaymentEvent) error {
	_ = ctx
	_ = event
	return nil
}

func nowUTC() time.Time {
	return time.Now().UTC()
}

func formatOptionalTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
