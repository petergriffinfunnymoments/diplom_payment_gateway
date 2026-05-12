package merchantauth

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestAuthenticateMerchantRejectsUnknownRole(t *testing.T) {
	auth := NewAuthenticator(staticTestProvider{merchant: Merchant{
		MerchantID: "merchant_12345",
		APIKeyHash: sha256Hex("api_key"),
		SecretKey:  "secret_key",
		Role:       "superuser",
		Active:     true,
	}})

	req := signedTestRequest(t, http.MethodGet, "/reports/transactions?merchant_id=merchant_12345", nil, "merchant_12345", "api_key", "secret_key")
	if _, err := auth.AuthenticateMerchant(context.Background(), req); err == nil {
		t.Fatal("expected unknown role to be rejected")
	}
}

func TestAuthenticateMerchantAllowsAuditorRole(t *testing.T) {
	auth := NewAuthenticator(staticTestProvider{merchant: Merchant{
		MerchantID: "auditor_1",
		APIKeyHash: sha256Hex("api_key"),
		SecretKey:  "secret_key",
		Role:       RoleAuditor,
		Active:     true,
	}})

	req := signedTestRequest(t, http.MethodGet, "/reports/transactions?merchant_id=merchant_12345", nil, "auditor_1", "api_key", "secret_key")
	merchant, err := auth.AuthenticateMerchant(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if merchant.Role != RoleAuditor {
		t.Fatalf("unexpected role: %q", merchant.Role)
	}
}

type staticTestProvider struct {
	merchant Merchant
}

func (p staticTestProvider) GetByID(ctx context.Context, merchantID string) (Merchant, bool, error) {
	_ = ctx
	if merchantID != p.merchant.MerchantID {
		return Merchant{}, false, nil
	}
	return p.merchant, true, nil
}

func signedTestRequest(t *testing.T, method string, target string, body []byte, merchantID string, apiKey string, secretKey string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req.Header.Set(HeaderMerchantID, merchantID)
	req.Header.Set(HeaderAPIKey, apiKey)
	req.Header.Set(HeaderTimestamp, ts)
	req.Header.Set(HeaderSignature, BuildSignature(secretKey, ts, method, req.URL.RequestURI(), body))
	return req
}
