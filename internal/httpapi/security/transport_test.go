package security

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddlewareAllowsHTTPWhenHTTPSNotRequired(t *testing.T) {
	handler := Middleware(TransportConfig{}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://example.test/health", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("expected security headers to be set")
	}
}

func TestMiddlewareRejectsHTTPWhenHTTPSRequired(t *testing.T) {
	handler := Middleware(TransportConfig{RequireHTTPS: true}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler must not be called")
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "http://example.test/payments", strings.NewReader("{}")))

	if rec.Code != http.StatusUpgradeRequired {
		t.Fatalf("expected 426, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "HTTPS_REQUIRED") {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestMiddlewareAllowsTrustedForwardedHTTPS(t *testing.T) {
	proxyCIDRs, err := ParseCIDRList("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := Middleware(TransportConfig{RequireHTTPS: true, TrustProxyHeaders: true, TrustedProxyCIDRs: proxyCIDRs}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "http://example.test/payments", strings.NewReader("{}"))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-Proto", "https")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("expected HSTS for trusted HTTPS request")
	}
}

func TestMiddlewareIgnoresForwardedHTTPSFromUntrustedProxy(t *testing.T) {
	proxyCIDRs, err := ParseCIDRList("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := Middleware(TransportConfig{RequireHTTPS: true, TrustProxyHeaders: true, TrustedProxyCIDRs: proxyCIDRs}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler must not be called")
	}))
	req := httptest.NewRequest(http.MethodPost, "http://example.test/payments", strings.NewReader("{}"))
	req.RemoteAddr = "198.51.100.10:12345"
	req.Header.Set("X-Forwarded-Proto", "https")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUpgradeRequired {
		t.Fatalf("expected 426, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestValidateOutboundURLsRequiresHTTPS(t *testing.T) {
	err := ValidateOutboundURLs(true, map[string]string{
		"MERCHANT_WEBHOOK_URL": "http://merchant.example/webhook",
	})
	if err == nil {
		t.Fatal("expected http URL to be rejected")
	}

	err = ValidateOutboundURLs(true, map[string]string{
		"MERCHANT_WEBHOOK_URL": "https://merchant.example/webhook",
		"PAYMENT_RETURN_URL":   "",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateTLSConfigRequiresBothFiles(t *testing.T) {
	if err := ValidateTLSConfig("cert.pem", ""); err == nil {
		t.Fatal("expected missing key file to be rejected")
	}
	if err := ValidateTLSConfig("cert.pem", "key.pem"); err != nil {
		t.Fatal(err)
	}
}
