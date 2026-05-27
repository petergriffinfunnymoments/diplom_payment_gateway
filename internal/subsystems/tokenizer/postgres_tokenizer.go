package tokenizer

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"payment-gateway/internal/contracts"
	"payment-gateway/internal/dto"

	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultTokenTTL = 24 * time.Hour

type PostgresTokenizer struct {
	pool *pgxpool.Pool
	ttl  time.Duration
}

func NewPostgresTokenizer(ctx context.Context, dsn string) (contracts.Tokenizer, error) {
	if dsn == "" {
		return nil, errors.New("dsn is empty")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}

	t := &PostgresTokenizer{pool: pool, ttl: defaultTokenTTL}
	if err := t.ensureSchema(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return t, nil
}

func (t *PostgresTokenizer) ensureSchema(ctx context.Context) error {
	_, err := t.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS payment_tokens (
  id BIGSERIAL PRIMARY KEY,
  merchant_id TEXT NOT NULL,
  payment_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  token_preview TEXT NOT NULL,
  payment_method TEXT NOT NULL,
  masked_value TEXT,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  revoked_at TIMESTAMPTZ,
  UNIQUE (merchant_id, payment_id)
);

CREATE INDEX IF NOT EXISTS idx_payment_tokens_merchant_payment
  ON payment_tokens (merchant_id, payment_id);

CREATE INDEX IF NOT EXISTS idx_payment_tokens_idempotency
  ON payment_tokens (merchant_id, idempotency_key);

CREATE INDEX IF NOT EXISTS idx_payment_tokens_hash
  ON payment_tokens (token_hash);

CREATE INDEX IF NOT EXISTS idx_payment_tokens_expires_at
  ON payment_tokens (expires_at);
`)
	return err
}

func (t *PostgresTokenizer) Tokenize(ctx context.Context, req dto.CreatePaymentRequest) (string, error) {
	if req.MerchantID == "" || req.PaymentID == "" || req.IdempotencyKey == "" {
		return "", errors.New("merchant_id, payment_id and idempotency_key are required for tokenization")
	}

	token, err := generateToken()
	if err != nil {
		return "", err
	}

	tokenHash := hashToken(token)
	tokenPreview := TokenPreview(token)
	paymentMethod := string(req.PaymentInfo.PaymentMethodData.Type)
	maskedValue := maskedPaymentValue(req)
	expiresAt := time.Now().UTC().Add(t.ttl)

	_, err = t.pool.Exec(ctx, `
INSERT INTO payment_tokens (
  merchant_id,
  payment_id,
  idempotency_key,
  token_hash,
  token_preview,
  payment_method,
  masked_value,
  expires_at,
  created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
ON CONFLICT (merchant_id, payment_id) DO UPDATE
SET
  idempotency_key = EXCLUDED.idempotency_key,
  token_hash = EXCLUDED.token_hash,
  token_preview = EXCLUDED.token_preview,
  payment_method = EXCLUDED.payment_method,
  masked_value = EXCLUDED.masked_value,
  expires_at = EXCLUDED.expires_at,
  revoked_at = NULL
`,
		req.MerchantID,
		req.PaymentID,
		req.IdempotencyKey,
		tokenHash,
		tokenPreview,
		paymentMethod,
		maskedValue,
		expiresAt,
	)
	if err != nil {
		return "", err
	}

	return token, nil
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "tok_" + hex.EncodeToString(b), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func maskedPaymentValue(req dto.CreatePaymentRequest) string {
	customer := req.PaymentInfo.CustomerData
	switch req.PaymentInfo.PaymentMethodData.Type {
	case dto.PaymentMethodCard:
		return MaskCardNumber(customer.CardNumber)
	case dto.PaymentMethodSBP:
		return MaskPhone(customer.Phone)
	case dto.PaymentMethodDigitalWallet:
		return MaskWalletID(customer.DigitalWalletID)
	case dto.PaymentMethodDigitalRuble:
		return MaskWalletID(firstNonEmpty(customer.DigitalRubleWalletID, customer.DigitalRubleIdentifier, customer.DigitalWalletID))
	default:
		return ""
	}
}

func TokenPreview(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 12 {
		return token
	}
	return token[:8] + "..." + token[len(token)-4:]
}

func MaskCardNumber(card string) string {
	digits := onlyDigits(card)
	if len(digits) < 10 {
		return ""
	}
	return digits[:6] + "******" + digits[len(digits)-4:]
}

func MaskPhone(phone string) string {
	digits := onlyDigits(phone)
	if len(digits) < 5 {
		return ""
	}
	prefix := "+"
	if strings.HasPrefix(strings.TrimSpace(phone), "+") {
		prefix = "+"
	} else {
		prefix = ""
	}
	if len(digits) <= 4 {
		return prefix + digits
	}
	return prefix + digits[:4] + "******" + digits[len(digits)-2:]
}

func MaskWalletID(walletID string) string {
	walletID = strings.TrimSpace(walletID)
	if walletID == "" {
		return ""
	}
	if len(walletID) <= 4 {
		return walletID[:1] + "***"
	}
	return walletID[:3] + "***" + walletID[len(walletID)-2:]
}

func onlyDigits(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
