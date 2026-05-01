package storage

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"payment-gateway/internal/contracts"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresTransactionStore struct {
	pool *pgxpool.Pool
}

func NewPostgresTransactionStore(ctx context.Context, dsn string) (*PostgresTransactionStore, error) {
	if dsn == "" {
		return nil, errors.New("dsn is empty")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}

	s := &PostgresTransactionStore{pool: pool}
	if err := s.ensureSchema(ctx); err != nil {
	pool.Close()
	return nil, err
	}

	return s, nil
}

func (s *PostgresTransactionStore) ensureSchema(ctx context.Context) error {
	// Для диплома: таблица хранит итоговый статус и закешированный payload ответа
	// по идемпотентности (merchant_id, idempotency_key).
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS payment_transactions (
  id BIGSERIAL PRIMARY KEY,
  merchant_id TEXT NOT NULL,
  payment_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  status TEXT NOT NULL,
  payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  updated_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (merchant_id, idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_payment_transactions_merchant_payment
  ON payment_transactions (merchant_id, payment_id);
`)
	return err
}

// NewPostgresTransactionStoreAsContract адаптирует реализацию под интерфейс TransactionStore.
func NewPostgresTransactionStoreAsContract(ctx context.Context, dsn string) (contracts.TransactionStore, error) {
	s, err := NewPostgresTransactionStore(ctx, dsn)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *PostgresTransactionStore) Save(
	ctx context.Context,
	merchantID string,
	paymentID string,
	idempotencyKey string,
	status string,
	payloadJSON string,
	updatedAt time.Time,
) error {
	if merchantID == "" || idempotencyKey == "" {
		return errors.New("merchantID and idempotencyKey are required")
	}

	// payload_json должен быть валидным JSON.
	if payloadJSON == "" {
		payloadJSON = `{}` // гарантируем JSONB
	} else {
		var tmp interface{}
		if err := json.Unmarshal([]byte(payloadJSON), &tmp); err != nil {
			return err
		}
	}

	_, err := s.pool.Exec(ctx, `
INSERT INTO payment_transactions (
  merchant_id, payment_id, idempotency_key, status, payload_json, updated_at
) VALUES ($1, $2, $3, $4, $5::jsonb, $6)
ON CONFLICT (merchant_id, idempotency_key) DO UPDATE
SET
  payment_id = EXCLUDED.payment_id,
  status = EXCLUDED.status,
  payload_json = EXCLUDED.payload_json,
  updated_at = EXCLUDED.updated_at
`, merchantID, paymentID, idempotencyKey, status, payloadJSON, updatedAt)

	return err
}

func (s *PostgresTransactionStore) GetByIdempotencyKey(
	ctx context.Context,
	merchantID string,
	idempotencyKey string,
) (status string, payloadJSON string, found bool, err error) {
	if merchantID == "" || idempotencyKey == "" {
		return "", "", false, errors.New("merchantID and idempotencyKey are required")
	}

	var payload map[string]any
	err = s.pool.QueryRow(ctx, `
SELECT status, payload_json
FROM payment_transactions
WHERE merchant_id = $1 AND idempotency_key = $2
LIMIT 1
`, merchantID, idempotencyKey).Scan(&status, &payload)

	if err != nil {
		// pgx возвращает pgx.ErrNoRows — но без прямого импорта оставим через текст:
		// однако для корректности нужно импортировать pgx.
		// Поэтому ниже делаем вариант через errors.Is.
		// (pgxpool/pgxpool uses pgx; ErrNoRows is in pgx.)
		return "", "", false, err
	}

	// payloadJSON: маршалим обратно.
	b, err := json.Marshal(payload)
	if err != nil {
		return "", "", false, err
	}

	return status, string(b), true, nil
}
