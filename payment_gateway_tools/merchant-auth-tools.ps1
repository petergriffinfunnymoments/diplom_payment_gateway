# Подключать после pg-tools.ps1:
# . .\payment_gateway_tools\pg-tools.ps1
# . .\payment_gateway_tools\merchant-auth-tools.ps1

function pgmerchants {
    & $PSQL $env:PGURL -c "
SELECT
    merchant_id,
    name,
    left(api_key_hash, 12) || '...' AS api_key_hash_preview,
    active,
    created_at,
    updated_at
FROM merchants
ORDER BY created_at DESC;
"
}
