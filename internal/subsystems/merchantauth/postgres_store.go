package merchantauth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"payment-gateway/internal/subsystems/secrets"
)

type PostgresMerchantStore struct {
	pool      *pgxpool.Pool
	protector secrets.Protector
}

func NewPostgresMerchantStore(ctx context.Context, dsn string) (*PostgresMerchantStore, error) {
	if dsn == "" {
		return nil, errors.New("dsn is empty")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}

	protector, err := secrets.NewProtectorFromEnv()
	if err != nil {
		pool.Close()
		return nil, err
	}

	store := &PostgresMerchantStore{pool: pool, protector: protector}
	if err := store.ensureSchema(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	if err := store.seedDefaultMerchant(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return store, nil
}

func (s *PostgresMerchantStore) ensureSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS merchants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    api_key_hash TEXT NOT NULL,
    secret_key TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'merchant',
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE merchants ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'merchant';

CREATE INDEX IF NOT EXISTS idx_merchants_active
    ON merchants (merchant_id, active);
`)
	return err
}

func (s *PostgresMerchantStore) seedDefaultMerchant(ctx context.Context) error {
	merchantID, hasMerchantID := lookupEnvTrimmed("MERCHANT_ID")
	apiKey, hasAPIKey := lookupEnvTrimmed("MERCHANT_API_KEY")
	secretKey, hasSecretKey := lookupEnvTrimmed("MERCHANT_SECRET_KEY")
	if !hasMerchantID && !hasAPIKey && !hasSecretKey {
		return nil
	}
	if !hasMerchantID || !hasAPIKey || !hasSecretKey {
		return errors.New("MERCHANT_ID, MERCHANT_API_KEY and MERCHANT_SECRET_KEY must be set together")
	}

	merchantName := getenv("MERCHANT_NAME", "Демонстрационный интернет-магазин")
	role := NormalizeRole(MerchantRole(getenv("MERCHANT_ROLE", string(RoleMerchant))))
	if role == "" {
		return errors.New("MERCHANT_ROLE must be one of: merchant, admin, auditor")
	}
	storedSecretKey, err := s.protector.Protect(ctx, secretKey)
	if err != nil {
		return fmt.Errorf("merchant secret protection failed: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
INSERT INTO merchants (merchant_id, name, api_key_hash, secret_key, role, active)
VALUES ($1, $2, $3, $4, $5, TRUE)
ON CONFLICT (merchant_id) DO UPDATE
SET
    name = EXCLUDED.name,
    api_key_hash = EXCLUDED.api_key_hash,
    secret_key = EXCLUDED.secret_key,
    role = EXCLUDED.role,
    active = TRUE,
    updated_at = NOW()
`, merchantID, merchantName, sha256Hex(apiKey), storedSecretKey, string(role))
	return err
}

func (s *PostgresMerchantStore) GetByID(ctx context.Context, merchantID string) (Merchant, bool, error) {
	var merchant Merchant
	var storedSecretKey string
	err := s.pool.QueryRow(ctx, `
SELECT merchant_id, name, api_key_hash, secret_key, role, active
FROM merchants
WHERE merchant_id = $1
LIMIT 1
`, merchantID).Scan(
		&merchant.MerchantID,
		&merchant.Name,
		&merchant.APIKeyHash,
		&storedSecretKey,
		&merchant.Role,
		&merchant.Active,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Merchant{}, false, nil
		}
		return Merchant{}, false, err
	}
	secretKey, err := s.protector.Reveal(ctx, storedSecretKey)
	if err != nil {
		return Merchant{}, false, fmt.Errorf("merchant secret reveal failed: %w", err)
	}
	merchant.SecretKey = secretKey
	return merchant, true, nil
}

func getenv(key string, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func lookupEnvTrimmed(key string) (string, bool) {
	v, ok := os.LookupEnv(key)
	v = strings.TrimSpace(v)
	return v, ok && v != ""
}
