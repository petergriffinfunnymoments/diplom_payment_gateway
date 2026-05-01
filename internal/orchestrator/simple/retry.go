package simple

import (
	"context"
	"time"

	"payment-gateway/internal/contracts"
)

type simpleRetryHandler struct {
	maxAttempts int
}

func newSimpleRetryHandler() contracts.RetryHandler {
	return &simpleRetryHandler{maxAttempts: 3}
}

func (h *simpleRetryHandler) ShouldRetry(ctx context.Context, adapterResult contracts.AdapterResult, retryCount int) bool {
	_ = ctx
	if retryCount >= h.maxAttempts-1 {
		return false
	}
	return adapterResult.Status != statusCaptured
}

func (h *simpleRetryHandler) NextRetryCount(current int) int {
	return current + 1
}

func (h *simpleRetryHandler) RetryAfter(attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}
	return time.Duration(attempt) * 100 * time.Millisecond
}
