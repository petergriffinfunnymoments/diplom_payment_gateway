package logging

import (
	"context"
	"errors"
	"time"

	"payment-gateway/internal/contracts"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresEventLogger struct {
	pool        *pgxpool.Pool
	environment string
}

func NewPostgresEventLogger(ctx context.Context, dsn string, environment string) (contracts.EventLogger, error) {
	if dsn == "" {
		return nil, errors.New("dsn is empty")
	}
	if environment == "" {
		environment = "dev"
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}

	logger := &PostgresEventLogger{pool: pool, environment: environment}
	if err := logger.ensureSchema(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	if err := logger.seedDictionaries(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return logger, nil
}

func (l *PostgresEventLogger) ensureSchema(ctx context.Context) error {
	_, err := l.pool.Exec(ctx, `
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS services (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(100) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    description TEXT
);

CREATE TABLE IF NOT EXISTS log_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(100) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    description TEXT
);

CREATE TABLE IF NOT EXISTS log_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    level VARCHAR(20) NOT NULL,
    message TEXT NOT NULL,
    environment VARCHAR(50) NOT NULL DEFAULT 'dev',
    correlation_id TEXT,

    merchant_id VARCHAR(255),
    payment_id VARCHAR(255),
    idempotency_key VARCHAR(255),
    current_status VARCHAR(50),

    service_id UUID NOT NULL REFERENCES services(id),
    event_id UUID NOT NULL REFERENCES log_events(id)
);

CREATE TABLE IF NOT EXISTS log_context (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    log_entry_id UUID NOT NULL REFERENCES log_entries(id) ON DELETE CASCADE,
    key VARCHAR(100) NOT NULL,
    value TEXT
);

CREATE INDEX IF NOT EXISTS idx_log_entries_correlation_id
    ON log_entries(correlation_id);

CREATE INDEX IF NOT EXISTS idx_log_entries_payment_id
    ON log_entries(payment_id);

CREATE INDEX IF NOT EXISTS idx_log_entries_idempotency_key
    ON log_entries(idempotency_key);

CREATE INDEX IF NOT EXISTS idx_log_entries_timestamp
    ON log_entries(timestamp);

CREATE INDEX IF NOT EXISTS idx_log_entries_service_id
    ON log_entries(service_id);

CREATE INDEX IF NOT EXISTS idx_log_entries_event_id
    ON log_entries(event_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_log_context_unique_key
    ON log_context(log_entry_id, key);
`)
	return err
}

func (l *PostgresEventLogger) seedDictionaries(ctx context.Context) error {
	_, err := l.pool.Exec(ctx, `
INSERT INTO services (code, name, description) VALUES
('api_gateway', 'API-шлюз', 'Принимает HTTP-запросы от интернет-магазинов'),
('validator', 'Модуль валидации', 'Проверяет корректность входных платежных данных'),
('orchestrator', 'Оркестратор платежей', 'Управляет жизненным циклом платежной транзакции'),
('antifraud', 'Антифрод', 'Проверяет транзакции на признаки мошенничества'),
('tokenizer', 'Модуль токенизации', 'Преобразует платежные данные в токен'),
('adapter', 'Адаптер платежной системы', 'Взаимодействует с внешней платежной системой'),
('notifications', 'Сервис уведомлений', 'Формирует уведомления о результате платежа'),
('logging', 'Модуль логирования', 'Сохраняет события обработки платежей'),
('database', 'База данных', 'Хранит транзакции и события системы')
ON CONFLICT (code) DO NOTHING;

INSERT INTO log_events (code, name, description) VALUES
('payment_received', 'Платеж получен', 'Платежный запрос получен системой'),
('payment_validated', 'Платеж валидирован', 'Входные платежные данные успешно прошли проверку'),
('fraud_checked', 'Антифрод-проверка выполнена', 'Транзакция прошла проверку антифродом'),
('tokenized', 'Данные токенизированы', 'Платежные данные преобразованы в токен'),
('adapter_called', 'Адаптер вызван', 'Запрос передан в адаптер платежной системы'),
('adapter_result_received', 'Ответ адаптера получен', 'Получен ответ от адаптера платежной системы'),
('payment_response_sent', 'Ответ отправлен', 'Ответ платежного шлюза сформирован для интернет-магазина'),
('payment_failed', 'Платеж завершился ошибкой', 'Во время обработки платежа произошла ошибка'),
('notification_sent', 'Уведомление отправлено', 'Сервис уведомлений отправил webhook интернет-магазину'),
('notification_failed', 'Ошибка уведомления', 'Сервис уведомлений не смог доставить webhook интернет-магазину'),
('merchant_webhook_received', 'Webhook магазина получен', 'Демонстрационный интернет-магазин получил уведомление от платежного шлюза'),
('authorization_failed', 'Отказ в доступе', 'Запрос отклонён из-за недостаточных прав или нарушения границ мерчанта')
ON CONFLICT (code) DO NOTHING;
`)
	return err
}

func (l *PostgresEventLogger) Log(ctx context.Context, event contracts.PaymentEvent) error {
	if event.Type == "" {
		return errors.New("event type is required")
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.Level == "" {
		event.Level = contracts.LogLevelInfo
	}
	if event.Service == "" {
		event.Service = "orchestrator"
	}
	if event.Message == "" {
		event.Message = event.Details
	}
	if event.Message == "" {
		event.Message = string(event.Type)
	}
	if event.CorrelationID == "" {
		event.CorrelationID = event.PaymentID
	}
	event.Message = MaskSensitive(event.Message)
	event.Details = MaskSensitive(event.Details)
	for k, v := range event.Context {
		event.Context[k] = MaskSensitive(v)
	}

	// На всякий случай создаём неизвестные service/event, чтобы логирование не ломало платежный процесс
	// при появлении нового типа события.
	if err := l.ensureDictionaryValues(ctx, event); err != nil {
		return err
	}

	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var logEntryID string
	err = tx.QueryRow(ctx, `
INSERT INTO log_entries (
    timestamp,
    level,
    message,
    environment,
    correlation_id,
    merchant_id,
    payment_id,
    idempotency_key,
    current_status,
    service_id,
    event_id
) VALUES (
    $1,
    $2,
    $3,
    $4,
    NULLIF($5, ''),
    $6,
    $7,
    $8,
    $9,
    (SELECT id FROM services WHERE code = $10),
    (SELECT id FROM log_events WHERE code = $11)
)
RETURNING id::text
`,
		event.Timestamp,
		string(event.Level),
		event.Message,
		l.environment,
		event.CorrelationID,
		event.MerchantID,
		event.PaymentID,
		event.IdempotencyKey,
		event.CurrentStatus,
		event.Service,
		string(event.Type),
	).Scan(&logEntryID)
	if err != nil {
		return err
	}

	for k, v := range event.Context {
		if k == "" {
			continue
		}
		_, err = tx.Exec(ctx, `
INSERT INTO log_context (log_entry_id, key, value)
VALUES ($1::uuid, $2, $3)
ON CONFLICT (log_entry_id, key) DO UPDATE
SET value = EXCLUDED.value
`, logEntryID, k, v)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (l *PostgresEventLogger) ensureDictionaryValues(ctx context.Context, event contracts.PaymentEvent) error {
	_, err := l.pool.Exec(ctx, `
INSERT INTO services (code, name, description)
VALUES ($1, $1, 'Автоматически добавленный сервис')
ON CONFLICT (code) DO NOTHING
`, event.Service)
	if err != nil {
		return err
	}

	_, err = l.pool.Exec(ctx, `
INSERT INTO log_events (code, name, description)
VALUES ($1, $1, 'Автоматически добавленный тип события')
ON CONFLICT (code) DO NOTHING
`, string(event.Type))
	return err
}
