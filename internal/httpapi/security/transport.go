package security

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type TransportConfig struct {
	RequireHTTPS      bool
	TrustProxyHeaders bool
}

type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func TransportConfigFromEnv() TransportConfig {
	return TransportConfig{
		RequireHTTPS:      BoolEnv("REQUIRE_HTTPS") || IsProductionEnv(os.Getenv("APP_ENV")),
		TrustProxyHeaders: BoolEnv("TRUST_PROXY_HEADERS"),
	}
}

func Middleware(cfg TransportConfig, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		SetSecurityHeaders(w, r, cfg)
		if cfg.RequireHTTPS && !IsSecureRequest(r, cfg.TrustProxyHeaders) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUpgradeRequired)
			_ = json.NewEncoder(w).Encode(ErrorResponse{
				Code:    "HTTPS_REQUIRED",
				Message: "HTTPS is required for payment gateway requests",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func SetSecurityHeaders(w http.ResponseWriter, r *http.Request, cfg TransportConfig) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	if IsSecureRequest(r, cfg.TrustProxyHeaders) {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	}
}

func IsSecureRequest(r *http.Request, trustProxyHeaders bool) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil || strings.EqualFold(r.URL.Scheme, "https") {
		return true
	}
	if !trustProxyHeaders {
		return false
	}
	if strings.EqualFold(firstHeaderValue(r.Header.Get("X-Forwarded-Proto")), "https") {
		return true
	}
	return forwardedHeaderHasHTTPS(r.Header.Get("Forwarded"))
}

func ValidateHTTPSURL(name string, rawValue string, required bool) error {
	rawValue = strings.TrimSpace(rawValue)
	if rawValue == "" {
		if required {
			return fmt.Errorf("%s is required", name)
		}
		return nil
	}
	u, err := url.Parse(rawValue)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", name, err)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("%s must use https:// when HTTPS enforcement is enabled", name)
	}
	if u.Host == "" {
		return fmt.Errorf("%s must include host", name)
	}
	return nil
}

func ValidateOutboundURLs(requireHTTPS bool, values map[string]string) error {
	if !requireHTTPS {
		return nil
	}
	for name, value := range values {
		if err := ValidateHTTPSURL(name, value, false); err != nil {
			return err
		}
	}
	return nil
}

func ValidateTLSConfig(certFile string, keyFile string) error {
	certFile = strings.TrimSpace(certFile)
	keyFile = strings.TrimSpace(keyFile)
	if certFile == "" && keyFile == "" {
		return nil
	}
	if certFile == "" || keyFile == "" {
		return errors.New("TLS_CERT_FILE and TLS_KEY_FILE must be set together")
	}
	return nil
}

func BoolEnv(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func IsProductionEnv(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "prod", "production":
		return true
	default:
		return false
	}
}

func firstHeaderValue(value string) string {
	if i := strings.Index(value, ","); i >= 0 {
		return strings.TrimSpace(value[:i])
	}
	return strings.TrimSpace(value)
}

func forwardedHeaderHasHTTPS(value string) bool {
	for _, part := range strings.Split(value, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(part), "proto=") {
			proto := strings.Trim(strings.TrimSpace(part[len("proto="):]), `"`)
			return strings.EqualFold(proto, "https")
		}
	}
	return false
}
