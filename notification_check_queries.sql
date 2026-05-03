-- Последние доставки уведомлений интернет-магазину
SELECT
    created_at,
    delivery_id,
    merchant_id,
    payment_id,
    event_type,
    status_code,
    success,
    error_message
FROM notification_deliveries
ORDER BY created_at DESC
LIMIT 30;

-- Логи сервиса уведомлений
SELECT
    le.timestamp,
    le.level,
    s.code AS service,
    e.code AS event,
    le.current_status,
    le.payment_id,
    le.message
FROM log_entries le
JOIN services s ON s.id = le.service_id
JOIN log_events e ON e.id = le.event_id
WHERE s.code = 'notifications'
ORDER BY le.timestamp DESC
LIMIT 30;

-- История последней транзакции, включая уведомления
SELECT
    le.timestamp,
    le.level,
    s.code AS service,
    e.code AS event,
    le.current_status,
    le.payment_id,
    le.message
FROM log_entries le
JOIN services s ON s.id = le.service_id
JOIN log_events e ON e.id = le.event_id
WHERE le.payment_id = (
    SELECT payment_id
    FROM payment_transactions
    ORDER BY updated_at DESC
    LIMIT 1
)
ORDER BY le.timestamp;
