package notifications

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"

	"github.com/jackc/pgx/v5/pgxpool"
)

type WebhookNotifications struct {
	callbackURL string
	secret      string
	client      *http.Client
	pool        *pgxpool.Pool
	logger      contracts.EventLogger
}

type MerchantNotificationPayload struct {
	Event                string          `json:"event"`
	PaymentID            string          `json:"payment_id"`
	MerchantID           string          `json:"merchant_id"`
	IdempotencyKey       string          `json:"idempotency_key"`
	Status               string          `json:"status"`
	Amount               dto.AmountMoney `json:"amount"`
	PaymentMethod        string          `json:"payment_method"`
	PaymentSystem        string          `json:"payment_system"`
	ProviderStatus       string          `json:"provider_status,omitempty"`
	ProviderErrorCode    string          `json:"provider_error_code,omitempty"`
	ProviderErrorMessage string          `json:"provider_error_message,omitempty"`
	CancellationParty    string          `json:"cancellation_party,omitempty"`
	CancellationReason   string          `json:"cancellation_reason,omitempty"`
	ExternalID           string          `json:"external_transaction_id,omitempty"`
	QRID                 string          `json:"qr_id,omitempty"`
	QRPayload            string          `json:"qr_payload,omitempty"`
	QRImageDataURI       string          `json:"qr_image_data_uri,omitempty"`
	QRExpiresAt          string          `json:"qr_expires_at,omitempty"`
	ParticipantBank      string          `json:"participant_bank,omitempty"`
	SchemaVersion        string          `json:"schema_version,omitempty"`
	SettlementHint       string          `json:"settlement_hint,omitempty"`
	ErrorCode            string          `json:"error_code,omitempty"`
	ErrorMessage         string          `json:"error_message,omitempty"`
	OccurredAt           time.Time       `json:"occurred_at"`
}

func NewWebhookNotificationsFromEnv(ctx context.Context, dsn string, logger contracts.EventLogger) (contracts.Notifications, error) {
	callbackURL := strings.TrimSpace(os.Getenv("MERCHANT_WEBHOOK_URL"))
	secret := strings.TrimSpace(os.Getenv("MERCHANT_WEBHOOK_SECRET"))

	n := &WebhookNotifications{
		callbackURL: callbackURL,
		secret:      secret,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}

	if dsn != "" {
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			return nil, err
		}
		n.pool = pool
		if err := n.ensureSchema(ctx); err != nil {
			pool.Close()
			return nil, err
		}
	}

	return n, nil
}

func (n *WebhookNotifications) Enabled() bool {
	return strings.TrimSpace(n.callbackURL) != ""
}

func (n *WebhookNotifications) Notify(ctx context.Context, resp dto.PaymentResponse) error {
	resp = resp.Sanitized()
	if !n.Enabled() {
		return nil
	}
	if resp.ID == "" || resp.MerchantID == "" {
		return errors.New("payment id and merchant id are required for notification")
	}

	payload := MerchantNotificationPayload{
		Event:                eventName(resp.CurrentStatus),
		PaymentID:            resp.ID,
		MerchantID:           resp.MerchantID,
		IdempotencyKey:       resp.IdempotencyKey,
		Status:               resp.CurrentStatus,
		Amount:               resp.PaymentInfo.Amount,
		PaymentMethod:        string(resp.PaymentInfo.PaymentMethodData.Type),
		PaymentSystem:        resp.TransactionDetails.PaymentSystem,
		ProviderStatus:       resp.TransactionDetails.ProviderStatus,
		ProviderErrorCode:    resp.TransactionDetails.ProviderErrorCode,
		ProviderErrorMessage: resp.TransactionDetails.ProviderErrorMessage,
		CancellationParty:    resp.TransactionDetails.CancellationParty,
		CancellationReason:   resp.TransactionDetails.CancellationReason,
		ExternalID:           resp.TransactionDetails.ExternalTransactionID,
		QRID:                 resp.TransactionDetails.QRID,
		QRPayload:            resp.TransactionDetails.QRPayload,
		QRImageDataURI:       resp.TransactionDetails.QRImageDataURI,
		QRExpiresAt:          resp.TransactionDetails.QRExpiresAt,
		ParticipantBank:      resp.TransactionDetails.ParticipantBank,
		SchemaVersion:        resp.TransactionDetails.SchemaVersion,
		SettlementHint:       resp.TransactionDetails.SettlementHint,
		OccurredAt:           time.Now().UTC(),
	}
	if resp.Error != nil {
		payload.ErrorCode = resp.Error.Code
		payload.ErrorMessage = resp.Error.Message
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	deliveryID := fmt.Sprintf("ntf_%d", time.Now().UnixNano())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.callbackURL, bytes.NewReader(body))
	if err != nil {
		_ = n.recordDelivery(ctx, deliveryID, resp, payload.Event, false, 0, err.Error())
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("X-Payment-Gateway-Event", payload.Event)
	req.Header.Set("X-Payment-Gateway-Delivery", deliveryID)
	req.Header.Set("Idempotency-Key", resp.IdempotencyKey)
	if n.secret != "" {
		req.Header.Set("X-Payment-Gateway-Signature", signBody(n.secret, body))
	}

	res, err := n.client.Do(req)
	if err != nil {
		_ = n.recordDelivery(ctx, deliveryID, resp, payload.Event, false, 0, err.Error())
		_ = n.log(ctx, resp, payload.Event, false, 0, err.Error(), deliveryID)
		return err
	}
	defer res.Body.Close()

	success := res.StatusCode >= 200 && res.StatusCode < 300
	var errMsg string
	if !success {
		errMsg = fmt.Sprintf("merchant webhook returned HTTP %d", res.StatusCode)
	}

	_ = n.recordDelivery(ctx, deliveryID, resp, payload.Event, success, res.StatusCode, errMsg)
	_ = n.log(ctx, resp, payload.Event, success, res.StatusCode, errMsg, deliveryID)

	if !success {
		return errors.New(errMsg)
	}
	return nil
}

func (n *WebhookNotifications) ensureSchema(ctx context.Context) error {
	_, err := n.pool.Exec(ctx, `
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS notification_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    delivery_id TEXT NOT NULL UNIQUE,
    merchant_id TEXT NOT NULL,
    payment_id TEXT NOT NULL,
    idempotency_key TEXT,
    event_type TEXT NOT NULL,
    callback_url TEXT NOT NULL,
    status_code INT,
    success BOOLEAN NOT NULL DEFAULT FALSE,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notification_deliveries_payment
    ON notification_deliveries (merchant_id, payment_id);

CREATE INDEX IF NOT EXISTS idx_notification_deliveries_created_at
    ON notification_deliveries (created_at);
`)
	return err
}

func (n *WebhookNotifications) recordDelivery(ctx context.Context, deliveryID string, resp dto.PaymentResponse, event string, success bool, statusCode int, errMsg string) error {
	if n.pool == nil {
		return nil
	}
	_, err := n.pool.Exec(ctx, `
INSERT INTO notification_deliveries (
    delivery_id,
    merchant_id,
    payment_id,
    idempotency_key,
    event_type,
    callback_url,
    status_code,
    success,
    error_message,
    created_at
) VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, 0), $8, NULLIF($9, ''), NOW())
ON CONFLICT (delivery_id) DO UPDATE
SET
    status_code = EXCLUDED.status_code,
    success = EXCLUDED.success,
    error_message = EXCLUDED.error_message
`, deliveryID, resp.MerchantID, resp.ID, resp.IdempotencyKey, event, n.callbackURL, statusCode, success, errMsg)
	return err
}

func (n *WebhookNotifications) log(ctx context.Context, resp dto.PaymentResponse, event string, success bool, statusCode int, errMsg string, deliveryID string) error {
	if n.logger == nil {
		return nil
	}
	level := contracts.LogLevelInfo
	eventType := contracts.EventNotificationSent
	message := "Merchant notification sent"
	if !success {
		level = contracts.LogLevelWarn
		eventType = contracts.EventNotificationFailed
		message = "Merchant notification failed"
	}
	return n.logger.Log(ctx, contracts.PaymentEvent{
		Type:           eventType,
		Level:          level,
		Service:        "notifications",
		MerchantID:     resp.MerchantID,
		PaymentID:      resp.ID,
		IdempotencyKey: resp.IdempotencyKey,
		CorrelationID:  resp.ID,
		CurrentStatus:  resp.CurrentStatus,
		Timestamp:      time.Now().UTC(),
		Message:        message,
		Details:        message,
		Context: map[string]string{
			"delivery_id":      deliveryID,
			"merchant_event":   event,
			"callback_url_set": strconv.FormatBool(n.callbackURL != ""),
			"status_code":      strconv.Itoa(statusCode),
			"success":          strconv.FormatBool(success),
			"error_message":    errMsg,
		},
	})
}

func eventName(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case string(dto.StatusCaptured):
		return "payment.captured"
	case string(dto.StatusDeclined):
		return "payment.declined"
	case string(dto.StatusFailed):
		return "payment.failed"
	case string(dto.StatusPending):
		return "payment.pending"
	case string(dto.StatusCancelled):
		return "payment.cancelled"
	default:
		return "payment.status_changed"
	}
}

func signBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
