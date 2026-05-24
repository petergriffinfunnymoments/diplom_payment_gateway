package webhooks

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

type RobokassaWebhookHandler struct {
	store         contracts.TransactionStore
	logger        contracts.EventLogger
	notifications contracts.Notifications
	password2     string
	hashAlgorithm string
	testMode      bool
}

func NewRobokassaWebhookHandler(store contracts.TransactionStore, logger contracts.EventLogger, notifications contracts.Notifications) http.Handler {
	testMode := robokassaWebhookBoolEnv("ROBOKASSA_TEST_MODE", true)
	password2 := strings.TrimSpace(os.Getenv("ROBOKASSA_PASSWORD2"))
	if testMode {
		password2 = firstNonEmpty(os.Getenv("ROBOKASSA_TEST_PASSWORD2"), password2)
	}
	hashAlgorithm := strings.TrimSpace(os.Getenv("ROBOKASSA_HASH_ALGORITHM"))
	if hashAlgorithm == "" {
		hashAlgorithm = "md5"
	}

	return &RobokassaWebhookHandler{
		store:         store,
		logger:        logger,
		notifications: notifications,
		password2:     password2,
		hashAlgorithm: hashAlgorithm,
		testMode:      testMode,
	}
}

func (h *RobokassaWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if h.password2 == "" {
		http.Error(w, "robokassa webhook password is not configured", http.StatusInternalServerError)
		return
	}

	outSum := robokassaFormValue(r.Form, "OutSum")
	invID := firstNonEmpty(robokassaFormValue(r.Form, "InvId"), robokassaFormValue(r.Form, "InvoiceID"))
	signature := robokassaFormValue(r.Form, "SignatureValue")
	if outSum == "" || invID == "" || signature == "" {
		http.Error(w, "missing required Robokassa fields", http.StatusBadRequest)
		return
	}

	shp := robokassaShpValues(r.Form)
	expected, err := robokassaWebhookSignature(h.hashAlgorithm, outSum, invID, h.password2, shp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !strings.EqualFold(signature, expected) {
		http.Error(w, "invalid Robokassa signature", http.StatusBadRequest)
		return
	}

	merchantID := robokassaFormValue(r.Form, "Shp_merchant_id")
	paymentID := robokassaFormValue(r.Form, "Shp_payment_id")
	idempotencyKey := robokassaFormValue(r.Form, "Shp_idempotency_key")

	if merchantID == "" || paymentID == "" || idempotencyKey == "" {
		logCtx, cancel := ctxWithTimeout(r.Context())
		defer cancel()
		_ = h.log(logCtx, contracts.PaymentEvent{
			Type:          contracts.EventPaymentFailed,
			Level:         contracts.LogLevelWarn,
			Service:       "adapter",
			CurrentStatus: string(dto.StatusCaptured),
			Timestamp:     time.Now().UTC(),
			Message:       "Robokassa ResultURL ignored: missing Shp metadata",
			Details:       "Robokassa ResultURL ignored: missing Shp metadata",
			Context: map[string]string{
				"provider":                "robokassa",
				"external_transaction_id": invID,
				"provider_status":         "paid",
				"test":                    strconv.FormatBool(h.testMode),
			},
		})
		writeRobokassaOK(w, invID)
		return
	}

	status := string(dto.StatusCaptured)
	resp := h.loadOrBuildResponse(r.Context(), merchantID, paymentID, idempotencyKey, outSum, invID)
	resp.ID = paymentID
	resp.MerchantID = merchantID
	resp.IdempotencyKey = idempotencyKey
	resp.CurrentStatus = status
	resp.PaymentInfo.UpdatedAt = time.Now().UTC()
	resp.TransactionDetails.ExternalTransactionID = invID
	resp.TransactionDetails.PaymentSystem = "ROBOKASSA"
	resp.TransactionDetails.ProviderStatus = "paid"
	resp.TransactionDetails.PaymentURL = ""
	resp.Error = nil
	resp = resp.Sanitized()

	payload, _ := json.Marshal(resp)
	if err := h.store.Save(r.Context(), merchantID, paymentID, idempotencyKey, status, string(payload), time.Now().UTC()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_ = h.log(r.Context(), contracts.PaymentEvent{
		Type:           contracts.EventAdapterResultReceived,
		Level:          contracts.LogLevelInfo,
		Service:        "adapter",
		MerchantID:     merchantID,
		PaymentID:      paymentID,
		IdempotencyKey: idempotencyKey,
		CorrelationID:  paymentID,
		CurrentStatus:  status,
		Timestamp:      time.Now().UTC(),
		Message:        "Robokassa ResultURL processed",
		Details:        "Robokassa ResultURL processed",
		Context: map[string]string{
			"provider":                "robokassa",
			"external_transaction_id": invID,
			"provider_status":         "paid",
			"out_sum":                 outSum,
			"fee":                     robokassaFormValue(r.Form, "Fee"),
			"test":                    strconv.FormatBool(h.testMode),
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
				Message:        "Merchant notification after Robokassa ResultURL failed",
				Details:        "Merchant notification after Robokassa ResultURL failed",
				Context: map[string]string{
					"provider":      "robokassa",
					"error_message": err.Error(),
				},
			})
		}
	}

	writeRobokassaOK(w, invID)
}

func (h *RobokassaWebhookHandler) loadOrBuildResponse(ctx context.Context, merchantID, paymentID, idempotencyKey, outSum, invID string) dto.PaymentResponse {
	_, payloadJSON, found, err := h.store.GetByIdempotencyKey(ctx, merchantID, idempotencyKey)
	if err == nil && found && payloadJSON != "" {
		var resp dto.PaymentResponse
		if json.Unmarshal([]byte(payloadJSON), &resp) == nil {
			return resp
		}
	}

	amount, _ := strconv.ParseFloat(strings.ReplaceAll(outSum, ",", "."), 64)
	return dto.PaymentResponse{
		ID:             paymentID,
		MerchantID:     merchantID,
		IdempotencyKey: idempotencyKey,
		CurrentStatus:  string(dto.StatusCaptured),
		PaymentInfo: dto.PaymentInfoResponse{
			Amount: dto.AmountMoney{
				Value:    amount,
				Currency: dto.PaymentCurrency("RUB"),
			},
			PaymentMethodData: dto.PaymentMethodData{Type: dto.PaymentMethodCard},
			Description:       "Robokassa payment",
			CreatedAt:         time.Now().UTC(),
			UpdatedAt:         time.Now().UTC(),
		},
		TransactionDetails: dto.TransactionDetails{
			ExternalTransactionID: invID,
			PaymentSystem:         "ROBOKASSA",
			ProviderStatus:        "paid",
			FraudCheckResult:      "PASSED",
		},
	}
}

func (h *RobokassaWebhookHandler) log(ctx context.Context, event contracts.PaymentEvent) error {
	if h.logger == nil {
		return nil
	}
	return h.logger.Log(ctx, event)
}

func writeRobokassaOK(w http.ResponseWriter, invID string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK" + invID))
}

func robokassaShpValues(form url.Values) map[string]string {
	out := map[string]string{}
	for key, values := range form {
		if len(values) == 0 {
			continue
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(key)), "shp_") {
			out[key] = values[0]
		}
	}
	return out
}

func robokassaFormValue(form url.Values, key string) string {
	if v := strings.TrimSpace(form.Get(key)); v != "" {
		return v
	}
	for formKey, values := range form {
		if strings.EqualFold(formKey, key) && len(values) > 0 {
			return strings.TrimSpace(values[0])
		}
	}
	return ""
}

func robokassaWebhookSignature(algorithm string, outSum string, invID string, password string, shp map[string]string) (string, error) {
	parts := []string{
		strings.TrimSpace(outSum),
		strings.TrimSpace(invID),
		password,
	}

	keys := make([]string, 0, len(shp))
	for key := range shp {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts = append(parts, key+"="+shp[key])
	}

	return robokassaWebhookHash(algorithm, strings.Join(parts, ":"))
}

func robokassaWebhookHash(algorithm string, value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(algorithm)) {
	case "", "md5":
		sum := md5.Sum([]byte(value))
		return hex.EncodeToString(sum[:]), nil
	case "sha1":
		sum := sha1.Sum([]byte(value))
		return hex.EncodeToString(sum[:]), nil
	case "sha256":
		sum := sha256.Sum256([]byte(value))
		return hex.EncodeToString(sum[:]), nil
	case "sha384":
		sum := sha512.Sum384([]byte(value))
		return hex.EncodeToString(sum[:]), nil
	case "sha512":
		sum := sha512.Sum512([]byte(value))
		return hex.EncodeToString(sum[:]), nil
	default:
		return "", fmt.Errorf("unsupported Robokassa hash algorithm %q", algorithm)
	}
}

func robokassaWebhookBoolEnv(name string, defaultValue bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if value == "" {
		return defaultValue
	}
	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return defaultValue
	}
}
