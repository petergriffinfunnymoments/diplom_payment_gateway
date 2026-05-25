package webhooks

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

type PayAnyWayWebhookHandler struct {
	store         contracts.TransactionStore
	logger        contracts.EventLogger
	notifications contracts.Notifications
	mntID         string
	integrityKey  string
	testMode      bool
}

func NewPayAnyWayWebhookHandler(store contracts.TransactionStore, logger contracts.EventLogger, notifications contracts.Notifications) http.Handler {
	return &PayAnyWayWebhookHandler{
		store:         store,
		logger:        logger,
		notifications: notifications,
		mntID: strings.TrimSpace(firstNonEmpty(
			os.Getenv("PAYANYWAY_MNT_ID"),
			os.Getenv("PAYANYWAY_BUSINESS_ACCOUNT"),
			os.Getenv("PAYANYWAY_ACCOUNT_ID"),
		)),
		integrityKey: strings.TrimSpace(os.Getenv("PAYANYWAY_INTEGRITY_CODE")),
		testMode:     payAnyWayWebhookBoolEnv("PAYANYWAY_TEST_MODE", true),
	}
}

func (h *PayAnyWayWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	h.logWebhookAttempt(r.Context(), r.Form, "received", "")
	if h.integrityKey == "" {
		h.logWebhookAttempt(r.Context(), r.Form, "rejected", "PayAnyWay integrity code is not configured")
		http.Error(w, "PayAnyWay integrity code is not configured", http.StatusInternalServerError)
		return
	}

	mntID := payAnyWayFormValue(r.Form, "MNT_ID")
	transactionID := payAnyWayFormValue(r.Form, "MNT_TRANSACTION_ID")
	operationID := payAnyWayFormValue(r.Form, "MNT_OPERATION_ID")
	amount := payAnyWayFormValue(r.Form, "MNT_AMOUNT")
	currency := payAnyWayFormValue(r.Form, "MNT_CURRENCY_CODE")
	subscriberID := payAnyWayFormValue(r.Form, "MNT_SUBSCRIBER_ID")
	testMode := payAnyWayFormValue(r.Form, "MNT_TEST_MODE")
	signature := payAnyWayFormValue(r.Form, "MNT_SIGNATURE")

	if mntID == "" || transactionID == "" || operationID == "" || amount == "" || currency == "" || signature == "" {
		h.logWebhookAttempt(r.Context(), r.Form, "rejected", "missing required PayAnyWay fields")
		http.Error(w, "missing required PayAnyWay fields", http.StatusBadRequest)
		return
	}
	if h.mntID != "" && !strings.EqualFold(mntID, h.mntID) {
		h.logWebhookAttempt(r.Context(), r.Form, "rejected", "unexpected PayAnyWay MNT_ID")
		http.Error(w, "unexpected PayAnyWay MNT_ID", http.StatusBadRequest)
		return
	}

	expected := payAnyWayWebhookSignature(mntID, transactionID, operationID, amount, currency, subscriberID, testMode, h.integrityKey)
	if !strings.EqualFold(signature, expected) {
		h.logWebhookAttempt(r.Context(), r.Form, "rejected", "invalid PayAnyWay signature")
		http.Error(w, "invalid PayAnyWay signature", http.StatusBadRequest)
		return
	}

	merchantID := firstNonEmpty(
		payAnyWayFormValue(r.Form, "merchant_id"),
		subscriberID,
	)
	paymentID := firstNonEmpty(
		payAnyWayFormValue(r.Form, "payment_id"),
		transactionID,
	)
	if merchantID == "" || paymentID == "" {
		logCtx, cancel := ctxWithTimeout(r.Context())
		defer cancel()
		_ = h.log(logCtx, contracts.PaymentEvent{
			Type:          contracts.EventPaymentFailed,
			Level:         contracts.LogLevelWarn,
			Service:       "adapter",
			CurrentStatus: string(dto.StatusCaptured),
			Timestamp:     time.Now().UTC(),
			Message:       "PayAnyWay Pay URL ignored: missing merchant or payment metadata",
			Details:       "PayAnyWay Pay URL ignored: missing merchant or payment metadata",
			Context: map[string]string{
				"provider":                "payanyway",
				"external_transaction_id": transactionID,
				"external_operation_id":   operationID,
				"provider_status":         "paid",
				"test":                    strconv.FormatBool(h.testMode),
			},
		})
		writePayAnyWayReceiptResponse(w, dto.PaymentResponse{}, mntID, transactionID, amount, h.integrityKey)
		return
	}

	status := string(dto.StatusCaptured)
	resp := h.loadOrBuildResponse(r.Context(), merchantID, paymentID, amount, currency, transactionID, operationID)
	resp.ID = paymentID
	resp.MerchantID = merchantID
	resp.CurrentStatus = status
	resp.PaymentInfo.UpdatedAt = time.Now().UTC()
	resp.TransactionDetails.ExternalTransactionID = operationID
	resp.TransactionDetails.PaymentSystem = "PAYANYWAY"
	resp.TransactionDetails.ProviderStatus = "paid"
	resp.TransactionDetails.PaymentURL = ""
	resp.Error = nil
	resp = resp.Sanitized()

	if resp.IdempotencyKey == "" {
		resp.IdempotencyKey = firstNonEmpty(payAnyWayFormValue(r.Form, "idempotency_key"), transactionID)
	}

	payload, _ := json.Marshal(resp)
	if err := h.store.Save(r.Context(), merchantID, paymentID, resp.IdempotencyKey, status, string(payload), time.Now().UTC()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_ = h.log(r.Context(), contracts.PaymentEvent{
		Type:           contracts.EventAdapterResultReceived,
		Level:          contracts.LogLevelInfo,
		Service:        "adapter",
		MerchantID:     merchantID,
		PaymentID:      paymentID,
		IdempotencyKey: resp.IdempotencyKey,
		CorrelationID:  paymentID,
		CurrentStatus:  status,
		Timestamp:      time.Now().UTC(),
		Message:        "PayAnyWay Pay URL processed",
		Details:        "PayAnyWay Pay URL processed",
		Context: map[string]string{
			"provider":                "payanyway",
			"external_transaction_id": transactionID,
			"external_operation_id":   operationID,
			"provider_status":         "paid",
			"amount":                  amount,
			"currency":                currency,
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
				IdempotencyKey: resp.IdempotencyKey,
				CorrelationID:  paymentID,
				CurrentStatus:  status,
				Timestamp:      time.Now().UTC(),
				Message:        "Merchant notification after PayAnyWay Pay URL failed",
				Details:        "Merchant notification after PayAnyWay Pay URL failed",
				Context: map[string]string{
					"provider":      "payanyway",
					"error_message": err.Error(),
				},
			})
		}
	}

	writePayAnyWayReceiptResponse(w, resp, mntID, transactionID, amount, h.integrityKey)
}

func (h *PayAnyWayWebhookHandler) loadOrBuildResponse(ctx context.Context, merchantID, paymentID, amount, currency, transactionID, operationID string) dto.PaymentResponse {
	_, payloadJSON, found, err := h.store.GetByPaymentID(ctx, merchantID, paymentID)
	if err == nil && found && payloadJSON != "" {
		var resp dto.PaymentResponse
		if json.Unmarshal([]byte(payloadJSON), &resp) == nil {
			return resp
		}
	}

	parsedAmount, _ := strconv.ParseFloat(strings.ReplaceAll(amount, ",", "."), 64)
	return dto.PaymentResponse{
		ID:             paymentID,
		MerchantID:     merchantID,
		IdempotencyKey: transactionID,
		CurrentStatus:  string(dto.StatusCaptured),
		PaymentInfo: dto.PaymentInfoResponse{
			Amount: dto.AmountMoney{
				Value:    parsedAmount,
				Currency: dto.PaymentCurrency(firstNonEmpty(currency, "RUB")),
			},
			PaymentMethodData: dto.PaymentMethodData{Type: dto.PaymentMethodSBP},
			Description:       "PayAnyWay payment",
			CreatedAt:         time.Now().UTC(),
			UpdatedAt:         time.Now().UTC(),
		},
		TransactionDetails: dto.TransactionDetails{
			ExternalTransactionID: operationID,
			PaymentSystem:         "PAYANYWAY",
			ProviderStatus:        "paid",
			FraudCheckResult:      "PASSED",
		},
	}
}

func (h *PayAnyWayWebhookHandler) log(ctx context.Context, event contracts.PaymentEvent) error {
	if h.logger == nil {
		return nil
	}
	return h.logger.Log(ctx, event)
}

func (h *PayAnyWayWebhookHandler) logWebhookAttempt(ctx context.Context, form url.Values, providerStatus, reason string) {
	if h.logger == nil {
		return
	}
	paymentID := firstNonEmpty(payAnyWayFormValue(form, "payment_id"), payAnyWayFormValue(form, "MNT_TRANSACTION_ID"))
	merchantID := firstNonEmpty(payAnyWayFormValue(form, "merchant_id"), payAnyWayFormValue(form, "MNT_SUBSCRIBER_ID"))
	level := contracts.LogLevelInfo
	eventType := contracts.EventMerchantWebhookReceived
	message := "PayAnyWay webhook received"
	if reason != "" {
		level = contracts.LogLevelWarn
		eventType = contracts.EventPaymentFailed
		message = "PayAnyWay webhook rejected: " + reason
	}
	_ = h.log(ctx, contracts.PaymentEvent{
		Type:           eventType,
		Level:          level,
		Service:        "adapter",
		MerchantID:     merchantID,
		PaymentID:      paymentID,
		IdempotencyKey: payAnyWayFormValue(form, "idempotency_key"),
		CorrelationID:  paymentID,
		CurrentStatus:  string(dto.StatusPending),
		Timestamp:      time.Now().UTC(),
		Message:        message,
		Details:        message,
		Context: map[string]string{
			"provider":                "payanyway",
			"provider_status":         providerStatus,
			"reject_reason":           reason,
			"mnt_id":                  payAnyWayFormValue(form, "MNT_ID"),
			"mnt_transaction_id":      payAnyWayFormValue(form, "MNT_TRANSACTION_ID"),
			"external_operation_id":   payAnyWayFormValue(form, "MNT_OPERATION_ID"),
			"amount":                  payAnyWayFormValue(form, "MNT_AMOUNT"),
			"currency":                payAnyWayFormValue(form, "MNT_CURRENCY_CODE"),
			"mnt_test_mode":           payAnyWayFormValue(form, "MNT_TEST_MODE"),
			"payment_system_unit_id":  payAnyWayFormValue(form, "paymentSystem.unitId"),
			"has_signature":           strconv.FormatBool(payAnyWayFormValue(form, "MNT_SIGNATURE") != ""),
			"configured_test_mode":    strconv.FormatBool(h.testMode),
			"configured_mnt_id_match": strconv.FormatBool(h.mntID == "" || strings.EqualFold(payAnyWayFormValue(form, "MNT_ID"), h.mntID)),
		},
	})
}

type payAnyWayMNTResponse struct {
	XMLName       xml.Name                `xml:"MNT_RESPONSE"`
	MNTID         string                  `xml:"MNT_ID"`
	TransactionID string                  `xml:"MNT_TRANSACTION_ID"`
	ResultCode    string                  `xml:"MNT_RESULT_CODE"`
	Description   string                  `xml:"MNT_DESCRIPTION,omitempty"`
	Amount        string                  `xml:"MNT_AMOUNT,omitempty"`
	Signature     string                  `xml:"MNT_SIGNATURE"`
	Attributes    []payAnyWayMNTAttribute `xml:"MNT_ATTRIBUTES>ATTRIBUTE,omitempty"`
}

type payAnyWayMNTAttribute struct {
	Key   string `xml:"KEY"`
	Value string `xml:"VALUE"`
}

type payAnyWayInventoryItem struct {
	Name          string `json:"name"`
	Price         string `json:"price"`
	Quantity      string `json:"quantity"`
	VATTag        string `json:"vatTag"`
	PaymentMethod string `json:"pm"`
	PaymentObject string `json:"po"`
	IDInternal    string `json:"idInternal,omitempty"`
}

type payAnyWayClient struct {
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
}

func writePayAnyWayReceiptResponse(w http.ResponseWriter, resp dto.PaymentResponse, mntID, transactionID, amount, integrityKey string) {
	resultCode := "200"
	attributes := []payAnyWayMNTAttribute{
		{
			Key:   "INVENTORY",
			Value: mustJSON(payAnyWayInventory(resp, transactionID, amount)),
		},
	}

	client := payAnyWayClient{
		Email: strings.TrimSpace(resp.PaymentInfo.CustomerData.Email),
		Phone: normalizePayAnyWayPhone(resp.PaymentInfo.CustomerData.Phone),
	}
	if client.Email != "" || client.Phone != "" {
		attributes = append(attributes, payAnyWayMNTAttribute{
			Key:   "CLIENT",
			Value: mustJSON([]payAnyWayClient{client}),
		})
	}
	if client.Email != "" {
		attributes = append(attributes, payAnyWayMNTAttribute{Key: "CUSTOMER", Value: client.Email})
	}
	if client.Phone != "" {
		attributes = append(attributes, payAnyWayMNTAttribute{Key: "PHONE", Value: client.Phone})
	}
	if sno := strings.TrimSpace(os.Getenv("PAYANYWAY_SNO")); sno != "" {
		attributes = append(attributes, payAnyWayMNTAttribute{Key: "SNO", Value: sno})
	}

	response := payAnyWayMNTResponse{
		MNTID:         strings.TrimSpace(mntID),
		TransactionID: strings.TrimSpace(transactionID),
		ResultCode:    resultCode,
		Description:   "Order paid",
		Amount:        strings.TrimSpace(amount),
		Signature:     payAnyWayResponseSignature(resultCode, mntID, transactionID, integrityKey),
		Attributes:    attributes,
	}

	body, err := xml.MarshalIndent(response, "", "  ")
	if err != nil {
		http.Error(w, "failed to build PayAnyWay response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(body)
}

func payAnyWayWebhookSignature(mntID, transactionID, operationID, amount, currency, subscriberID, testMode, integrityKey string) string {
	raw := strings.TrimSpace(mntID) +
		strings.TrimSpace(transactionID) +
		strings.TrimSpace(operationID) +
		strings.TrimSpace(amount) +
		strings.TrimSpace(currency) +
		strings.TrimSpace(subscriberID) +
		strings.TrimSpace(testMode) +
		integrityKey
	sum := md5.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func payAnyWayResponseSignature(resultCode, mntID, transactionID, integrityKey string) string {
	raw := strings.TrimSpace(resultCode) +
		strings.TrimSpace(mntID) +
		strings.TrimSpace(transactionID) +
		integrityKey
	sum := md5.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func payAnyWayInventory(resp dto.PaymentResponse, transactionID, amount string) []payAnyWayInventoryItem {
	items := resp.PaymentInfo.Items
	if len(items) == 0 {
		items = []dto.PaymentItem{
			{
				Name:          firstNonEmpty(resp.PaymentInfo.Description, "PayAnyWay payment "+transactionID),
				Price:         firstPositive(resp.PaymentInfo.Amount.Value, parsePayAnyWayAmount(amount)),
				Quantity:      1,
				VATTag:        "1105",
				PaymentMethod: "full_payment",
				PaymentObject: "service",
				IDInternal:    transactionID,
			},
		}
	}

	inventory := make([]payAnyWayInventoryItem, 0, len(items))
	for i, item := range items {
		idInternal := strings.TrimSpace(item.IDInternal)
		if idInternal == "" {
			idInternal = transactionID
			if len(items) > 1 {
				idInternal += "_" + strconv.Itoa(i+1)
			}
		}
		inventory = append(inventory, payAnyWayInventoryItem{
			Name:          payAnyWayTruncate(firstNonEmpty(item.Name, resp.PaymentInfo.Description, "PayAnyWay payment"), 128),
			Price:         formatPayAnyWayFloat(firstPositive(item.Price, resp.PaymentInfo.Amount.Value, parsePayAnyWayAmount(amount))),
			Quantity:      formatPayAnyWayFloat(firstPositive(item.Quantity, 1)),
			VATTag:        firstNonEmpty(item.VATTag, "1105"),
			PaymentMethod: firstNonEmpty(item.PaymentMethod, "full_payment"),
			PaymentObject: firstNonEmpty(item.PaymentObject, "service"),
			IDInternal:    payAnyWayTruncate(idInternal, 64),
		})
	}
	return inventory
}

func parsePayAnyWayAmount(amount string) float64 {
	value, _ := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(amount), ",", "."), 64)
	return value
}

func firstPositive(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 1
}

func formatPayAnyWayFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func normalizePayAnyWayPhone(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func mustJSON(value any) string {
	b, err := json.Marshal(value)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func payAnyWayTruncate(value string, max int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if max <= 0 || len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

func payAnyWayFormValue(form url.Values, key string) string {
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

func payAnyWayWebhookBoolEnv(name string, defaultValue bool) bool {
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
