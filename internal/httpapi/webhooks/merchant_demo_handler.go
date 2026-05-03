package webhooks

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"payment-gateway/internal/contracts"
)

type MerchantDemoWebhookHandler struct {
	logger contracts.EventLogger
}

func NewMerchantDemoWebhookHandler(logger contracts.EventLogger) http.Handler {
	return &MerchantDemoWebhookHandler{logger: logger}
}

func (h *MerchantDemoWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()

	var payload struct {
		Event          string `json:"event"`
		PaymentID      string `json:"payment_id"`
		MerchantID     string `json:"merchant_id"`
		IdempotencyKey string `json:"idempotency_key"`
		Status         string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if h.logger != nil {
		_ = h.logger.Log(context.Background(), contracts.PaymentEvent{
			Type:           contracts.EventMerchantWebhookReceived,
			Level:          contracts.LogLevelInfo,
			Service:        "api_gateway",
			MerchantID:     payload.MerchantID,
			PaymentID:      payload.PaymentID,
			IdempotencyKey: payload.IdempotencyKey,
			CorrelationID:  payload.PaymentID,
			CurrentStatus:  payload.Status,
			Timestamp:      time.Now().UTC(),
			Message:        "Demo merchant webhook received notification from payment gateway",
			Details:        "Demo merchant webhook received notification from payment gateway",
			Context: map[string]string{
				"merchant_event": payload.Event,
			},
		})
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
