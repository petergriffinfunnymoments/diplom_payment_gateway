SELECT
    merchant_id,
    name,
    left(api_key_hash, 12) || '...' AS api_key_hash_preview,
    active,
    created_at,
    updated_at
FROM merchants
ORDER BY created_at DESC;

SELECT
    merchant_id,
    active
FROM merchants
WHERE merchant_id = 'merchant_12345';
