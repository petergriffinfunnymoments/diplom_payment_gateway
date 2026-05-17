package adapter

import (
	"context"
	"strings"
	"testing"
	"time"

	"payment-gateway/internal/dto"
)

func TestDigitalRubleAdapterReturnsPendingQRCode(t *testing.T) {
	a := NewDigitalRubleAdapterFromEnv()

	result, err := a.Send(context.Background(), "tok_test", digitalRubleRequest("dr_wallet_123"))
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	if result.PaymentSystem != "DIGITAL_RUBLE" {
		t.Fatalf("unexpected payment system: %s", result.PaymentSystem)
	}
	if result.Status != string(dto.StatusPending) {
		t.Fatalf("expected pending, got %s", result.Status)
	}
	if result.ProviderStatus != "qr_issued" {
		t.Fatalf("expected qr_issued, got %s", result.ProviderStatus)
	}
	if result.QRID == "" || result.QRPayload == "" {
		t.Fatalf("expected qr fields to be set: %+v", result)
	}
	if !strings.HasPrefix(result.QRImageDataURI, "data:image/png;base64,") {
		t.Fatalf("expected qr image data URI, got %q", result.QRImageDataURI)
	}
	if !strings.Contains(result.QRPayload, "rail=DIGITAL_RUBLE") {
		t.Fatalf("unexpected qr payload: %s", result.QRPayload)
	}
}

func digitalRubleRequest(walletID string) dto.CreatePaymentRequest {
	return dto.CreatePaymentRequest{
		MerchantID:     "merchant_12345",
		IdempotencyKey: "idem_dr_test",
		PaymentID:      "pay_dr_test",
		CurrentStatus:  string(dto.StatusCreated),
		PaymentInfo: dto.PaymentInfo{
			Amount: dto.AmountMoney{
				Value:    1500,
				Currency: "RUB",
			},
			PaymentMethodData: dto.PaymentMethodData{Type: dto.PaymentMethodDigitalRuble},
			CustomerData: dto.CustomerData{
				Email:                  "customer@example.com",
				Phone:                  "+79991234567",
				DigitalRubleWalletID:   walletID,
				DigitalRubleAccount:    "0000000000000000000000000000000000",
				DigitalRubleIdentifier: "merchant:wallet:demo",
			},
			CreatedAt:   time.Now().UTC(),
			Description: "digital ruble test payment",
		},
	}
}
