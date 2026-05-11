package secrets

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVaultTransitProtectorProtectAndReveal(t *testing.T) {
	const secret = "pg_sk_test_secret"
	const ciphertext = "vault:v1:test-ciphertext"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Vault-Token"); got != "test-token" {
			t.Fatalf("unexpected X-Vault-Token header: %q", got)
		}
		if got := r.Header.Get("X-Vault-Namespace"); got != "admin" {
			t.Fatalf("unexpected X-Vault-Namespace header: %q", got)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}

		switch {
		case strings.HasSuffix(r.URL.Path, "/transit/encrypt/payment-gateway-merchant-secrets"):
			var req vaultTransitEncryptRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			plaintext, err := base64.StdEncoding.DecodeString(req.Plaintext)
			if err != nil {
				t.Fatal(err)
			}
			if string(plaintext) != secret {
				t.Fatalf("unexpected plaintext: %q", string(plaintext))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]string{"ciphertext": ciphertext},
			})
		case strings.HasSuffix(r.URL.Path, "/transit/decrypt/payment-gateway-merchant-secrets"):
			var req vaultTransitDecryptRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			if req.Ciphertext != ciphertext {
				t.Fatalf("unexpected ciphertext: %q", req.Ciphertext)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]string{"plaintext": base64.StdEncoding.EncodeToString([]byte(secret))},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	protector, err := NewVaultTransitProtector(VaultTransitConfig{
		Address:    server.URL,
		Token:      "test-token",
		Namespace:  "admin",
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}

	protected, err := protector.Protect(context.Background(), secret)
	if err != nil {
		t.Fatal(err)
	}
	if protected != ciphertext {
		t.Fatalf("unexpected protected value: %q", protected)
	}

	revealed, err := protector.Reveal(context.Background(), protected)
	if err != nil {
		t.Fatal(err)
	}
	if revealed != secret {
		t.Fatalf("unexpected revealed value: %q", revealed)
	}
}

func TestVaultTransitProtectorRevealAllowsLegacyPlaintext(t *testing.T) {
	protector, err := NewVaultTransitProtector(VaultTransitConfig{
		Token: "test-token",
	})
	if err != nil {
		t.Fatal(err)
	}

	revealed, err := protector.Reveal(context.Background(), "legacy-secret")
	if err != nil {
		t.Fatal(err)
	}
	if revealed != "legacy-secret" {
		t.Fatalf("unexpected revealed value: %q", revealed)
	}
}
