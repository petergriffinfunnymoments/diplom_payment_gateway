package secrets

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"
)

type Protector interface {
	Protect(ctx context.Context, plaintext string) (string, error)
	Reveal(ctx context.Context, storedValue string) (string, error)
	Enabled() bool
}

type NoopProtector struct{}

func (NoopProtector) Protect(_ context.Context, plaintext string) (string, error) {
	return plaintext, nil
}

func (NoopProtector) Reveal(_ context.Context, storedValue string) (string, error) {
	return storedValue, nil
}

func (NoopProtector) Enabled() bool {
	return false
}

func NewProtectorFromEnv() (Protector, error) {
	provider := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		os.Getenv("SECRET_PROTECTOR"),
		os.Getenv("SECRET_PROTECTOR_PROVIDER"),
	)))

	switch provider {
	case "", "none", "noop", "plain", "plaintext":
		return NoopProtector{}, nil
	case "vault", "vault_transit", "vault-transit", "hashicorp_vault", "hashicorp-vault":
		return NewVaultTransitProtector(VaultTransitConfig{
			Address:    os.Getenv("VAULT_ADDR"),
			Token:      os.Getenv("VAULT_TOKEN"),
			TokenFile:  os.Getenv("VAULT_TOKEN_FILE"),
			Namespace:  os.Getenv("VAULT_NAMESPACE"),
			MountPath:  os.Getenv("VAULT_TRANSIT_MOUNT"),
			KeyName:    os.Getenv("VAULT_TRANSIT_KEY"),
			Context:    os.Getenv("VAULT_TRANSIT_CONTEXT"),
			Timeout:    10 * time.Second,
			APIVersion: os.Getenv("VAULT_API_VERSION"),
		})
	default:
		return nil, errors.New("unsupported SECRET_PROTECTOR value")
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
