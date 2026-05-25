package adapter

import (
	"context"
	"net/url"
	"testing"
	"time"

	"payment-gateway/internal/dto"
)

func TestPayAnyWayAdapterBuildsTestPaymentURL(t *testing.T) {
	t.Setenv("PAYANYWAY_MNT_ID", "12345678")
	t.Setenv("PAYANYWAY_INTEGRITY_CODE", "integrity")
	t.Setenv("PAYANYWAY_TEST_MODE", "true")
	t.Setenv("PAYANYWAY_PAYMENT_URL", "https://www.payanyway.ru/assistant.htm")

	adapter, err := NewPayAnyWayAdapterFromEnv()
	if err != nil {
		t.Fatalf("NewPayAnyWayAdapterFromEnv() error = %v", err)
	}

	req := dto.CreatePaymentRequest{
		MerchantID:     "merchant_12345",
		IdempotencyKey: "idem_1",
		PaymentID:      "pay_payanyway_1",
		CurrentStatus:  string(dto.StatusCreated),
		PaymentInfo: dto.PaymentInfo{
			Amount: dto.AmountMoney{
				Value:    1500,
				Currency: dto.PaymentCurrency("RUB"),
			},
			PaymentMethodData: dto.PaymentMethodData{Type: dto.PaymentMethodSBP},
			CustomerData:      dto.CustomerData{Phone: "+79991234567"},
			Description:       "PayAnyWay test payment",
			CreatedAt:         time.Now().UTC(),
		},
	}

	result, err := adapter.Send(context.Background(), "tok_preview", req)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if result.PaymentSystem != "PAYANYWAY" {
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
	if values.Get("MNT_ID") != "12345678" {
		t.Fatalf("MNT_ID = %q", values.Get("MNT_ID"))
	}
	if values.Get("MNT_TRANSACTION_ID") != "pay_payanyway_1" {
		t.Fatalf("MNT_TRANSACTION_ID = %q", values.Get("MNT_TRANSACTION_ID"))
	}
	if values.Get("MNT_AMOUNT") != "1500.00" {
		t.Fatalf("MNT_AMOUNT = %q", values.Get("MNT_AMOUNT"))
	}
	if values.Get("MNT_TEST_MODE") != "1" {
		t.Fatalf("MNT_TEST_MODE = %q", values.Get("MNT_TEST_MODE"))
	}
	if values.Get("MNT_SUBSCRIBER_ID") != "merchant_12345" {
		t.Fatalf("MNT_SUBSCRIBER_ID = %q", values.Get("MNT_SUBSCRIBER_ID"))
	}
	if values.Get("paymentSystem.unitId") != "sbpc2b" {
		t.Fatalf("paymentSystem.unitId = %q", values.Get("paymentSystem.unitId"))
	}

	expectedSignature := payAnyWayPaymentSignature(
		"12345678",
		"pay_payanyway_1",
		"1500.00",
		"RUB",
		"merchant_12345",
		"1",
		"integrity",
	)
	if values.Get("MNT_SIGNATURE") != expectedSignature {
		t.Fatalf("MNT_SIGNATURE = %q, want %q", values.Get("MNT_SIGNATURE"), expectedSignature)
	}
}
