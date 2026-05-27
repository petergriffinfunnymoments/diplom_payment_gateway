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

SELECT
    payment_id,
    token_preview,
    payment_method,
    masked_value
FROM payment_tokens
ORDER BY id DESC
LIMIT 20;

