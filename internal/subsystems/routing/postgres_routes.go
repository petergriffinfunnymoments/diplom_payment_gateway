package routing

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresPaymentRouteStore struct {
	pool *pgxpool.Pool
}

func NewPostgresPaymentRouteStore(ctx context.Context, dsn string) (*PostgresPaymentRouteStore, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("dsn is empty")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}

	s := &PostgresPaymentRouteStore{pool: pool}
	if err := s.ensureSchema(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return s, nil
}

func NewPostgresPaymentRouteStoreAsContract(ctx context.Context, dsn string) (contracts.PaymentRouteStore, error) {
	return NewPostgresPaymentRouteStore(ctx, dsn)
}

func (s *PostgresPaymentRouteStore) ensureSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS merchant_payment_routes (
  id BIGSERIAL PRIMARY KEY,
  merchant_id TEXT NOT NULL,
  payment_method TEXT NOT NULL,
  provider TEXT NOT NULL,
  payment_system TEXT NOT NULL,
  priority INTEGER NOT NULL DEFAULT 100,
  active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (merchant_id, payment_method, provider)
);

CREATE INDEX IF NOT EXISTS idx_merchant_payment_routes_lookup
  ON merchant_payment_routes (merchant_id, payment_method, active, priority);

CREATE INDEX IF NOT EXISTS idx_merchant_payment_routes_provider
  ON merchant_payment_routes (provider);
`)
	return err
}

func (s *PostgresPaymentRouteStore) GetActiveRoute(
	ctx context.Context,
	merchantID string,
	paymentMethod dto.PaymentMethodType,
) (route contracts.PaymentRoute, found bool, err error) {
	merchantID = strings.TrimSpace(merchantID)
	if merchantID == "" || paymentMethod == "" {
		return contracts.PaymentRoute{}, false, fmt.Errorf("merchant_id and payment_method are required")
	}

	err = s.pool.QueryRow(ctx, `
SELECT merchant_id, payment_method, provider, payment_system, priority
FROM merchant_payment_routes
WHERE merchant_id = $1
  AND payment_method = $2
  AND active = TRUE
ORDER BY priority ASC, id ASC
LIMIT 1
`, merchantID, string(paymentMethod)).Scan(
		&route.MerchantID,
		&route.PaymentMethod,
		&route.Provider,
		&route.PaymentSystem,
		&route.Priority,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return contracts.PaymentRoute{}, false, nil
		}
		return contracts.PaymentRoute{}, false, err
	}

	route.Provider = normalizeProvider(route.Provider)
	route.PaymentSystem = normalizePaymentSystem(route.PaymentSystem, route.Provider)
	return route, true, nil
}

func normalizeProvider(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func normalizePaymentSystem(system string, provider string) string {
	system = strings.ToUpper(strings.TrimSpace(system))
	if system != "" {
		return system
	}
	provider = normalizeProvider(provider)
	if provider == "" {
		return ""
	}
	return strings.ToUpper(provider)
}
