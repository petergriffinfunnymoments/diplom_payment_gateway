package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

type StripeWebhookHandler struct {
	store         contracts.TransactionStore
	logger        contracts.EventLogger
	notifications contracts.Notifications
	webhookSecret string
}

func NewStripeWebhookHandler(store contracts.TransactionStore, logger contracts.EventLogger, notifications contracts.Notifications) http.Handler {
	return &StripeWebhookHandler{
		store:         store,
		logger:        logger,
		notifications: notifications,
		webhookSecret: strings.TrimSpace(os.Getenv("STRIPE_WEBHOOK_SECRET")),
	}
}

func (h *StripeWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	if h.webhookSecret != "" {
		if err := verifyStripeSignature(body, r.Header.Get("Stripe-Signature"), h.webhookSecret, 5*time.Minute); err != nil {
			http.Error(w, "invalid stripe signature", http.StatusBadRequest)
			return
		}
	}

	var event stripeEvent
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if event.ID == "" || event.Type == "" {
		http.Error(w, "invalid stripe event", http.StatusBadRequest)
		return
	}

	session, err := stripeObjectToSession(event.Data.Object)
	if err != nil {
		_ = h.log(r.Context(), contracts.PaymentEvent{
			Type:          contracts.EventPaymentFailed,
			Level:         contracts.LogLevelWarn,
			Service:       "adapter",
			CurrentStatus: string(dto.StatusFailed),
			Timestamp:     time.Now().UTC(),
			Message:       "Stripe webhook ignored: unsupported payload",
			Details:       err.Error(),
			Context: map[string]string{
				"provider":     "stripe",
				"stripe_event": event.Type,
				"stripe_id":    event.ID,
			},
		})
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		return
	}

	merchantID := strings.TrimSpace(firstNonEmpty(session.Metadata.MerchantID, session.MetadataMap["merchant_id"]))
	paymentID := strings.TrimSpace(firstNonEmpty(session.Metadata.PaymentID, session.MetadataMap["payment_id"], session.ClientReferenceID))
	idempotencyKey := strings.TrimSpace(firstNonEmpty(session.Metadata.IdempotencyKey, session.MetadataMap["idempotency_key"]))

	if merchantID == "" || paymentID == "" || idempotencyKey == "" {
		logCtx, cancel := ctxWithTimeout(r.Context())
		defer cancel()
		_ = h.log(logCtx, contracts.PaymentEvent{
			Type:          contracts.EventPaymentFailed,
			Level:         contracts.LogLevelWarn,
			Service:       "adapter",
			CurrentStatus: string(dto.StatusFailed),
			Timestamp:     time.Now().UTC(),
			Message:       "Stripe webhook ignored: missing metadata",
			Details:       "Stripe webhook ignored: missing metadata",
			Context: map[string]string{
				"provider":                "stripe",
				"stripe_event":            event.Type,
				"external_transaction_id": session.ID,
				"provider_status":         firstNonEmpty(session.PaymentStatus, session.Status),
			},
		})
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		return
	}

	status := mapStripeWebhookStatus(event.Type, session.Status, session.PaymentStatus)
	providerStatus := firstNonEmpty(session.PaymentStatus, session.Status)

	resp := h.loadOrBuildResponse(r.Context(), merchantID, paymentID, idempotencyKey, session)
	resp.ID = paymentID
	resp.MerchantID = merchantID
	resp.IdempotencyKey = idempotencyKey
	resp.CurrentStatus = status
	resp.PaymentInfo.UpdatedAt = time.Now().UTC()
	resp.TransactionDetails.ExternalTransactionID = session.ID
	resp.TransactionDetails.PaymentSystem = "STRIPE"
	resp.TransactionDetails.ProviderStatus = providerStatus
	resp.TransactionDetails.PaymentURL = ""

	if status == string(dto.StatusDeclined) || status == string(dto.StatusFailed) {
		msg := stripeDeclineMessage(event.Type, session.Status, session.PaymentStatus)
		resp.TransactionDetails.ProviderErrorCode = event.Type
		resp.TransactionDetails.ProviderErrorMessage = msg
		resp.Error = &dto.GatewayError{Code: "STRIPE_PAYMENT_DECLINED", Message: msg}
	} else {
		resp.Error = nil
	}
	resp = resp.Sanitized()

	payload, _ := json.Marshal(resp)
	if err := h.store.Save(r.Context(), merchantID, paymentID, idempotencyKey, status, string(payload), time.Now().UTC()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	webhookLogType := contracts.EventAdapterResultReceived
	webhookLogLevel := contracts.LogLevelInfo
	webhookLogMessage := "Stripe webhook processed"
	if status == string(dto.StatusDeclined) || status == string(dto.StatusFailed) {
		webhookLogType = contracts.EventPaymentFailed
		webhookLogLevel = contracts.LogLevelWarn
		webhookLogMessage = "Stripe webhook processed: payment declined"
	}

	_ = h.log(r.Context(), contracts.PaymentEvent{
		Type:           webhookLogType,
		Level:          webhookLogLevel,
		Service:        "adapter",
		MerchantID:     merchantID,
		PaymentID:      paymentID,
		IdempotencyKey: idempotencyKey,
		CorrelationID:  paymentID,
		CurrentStatus:  status,
		Timestamp:      time.Now().UTC(),
		Message:        webhookLogMessage,
		Details:        webhookLogMessage,
		Context: map[string]string{
			"provider":                "stripe",
			"stripe_event":            event.Type,
			"stripe_event_id":         event.ID,
			"external_transaction_id": session.ID,
			"provider_status":         providerStatus,
			"payment_status":          session.PaymentStatus,
			"checkout_status":         session.Status,
			"amount_total":            strconv.FormatInt(session.AmountTotal, 10),
			"currency":                session.Currency,
		},
	})

	if h.notifications != nil {
		if err := h.notifications.Notify(r.Context(), resp); err != nil {
			_ = h.log(r.Context(), contracts.PaymentEvent{
				Type:           contracts.EventNotificationFailed,
				Level:          contracts.LogLevelWarn,
				Service:        "notifications",
				MerchantID:     merchantID,
				PaymentID:      paymentID,
				IdempotencyKey: idempotencyKey,
				CorrelationID:  paymentID,
				CurrentStatus:  status,
				Timestamp:      time.Now().UTC(),
				Message:        "Merchant notification after Stripe webhook failed",
				Details:        "Merchant notification after Stripe webhook failed",
				Context: map[string]string{
					"provider":      "stripe",
					"error_message": err.Error(),
				},
			})
		}
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (h *StripeWebhookHandler) loadOrBuildResponse(ctx context.Context, merchantID, paymentID, idempotencyKey string, session stripeSessionObject) dto.PaymentResponse {
	_, payloadJSON, found, err := h.store.GetByIdempotencyKey(ctx, merchantID, idempotencyKey)
	if err == nil && found && payloadJSON != "" {
		var resp dto.PaymentResponse
		if json.Unmarshal([]byte(payloadJSON), &resp) == nil {
			return resp
		}
	}

	amount := float64(session.AmountTotal) / 100.0
	createdAt := time.Now().UTC()
	if session.Created > 0 {
		createdAt = time.Unix(session.Created, 0).UTC()
	}

	return dto.PaymentResponse{
		ID:             paymentID,
		MerchantID:     merchantID,
		IdempotencyKey: idempotencyKey,
		CurrentStatus:  mapStripeWebhookStatus("", session.Status, session.PaymentStatus),
		PaymentInfo: dto.PaymentInfoResponse{
			Amount: dto.AmountMoney{
				Value:    amount,
				Currency: dto.PaymentCurrency(strings.ToUpper(session.Currency)),
			},
			PaymentMethodData: dto.PaymentMethodData{Type: dto.PaymentMethodCard},
			Description:       "Stripe payment",
			CreatedAt:         createdAt,
			UpdatedAt:         time.Now().UTC(),
		},
		TransactionDetails: dto.TransactionDetails{
			ExternalTransactionID: session.ID,
			PaymentSystem:         "STRIPE",
			ProviderStatus:        firstNonEmpty(session.PaymentStatus, session.Status),
			FraudCheckResult:      "PASSED",
		},
	}
}

func (h *StripeWebhookHandler) log(ctx context.Context, event contracts.PaymentEvent) error {
	if h.logger == nil {
		return nil
	}
	return h.logger.Log(ctx, event)
}

type stripeEvent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data struct {
		Object json.RawMessage `json:"object"`
	} `json:"data"`
}

type stripeSessionObject struct {
	ID                string            `json:"id"`
	Object            string            `json:"object"`
	Status            string            `json:"status"`
	PaymentStatus     string            `json:"payment_status"`
	PaymentIntent     string            `json:"payment_intent"`
	URL               string            `json:"url"`
	ClientReferenceID string            `json:"client_reference_id"`
	AmountTotal       int64             `json:"amount_total"`
	Currency          string            `json:"currency"`
	Created           int64             `json:"created"`
	MetadataMap       map[string]string `json:"metadata"`
	Metadata          struct {
		MerchantID     string `json:"merchant_id"`
		PaymentID      string `json:"payment_id"`
		IdempotencyKey string `json:"idempotency_key"`
	} `json:"-"`
}

func stripeObjectToSession(raw json.RawMessage) (stripeSessionObject, error) {
	var obj stripeSessionObject
	if err := json.Unmarshal(raw, &obj); err != nil {
		return stripeSessionObject{}, err
	}
	if obj.MetadataMap == nil {
		obj.MetadataMap = map[string]string{}
	}
	obj.Metadata.MerchantID = obj.MetadataMap["merchant_id"]
	obj.Metadata.PaymentID = obj.MetadataMap["payment_id"]
	obj.Metadata.IdempotencyKey = obj.MetadataMap["idempotency_key"]
	if obj.ID == "" {
		return stripeSessionObject{}, fmt.Errorf("stripe object id is empty")
	}
	return obj, nil
}

func mapStripeWebhookStatus(eventType string, status string, paymentStatus string) string {
	eventType = strings.ToLower(strings.TrimSpace(eventType))
	status = strings.ToLower(strings.TrimSpace(status))
	paymentStatus = strings.ToLower(strings.TrimSpace(paymentStatus))

	switch eventType {
	case "checkout.session.completed":
		if paymentStatus == "paid" || paymentStatus == "no_payment_required" {
			return string(dto.StatusCaptured)
		}
		return string(dto.StatusPending)
	case "checkout.session.expired", "checkout.session.async_payment_failed", "payment_intent.payment_failed":
		return string(dto.StatusDeclined)
	}

	if paymentStatus == "paid" || paymentStatus == "no_payment_required" {
		return string(dto.StatusCaptured)
	}
	if status == "expired" {
		return string(dto.StatusDeclined)
	}
	return string(dto.StatusPending)
}

func stripeDeclineMessage(eventType string, status string, paymentStatus string) string {
	if strings.TrimSpace(eventType) != "" {
		return "Stripe event " + eventType + " changed payment status to " + firstNonEmpty(paymentStatus, status, "unknown")
	}
	return "Stripe payment was declined"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func verifyStripeSignature(payload []byte, header string, secret string, tolerance time.Duration) error {
	if strings.TrimSpace(header) == "" {
		return fmt.Errorf("missing Stripe-Signature header")
	}
	parts := strings.Split(header, ",")
	var timestamp string
	var signatures []string
	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			timestamp = kv[1]
		case "v1":
			signatures = append(signatures, kv[1])
		}
	}
	if timestamp == "" || len(signatures) == 0 {
		return fmt.Errorf("invalid Stripe-Signature header")
	}

	if tolerance > 0 {
		ts, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid Stripe timestamp")
		}
		if time.Since(time.Unix(ts, 0)) > tolerance {
			return fmt.Errorf("Stripe webhook timestamp is too old")
		}
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))

	for _, sig := range signatures {
		if hmac.Equal([]byte(expected), []byte(strings.TrimSpace(sig))) {
			return nil
		}
	}
	return fmt.Errorf("Stripe signature mismatch")
}
