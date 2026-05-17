package webhooks

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

type DigitalRubleSandboxHandler struct {
	store         contracts.TransactionStore
	logger        contracts.EventLogger
	notifications contracts.Notifications
}

func NewDigitalRubleSandboxHandler(store contracts.TransactionStore, logger contracts.EventLogger, notifications contracts.Notifications) http.Handler {
	return &DigitalRubleSandboxHandler{
		store:         store,
		logger:        logger,
		notifications: notifications,
	}
}

type digitalRubleSandboxRequest struct {
	MerchantID string `json:"merchant_id"`
	PaymentID  string `json:"payment_id"`
	QRID       string `json:"qr_id"`
	Result     string `json:"result"`
	Reason     string `json:"reason"`
}

func (h *DigitalRubleSandboxHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeDigitalRubleSandboxError(w, http.StatusMethodNotAllowed, dto.ErrorMethodNotAllowed, "use POST")
		return
	}
	defer r.Body.Close()

	var req digitalRubleSandboxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeDigitalRubleSandboxError(w, http.StatusBadRequest, dto.ErrorBadRequest, "invalid json")
		return
	}
	req.MerchantID = strings.TrimSpace(req.MerchantID)
	req.PaymentID = strings.TrimSpace(req.PaymentID)
	req.QRID = strings.TrimSpace(req.QRID)
	req.Result = strings.ToLower(strings.TrimSpace(req.Result))
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Result == "" {
		req.Result = "captured"
	}
	if req.MerchantID == "" || req.PaymentID == "" {
		writeDigitalRubleSandboxError(w, http.StatusBadRequest, dto.ErrorBadRequest, "merchant_id and payment_id are required")
		return
	}

	_, payloadJSON, found, err := h.store.GetByPaymentID(r.Context(), req.MerchantID, req.PaymentID)
	if err != nil {
		writeDigitalRubleSandboxError(w, http.StatusInternalServerError, dto.ErrorPaymentStorage, err.Error())
		return
	}
	if !found {
		writeDigitalRubleSandboxError(w, http.StatusNotFound, dto.ErrorPaymentNotFound, "payment not found")
		return
	}

	var resp dto.PaymentResponse
	if err := json.Unmarshal([]byte(payloadJSON), &resp); err != nil {
		writeDigitalRubleSandboxError(w, http.StatusInternalServerError, dto.ErrorInvalidStoredResponse, err.Error())
		return
	}
	if strings.ToUpper(strings.TrimSpace(resp.TransactionDetails.PaymentSystem)) != "DIGITAL_RUBLE" {
		writeDigitalRubleSandboxError(w, http.StatusBadRequest, dto.ErrorBadRequest, "payment is not a digital ruble payment")
		return
	}
	if req.QRID != "" && resp.TransactionDetails.QRID != "" && req.QRID != resp.TransactionDetails.QRID {
		writeDigitalRubleSandboxError(w, http.StatusBadRequest, dto.ErrorBadRequest, "qr_id does not match payment")
		return
	}

	status, providerStatus, errCode, errMsg := digitalRubleSandboxResult(req.Result, req.Reason)
	if qrExpired(resp.TransactionDetails.QRExpiresAt) && status == string(dto.StatusCaptured) {
		status = string(dto.StatusCancelled)
		providerStatus = "qr_expired"
		errCode = dto.ErrorDigitalRubleQRExpired
		errMsg = "digital ruble QR code expired before confirmation"
	}

	resp.CurrentStatus = status
	resp.PaymentInfo.UpdatedAt = time.Now().UTC()
	resp.TransactionDetails.ProviderStatus = providerStatus
	resp.TransactionDetails.PaymentURL = ""
	resp.TransactionDetails.ProviderErrorCode = ""
	resp.TransactionDetails.ProviderErrorMessage = ""
	resp.TransactionDetails.FraudCheckResult = "PASSED"
	resp.Error = nil
	if errCode != "" {
		resp.TransactionDetails.ProviderErrorCode = providerStatus
		resp.TransactionDetails.ProviderErrorMessage = errMsg
		resp.Error = dto.NewGatewayError(errCode, errMsg)
	}
	resp = resp.Sanitized()

	payload, _ := json.Marshal(resp)
	if err := h.store.Save(r.Context(), resp.MerchantID, resp.ID, resp.IdempotencyKey, resp.CurrentStatus, string(payload), time.Now().UTC()); err != nil {
		writeDigitalRubleSandboxError(w, http.StatusInternalServerError, dto.ErrorPaymentStorage, err.Error())
		return
	}

	eventType := contracts.EventAdapterResultReceived
	level := contracts.LogLevelInfo
	message := "Digital ruble sandbox scan processed"
	if resp.Error != nil {
		eventType = contracts.EventPaymentFailed
		level = contracts.LogLevelWarn
		message = "Digital ruble sandbox scan processed with failure"
	}
	_ = h.log(r, contracts.PaymentEvent{
		Type:           eventType,
		Level:          level,
		Service:        "adapter",
		MerchantID:     resp.MerchantID,
		PaymentID:      resp.ID,
		IdempotencyKey: resp.IdempotencyKey,
		CorrelationID:  resp.ID,
		CurrentStatus:  resp.CurrentStatus,
		Timestamp:      time.Now().UTC(),
		Message:        message,
		Details:        message,
		Context: map[string]string{
			"provider":        "digital_ruble",
			"sandbox_result":  req.Result,
			"qr_id":           resp.TransactionDetails.QRID,
			"provider_status": providerStatus,
		},
	})

	if h.notifications != nil {
		if err := h.notifications.Notify(r.Context(), resp); err != nil {
			_ = h.log(r, contracts.PaymentEvent{
				Type:           contracts.EventNotificationFailed,
				Level:          contracts.LogLevelWarn,
				Service:        "notifications",
				MerchantID:     resp.MerchantID,
				PaymentID:      resp.ID,
				IdempotencyKey: resp.IdempotencyKey,
				CorrelationID:  resp.ID,
				CurrentStatus:  resp.CurrentStatus,
				Timestamp:      time.Now().UTC(),
				Message:        "Merchant notification after digital ruble sandbox scan failed",
				Details:        "Merchant notification after digital ruble sandbox scan failed",
				Context: map[string]string{
					"provider":      "digital_ruble",
					"error_message": err.Error(),
				},
			})
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func digitalRubleSandboxResult(result string, reason string) (status string, providerStatus string, errCode string, errMsg string) {
	switch strings.ToLower(strings.TrimSpace(result)) {
	case "captured", "capture", "success", "settled", "paid":
		return string(dto.StatusCaptured), "settled", "", ""
	case "declined", "reject", "rejected":
		if reason == "" {
			reason = "digital ruble payment rejected by participant bank emulator"
		}
		return string(dto.StatusDeclined), "participant_rejected", dto.ErrorDigitalRubleDeclined, reason
	case "failed", "error":
		if reason == "" {
			reason = "digital ruble participant bank emulator returned technical error"
		}
		return string(dto.StatusFailed), "technical_error", dto.ErrorDigitalRubleTechnical, reason
	case "expired", "cancelled", "canceled":
		if reason == "" {
			reason = "digital ruble QR code expired before confirmation"
		}
		return string(dto.StatusCancelled), "qr_expired", dto.ErrorDigitalRubleQRExpired, reason
	default:
		return string(dto.StatusCaptured), "settled", "", ""
	}
}

func qrExpired(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			t = parsed
		} else {
			return false
		}
	}
	return time.Now().UTC().After(t.UTC())
}

func (h *DigitalRubleSandboxHandler) log(r *http.Request, event contracts.PaymentEvent) error {
	if h.logger == nil {
		return nil
	}
	return h.logger.Log(r.Context(), event)
}

func writeDigitalRubleSandboxError(w http.ResponseWriter, status int, code string, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(dto.NewGatewayError(code, message))
}
