package webhooks

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"payment-gateway/internal/subsystems/digitalruble"
)

func TestDigitalRubleSOAPHandlerProcessesPlatformMessage(t *testing.T) {
	check := digitalruble.PaymentCheck{
		MessageID:       "msg_test",
		MerchantID:      "merchant_12345",
		PaymentID:       "pay_test",
		WalletID:        "dr_wallet_123",
		Amount:          1500,
		Currency:        "RUB",
		Category:        "education",
		SmartContractID: digitalruble.DefaultSmartContractID,
	}
	body, err := digitalruble.BuildPaymentCheckSOAP(check)
	if err != nil {
		t.Fatal(err)
	}

	handler := NewDigitalRubleSOAPHandler()
	req := httptest.NewRequest(http.MethodPost, "/sandbox/digital-ruble/soap", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<Result>PASSED</Result>") {
		t.Fatalf("expected PASSED SOAP response, got %s", rec.Body.String())
	}
}
