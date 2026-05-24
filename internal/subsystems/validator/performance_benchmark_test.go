package validator

import (
	"context"
	"testing"
	"time"

	"payment-gateway/internal/dto"
)

func BenchmarkValidateCardPayment(b *testing.B) {
	ctx := context.Background()
	v := NewPaymentDataValidator()
	req := benchmarkCardRequest()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := v.Validate(ctx, req); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidateDigitalRublePayment(b *testing.B) {
	ctx := context.Background()
	v := NewPaymentDataValidator()
	req := benchmarkDigitalRubleRequest()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := v.Validate(ctx, req); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkCardRequest() dto.CreatePaymentRequest {
	return dto.CreatePaymentRequest{
		MerchantID:     "merchant_benchmark",
		IdempotencyKey: "bench_idem_00000001",
		PaymentID:      "pay_bench_card_00000001",
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
				Phone:      "+7 (999) 123-45-67",
				CardNumber: "4111 1111 1111 1111",
				CardDate:   time.Now().UTC().AddDate(2, 0, 0).Format("01/06"),
				CvvCode:    "123",
			},
			CreatedAt:   time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
			Description: "Benchmark card payment",
		},
	}
}

func benchmarkDigitalRubleRequest() dto.CreatePaymentRequest {
	return dto.CreatePaymentRequest{
		MerchantID:     "merchant_benchmark",
		IdempotencyKey: "bench_idem_00000002",
		PaymentID:      "pay_bench_dr_00000002",
		CurrentStatus:  string(dto.StatusCreated),
		PaymentInfo: dto.PaymentInfo{
			Amount: dto.AmountMoney{
				Value:    1500,
				Currency: "RUB",
			},
			PaymentMethodData: dto.PaymentMethodData{
				Type: dto.PaymentMethodDigitalRuble,
			},
			CustomerData: dto.CustomerData{
				Email:                "customer@example.com",
				Phone:                "+7 (999) 123-45-67",
				DigitalRubleWalletID: "dr_wallet_123",
			},
			CreatedAt:   time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
			Description: "Benchmark digital ruble payment",
		},
	}
}
