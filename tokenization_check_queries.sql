-- Последние токены платежей
SELECT
    id,
    merchant_id,
    payment_id,
    idempotency_key,
    token_preview,
    payment_method,
    masked_value,
    expires_at,
    created_at,
    revoked_at
FROM payment_tokens
ORDER BY id DESC
LIMIT 50;

-- Токен конкретного платежа
SELECT
    id,
    merchant_id,
    payment_id,
    idempotency_key,
    token_preview,
    payment_method,
    masked_value,
    expires_at,
    created_at,
    revoked_at
FROM payment_tokens
WHERE payment_id = 'pay_ТВОЙ_ID'
ORDER BY id DESC;

-- Проверка, что CVV и полный номер карты не сохраняются
SELECT
    payment_id,
    token_preview,
    payment_method,
    masked_value
FROM payment_tokens
ORDER BY id DESC
LIMIT 20;

-- Очистка токенов для тестов, если понадобится
-- TRUNCATE TABLE payment_tokens RESTART IDENTITY CASCADE;
