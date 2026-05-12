package reports

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"payment-gateway/internal/dto"
	"payment-gateway/internal/subsystems/merchantauth"
)

func TestMerchantCanAccessOnlyOwnTransactionReport(t *testing.T) {
	store := &capturingReportStore{}
	handler := NewTransactionReportHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/reports/transactions?merchant_id=merchant_other", nil)
	req = req.WithContext(merchantauth.WithMerchant(req.Context(), merchantauth.Merchant{
		MerchantID: "merchant_12345",
		Role:       merchantauth.RoleMerchant,
	}))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
	if store.called {
		t.Fatal("report store should not be called for another merchant")
	}

	var body dto.TransactionReportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error == nil || body.Error.Code != "MERCHANT_SCOPE_MISMATCH" {
		t.Fatalf("unexpected error response: %+v", body.Error)
	}
}

func TestMerchantCanAccessOwnTransactionReport(t *testing.T) {
	store := &capturingReportStore{}
	handler := NewTransactionReportHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/reports/transactions?merchant_id=merchant_12345", nil)
	req = req.WithContext(merchantauth.WithMerchant(req.Context(), merchantauth.Merchant{
		MerchantID: "merchant_12345",
		Role:       merchantauth.RoleMerchant,
	}))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !store.called {
		t.Fatal("report store should be called")
	}
	if store.filter.MerchantID != "merchant_12345" {
		t.Fatalf("unexpected filter merchant_id: %q", store.filter.MerchantID)
	}
}

func TestAdminCanAccessAnyMerchantTransactionReport(t *testing.T) {
	store := &capturingReportStore{}
	handler := NewTransactionReportHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/reports/transactions?merchant_id=merchant_other", nil)
	req = req.WithContext(merchantauth.WithMerchant(req.Context(), merchantauth.Merchant{
		MerchantID: "admin_1",
		Role:       merchantauth.RoleAdmin,
	}))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !store.called {
		t.Fatal("report store should be called")
	}
	if store.filter.MerchantID != "merchant_other" {
		t.Fatalf("unexpected filter merchant_id: %q", store.filter.MerchantID)
	}
}

func TestAuditorCanAccessAnyMerchantTransactionReport(t *testing.T) {
	store := &capturingReportStore{}
	handler := NewTransactionReportHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/reports/transactions?merchant_id=merchant_other", nil)
	req = req.WithContext(merchantauth.WithMerchant(req.Context(), merchantauth.Merchant{
		MerchantID: "auditor_1",
		Role:       merchantauth.RoleAuditor,
	}))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !store.called {
		t.Fatal("report store should be called")
	}
	if store.filter.MerchantID != "merchant_other" {
		t.Fatalf("unexpected filter merchant_id: %q", store.filter.MerchantID)
	}
}

type capturingReportStore struct {
	called bool
	filter dto.TransactionReportFilter
}

func (s *capturingReportStore) BuildTransactionReport(ctx context.Context, filter dto.TransactionReportFilter) (dto.TransactionReport, error) {
	_ = ctx
	s.called = true
	s.filter = filter
	return dto.TransactionReport{
		MerchantID: filter.MerchantID,
		Summary: dto.TransactionReportSummary{
			ByStatus:        map[string]dto.TransactionReportBucket{},
			ByPaymentSystem: map[string]dto.TransactionReportBucket{},
			ByPaymentMethod: map[string]dto.TransactionReportBucket{},
		},
	}, nil
}
