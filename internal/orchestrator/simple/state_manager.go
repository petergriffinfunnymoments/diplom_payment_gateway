package simple

import (
	"context"
	"sync"

	"payment-gateway/internal/contracts"
)

type inMemoryStateManager struct {
	mu        sync.RWMutex
	byPayment map[string]string
}

func newInMemoryStateManager() contracts.TransactionStateManager {
	return &inMemoryStateManager{byPayment: make(map[string]string)}
}

func (s *inMemoryStateManager) GetStatus(ctx context.Context, merchantID, paymentID string) (string, error) {
	_ = ctx
	key := merchantID + ":" + paymentID
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.byPayment[key], nil
}

func (s *inMemoryStateManager) SetStatus(ctx context.Context, merchantID, paymentID, status string) error {
	_ = ctx
	key := merchantID + ":" + paymentID
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byPayment[key] = status
	return nil
}
