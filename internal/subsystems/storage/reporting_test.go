package storage

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
)

func TestInMemoryTransactionReportBuildsMerchantScopedSummary(t *testing.T) {
	store := NewInMemoryTransactionStore()
	reportStore, ok := store.(contracts.TransactionReportStore)
	if !ok {
		t.Fatal("store does not implement TransactionReportStore")
	}

	now := time.Now().UTC()
	savePayment(t, store, "merchant_12345", "pay_1", "idem_1", string(dto.StatusCaptured), 1500, "YOOKASSA", now)
	savePayment(t, store, "merchant_12345", "pay_2", "idem_2", string(dto.StatusDeclined), 700, "PAYANYWAY", now.Add(time.Second))
	savePayment(t, store, "merchant_other", "pay_3", "idem_3", string(dto.StatusCaptured), 9999, "PAYANYWAY", now.Add(2*time.Second))

	report, err := reportStore.BuildTransactionReport(context.Background(), dto.TransactionReportFilter{
		MerchantID: "merchant_12345",
		Limit:      100,
	})
	if err != nil {
		t.Fatal(err)
	}

	if report.Summary.TotalCount != 2 {
		t.Fatalf("unexpected total count: %d", report.Summary.TotalCount)
	}
	if report.Summary.CapturedCount != 1 {
		t.Fatalf("unexpected captured count: %d", report.Summary.CapturedCount)
	}
	if report.Summary.TotalAmount != 2200 {
		t.Fatalf("unexpected total amount: %.2f", report.Summary.TotalAmount)
	}
	if report.Summary.ByPaymentSystem["YOOKASSA"].Count != 1 {
		t.Fatalf("expected YOOKASSA bucket")
	}
	if report.Transactions[0].PaymentID != "pay_2" {
		t.Fatalf("expected newest transaction first, got %s", report.Transactions[0].PaymentID)
	}
}

func TestInMemoryTransactionReportFiltersByStatus(t *testing.T) {
	store := NewInMemoryTransactionStore()
	reportStore := store.(contracts.TransactionReportStore)

	now := time.Now().UTC()
	savePayment(t, store, "merchant_12345", "pay_1", "idem_1", string(dto.StatusCaptured), 1500, "YOOKASSA", now)
	savePayment(t, store, "merchant_12345", "pay_2", "idem_2", string(dto.StatusDeclined), 700, "PAYANYWAY", now.Add(time.Second))

	report, err := reportStore.BuildTransactionReport(context.Background(), dto.TransactionReportFilter{
		MerchantID: "merchant_12345",
		Status:     string(dto.StatusCaptured),
		Limit:      100,
	})
	if err != nil {
		t.Fatal(err)
	}

	if report.Summary.TotalCount != 1 {
		t.Fatalf("unexpected total count: %d", report.Summary.TotalCount)
	}
	if report.Transactions[0].Status != string(dto.StatusCaptured) {
		t.Fatalf("unexpected status: %s", report.Transactions[0].Status)
	}
}

func savePayment(
	t *testing.T,
	store contracts.TransactionStore,
	merchantID string,
	paymentID string,
	idempotencyKey string,
	status string,
	amount float64,
	paymentSystem string,
	createdAt time.Time,
) {
	t.Helper()
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
			ProviderStatus:   "test",
			FraudCheckResult: "PASSED",
		},
	}
	if err := store.Save(context.Background(), merchantID, paymentID, idempotencyKey, status, mustMarshal(resp), createdAt); err != nil {
		t.Fatal(err)
	}
}

func mustMarshal(resp dto.PaymentResponse) string {
	b, _ := json.Marshal(resp)
	return string(b)
}
