package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

func BenchmarkInMemoryTransactionReport1000Rows(b *testing.B) {
	ctx := context.Background()
	store := NewInMemoryTransactionStore()
	reportStore := store.(contracts.TransactionReportStore)

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 1000; i++ {
		status := string(dto.StatusCaptured)
		system := "YOOKASSA"
		if i%3 == 1 {
			status = string(dto.StatusPending)
			system = "ROBOKASSA"
		}
		if i%3 == 2 {
			status = string(dto.StatusDeclined)
			system = "STRIPE"
		}
		benchmarkSavePayment(b, store, "merchant_benchmark", fmt.Sprintf("pay_bench_%04d", i), fmt.Sprintf("idem_bench_%04d", i), status, float64(1000+i), system, now.Add(time.Duration(i)*time.Second))
	}

	filter := dto.TransactionReportFilter{
		MerchantID: "merchant_benchmark",
		Limit:      500,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		report, err := reportStore.BuildTransactionReport(ctx, filter)
		if err != nil {
			b.Fatal(err)
		}
		if report.Summary.TotalCount != 500 {
			b.Fatalf("unexpected report count: %d", report.Summary.TotalCount)
		}
	}
}

func benchmarkSavePayment(
	b *testing.B,
	store contracts.TransactionStore,
	merchantID string,
	paymentID string,
	idempotencyKey string,
	status string,
	amount float64,
	paymentSystem string,
	createdAt time.Time,
) {
	b.Helper()
	resp := dto.PaymentResponse{
		ID:             paymentID,
		MerchantID:     merchantID,
		IdempotencyKey: idempotencyKey,
		CurrentStatus:  status,
		PaymentInfo: dto.PaymentInfoResponse{
			Amount: dto.AmountMoney{
				Value:    amount,
				Currency: "RUB",
			},
			PaymentMethodData: dto.PaymentMethodData{
				Type: dto.PaymentMethodCard,
			},
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
		},
		TransactionDetails: dto.TransactionDetails{
			PaymentSystem:    paymentSystem,
			ProviderStatus:   "benchmark",
			FraudCheckResult: "PASSED",
		},
	}
	payload, err := json.Marshal(resp)
	if err != nil {
		b.Fatal(err)
	}
	if err := store.Save(context.Background(), merchantID, paymentID, idempotencyKey, status, string(payload), createdAt); err != nil {
		b.Fatal(err)
	}
}
