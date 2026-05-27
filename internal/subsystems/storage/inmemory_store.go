package storage

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

type InMemoryTransactionStore struct {
	mu sync.RWMutex

	byIdempotency map[string]storedTx

	byPaymentID map[string]storedTx
	refunds     map[string]storedRefund
}

type storedTx struct {
	status         string
	payloadJSON    string
	idempotencyKey string
	createdAt      time.Time
	updatedAt      time.Time
}

type storedRefund struct {
	refund dto.Refund
}

func NewInMemoryTransactionStore() contracts.TransactionStore {
	return &InMemoryTransactionStore{
		byIdempotency: make(map[string]storedTx),
		byPaymentID:   make(map[string]storedTx),
		refunds:       make(map[string]storedRefund),
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
	if merchantID == "" || idempotencyKey == "" {
		return errors.New("merchantID and idempotencyKey are required")
	}

	if payloadJSON != "" {
		payloadJSON = dto.SanitizePaymentPayloadJSON(payloadJSON)
		var tmp interface{}
		if err := json.Unmarshal([]byte(payloadJSON), &tmp); err != nil {
			return err
		}
	}

	key := merchantID + ":" + idempotencyKey

	s.mu.Lock()
	defer s.mu.Unlock()

	tx := storedTx{
		status:         status,
		payloadJSON:    payloadJSON,
		idempotencyKey: idempotencyKey,
		createdAt:      updatedAt,
		updatedAt:      updatedAt,
	}
	if existing, ok := s.byIdempotency[key]; ok && !existing.createdAt.IsZero() {
		tx.createdAt = existing.createdAt
	}

	s.byIdempotency[key] = tx
	if paymentID != "" {
		s.byPaymentID[merchantID+":"+paymentID] = tx
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

func (s *InMemoryTransactionStore) GetByPaymentID(
	ctx context.Context,
	merchantID string,
	paymentID string,
) (status string, payloadJSON string, found bool, err error) {
	_ = ctx

	if merchantID == "" || paymentID == "" {
		return "", "", false, errors.New("merchantID and paymentID are required")
	}

	key := merchantID + ":" + paymentID

	s.mu.RLock()
	defer s.mu.RUnlock()

	tx, ok := s.byPaymentID[key]
	if !ok {
		return "", "", false, nil
	}

	return tx.status, tx.payloadJSON, true, nil
}

func (s *InMemoryTransactionStore) SaveRefund(ctx context.Context, refund dto.Refund) error {
	_ = ctx
	if refund.MerchantID == "" || refund.ID == "" {
		return errors.New("merchantID and refundID are required")
	}
	refund.Status = dto.NormalizeRefundStatus(refund.Status)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.refunds[refund.MerchantID+":"+refund.ID] = storedRefund{refund: refund}
	return nil
}

func (s *InMemoryTransactionStore) GetRefundByID(ctx context.Context, merchantID string, refundID string) (dto.Refund, bool, error) {
	_ = ctx
	if merchantID == "" || refundID == "" {
		return dto.Refund{}, false, errors.New("merchantID and refundID are required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	refund, ok := s.refunds[merchantID+":"+refundID]
	return refund.refund, ok, nil
}

func (s *InMemoryTransactionStore) GetRefundByIdempotencyKey(ctx context.Context, merchantID string, idempotencyKey string) (dto.Refund, bool, error) {
	_ = ctx
	if merchantID == "" || idempotencyKey == "" {
		return dto.Refund{}, false, errors.New("merchantID and idempotencyKey are required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, refund := range s.refunds {
		if refund.refund.MerchantID == merchantID && refund.refund.IdempotencyKey == idempotencyKey {
			return refund.refund, true, nil
		}
	}
	return dto.Refund{}, false, nil
}

func (s *InMemoryTransactionStore) ListRefundsByPaymentID(ctx context.Context, merchantID string, paymentID string) ([]dto.Refund, error) {
	_ = ctx
	if merchantID == "" {
		return nil, errors.New("merchantID is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]dto.Refund, 0)
	for _, refund := range s.refunds {
		if refund.refund.MerchantID != merchantID {
			continue
		}
		if paymentID != "" && refund.refund.PaymentID != paymentID {
			continue
		}
		result = append(result, refund.refund)
	}
	return result, nil
}
