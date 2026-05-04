# Подключение:
# . .\payment_gateway_tools\merchant-admin-tools.ps1
# Требует $env:DATABASE_URL и переменную $PSQL из pg-tools.ps1

function pgmerchants {
    param([int]$Limit = 20)

    & $PSQL $env:DATABASE_URL -c "
SELECT
    merchant_id,
    name,
    left(api_key_hash, 12) || '...' AS api_key_hash_preview,
    webhook_url,
    active,
    created_at,
    updated_at
FROM merchants
ORDER BY created_at DESC
LIMIT $Limit;
"
}

function pgmerchant {
    param(
        [Parameter(Mandatory=$true)]
        [string]$MerchantID
    )

    & $PSQL $env:DATABASE_URL -c "
SELECT
    merchant_id,
    name,
    left(api_key_hash, 12) || '...' AS api_key_hash_preview,
    webhook_url,
    active,
    created_at,
    updated_at
FROM merchants
WHERE merchant_id = '$MerchantID';
"
}

function pgmerchantdisable {
    param(
        [Parameter(Mandatory=$true)]
        [string]$MerchantID
    )

    & $PSQL $env:DATABASE_URL -c "
UPDATE merchants
SET active = FALSE,
    updated_at = NOW()
WHERE merchant_id = '$MerchantID';
"
}

function pgmerchantenable {
    param(
        [Parameter(Mandatory=$true)]
        [string]$MerchantID
    )

    & $PSQL $env:DATABASE_URL -c "
UPDATE merchants
SET active = TRUE,
    updated_at = NOW()
WHERE merchant_id = '$MerchantID';
"
}
