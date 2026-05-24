package adapter

import (
	"context"
	"net/url"
	"testing"
	"time"

	"payment-gateway/internal/dto"
)

func TestRobokassaAdapterBuildsTestPaymentURL(t *testing.T) {
	t.Setenv("ROBOKASSA_MERCHANT_LOGIN", "TestMerchant")
	t.Setenv("ROBOKASSA_TEST_PASSWORD1", "pass1")
	t.Setenv("ROBOKASSA_TEST_MODE", "true")
	t.Setenv("ROBOKASSA_PAYMENT_URL", "https://auth.robokassa.ru/Merchant/Index.aspx")
	t.Setenv("ROBOKASSA_HASH_ALGORITHM", "md5")

	adapter, err := NewRobokassaAdapterFromEnv()
	if err != nil {
		t.Fatalf("NewRobokassaAdapterFromEnv() error = %v", err)
	}

	req := dto.CreatePaymentRequest{
		MerchantID:     "merchant_12345",
		IdempotencyKey: "idem_1",
		PaymentID:      "pay_robokassa_1",
		CurrentStatus:  string(dto.StatusCreated),
		PaymentInfo: dto.PaymentInfo{
			Amount: dto.AmountMoney{
				Value:    1500,
				Currency: dto.PaymentCurrency("RUB"),
			},
			PaymentMethodData: dto.PaymentMethodData{Type: dto.PaymentMethodSBP},
			CustomerData:      dto.CustomerData{Phone: "+79991234567"},
			Description:       "Robokassa test payment",
			CreatedAt:         time.Now().UTC(),
		},
	}

	result, err := adapter.Send(context.Background(), "tok_preview", req)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if result.PaymentSystem != "ROBOKASSA" {
		t.Fatalf("PaymentSystem = %q", result.PaymentSystem)
	}
	if result.Status != string(dto.StatusPending) {
		t.Fatalf("Status = %q", result.Status)
	}
	if result.PaymentURL == "" {
		t.Fatal("PaymentURL is empty")
	}

	parsed, err := url.Parse(result.PaymentURL)
	if err != nil {
		t.Fatalf("payment url parse error = %v", err)
	}
	values := parsed.Query()
	if values.Get("IsTest") != "1" {
		t.Fatalf("IsTest = %q", values.Get("IsTest"))
	}
	if values.Get("MerchantLogin") != "TestMerchant" {
		t.Fatalf("MerchantLogin = %q", values.Get("MerchantLogin"))
	}
	if values.Get("OutSum") != "1500.00" {
		t.Fatalf("OutSum = %q", values.Get("OutSum"))
	}

	shp := map[string]string{
		"Shp_idempotency_key": "idem_1",
		"Shp_merchant_id":     "merchant_12345",
		"Shp_payment_id":      "pay_robokassa_1",
	}
	expectedSignature, err := robokassaSignature("md5", "TestMerchant", "1500.00", values.Get("InvId"), "pass1", nil, shp)
	if err != nil {
		t.Fatalf("signature error = %v", err)
	}
	if values.Get("SignatureValue") != expectedSignature {
		t.Fatalf("SignatureValue = %q, want %q", values.Get("SignatureValue"), expectedSignature)
	}
}
