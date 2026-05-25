package simple

import (
	"context"
	"fmt"
	"testing"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
	"payment-gateway/internal/subsystems/adapter"
	"payment-gateway/internal/subsystems/storage"
)

type benchmarkPassedAntiFraud struct{}

func (a benchmarkPassedAntiFraud) Check(ctx context.Context, req dto.CreatePaymentRequest) (contracts.AntiFraudResult, error) {
	_ = ctx
	_ = req
	return contracts.AntiFraudResult{Result: "PASSED", Reason: "benchmark"}, nil
}

func BenchmarkCreatePaymentFullFlowSimulated(b *testing.B) {
	b.Setenv("SBP_PAYMENT_PROVIDER", "simulated")

	ctx := context.Background()
	store := storage.NewInMemoryTransactionStore()
	factory := adapter.NewFactory()
	orchestrator := NewSimpleOrchestratorWithRouting(store, noOpLogger{}, nil, nil, nil, factory)
	orchestrator.antiFraud = benchmarkPassedAntiFraud{}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req := benchmarkSBPPaymentRequest(i)
		resp, err := orchestrator.CreatePayment(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
		if resp.CurrentStatus != string(dto.StatusCaptured) {
			b.Fatalf("unexpected status: %s", resp.CurrentStatus)
		}
	}
}

func BenchmarkCreatePaymentIdempotencyHit(b *testing.B) {
	b.Setenv("SBP_PAYMENT_PROVIDER", "simulated")

	ctx := context.Background()
	store := storage.NewInMemoryTransactionStore()
	factory := adapter.NewFactory()
	orchestrator := NewSimpleOrchestratorWithRouting(store, noOpLogger{}, nil, nil, nil, factory)
	orchestrator.antiFraud = benchmarkPassedAntiFraud{}

	req := benchmarkSBPPaymentRequest(1)
	if _, err := orchestrator.CreatePayment(ctx, req); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		resp, err := orchestrator.CreatePayment(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
		if resp.CurrentStatus != string(dto.StatusCaptured) {
			b.Fatalf("unexpected status: %s", resp.CurrentStatus)
		}
	}
}

func benchmarkSBPPaymentRequest(i int) dto.CreatePaymentRequest {
	return dto.CreatePaymentRequest{
		MerchantID:     "merchant_benchmark",
		IdempotencyKey: fmt.Sprintf("bench_idem_%08d", i),
		PaymentID:      fmt.Sprintf("pay_bench_%08d", i),
		CurrentStatus:  string(dto.StatusCreated),
		PaymentInfo: dto.PaymentInfo{
			Amount: dto.AmountMoney{
				Value:    1500,
				Currency: "RUB",
			},
			PaymentMethodData: dto.PaymentMethodData{
				Type: dto.PaymentMethodSBP,
			},
			CustomerData: dto.CustomerData{
				Email: fmt.Sprintf("customer_%08d@example.com", i),
				Phone: "+79991234567",
			},
			CreatedAt:   time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
			Description: "Benchmark SBP payment",
		},
	}
}
