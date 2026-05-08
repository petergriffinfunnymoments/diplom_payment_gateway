package antifraud

import (
	"context"
	"strings"
	"testing"
	"time"

	"payment-gateway/internal/dto"
)

func TestVelocitySameCardBlocksBurst(t *testing.T) {
	af := NewRuleBasedAntiFraud()

	var result string
	var reason string
	for i := 0; i < 5; i++ {
		res, err := af.Check(context.Background(), testPaymentRequest("pay_card_burst_", i, "4111111111111111", "customer@example.com", "+79991234567", ""))
		if err != nil {
			t.Fatalf("check failed: %v", err)
		}
		result = res.Result
		reason = res.Reason
	}

	if result != ResultBlocked {
		t.Fatalf("expected %s, got %s (%s)", ResultBlocked, result, reason)
	}
	if !strings.Contains(reason, "same card fingerprint") {
		t.Fatalf("expected same card velocity reason, got %q", reason)
	}
}

func TestVelocityDistinctCardsPerEmailReviews(t *testing.T) {
	af := NewRuleBasedAntiFraud()
	cards := []string{
		"4111111111111111",
		"5555555555554444",
		"4000000000000000",
	}

	var result string
	var reason string
	for i, card := range cards {
		res, err := af.Check(context.Background(), testPaymentRequest("pay_email_cards_", i, card, "shared@example.com", "+79991234567", ""))
		if err != nil {
			t.Fatalf("check failed: %v", err)
		}
		result = res.Result
		reason = res.Reason
	}

	if result != ResultReview {
		t.Fatalf("expected %s, got %s (%s)", ResultReview, result, reason)
	}
	if !strings.Contains(reason, "email used 3 distinct cards") {
		t.Fatalf("expected distinct cards per email reason, got %q", reason)
	}
}

func TestIdempotentVelocityDoesNotDoubleCount(t *testing.T) {
	af := NewRuleBasedAntiFraud()
	req := testPaymentRequest("pay_same_", 1, "4111111111111111", "repeat@example.com", "+79991234567", "")

	for i := 0; i < 10; i++ {
		res, err := af.Check(context.Background(), req)
		if err != nil {
			t.Fatalf("check failed: %v", err)
		}
		if res.Result != ResultPassed {
			t.Fatalf("expected idempotent repeated check to stay %s, got %s (%s)", ResultPassed, res.Result, res.Reason)
		}
	}
}

func testPaymentRequest(prefix string, idx int, card string, email string, phone string, wallet string) dto.CreatePaymentRequest {
	return dto.CreatePaymentRequest{
		MerchantID:     "merchant_12345",
		IdempotencyKey: prefix + "idem_" + string(rune('a'+idx)),
		PaymentID:      prefix + string(rune('a'+idx)),
		CurrentStatus:  string(dto.StatusCreated),
		PaymentInfo: dto.PaymentInfo{
			Amount: dto.AmountMoney{
				Value:    1500,
				Currency: "RUB",
			},
			PaymentMethodData: dto.PaymentMethodData{Type: dto.PaymentMethodCard},
			CustomerData: dto.CustomerData{
				Email:           email,
				Phone:           phone,
				CardNumber:      card,
				CardDate:        "12/29",
				CvvCode:         "123",
				DigitalWalletID: wallet,
			},
			CreatedAt:   time.Now().UTC(),
			Description: "test payment",
		},
	}
}
