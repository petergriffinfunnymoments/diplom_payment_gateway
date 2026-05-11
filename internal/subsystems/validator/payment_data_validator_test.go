package validator

import (
	"context"
	"testing"
	"time"

	"payment-gateway/internal/dto"
)

func TestValidateRemovesCVVAfterSuccessfulCardValidation(t *testing.T) {
	v := NewPaymentDataValidator()
	req := validCardRequest()

	validated, err := v.Validate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	if validated.PaymentInfo.CustomerData.CvvCode != "" {
		t.Fatalf("expected CVV to be cleared after validation")
	}
	if validated.PaymentInfo.CustomerData.CardNumber == "" {
		t.Fatalf("expected card number to remain available after validation")
	}
}

func TestValidateStillRequiresCVVForCardPayments(t *testing.T) {
	v := NewPaymentDataValidator()
	req := validCardRequest()
	req.PaymentInfo.CustomerData.CvvCode = ""

	if _, err := v.Validate(context.Background(), req); err == nil {
		t.Fatalf("expected missing CVV validation error")
	}
}

func TestValidateRemovesCVVFromReturnedRequestOnError(t *testing.T) {
	v := NewPaymentDataValidator()
	req := validCardRequest()
	req.PaymentInfo.CustomerData.CvvCode = "invalid"

	validated, err := v.Validate(context.Background(), req)
	if err == nil {
		t.Fatalf("expected invalid CVV validation error")
	}
	if validated.PaymentInfo.CustomerData.CvvCode != "" {
		t.Fatalf("expected CVV to be cleared even when validation fails")
	}
}

func validCardRequest() dto.CreatePaymentRequest {
	return dto.CreatePaymentRequest{
		MerchantID:     "merchant_12345",
		IdempotencyKey: "idem_12345678",
		PaymentID:      "pay_12345",
		CurrentStatus:  string(dto.StatusCreated),
		PaymentInfo: dto.PaymentInfo{
			Amount: dto.AmountMoney{
				Value:    1500,
				Currency: "RUB",
			},
			PaymentMethodData: dto.PaymentMethodData{
				Type: dto.PaymentMethodCard,
			},
			CustomerData: dto.CustomerData{
				Email:      "customer@example.com",
				Phone:      "+79991234567",
				CardNumber: "4111111111111111",
				CardDate:   time.Now().UTC().AddDate(2, 0, 0).Format("01/06"),
				CvvCode:    "123",
			},
			CreatedAt:   time.Now().UTC(),
			Description: "Test card payment",
		},
	}
}
