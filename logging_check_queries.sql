-- Последние события логирования
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
ORDER BY le.timestamp DESC
LIMIT 50;

-- История одного платежа: замени pay_123 на свой payment_id
SELECT
    le.timestamp,
    le.level,
    s.code AS service,
    e.code AS event,
    le.current_status,
    le.message
FROM log_entries le
JOIN services s ON s.id = le.service_id
JOIN log_events e ON e.id = le.event_id
WHERE le.payment_id = 'pay_123'
ORDER BY le.timestamp;

-- Контекст событий одного платежа: замени pay_123 на свой payment_id
SELECT
    le.timestamp,
    e.code AS event,
    lc.key,
    lc.value
FROM log_entries le
JOIN log_events e ON e.id = le.event_id
LEFT JOIN log_context lc ON lc.log_entry_id = le.id
WHERE le.payment_id = 'pay_123'
ORDER BY le.timestamp, lc.key;

-- Все ошибки
SELECT
    le.timestamp,
    s.code AS service,
    e.code AS event,
    le.payment_id,
    le.message
FROM log_entries le
JOIN services s ON s.id = le.service_id
JOIN log_events e ON e.id = le.event_id
WHERE le.level = 'ERROR'
ORDER BY le.timestamp DESC;
