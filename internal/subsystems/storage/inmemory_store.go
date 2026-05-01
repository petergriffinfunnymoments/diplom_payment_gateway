package storage

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"payment-gateway/internal/contracts"
)

type InMemoryTransactionStore struct {
	mu sync.RWMutex
	// key: merchant_id + ":" + idempotency_key
	byIdempotency map[string]storedTx
}

type storedTx struct {
	status     string
	payloadJSON string
	updatedAt  time.Time
}

func NewInMemoryTransactionStore() contracts.TransactionStore {
	return &InMemoryTransactionStore{
		byIdempotency: make(map[string]storedTx),
	}
}

func (s *InMemoryTransactionStore) Save(
	ctx context.Context,
	merchantID string,
	paymentID string,
	idempotencyKey string,
	status string,
	payloadJSON string,
	updatedAt time.Time,
) error {
	_ = ctx
	_ = paymentID

	if merchantID == "" || idempotencyKey == "" {
		return errors.New("merchantID and idempotencyKey are required")
	}
	// В дипломе: здесь можно валидировать payloadJSON (json.Valid). Для каркаса — сделаем базовую проверку.
	if payloadJSON != "" {
		var tmp interface{}
		if err := json.Unmarshal([]byte(payloadJSON), &tmp); err != nil {
			return err
		}
	}

	key := merchantID + ":" + idempotencyKey

	s.mu.Lock()
	defer s.mu.Unlock()

	s.byIdempotency[key] = storedTx{
		status:      status,
		payloadJSON: payloadJSON,
		updatedAt:   updatedAt,
	}
	return nil
}

func (s *InMemoryTransactionStore) GetByIdempotencyKey(
	ctx context.Context,
	merchantID string,
	idempotencyKey string,
) (status string, payloadJSON string, found bool, err error) {
	_ = ctx

	if merchantID == "" || idempotencyKey == "" {
		return "", "", false, errors.New("merchantID and idempotencyKey are required")
	}

	key := merchantID + ":" + idempotencyKey

	s.mu.RLock()
	defer s.mu.RUnlock()

	tx, ok := s.byIdempotency[key]
	if !ok {
		return "", "", false, nil
	}

	return tx.status, tx.payloadJSON, true, nil
}
