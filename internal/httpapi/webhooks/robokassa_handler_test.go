package webhooks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"payment-gateway/internal/dto"
	"payment-gateway/internal/subsystems/storage"
)

func TestRobokassaWebhookCapturesPayment(t *testing.T) {
	t.Setenv("ROBOKASSA_TEST_PASSWORD2", "pass2")
	t.Setenv("ROBOKASSA_TEST_MODE", "true")
	t.Setenv("ROBOKASSA_HASH_ALGORITHM", "md5")

	store := storage.NewInMemoryTransactionStore()
	payment := dto.PaymentResponse{
		ID:             "pay_robokassa_1",
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
			Description:       "Robokassa test payment",
			CreatedAt:         time.Now().UTC(),
			UpdatedAt:         time.Now().UTC(),
		},
		TransactionDetails: dto.TransactionDetails{
			ExternalTransactionID: "123456",
			PaymentSystem:         "ROBOKASSA",
			ProviderStatus:        "test_payment_url_created",
			PaymentURL:            "https://auth.robokassa.ru/Merchant/Index.aspx?...",
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
	form.Set("OutSum", "1500.00")
	form.Set("InvId", "123456")
	form.Set("Shp_idempotency_key", "idem_1")
	form.Set("Shp_merchant_id", "merchant_12345")
	form.Set("Shp_payment_id", "pay_robokassa_1")

	shp := map[string]string{
		"Shp_idempotency_key": "idem_1",
		"Shp_merchant_id":     "merchant_12345",
		"Shp_payment_id":      "pay_robokassa_1",
	}
	signature, err := robokassaWebhookSignature("md5", "1500.00", "123456", "pass2", shp)
	if err != nil {
		t.Fatalf("signature error: %v", err)
	}
	form.Set("SignatureValue", signature)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/robokassa", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	NewRobokassaWebhookHandler(store, nil, nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.TrimSpace(rec.Body.String()) != "OK123456" {
		t.Fatalf("body = %q", rec.Body.String())
	}

	status, storedPayload, found, err := store.GetByPaymentID(nil, "merchant_12345", "pay_robokassa_1")
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
	if stored.TransactionDetails.PaymentSystem != "ROBOKASSA" {
		t.Fatalf("payment system = %q", stored.TransactionDetails.PaymentSystem)
	}
	if stored.TransactionDetails.PaymentURL != "" {
		t.Fatalf("payment URL should be cleared after capture, got %q", stored.TransactionDetails.PaymentURL)
	}
}
