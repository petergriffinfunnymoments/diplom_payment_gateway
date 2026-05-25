package security

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"payment-gateway/internal/subsystems/merchantauth"
)

func TestParseCIDRListAcceptsCIDRsAndSingleIPs(t *testing.T) {
	cidrs, err := ParseCIDRList("203.0.113.10, 198.51.100.0/24")
	if err != nil {
		t.Fatal(err)
	}
	if len(cidrs) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(cidrs))
	}
	if !IPAllowed(RemoteIP("203.0.113.10:443"), cidrs) {
		t.Fatal("expected single IP entry to match")
	}
	if !IPAllowed(RemoteIP("198.51.100.42:443"), cidrs) {
		t.Fatal("expected CIDR entry to match")
	}
}

func TestAuthenticatedNetworkMiddlewareRejectsMerchantOutsideAllowlist(t *testing.T) {
	merchantCIDRs, err := ParseCIDRList("203.0.113.0/24")
	if err != nil {
		t.Fatal(err)
	}
	handler := AuthenticatedNetworkMiddleware(
		NetworkConfig{MerchantAllowedCIDRs: merchantCIDRs},
		nil,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("next handler must not be called")
		}),
	)

	req := httptest.NewRequest(http.MethodPost, "http://example.test/payments", strings.NewReader("{}"))
	req.RemoteAddr = "198.51.100.10:12345"
	req = req.WithContext(merchantauth.WithMerchant(req.Context(), merchantauth.Merchant{
		MerchantID: "merchant_12345",
		Role:       merchantauth.RoleMerchant,
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "NETWORK_ACCESS_DENIED") {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestAuthenticatedNetworkMiddlewareAllowsTrustedForwardedMerchantIP(t *testing.T) {
	merchantCIDRs, err := ParseCIDRList("203.0.113.0/24")
	if err != nil {
		t.Fatal(err)
	}
	proxyCIDRs, err := ParseCIDRList("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	called := false
	handler := AuthenticatedNetworkMiddleware(
		NetworkConfig{TrustedProxyCIDRs: proxyCIDRs, MerchantAllowedCIDRs: merchantCIDRs},
		nil,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		}),
	)

	req := httptest.NewRequest(http.MethodPost, "http://example.test/payments", strings.NewReader("{}"))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.42, 198.51.100.9")
	req = req.WithContext(merchantauth.WithMerchant(req.Context(), merchantauth.Merchant{
		MerchantID: "merchant_12345",
		Role:       merchantauth.RoleMerchant,
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if !called {
		t.Fatal("expected next handler to be called")
	}
}

func TestAuthenticatedNetworkMiddlewareUsesAdminAllowlistForAuditor(t *testing.T) {
	adminCIDRs, err := ParseCIDRList("203.0.113.0/24")
	if err != nil {
		t.Fatal(err)
	}
	handler := AuthenticatedNetworkMiddleware(
		NetworkConfig{AdminAllowedCIDRs: adminCIDRs},
		nil,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "http://example.test/reports/transactions", nil)
	req.RemoteAddr = "203.0.113.99:12345"
	req = req.WithContext(merchantauth.WithMerchant(req.Context(), merchantauth.Merchant{
		MerchantID: "auditor_1",
		Role:       merchantauth.RoleAuditor,
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestWebhookNetworkMiddlewareRejectsOutsideAllowlist(t *testing.T) {
	webhookCIDRs, err := ParseCIDRList("203.0.113.0/24")
	if err != nil {
		t.Fatal(err)
	}
	handler := WebhookNetworkMiddleware(
		NetworkConfig{WebhookAllowedCIDRs: webhookCIDRs},
		nil,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("next handler must not be called")
		}),
	)

	req := httptest.NewRequest(http.MethodPost, "http://example.test/webhooks/payanyway", strings.NewReader("{}"))
	req.RemoteAddr = "198.51.100.10:12345"

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}
