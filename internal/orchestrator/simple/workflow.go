package simple

import (
	"context"
	"fmt"
	"sync"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

type workflowSession struct {
	ID         string
	PaymentID  string
	MerchantID string
	Status     string
	StartedAt  time.Time
	FinishedAt *time.Time
}

type simpleWorkflowEngine struct {
	mu       sync.RWMutex
	sessions map[string]workflowSession
}

func newSimpleWorkflowEngine() contracts.WorkflowEngine {
	return &simpleWorkflowEngine{sessions: make(map[string]workflowSession)}
}

func (w *simpleWorkflowEngine) StartSession(ctx context.Context, req dto.CreatePaymentRequest) (string, error) {
	_ = ctx
	sessionID := fmt.Sprintf("sess_%s_%d", req.PaymentID, time.Now().UnixNano())

	w.mu.Lock()
	defer w.mu.Unlock()

	w.sessions[sessionID] = workflowSession{
		ID:         sessionID,
		PaymentID:  req.PaymentID,
		MerchantID: req.MerchantID,
		Status:     statusPending,
		StartedAt:  nowUTC(),
	}

	return sessionID, nil
}

func (w *simpleWorkflowEngine) CompleteSession(ctx context.Context, sessionID string, finalStatus string) error {
	_ = ctx
	finishedAt := nowUTC()

	w.mu.Lock()
	defer w.mu.Unlock()

	s, ok := w.sessions[sessionID]
	if !ok {
		return nil
	}
	s.Status = finalStatus
	s.FinishedAt = &finishedAt
	w.sessions[sessionID] = s
	return nil
}
