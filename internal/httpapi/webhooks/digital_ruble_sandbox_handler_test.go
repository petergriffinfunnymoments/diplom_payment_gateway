package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"
	"payment-gateway/internal/subsystems/storage"
)

func TestDigitalRubleSandboxScanCapturesPendingPayment(t *testing.T) {
	store := storage.NewInMemoryTransactionStore()
	saveDigitalRublePayment(t, store, time.Now().UTC().Add(15*time.Minute))

	handler := NewDigitalRubleSandboxHandler(store, nil, nil)
	reqBody := []byte(`{"merchant_id":"merchant_12345","payment_id":"pay_dr_test","qr_id":"drqr_test","result":"captured"}`)
	req := httptest.NewRequest(http.MethodPost, "/sandbox/digital-ruble/scan", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp dto.PaymentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.CurrentStatus != string(dto.StatusCaptured) {
		t.Fatalf("expected CAPTURED, got %s", resp.CurrentStatus)
	}
	if resp.TransactionDetails.ProviderStatus != "settled" {
		t.Fatalf("expected settled, got %s", resp.TransactionDetails.ProviderStatus)
	}
	if resp.TransactionDetails.SmartContractResult != "PASSED" {
		t.Fatalf("expected smart contract PASSED, got %s", resp.TransactionDetails.SmartContractResult)
	}

	status, _, found, err := store.GetByPaymentID(context.Background(), "merchant_12345", "pay_dr_test")
	if err != nil {
		t.Fatal(err)
	}
	if !found || status != string(dto.StatusCaptured) {
		t.Fatalf("expected stored CAPTURED, found=%v status=%s", found, status)
	}
}

func TestDigitalRubleSandboxScanRejectsMarkedMoneyMismatch(t *testing.T) {
	store := storage.NewInMemoryTransactionStore()
	saveDigitalRublePayment(t, store, time.Now().UTC().Add(15*time.Minute))

	handler := NewDigitalRubleSandboxHandler(store, nil, nil)
	reqBody := []byte(`{"merchant_id":"merchant_12345","payment_id":"pay_dr_test","qr_id":"drqr_test","payer_wallet_id":"dr_wallet_no_mark","result":"captured"}`)
	req := httptest.NewRequest(http.MethodPost, "/sandbox/digital-ruble/scan", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp dto.PaymentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.CurrentStatus != string(dto.StatusDeclined) {
		t.Fatalf("expected DECLINED, got %s", resp.CurrentStatus)
	}
	if resp.Error == nil || resp.Error.Code != dto.ErrorDigitalRubleMarkFailed {
		t.Fatalf("expected marked money error, got %+v", resp.Error)
	}
	if resp.TransactionDetails.MoneyMark != "EDUCATION" {
		t.Fatalf("expected EDUCATION money mark, got %s", resp.TransactionDetails.MoneyMark)
	}
}

func TestDigitalRubleSandboxScanExpiresOldQRCode(t *testing.T) {
	store := storage.NewInMemoryTransactionStore()
	saveDigitalRublePayment(t, store, time.Now().UTC().Add(-time.Minute))

	handler := NewDigitalRubleSandboxHandler(store, nil, nil)
	reqBody := []byte(`{"merchant_id":"merchant_12345","payment_id":"pay_dr_test","qr_id":"drqr_test","result":"captured"}`)
	req := httptest.NewRequest(http.MethodPost, "/sandbox/digital-ruble/scan", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp dto.PaymentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.CurrentStatus != string(dto.StatusCancelled) {
		t.Fatalf("expected CANCELLED, got %s", resp.CurrentStatus)
	}
	if resp.Error == nil || resp.Error.Code != dto.ErrorDigitalRubleQRExpired {
		t.Fatalf("expected QR expired error, got %+v", resp.Error)
	}
}

func saveDigitalRublePayment(t *testing.T, store contracts.TransactionStore, expiresAt time.Time) {
	t.Helper()

	resp := dto.PaymentResponse{
		ID:             "pay_dr_test",
		MerchantID:     "merchant_12345",
		IdempotencyKey: "idem_dr_test",
		CurrentStatus:  string(dto.StatusPending),
		PaymentInfo: dto.PaymentInfoResponse{
			Amount: dto.AmountMoney{
				Value:    1500,
				Currency: "RUB",
			},
			PaymentMethodData: dto.PaymentMethodData{Type: dto.PaymentMethodDigitalRuble},
			CustomerData: dto.CustomerData{
				DigitalRubleWalletID: "dr_wallet_123",
			},
			Items: []dto.PaymentItem{
				{
					Name:     "Учебник",
					Price:    1500,
					Quantity: 1,
					Category: "education",
				},
			},
			DigitalRubleData: dto.DigitalRubleData{
				SmartContractID:    "SC_MARKED_MONEY_V1",
				RequireMarkedMoney: true,
			},
			Description: "digital ruble test",
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		},
		TransactionDetails: dto.TransactionDetails{
			ExternalTransactionID: "drub_test",
			PaymentSystem:         "DIGITAL_RUBLE",
			ProviderStatus:        "qr_issued",
			QRID:                  "drqr_test",
			QRPayload:             "drub://qr_id=drqr_test",
			QRExpiresAt:           expiresAt.Format(time.RFC3339),
			MoneyMark:             "EDUCATION",
			SmartContractID:       "SC_MARKED_MONEY_V1",
			SmartContractResult:   "PENDING",
			PlatformMessageID:     "drmsg_test",
			FraudCheckResult:      "PASSED",
		},
	}
	payload, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), resp.MerchantID, resp.ID, resp.IdempotencyKey, resp.CurrentStatus, string(payload), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
}
