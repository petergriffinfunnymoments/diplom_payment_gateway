package merchantauth

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	HeaderMerchantID = "X-Merchant-ID"
	HeaderAPIKey     = "X-API-Key"
	HeaderTimestamp  = "X-Timestamp"
	HeaderSignature  = "X-Signature"
)

type Merchant struct {
	MerchantID string
	Name       string
	APIKeyHash string
	SecretKey  string
	Active     bool
}

type MerchantProvider interface {
	GetByID(ctx context.Context, merchantID string) (Merchant, bool, error)
}

type Authenticator struct {
	provider        MerchantProvider
	timestampWindow time.Duration
}

func NewAuthenticator(provider MerchantProvider) *Authenticator {
	return &Authenticator{
		provider:        provider,
		timestampWindow: 5 * time.Minute,
	}
}

type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := a.Authenticate(r.Context(), r); err != nil {
			writeAuthError(w, err)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (a *Authenticator) Authenticate(ctx context.Context, r *http.Request) error {
	if a == nil || a.provider == nil {
		return errors.New("merchant authentication is not configured")
	}

	merchantID := strings.TrimSpace(r.Header.Get(HeaderMerchantID))
	apiKey := strings.TrimSpace(r.Header.Get(HeaderAPIKey))
	timestamp := strings.TrimSpace(r.Header.Get(HeaderTimestamp))
	signature := strings.TrimSpace(r.Header.Get(HeaderSignature))

	if merchantID == "" || apiKey == "" || timestamp == "" || signature == "" {
		return errors.New("missing merchant authentication headers")
	}

	merchant, found, err := a.provider.GetByID(ctx, merchantID)
	if err != nil {
		return fmt.Errorf("merchant lookup failed: %w", err)
	}
	if !found || !merchant.Active {
		return errors.New("merchant is not found or disabled")
	}

	if !constantTimeEqual(sha256Hex(apiKey), merchant.APIKeyHash) {
		return errors.New("invalid api key")
	}

	if err := a.validateTimestamp(timestamp); err != nil {
		return err
	}

	body, err := readAndRestoreBody(r)
	if err != nil {
		return fmt.Errorf("request body read failed: %w", err)
	}

	if r.Method == http.MethodPost && r.URL.Path == "/payments" {
		if err := checkBodyMerchantID(body, merchantID); err != nil {
			return err
		}
	}

	expected := BuildSignature(merchant.SecretKey, timestamp, r.Method, r.URL.RequestURI(), body)
	if !constantTimeEqual(signature, expected) {
		return errors.New("invalid request signature")
	}

	return nil
}

func (a *Authenticator) validateTimestamp(timestamp string) error {
	unix, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return errors.New("invalid timestamp")
	}

	t := time.Unix(unix, 0)
	now := time.Now()
	if t.Before(now.Add(-a.timestampWindow)) || t.After(now.Add(a.timestampWindow)) {
		return errors.New("timestamp is outside the allowed window")
	}

	return nil
}

func BuildSignature(secret string, timestamp string, method string, requestURI string, body []byte) string {
	bodyHash := sha256.Sum256(body)
	canonical := strings.Join([]string{
		timestamp,
		strings.ToUpper(method),
		requestURI,
		hex.EncodeToString(bodyHash[:]),
	}, ".")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

func readAndRestoreBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return []byte{}, nil
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if err := r.Body.Close(); err != nil {
		return nil, err
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func checkBodyMerchantID(body []byte, headerMerchantID string) error {
	if len(bytes.TrimSpace(body)) == 0 {
		return errors.New("empty request body")
	}

	var req struct {
		MerchantID string `json:"merchant_id"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return errors.New("invalid json body")
	}
	if strings.TrimSpace(req.MerchantID) == "" {
		return errors.New("merchant_id is required in request body")
	}
	if req.MerchantID != headerMerchantID {
		return errors.New("merchant_id in body does not match X-Merchant-ID")
	}
	return nil
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func constantTimeEqual(a string, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func writeAuthError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Code:    "AUTHENTICATION_ERROR",
		Message: err.Error(),
	})
}
