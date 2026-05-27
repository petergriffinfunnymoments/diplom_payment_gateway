package refunds

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
	"payment-gateway/internal/subsystems/adapter"
	"payment-gateway/internal/subsystems/merchantauth"
)

type Handler struct {
	transactions   contracts.TransactionStore
	refunds        contracts.RefundStore
	adapterFactory *adapter.Factory
	logger         contracts.EventLogger
}

func NewRefundHandler(
	transactions contracts.TransactionStore,
	refunds contracts.RefundStore,
	adapterFactory *adapter.Factory,
	logger contracts.EventLogger,
) http.Handler {
	if adapterFactory == nil {
		adapterFactory = adapter.NewFactoryFromEnv()
	}
	return &Handler{
		transactions:   transactions,
		refunds:        refunds,
		adapterFactory: adapterFactory,
		logger:         logger,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/refunds/full/create":
		h.create(w, r, true)
	case r.Method == http.MethodPost && r.URL.Path == "/refunds/partial/create":
		h.create(w, r, false)
	case r.Method == http.MethodGet && r.URL.Path == "/refunds/status":
		h.status(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/refunds/search":
		h.search(w, r)
	default:
		writeJSON(w, http.StatusNotFound, dto.RefundAPIResponse{
			Success: false,
			Error:   dto.NewGatewayError(dto.ErrorNotFound, "refund endpoint not found"),
		})
	}
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request, full bool) {
	if h.transactions == nil || h.refunds == nil {
		writeRefundError(w, http.StatusInternalServerError, dto.ErrorRefundStoreUnavailable, "refund store is not configured")
		return
	}

	req, err := decodeCreateRefundRequest(r)
	if err != nil {
		writeRefundError(w, http.StatusBadRequest, dto.ErrorBadRequest, err.Error())
		return
	}
	req.MerchantID = strings.TrimSpace(req.MerchantID)
	req.PaymentID = strings.TrimSpace(req.PaymentID)
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	req.Reason = strings.TrimSpace(req.Reason)

	if err := validateCreateRefundRequest(req, full); err != nil {
		writeRefundError(w, http.StatusBadRequest, dto.ErrorValidation, err.Error())
		return
	}
	authMerchant, ok := merchantauth.MerchantFromContext(r.Context())
	if !ok {
		writeRefundError(w, http.StatusUnauthorized, dto.ErrorAuthContextMissing, "authenticated merchant context is required")
		return
	}
	if !merchantauth.CanWriteMerchantData(authMerchant, req.MerchantID) {
		merchantauth.LogAuthorizationFailed(r.Context(), h.logger, authMerchant, req.MerchantID, r.Method+" "+r.URL.Path, "refund creation is not allowed for this role or merchant")
		writeRefundError(w, http.StatusForbidden, dto.ErrorForbidden, "refund creation is not allowed for this role or merchant")
		return
	}

	if cached, found, err := h.refunds.GetRefundByIdempotencyKey(r.Context(), req.MerchantID, req.IdempotencyKey); err != nil {
		writeRefundError(w, http.StatusInternalServerError, dto.ErrorRefundStorage, err.Error())
		return
	} else if found {
		writeJSON(w, http.StatusOK, dto.RefundAPIResponse{Data: &cached, Success: cached.Status != string(dto.RefundStatusFail)})
		return
	}

	_, payloadJSON, found, err := h.transactions.GetByPaymentID(r.Context(), req.MerchantID, req.PaymentID)
	if err != nil {
		writeRefundError(w, http.StatusInternalServerError, dto.ErrorPaymentStorage, err.Error())
		return
	}
	if !found {
		writeRefundError(w, http.StatusNotFound, dto.ErrorPaymentNotFound, "payment not found")
		return
	}

	var payment dto.PaymentResponse
	if err := json.Unmarshal([]byte(payloadJSON), &payment); err != nil {
		writeRefundError(w, http.StatusInternalServerError, dto.ErrorInvalidStoredPayment, err.Error())
		return
	}
	if payment.CurrentStatus != string(dto.StatusCaptured) {
		writeRefundError(w, http.StatusBadRequest, dto.ErrorPaymentNotCaptured, "refund is allowed only for CAPTURED payments")
		return
	}

	amount := payment.PaymentInfo.Amount
	refundType := "full"
	if !full {
		amount = *req.Amount
		refundType = "partial"
	}
	if amount.Currency == "" {
		amount.Currency = payment.PaymentInfo.Amount.Currency
	}

	provider := providerKey(payment.TransactionDetails.PaymentSystem)
	paymentAdapter, selectedProvider, err := h.adapterFactory.Resolve(provider, payment.TransactionDetails.PaymentSystem)
	if err != nil {
		writeRefundError(w, http.StatusBadGateway, dto.ErrorAdapterFactory, err.Error())
		return
	}
	refundAdapter, ok := paymentAdapter.(contracts.RefundAdapter)
	if !ok {
		writeRefundError(w, http.StatusNotImplemented, dto.ErrorRefundNotSupported, fmt.Sprintf("provider %q does not support refunds", selectedProvider))
		return
	}

	now := time.Now().UTC()
	refund := dto.Refund{
		ID:             newRefundID(),
		MerchantID:     req.MerchantID,
		Status:         string(dto.RefundStatusProcess),
		Amount:         amount.Value,
		Currency:       amount.Currency,
		EntityType:     "payment",
		EntityID:       req.PaymentID,
		PaymentID:      req.PaymentID,
		IdempotencyKey: req.IdempotencyKey,
		Provider:       selectedProvider,
		PaymentSystem:  payment.TransactionDetails.PaymentSystem,
		RefundType:     refundType,
		Reason:         req.Reason,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_ = h.refunds.SaveRefund(r.Context(), refund)
	h.log(r.Context(), refund, contracts.EventRefundRequested, contracts.LogLevelInfo, "Refund request received")
	h.log(r.Context(), refund, contracts.EventRefundAdapterCalled, contracts.LogLevelInfo, "Refund adapter call started")

	result, err := refundAdapter.Refund(r.Context(), contracts.RefundRequest{
		RefundID:          refund.ID,
		MerchantID:        req.MerchantID,
		PaymentID:         req.PaymentID,
		IdempotencyKey:    req.IdempotencyKey,
		ExternalPaymentID: payment.TransactionDetails.ExternalTransactionID,
		Amount:            amount,
		Reason:            req.Reason,
		Full:              full,
		Payment:           payment,
	})
	if err != nil {
		result = contracts.RefundResult{
			PaymentSystem:  payment.TransactionDetails.PaymentSystem,
			Status:         string(dto.RefundStatusFail),
			ProviderStatus: "error",
			ErrorMessage:   err.Error(),
		}
	}

	refund.Status = dto.NormalizeRefundStatus(result.Status)
	refund.PaymentSystem = firstNonEmpty(result.PaymentSystem, refund.PaymentSystem)
	refund.ExternalRefundID = result.ExternalRefundID
	refund.ProviderStatus = result.ProviderStatus
	refund.ProviderErrorMsg = result.ErrorMessage
	refund.UpdatedAt = time.Now().UTC()
	if refund.Status == string(dto.RefundStatusFail) {
		refund.ProviderErrorCode = dto.ErrorRefundFailed
	}

	if err := h.refunds.SaveRefund(r.Context(), refund); err != nil {
		writeRefundError(w, http.StatusInternalServerError, dto.ErrorRefundStorage, err.Error())
		return
	}

	level := contracts.LogLevelInfo
	event := contracts.EventRefundAdapterResult
	if refund.Status == string(dto.RefundStatusFail) {
		level = contracts.LogLevelWarn
		event = contracts.EventRefundFailed
	}
	h.log(r.Context(), refund, event, level, "Refund adapter returned result")
	h.log(r.Context(), refund, contracts.EventRefundResponseSent, contracts.LogLevelInfo, "Refund response sent to merchant")

	statusCode := http.StatusOK
	if refund.Status == string(dto.RefundStatusFail) {
		statusCode = http.StatusBadGateway
	}
	writeJSON(w, statusCode, dto.RefundAPIResponse{Data: &refund, Success: refund.Status != string(dto.RefundStatusFail)})
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	merchantID := strings.TrimSpace(r.URL.Query().Get("merchant_id"))
	refundID := strings.TrimSpace(firstNonEmpty(r.URL.Query().Get("id"), r.URL.Query().Get("refund_id")))
	if merchantID == "" || refundID == "" {
		writeRefundError(w, http.StatusBadRequest, dto.ErrorBadRequest, "merchant_id and id are required")
		return
	}
	authMerchant, ok := merchantauth.MerchantFromContext(r.Context())
	if !ok {
		writeRefundError(w, http.StatusUnauthorized, dto.ErrorAuthContextMissing, "authenticated merchant context is required")
		return
	}
	if !merchantauth.CanReadMerchantData(authMerchant, merchantID) {
		merchantauth.LogAuthorizationFailed(r.Context(), h.logger, authMerchant, merchantID, "GET /refunds/status", "refund status access is not allowed for this role or merchant")
		writeRefundError(w, http.StatusForbidden, dto.ErrorForbidden, "refund status access is not allowed for this role or merchant")
		return
	}

	refund, found, err := h.refunds.GetRefundByID(r.Context(), merchantID, refundID)
	if err != nil {
		writeRefundError(w, http.StatusInternalServerError, dto.ErrorRefundStorage, err.Error())
		return
	}
	if !found {
		writeRefundError(w, http.StatusNotFound, dto.ErrorRefundNotFound, "refund not found")
		return
	}
	writeJSON(w, http.StatusOK, dto.RefundAPIResponse{Data: &refund, Success: refund.Status != string(dto.RefundStatusFail)})
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	merchantID := strings.TrimSpace(r.URL.Query().Get("merchant_id"))
	paymentID := strings.TrimSpace(r.URL.Query().Get("payment_id"))
	if merchantID == "" {
		writeJSON(w, http.StatusBadRequest, dto.RefundSearchResponse{
			Success: false,
			Error:   dto.NewGatewayError(dto.ErrorBadRequest, "merchant_id is required"),
		})
		return
	}
	authMerchant, ok := merchantauth.MerchantFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, dto.RefundSearchResponse{
			Success: false,
			Error:   dto.NewGatewayError(dto.ErrorAuthContextMissing, "authenticated merchant context is required"),
		})
		return
	}
	if !merchantauth.CanReadMerchantData(authMerchant, merchantID) {
		merchantauth.LogAuthorizationFailed(r.Context(), h.logger, authMerchant, merchantID, "GET /refunds/search", "refund search access is not allowed for this role or merchant")
		writeJSON(w, http.StatusForbidden, dto.RefundSearchResponse{
			Success: false,
			Error:   dto.NewGatewayError(dto.ErrorForbidden, "refund search access is not allowed for this role or merchant"),
		})
		return
	}

	refunds, err := h.refunds.ListRefundsByPaymentID(r.Context(), merchantID, paymentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, dto.RefundSearchResponse{
			Success: false,
			Error:   dto.NewGatewayError(dto.ErrorRefundStorage, err.Error()),
		})
		return
	}
	writeJSON(w, http.StatusOK, dto.RefundSearchResponse{Data: refunds, Success: true})
}

func decodeCreateRefundRequest(r *http.Request) (dto.CreateRefundRequest, error) {
	var req dto.CreateRefundRequest
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(contentType, "application/x-www-form-urlencoded") || strings.Contains(contentType, "multipart/form-data") {
		if err := r.ParseForm(); err != nil {
			return req, err
		}
		req.MerchantID = r.Form.Get("merchant_id")
		req.PaymentID = r.Form.Get("payment_id")
		req.IdempotencyKey = r.Form.Get("idempotency_key")
		req.Reason = r.Form.Get("reason")
		if raw := strings.TrimSpace(r.Form.Get("amount")); raw != "" {
			amount, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				return req, errors.New("amount must be a number")
			}
			currency := dto.PaymentCurrency(firstNonEmpty(r.Form.Get("currency"), "RUB"))
			req.Amount = &dto.AmountMoney{Value: amount, Currency: currency}
		}
		return req, nil
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, errors.New("invalid json")
	}
	return req, nil
}

func validateCreateRefundRequest(req dto.CreateRefundRequest, full bool) error {
	if req.MerchantID == "" {
		return errors.New("merchant_id is required")
	}
	if req.PaymentID == "" {
		return errors.New("payment_id is required")
	}
	if req.IdempotencyKey == "" {
		return errors.New("idempotency_key is required")
	}
	if !full {
		if req.Amount == nil {
			return errors.New("amount is required for partial refund")
		}
		if req.Amount.Value <= 0 {
			return errors.New("amount.value must be greater than zero")
		}
	}
	return nil
}

func (h *Handler) log(ctx context.Context, refund dto.Refund, eventType contracts.PaymentEventType, level contracts.LogLevel, message string) {
	if h.logger == nil {
		return
	}
	_ = h.logger.Log(ctx, contracts.PaymentEvent{
		Type:           eventType,
		Level:          level,
		Service:        "refunds",
		MerchantID:     refund.MerchantID,
		PaymentID:      refund.PaymentID,
		IdempotencyKey: refund.IdempotencyKey,
		CorrelationID:  refund.ID,
		CurrentStatus:  refund.Status,
		Timestamp:      time.Now().UTC(),
		Message:        message,
		Details:        message,
		Context: map[string]string{
			"refund_id":          refund.ID,
			"refund_type":        refund.RefundType,
			"provider":           refund.Provider,
			"payment_system":     refund.PaymentSystem,
			"external_refund_id": refund.ExternalRefundID,
			"provider_status":    refund.ProviderStatus,
			"amount":             fmt.Sprintf("%.2f", refund.Amount),
			"currency":           string(refund.Currency),
			"error_message":      refund.ProviderErrorMsg,
		},
	})
}

func providerKey(paymentSystem string) string {
	switch strings.ToUpper(strings.TrimSpace(paymentSystem)) {
	case "YOOKASSA":
		return "yookassa"
	case "PAYANYWAY":
		return "payanyway"
	case "SIMULATED":
		return "simulated"
	case "DIGITAL_RUBLE":
		return "digital_ruble"
	default:
		return strings.ToLower(strings.TrimSpace(paymentSystem))
	}
}

func newRefundID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("ref_%d", time.Now().UnixNano())
	}
	return "ref_" + hex.EncodeToString(b)
}

func writeRefundError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, dto.RefundAPIResponse{
		Success: false,
		Error:   dto.NewGatewayError(code, message),
	})
}

func writeJSON(w http.ResponseWriter, status int, response any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
