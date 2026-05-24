package antifraud

import (
	"context"
	"fmt"
	"testing"
	"time"

	"payment-gateway/internal/dto"
)

func BenchmarkRuleBasedAntiFraudPassedIdempotent(b *testing.B) {
	ctx := context.Background()
	af := NewRuleBasedAntiFraud()
	req := benchmarkAntiFraudPaymentRequest("pay_bench_same", "idem_bench_same", 1500, "customer@example.com")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		res, err := af.Check(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
		if res.Result != ResultPassed {
			b.Fatalf("unexpected result: %s (%s)", res.Result, res.Reason)
		}
	}
}

func BenchmarkRuleBasedAntiFraudBlockedAmount(b *testing.B) {
	ctx := context.Background()
	af := NewRuleBasedAntiFraud()
	req := benchmarkAntiFraudPaymentRequest("pay_bench_blocked", "idem_bench_blocked", 500000, "customer@example.com")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		res, err := af.Check(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
		if res.Result != ResultBlocked {
			b.Fatalf("unexpected result: %s (%s)", res.Result, res.Reason)
		}
	}
}

func BenchmarkRuleBasedAntiFraudUniquePayments(b *testing.B) {
	ctx := context.Background()
	af := NewRuleBasedAntiFraud()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req := benchmarkAntiFraudPaymentRequest(
			fmt.Sprintf("pay_bench_%08d", i),
			fmt.Sprintf("idem_bench_%08d", i),
			1500,
			fmt.Sprintf("customer_%08d@example.com", i),
		)
		req.PaymentInfo.CustomerData.Phone = fmt.Sprintf("+7988%07d", i%10000000)
		req.PaymentInfo.CustomerData.CardNumber = ""
		req.PaymentInfo.PaymentMethodData.Type = dto.PaymentMethodSBP
		res, err := af.Check(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
		if res.Result != ResultPassed && res.Result != ResultReview {
			b.Fatalf("unexpected result: %s (%s)", res.Result, res.Reason)
		}
	}
}

func benchmarkAntiFraudPaymentRequest(paymentID string, idempotencyKey string, amount float64, email string) dto.CreatePaymentRequest {
	return dto.CreatePaymentRequest{
		MerchantID:     "merchant_benchmark",
		IdempotencyKey: idempotencyKey,
		PaymentID:      paymentID,
		CurrentStatus:  string(dto.StatusCreated),
		PaymentInfo: dto.PaymentInfo{
			Amount: dto.AmountMoney{
				Value:    amount,
				Currency: "RUB",
			},
			PaymentMethodData: dto.PaymentMethodData{Type: dto.PaymentMethodCard},
			CustomerData: dto.CustomerData{
				Email:      email,
				Phone:      "+79991234567",
				CardNumber: "4111111111111111",
				CardDate:   "12/29",
				CvvCode:    "123",
			},
			CreatedAt:   time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
			Description: "Performance payment",
		},
	}
}
