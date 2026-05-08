package storage

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"

	"github.com/jackc/pgx/v5"
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

CREATE TABLE IF NOT EXISTS payment_refunds (
  id BIGSERIAL PRIMARY KEY,
  merchant_id TEXT NOT NULL,
  refund_id TEXT NOT NULL,
  payment_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  status TEXT NOT NULL,
  amount NUMERIC(18,2) NOT NULL,
  currency TEXT NOT NULL,
  entity_type TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  refund_type TEXT NOT NULL,
  provider TEXT,
  payment_system TEXT,
  external_refund_id TEXT,
  provider_status TEXT,
  provider_error_code TEXT,
  provider_error_message TEXT,
  reason TEXT,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  UNIQUE (merchant_id, refund_id),
  UNIQUE (merchant_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_payment_refunds_merchant_payment
  ON payment_refunds (merchant_id, payment_id);
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

	if payloadJSON == "" {
		payloadJSON = `{}`
	} else {
		payloadJSON = dto.SanitizePaymentPayloadJSON(payloadJSON)
		var tmp interface{}
		if err := json.Unmarshal([]byte(payloadJSON), &tmp); err != nil {
			return err
		}
	}

	_, err := s.pool.Exec(ctx, `
INSERT INTO payment_transactions (
  merchant_id,
  payment_id,
  idempotency_key,
  status,
  payload_json,
  updated_at
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
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", false, nil
		}

		return "", "", false, err
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return "", "", false, err
	}

	return status, string(b), true, nil
}

func (s *PostgresTransactionStore) SaveRefund(ctx context.Context, refund dto.Refund) error {
	if refund.MerchantID == "" || refund.ID == "" || refund.PaymentID == "" || refund.IdempotencyKey == "" {
		return errors.New("merchantID, refundID, paymentID and idempotencyKey are required")
	}

	_, err := s.pool.Exec(ctx, `
INSERT INTO payment_refunds (
  merchant_id,
  refund_id,
  payment_id,
  idempotency_key,
  status,
  amount,
  currency,
  entity_type,
  entity_id,
  refund_type,
  provider,
  payment_system,
  external_refund_id,
  provider_status,
  provider_error_code,
  provider_error_message,
  reason,
  created_at,
  updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
  $11, $12, $13, $14, $15, $16, $17, $18, $19
)
ON CONFLICT (merchant_id, refund_id) DO UPDATE
SET
  payment_id = EXCLUDED.payment_id,
  idempotency_key = EXCLUDED.idempotency_key,
  status = EXCLUDED.status,
  amount = EXCLUDED.amount,
  currency = EXCLUDED.currency,
  entity_type = EXCLUDED.entity_type,
  entity_id = EXCLUDED.entity_id,
  refund_type = EXCLUDED.refund_type,
  provider = EXCLUDED.provider,
  payment_system = EXCLUDED.payment_system,
  external_refund_id = EXCLUDED.external_refund_id,
  provider_status = EXCLUDED.provider_status,
  provider_error_code = EXCLUDED.provider_error_code,
  provider_error_message = EXCLUDED.provider_error_message,
  reason = EXCLUDED.reason,
  updated_at = EXCLUDED.updated_at
`,
		refund.MerchantID,
		refund.ID,
		refund.PaymentID,
		refund.IdempotencyKey,
		refund.Status,
		refund.Amount,
		string(refund.Currency),
		refund.EntityType,
		refund.EntityID,
		refund.RefundType,
		refund.Provider,
		refund.PaymentSystem,
		refund.ExternalRefundID,
		refund.ProviderStatus,
		refund.ProviderErrorCode,
		refund.ProviderErrorMsg,
		refund.Reason,
		refund.CreatedAt,
		refund.UpdatedAt,
	)
	return err
}

func (s *PostgresTransactionStore) GetRefundByID(ctx context.Context, merchantID string, refundID string) (dto.Refund, bool, error) {
	if merchantID == "" || refundID == "" {
		return dto.Refund{}, false, errors.New("merchantID and refundID are required")
	}
	return s.getRefund(ctx, `
SELECT merchant_id, refund_id, payment_id, idempotency_key, status, amount, currency,
       entity_type, entity_id, refund_type, COALESCE(provider, ''), COALESCE(payment_system, ''),
       COALESCE(external_refund_id, ''), COALESCE(provider_status, ''),
       COALESCE(provider_error_code, ''), COALESCE(provider_error_message, ''),
       COALESCE(reason, ''), created_at, updated_at
FROM payment_refunds
WHERE merchant_id = $1 AND refund_id = $2
LIMIT 1
`, merchantID, refundID)
}

func (s *PostgresTransactionStore) GetRefundByIdempotencyKey(ctx context.Context, merchantID string, idempotencyKey string) (dto.Refund, bool, error) {
	if merchantID == "" || idempotencyKey == "" {
		return dto.Refund{}, false, errors.New("merchantID and idempotencyKey are required")
	}
	return s.getRefund(ctx, `
SELECT merchant_id, refund_id, payment_id, idempotency_key, status, amount, currency,
       entity_type, entity_id, refund_type, COALESCE(provider, ''), COALESCE(payment_system, ''),
       COALESCE(external_refund_id, ''), COALESCE(provider_status, ''),
       COALESCE(provider_error_code, ''), COALESCE(provider_error_message, ''),
       COALESCE(reason, ''), created_at, updated_at
FROM payment_refunds
WHERE merchant_id = $1 AND idempotency_key = $2
LIMIT 1
`, merchantID, idempotencyKey)
}

func (s *PostgresTransactionStore) ListRefundsByPaymentID(ctx context.Context, merchantID string, paymentID string) ([]dto.Refund, error) {
	if merchantID == "" {
		return nil, errors.New("merchantID is required")
	}

	rows, err := s.pool.Query(ctx, `
SELECT merchant_id, refund_id, payment_id, idempotency_key, status, amount, currency,
       entity_type, entity_id, refund_type, COALESCE(provider, ''), COALESCE(payment_system, ''),
       COALESCE(external_refund_id, ''), COALESCE(provider_status, ''),
       COALESCE(provider_error_code, ''), COALESCE(provider_error_message, ''),
       COALESCE(reason, ''), created_at, updated_at
FROM payment_refunds
WHERE merchant_id = $1 AND ($2 = '' OR payment_id = $2)
ORDER BY created_at DESC
`, merchantID, paymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	refunds := make([]dto.Refund, 0)
	for rows.Next() {
		refund, err := scanRefund(rows)
		if err != nil {
			return nil, err
		}
		refunds = append(refunds, refund)
	}
	return refunds, rows.Err()
}

func (s *PostgresTransactionStore) getRefund(ctx context.Context, query string, args ...any) (dto.Refund, bool, error) {
	refund, err := scanRefund(s.pool.QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dto.Refund{}, false, nil
		}
		return dto.Refund{}, false, err
	}
	return refund, true, nil
}

type refundScanner interface {
	Scan(dest ...any) error
}

func scanRefund(row refundScanner) (dto.Refund, error) {
	var refund dto.Refund
	var currency string
	err := row.Scan(
		&refund.MerchantID,
		&refund.ID,
		&refund.PaymentID,
		&refund.IdempotencyKey,
		&refund.Status,
		&refund.Amount,
		&currency,
		&refund.EntityType,
		&refund.EntityID,
		&refund.RefundType,
		&refund.Provider,
		&refund.PaymentSystem,
		&refund.ExternalRefundID,
		&refund.ProviderStatus,
		&refund.ProviderErrorCode,
		&refund.ProviderErrorMsg,
		&refund.Reason,
		&refund.CreatedAt,
		&refund.UpdatedAt,
	)
	refund.Currency = dto.PaymentCurrency(currency)
	return refund, err
}
func (s *PostgresTransactionStore) GetByPaymentID(
	ctx context.Context,
	merchantID string,
	paymentID string,
) (status string, payloadJSON string, found bool, err error) {
	if merchantID == "" || paymentID == "" {
		return "", "", false, errors.New("merchantID and paymentID are required")
	}

	var payload map[string]any

	err = s.pool.QueryRow(ctx, `
SELECT status, payload_json
FROM payment_transactions
WHERE merchant_id = $1 AND payment_id = $2
ORDER BY updated_at DESC
LIMIT 1
`, merchantID, paymentID).Scan(&status, &payload)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", false, nil
		}

		return "", "", false, err
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return "", "", false, err
	}

	return status, string(b), true, nil
}
