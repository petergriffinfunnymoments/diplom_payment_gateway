package secrets

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	VaultCiphertextPrefix    = "vault:v"
	defaultVaultAddress      = "http://127.0.0.1:8200"
	defaultVaultTransitMount = "transit"
	defaultVaultTransitKey   = "payment-gateway-merchant-secrets"
	maxVaultErrorBodySize    = 4096
)

type VaultTransitConfig struct {
	Address    string
	Token      string
	TokenFile  string
	Namespace  string
	MountPath  string
	KeyName    string
	Context    string
	Timeout    time.Duration
	APIVersion string
	HTTPClient *http.Client
}

type VaultTransitProtector struct {
	address    string
	token      string
	tokenFile  string
	namespace  string
	mountPath  string
	keyName    string
	context    string
	apiVersion string
	client     *http.Client
}

func NewVaultTransitProtector(cfg VaultTransitConfig) (*VaultTransitProtector, error) {
	address := strings.TrimRight(strings.TrimSpace(cfg.Address), "/")
	if address == "" {
		address = defaultVaultAddress
	}

	token := strings.TrimSpace(cfg.Token)
	tokenFile := strings.TrimSpace(cfg.TokenFile)
	if token == "" && tokenFile == "" {
		return nil, errors.New("VAULT_TOKEN or VAULT_TOKEN_FILE is required")
	}

	mountPath := strings.Trim(strings.TrimSpace(cfg.MountPath), "/")
	if mountPath == "" {
		mountPath = defaultVaultTransitMount
	}

	keyName := strings.Trim(strings.TrimSpace(cfg.KeyName), "/")
	if keyName == "" {
		keyName = defaultVaultTransitKey
	}

	client := cfg.HTTPClient
	if client == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}

	apiVersion := strings.TrimSpace(cfg.APIVersion)
	if apiVersion == "" {
		apiVersion = "v1"
	}

	return &VaultTransitProtector{
		address:    address,
		token:      token,
		tokenFile:  tokenFile,
		namespace:  strings.TrimSpace(cfg.Namespace),
		mountPath:  mountPath,
		keyName:    keyName,
		context:    cfg.Context,
		apiVersion: apiVersion,
		client:     client,
	}, nil
}

func (p *VaultTransitProtector) Protect(ctx context.Context, plaintext string) (string, error) {
	if plaintext == "" || strings.HasPrefix(plaintext, VaultCiphertextPrefix) {
		return plaintext, nil
	}

	req := vaultTransitEncryptRequest{
		Plaintext: base64.StdEncoding.EncodeToString([]byte(plaintext)),
		Context:   p.encodedContext(),
	}
	var resp vaultTransitResponse
	if err := p.call(ctx, "encrypt", req, &resp); err != nil {
		return "", err
	}
	if strings.TrimSpace(resp.Data.Ciphertext) == "" {
		return "", errors.New("vault transit encrypt returned empty ciphertext")
	}
	return resp.Data.Ciphertext, nil
}

func (p *VaultTransitProtector) Reveal(ctx context.Context, storedValue string) (string, error) {
	if storedValue == "" || !strings.HasPrefix(storedValue, VaultCiphertextPrefix) {
		return storedValue, nil
	}

	req := vaultTransitDecryptRequest{
		Ciphertext: storedValue,
		Context:    p.encodedContext(),
	}
	var resp vaultTransitResponse
	if err := p.call(ctx, "decrypt", req, &resp); err != nil {
		return "", err
	}
	if strings.TrimSpace(resp.Data.Plaintext) == "" {
		return "", errors.New("vault transit decrypt returned empty plaintext")
	}

	plaintext, err := base64.StdEncoding.DecodeString(resp.Data.Plaintext)
	if err != nil {
		return "", fmt.Errorf("vault transit decrypt returned invalid base64 plaintext: %w", err)
	}
	return string(plaintext), nil
}

func (p *VaultTransitProtector) Enabled() bool {
	return true
}

func (p *VaultTransitProtector) call(ctx context.Context, operation string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	endpoint := fmt.Sprintf(
		"%s/%s/%s/%s/%s",
		p.address,
		strings.Trim(p.apiVersion, "/"),
		escapeVaultPath(p.mountPath),
		operation,
		escapeVaultPath(p.keyName),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}

	token, err := p.readToken()
	if err != nil {
		return err
	}
	req.Header.Set("X-Vault-Token", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if p.namespace != "" {
		req.Header.Set("X-Vault-Namespace", p.namespace)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("vault transit %s request failed: %w", operation, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errorBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxVaultErrorBodySize))
		return fmt.Errorf("vault transit %s failed: http %d: %s", operation, resp.StatusCode, strings.TrimSpace(string(errorBody)))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("vault transit %s response decode failed: %w", operation, err)
	}
	return nil
}

func (p *VaultTransitProtector) readToken() (string, error) {
	if p.tokenFile != "" {
		raw, err := os.ReadFile(p.tokenFile)
		if err != nil {
			return "", fmt.Errorf("VAULT_TOKEN_FILE read failed: %w", err)
		}
		token := strings.TrimSpace(string(raw))
		if token == "" {
			return "", errors.New("VAULT_TOKEN_FILE is empty")
		}
		return token, nil
	}

	token := strings.TrimSpace(p.token)
	if token == "" {
		return "", errors.New("VAULT_TOKEN is empty")
	}
	return token, nil
}

func (p *VaultTransitProtector) encodedContext() string {
	if p.context == "" {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(p.context))
}

func escapeVaultPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

type vaultTransitEncryptRequest struct {
	Plaintext string `json:"plaintext"`
	Context   string `json:"context,omitempty"`
}

type vaultTransitDecryptRequest struct {
	Ciphertext string `json:"ciphertext"`
	Context    string `json:"context,omitempty"`
}

type vaultTransitResponse struct {
	Data struct {
		Ciphertext string `json:"ciphertext"`
		Plaintext  string `json:"plaintext"`
	} `json:"data"`
}
