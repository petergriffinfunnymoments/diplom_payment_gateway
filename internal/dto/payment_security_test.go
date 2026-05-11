package dto

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPaymentResponseSanitizedRemovesCVVAndFullToken(t *testing.T) {
	resp := PaymentResponse{
		ID:             "pay_1",
		MerchantID:     "merchant_12345",
		IdempotencyKey: "idem_1",
		CurrentStatus:  string(StatusCaptured),
		PaymentInfo: PaymentInfoResponse{
			Amount: AmountMoney{Value: 1500, Currency: "RUB"},
			PaymentMethodData: PaymentMethodData{
				Type: PaymentMethodCard,
			},
			CustomerData: CustomerData{
				Email:      "customer@example.com",
				Phone:      "+79991234567",
				CardNumber: "4111111111111111",
				CardDate:   "12/29",
				CvvCode:    "123",
			},
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		},
		TransactionDetails: TransactionDetails{
			Token:            "tok_1234567890abcdef",
			FraudCheckResult: "PASSED",
		},
	}

	sanitized := resp.Sanitized()
	if sanitized.PaymentInfo.CustomerData.CardNumber != "411111******1111" {
		t.Fatalf("unexpected masked card: %q", sanitized.PaymentInfo.CustomerData.CardNumber)
	}
	if sanitized.PaymentInfo.CustomerData.CvvCode != "" {
		t.Fatalf("expected CVV to be removed")
	}
	if sanitized.TransactionDetails.Token != "" {
		t.Fatalf("expected full token to be removed")
	}
	if sanitized.TransactionDetails.TokenPreview == "" {
		t.Fatalf("expected token preview to be present")
	}

	b, err := json.Marshal(sanitized)
	if err != nil {
		t.Fatal(err)
	}
	payload := string(b)
	for _, forbidden := range []string{"4111111111111111", "CVV_code", "tok_1234567890abcdef"} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("sanitized payload contains forbidden value %q: %s", forbidden, payload)
		}
	}
}

func TestSanitizePaymentPayloadJSON(t *testing.T) {
	raw := `{"id":"pay_1","merchant_id":"merchant_12345","idempotency_key":"idem_1","current_status":"CAPTURED","payment_info":{"customer_data":{"card_number":"4111111111111111","CVV_code":"123"}},"transaction_details":{"token":"tok_1234567890abcdef","fraud_check_result":"PASSED"}}`

	sanitized := SanitizePaymentPayloadJSON(raw)
	for _, forbidden := range []string{"4111111111111111", "CVV_code", "tok_1234567890abcdef"} {
		if strings.Contains(sanitized, forbidden) {
			t.Fatalf("sanitized json contains forbidden value %q: %s", forbidden, sanitized)
		}
	}
	if !strings.Contains(sanitized, "411111******1111") {
		t.Fatalf("sanitized json does not contain card mask: %s", sanitized)
	}
}

func TestSanitizePaymentPayloadJSONRemovesCVVFromGenericPayload(t *testing.T) {
	raw := `{"payment_info":{"customer_data":{"card_number":"4111111111111111","CVV_code":"123","nested":{"cvc":"999"}}}}`

	sanitized := SanitizePaymentPayloadJSON(raw)
	for _, forbidden := range []string{"CVV_code", `"cvc"`, "123", "999"} {
		if strings.Contains(sanitized, forbidden) {
			t.Fatalf("sanitized json contains forbidden value %q: %s", forbidden, sanitized)
		}
	}
	if !strings.Contains(sanitized, "4111111111111111") {
		t.Fatalf("generic sanitizer should not alter unrelated fields: %s", sanitized)
	}
}

func TestCreatePaymentRequestWithoutSensitiveAuthenticationDataRemovesCVV(t *testing.T) {
	req := CreatePaymentRequest{
		PaymentInfo: PaymentInfo{
			CustomerData: CustomerData{
				CardNumber: "4111111111111111",
				CvvCode:    "123",
			},
		},
	}

	safe := req.WithoutSensitiveAuthenticationData()
	if safe.PaymentInfo.CustomerData.CvvCode != "" {
		t.Fatalf("expected CVV to be removed")
	}
	if safe.PaymentInfo.CustomerData.CardNumber != req.PaymentInfo.CustomerData.CardNumber {
		t.Fatalf("expected PAN to remain available for routing/tokenization")
	}
	if req.PaymentInfo.CustomerData.CvvCode == "" {
		t.Fatalf("expected original request value copy to remain unchanged")
	}
}
