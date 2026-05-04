package merchantauth

import (
	"context"
	"errors"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresMerchantStore struct {
	pool *pgxpool.Pool
}

func NewPostgresMerchantStore(ctx context.Context, dsn string) (*PostgresMerchantStore, error) {
	if dsn == "" {
		return nil, errors.New("dsn is empty")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}

	store := &PostgresMerchantStore{pool: pool}
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
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_merchants_active
    ON merchants (merchant_id, active);
`)
	return err
}

func (s *PostgresMerchantStore) seedDefaultMerchant(ctx context.Context) error {
	merchantID := getenv("MERCHANT_ID", "merchant_12345")
	merchantName := getenv("MERCHANT_NAME", "Демонстрационный интернет-магазин")
	apiKey := getenv("MERCHANT_API_KEY", "demo_api_key")
	secretKey := getenv("MERCHANT_SECRET_KEY", "demo_secret_key")

	_, err := s.pool.Exec(ctx, `
INSERT INTO merchants (merchant_id, name, api_key_hash, secret_key, active)
VALUES ($1, $2, $3, $4, TRUE)
ON CONFLICT (merchant_id) DO UPDATE
SET
    name = EXCLUDED.name,
    api_key_hash = EXCLUDED.api_key_hash,
    secret_key = EXCLUDED.secret_key,
    active = TRUE,
    updated_at = NOW()
`, merchantID, merchantName, sha256Hex(apiKey), secretKey)
	return err
}

func (s *PostgresMerchantStore) GetByID(ctx context.Context, merchantID string) (Merchant, bool, error) {
	var merchant Merchant
	err := s.pool.QueryRow(ctx, `
SELECT merchant_id, name, api_key_hash, secret_key, active
FROM merchants
WHERE merchant_id = $1
LIMIT 1
`, merchantID).Scan(
		&merchant.MerchantID,
		&merchant.Name,
		&merchant.APIKeyHash,
		&merchant.SecretKey,
		&merchant.Active,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Merchant{}, false, nil
		}
		return Merchant{}, false, err
	}
	return merchant, true, nil
}

func getenv(key string, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
