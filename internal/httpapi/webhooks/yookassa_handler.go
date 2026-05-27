package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

type YooKassaWebhookHandler struct {
	store         contracts.TransactionStore
	logger        contracts.EventLogger
	notifications contracts.Notifications
	shopID        string
	secretKey     string
	apiURL        string
	client        *http.Client
}

func NewYooKassaWebhookHandler(store contracts.TransactionStore, logger contracts.EventLogger) http.Handler {
	return NewYooKassaWebhookHandlerWithNotifications(store, logger, nil)
}

func NewYooKassaWebhookHandlerWithNotifications(store contracts.TransactionStore, logger contracts.EventLogger, notifications contracts.Notifications) http.Handler {
	apiURL := strings.TrimSpace(os.Getenv("YOOKASSA_API_BASE_URL"))
	if apiURL == "" {
		apiURL = "https://api.yookassa.ru/v3"
	}

	return &YooKassaWebhookHandler{
		store:         store,
		logger:        logger,
		notifications: notifications,
		shopID:        strings.TrimSpace(os.Getenv("YOOKASSA_SHOP_ID")),
		secretKey:     strings.TrimSpace(os.Getenv("YOOKASSA_SECRET_KEY")),
		apiURL:        strings.TrimRight(apiURL, "/"),
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (h *YooKassaWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()

	var notification yookassaNotification
	if err := json.NewDecoder(r.Body).Decode(&notification); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if notification.Type != "notification" || notification.Event == "" || notification.Object.ID == "" {
		http.Error(w, "invalid notification", http.StatusBadRequest)
		return
	}

	payment := notification.Object
	if verified, err := h.fetchPayment(r.Context(), notification.Object.ID); err == nil && verified.ID != "" {
		payment = verified
	}

	merchantID := strings.TrimSpace(payment.Metadata.MerchantID)
	paymentID := strings.TrimSpace(payment.Metadata.PaymentID)
	idempotencyKey := strings.TrimSpace(payment.Metadata.IdempotencyKey)

	if merchantID == "" || paymentID == "" || idempotencyKey == "" {

		logCtx, cancel := ctxWithTimeout(r.Context())
		defer cancel()
		_ = h.log(logCtx, contracts.PaymentEvent{
			Type:          contracts.EventPaymentFailed,
			Level:         contracts.LogLevelWarn,
			Service:       "adapter",
			CurrentStatus: string(dto.StatusFailed),
			Timestamp:     time.Now().UTC(),
			Message:       "YooKassa webhook ignored: missing metadata",
			Details:       "YooKassa webhook ignored: missing metadata",
			Context: map[string]string{
				"provider":                "yookassa",
				"yookassa_event":          notification.Event,
				"external_transaction_id": payment.ID,
				"provider_status":         payment.Status,
			},
		})
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		return
	}

	status := mapYooKassaStatus(payment.Status)
	providerStatus := payment.Status

	resp := h.loadOrBuildResponse(r.Context(), merchantID, paymentID, idempotencyKey, payment)
	resp.ID = paymentID
	resp.MerchantID = merchantID
	resp.IdempotencyKey = idempotencyKey
	resp.CurrentStatus = status
	resp.PaymentInfo.UpdatedAt = time.Now().UTC()
	resp.TransactionDetails.ExternalTransactionID = payment.ID
	resp.TransactionDetails.PaymentSystem = "YOOKASSA"
	resp.TransactionDetails.ProviderStatus = providerStatus
	resp.TransactionDetails.PaymentURL = ""
	resp.TransactionDetails.CancellationParty = payment.CancellationDetails.Party
	resp.TransactionDetails.CancellationReason = payment.CancellationDetails.Reason

	if status == string(dto.StatusDeclined) || status == string(dto.StatusFailed) {
		providerReason := strings.TrimSpace(payment.CancellationDetails.Reason)
		providerParty := strings.TrimSpace(payment.CancellationDetails.Party)
		msg := providerReason
		if msg == "" {
			msg = "payment was canceled by YooKassa"
		}

		resp.TransactionDetails.ProviderErrorCode = providerReason
		resp.TransactionDetails.ProviderErrorMessage = msg
		resp.TransactionDetails.FraudCheckResult = fraudResultFromYooKassaCancellation(providerReason)
		resp.Error = dto.NewGatewayError(
			gatewayErrorCodeFromYooKassaCancellation(providerReason),
			formatYooKassaDeclineMessage(providerParty, providerReason),
		)
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
	webhookLogMessage := "YooKassa webhook processed"
	if status == string(dto.StatusDeclined) || status == string(dto.StatusFailed) {
		webhookLogType = contracts.EventPaymentFailed
		webhookLogLevel = contracts.LogLevelWarn
		webhookLogMessage = "YooKassa webhook processed: payment declined"
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
			"provider":                "yookassa",
			"yookassa_event":          notification.Event,
			"external_transaction_id": payment.ID,
			"provider_status":         providerStatus,
			"cancellation_party":      payment.CancellationDetails.Party,
			"cancellation_reason":     payment.CancellationDetails.Reason,
			"gateway_error_code":      gatewayErrorCodeFromYooKassaCancellation(payment.CancellationDetails.Reason),
			"paid":                    strconv.FormatBool(payment.Paid),
			"test":                    strconv.FormatBool(payment.Test),
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
				Message:        "Merchant notification after YooKassa webhook failed",
				Details:        "Merchant notification after YooKassa webhook failed",
				Context: map[string]string{
					"provider":      "yookassa",
					"error_message": err.Error(),
				},
			})
		}
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (h *YooKassaWebhookHandler) fetchPayment(ctx context.Context, paymentID string) (yookassaPaymentObject, error) {
	if h.shopID == "" || h.secretKey == "" || paymentID == "" {
		return yookassaPaymentObject{}, fmt.Errorf("yookassa credentials are empty")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.apiURL+"/payments/"+paymentID, nil)
	if err != nil {
		return yookassaPaymentObject{}, err
	}
	req.SetBasicAuth(h.shopID, h.secretKey)

	res, err := h.client.Do(req)
	if err != nil {
		return yookassaPaymentObject{}, err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return yookassaPaymentObject{}, fmt.Errorf("yookassa get payment returned HTTP %d", res.StatusCode)
	}

	var payment yookassaPaymentObject
	if err := json.NewDecoder(res.Body).Decode(&payment); err != nil {
		return yookassaPaymentObject{}, err
	}
	return payment, nil
}

func (h *YooKassaWebhookHandler) loadOrBuildResponse(ctx context.Context, merchantID, paymentID, idempotencyKey string, payment yookassaPaymentObject) dto.PaymentResponse {
	_, payloadJSON, found, err := h.store.GetByIdempotencyKey(ctx, merchantID, idempotencyKey)
	if err == nil && found && payloadJSON != "" {
		var resp dto.PaymentResponse
		if json.Unmarshal([]byte(payloadJSON), &resp) == nil {
			return resp
		}
	}

	amount, _ := strconv.ParseFloat(payment.Amount.Value, 64)
	createdAt := time.Now().UTC()
	if payment.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339Nano, payment.CreatedAt); err == nil {
			createdAt = t
		}
	}

	return dto.PaymentResponse{
		ID:             paymentID,
		MerchantID:     merchantID,
		IdempotencyKey: idempotencyKey,
		CurrentStatus:  mapYooKassaStatus(payment.Status),
		PaymentInfo: dto.PaymentInfoResponse{
			Amount: dto.AmountMoney{
				Value:    amount,
				Currency: dto.PaymentCurrency(payment.Amount.Currency),
			},
			PaymentMethodData: dto.PaymentMethodData{Type: dto.PaymentMethodCard},
			Description:       payment.Description,
			CreatedAt:         createdAt,
			UpdatedAt:         time.Now().UTC(),
		},
		TransactionDetails: dto.TransactionDetails{
			ExternalTransactionID: payment.ID,
			PaymentSystem:         "YOOKASSA",
			ProviderStatus:        payment.Status,
			FraudCheckResult:      "PASSED",
		},
	}
}

func (h *YooKassaWebhookHandler) log(ctx context.Context, event contracts.PaymentEvent) error {
	if h.logger == nil {
		return nil
	}
	return h.logger.Log(ctx, event)
}

func ctxWithTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 3*time.Second)
}

func gatewayErrorCodeFromYooKassaCancellation(reason string) string {
	reason = strings.ToLower(strings.TrimSpace(reason))
	switch reason {
	case "fraud_suspected":
		return dto.ErrorYooKassaFraudSuspected
	default:
		return dto.ErrorYooKassaPaymentDeclined
	}
}

func fraudResultFromYooKassaCancellation(reason string) string {
	if strings.EqualFold(strings.TrimSpace(reason), "fraud_suspected") {
		return dto.ErrorBlockedByProviderFraud
	}
	return dto.ErrorDeclinedByProvider
}

func formatYooKassaDeclineMessage(party string, reason string) string {
	party = strings.TrimSpace(party)
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "payment was canceled by YooKassa"
	}
	if party == "" {
		return reason
	}
	return fmt.Sprintf("%s: %s", party, reason)
}

func mapYooKassaStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "succeeded":
		return string(dto.StatusCaptured)
	case "pending", "waiting_for_capture":
		return string(dto.StatusPending)
	case "canceled":
		return string(dto.StatusDeclined)
	default:
		return string(dto.StatusPending)
	}
}

type yookassaNotification struct {
	Type   string                `json:"type"`
	Event  string                `json:"event"`
	Object yookassaPaymentObject `json:"object"`
}

type yookassaPaymentObject struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Paid        bool   `json:"paid"`
	Test        bool   `json:"test"`
	CreatedAt   string `json:"created_at"`
	Description string `json:"description"`
	Amount      struct {
		Value    string `json:"value"`
		Currency string `json:"currency"`
	} `json:"amount"`
	Metadata struct {
		MerchantID     string `json:"merchant_id"`
		PaymentID      string `json:"payment_id"`
		IdempotencyKey string `json:"idempotency_key"`
	} `json:"metadata"`
	CancellationDetails struct {
		Party  string `json:"party"`
		Reason string `json:"reason"`
	} `json:"cancellation_details"`
}
