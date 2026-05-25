package webhooks

import (
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"payment-gateway/internal/dto"
	"payment-gateway/internal/subsystems/storage"
)

func TestPayAnyWayWebhookCapturesPayment(t *testing.T) {
	t.Setenv("PAYANYWAY_MNT_ID", "12345678")
	t.Setenv("PAYANYWAY_INTEGRITY_CODE", "integrity")
	t.Setenv("PAYANYWAY_TEST_MODE", "true")

	store := storage.NewInMemoryTransactionStore()
	payment := dto.PaymentResponse{
		ID:             "pay_payanyway_1",
		MerchantID:     "merchant_12345",
		IdempotencyKey: "idem_1",
		CurrentStatus:  string(dto.StatusPending),
		PaymentInfo: dto.PaymentInfoResponse{
			Amount: dto.AmountMoney{
				Value:    1500,
				Currency: dto.PaymentCurrency("RUB"),
			},
			PaymentMethodData: dto.PaymentMethodData{Type: dto.PaymentMethodSBP},
			CustomerData:      dto.CustomerData{Phone: "+79991234567"},
			Items: []dto.PaymentItem{
				{
					Name:          "Тестовая услуга PayAnyWay",
					Price:         1500,
					Quantity:      1,
					VATTag:        "1105",
					PaymentMethod: "full_payment",
					PaymentObject: "service",
					IDInternal:    "item_1",
				},
			},
			Description: "PayAnyWay test payment",
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		},
		TransactionDetails: dto.TransactionDetails{
			ExternalTransactionID: "pay_payanyway_1",
			PaymentSystem:         "PAYANYWAY",
			ProviderStatus:        "test_payment_url_created",
			PaymentURL:            "https://www.payanyway.ru/assistant.htm?...",
			FraudCheckResult:      "PASSED",
		},
	}
	payload, err := json.Marshal(payment)
	if err != nil {
		t.Fatalf("marshal payment: %v", err)
	}
	if err := store.Save(nil, payment.MerchantID, payment.ID, payment.IdempotencyKey, payment.CurrentStatus, string(payload), time.Now().UTC()); err != nil {
		t.Fatalf("save payment: %v", err)
	}

	form := url.Values{}
	form.Set("MNT_ID", "12345678")
	form.Set("MNT_TRANSACTION_ID", "pay_payanyway_1")
	form.Set("MNT_OPERATION_ID", "op_123")
	form.Set("MNT_AMOUNT", "1500.00")
	form.Set("MNT_CURRENCY_CODE", "RUB")
	form.Set("MNT_SUBSCRIBER_ID", "merchant_12345")
	form.Set("MNT_TEST_MODE", "1")
	signature := payAnyWayWebhookSignature(
		"12345678",
		"pay_payanyway_1",
		"op_123",
		"1500.00",
		"RUB",
		"merchant_12345",
		"1",
		"integrity",
	)
	form.Set("MNT_SIGNATURE", signature)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/payanyway", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	NewPayAnyWayWebhookHandler(store, nil, nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.Contains(contentType, "application/xml") {
		t.Fatalf("content type = %q", contentType)
	}
	var providerResponse payAnyWayMNTResponse
	if err := xml.Unmarshal(rec.Body.Bytes(), &providerResponse); err != nil {
		t.Fatalf("PayAnyWay response XML unmarshal error = %v; body = %s", err, rec.Body.String())
	}
	if providerResponse.ResultCode != "200" {
		t.Fatalf("result code = %q", providerResponse.ResultCode)
	}
	if providerResponse.Signature == "" {
		t.Fatal("response signature is empty")
	}
	if !strings.Contains(rec.Body.String(), "INVENTORY") {
		t.Fatalf("response does not contain inventory: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Тестовая услуга PayAnyWay") {
		t.Fatalf("response does not contain item name: %s", rec.Body.String())
	}

	status, storedPayload, found, err := store.GetByPaymentID(nil, "merchant_12345", "pay_payanyway_1")
	if err != nil {
		t.Fatalf("GetByPaymentID error = %v", err)
	}
	if !found {
		t.Fatal("payment not found")
	}
	if status != string(dto.StatusCaptured) {
		t.Fatalf("status = %q", status)
	}
	var stored dto.PaymentResponse
	if err := json.Unmarshal([]byte(storedPayload), &stored); err != nil {
		t.Fatalf("stored payload unmarshal error = %v", err)
	}
	if stored.TransactionDetails.PaymentSystem != "PAYANYWAY" {
		t.Fatalf("payment system = %q", stored.TransactionDetails.PaymentSystem)
	}
	if stored.TransactionDetails.ExternalTransactionID != "op_123" {
		t.Fatalf("external transaction id = %q", stored.TransactionDetails.ExternalTransactionID)
	}
	if stored.TransactionDetails.PaymentURL != "" {
		t.Fatalf("payment URL should be cleared after capture, got %q", stored.TransactionDetails.PaymentURL)
	}
}
