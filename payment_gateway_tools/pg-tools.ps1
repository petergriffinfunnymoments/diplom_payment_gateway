# ==============================
# PostgreSQL helper commands
# for payment gateway project
# ==============================

# Укажи свою строку подключения
$PSQL = "C:\Program Files\PostgreSQL\18\bin\psql.exe"
$env:PGURL = "postgres://postgres:886540@localhost:5432/payment_gateway?sslmode=disable"

function pgtables {
    psql $env:PGURL -c "\dt"
}

function pgtx {
    param(
        [int]$Limit = 10
    )

    psql $env:PGURL -c "
SELECT
    id,
    merchant_id,
    payment_id,
    idempotency_key,
    status,
    updated_at,
    created_at
FROM payment_transactions
ORDER BY updated_at DESC
LIMIT $Limit;
"
}

function pglogs {
    param(
        [int]$Limit = 20
    )

    psql $env:PGURL -c "
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
LIMIT $Limit;
"
}

function pgpay {
    param(
        [Parameter(Mandatory=$true)]
        [string]$PaymentId
    )

    psql $env:PGURL -c "
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
WHERE le.payment_id = '$PaymentId'
ORDER BY le.timestamp;
"
}

function pgctx {
    param(
        [Parameter(Mandatory=$true)]
        [string]$PaymentId
    )

    psql $env:PGURL -c "
SELECT
    le.timestamp,
    e.code AS event,
    lc.key,
    lc.value
FROM log_entries le
JOIN log_events e ON e.id = le.event_id
LEFT JOIN log_context lc ON lc.log_entry_id = le.id
WHERE le.payment_id = '$PaymentId'
ORDER BY le.timestamp, lc.key;
"
}

function pgerrors {
    param(
        [int]$Limit = 20
    )

    psql $env:PGURL -c "
SELECT
    le.timestamp,
    s.code AS service,
    e.code AS event,
    le.payment_id,
    le.current_status,
    le.message
FROM log_entries le
JOIN services s ON s.id = le.service_id
JOIN log_events e ON e.id = le.event_id
WHERE le.level = 'ERROR'
ORDER BY le.timestamp DESC
LIMIT $Limit;
"
}

function pgstatus {
    psql $env:PGURL -c "
SELECT
    status,
    COUNT(*) AS count
FROM payment_transactions
GROUP BY status
ORDER BY count DESC;
"
}

function pglogtypes {
    psql $env:PGURL -c "
SELECT
    e.code AS event,
    COUNT(*) AS count
FROM log_entries le
JOIN log_events e ON e.id = le.event_id
GROUP BY e.code
ORDER BY count DESC;
"
}

function pgidem {
    param(
        [Parameter(Mandatory=$true)]
        [string]$IdempotencyKey
    )

    psql $env:PGURL -c "
SELECT
    id,
    merchant_id,
    payment_id,
    idempotency_key,
    status,
    payload_json,
    updated_at,
    created_at
FROM payment_transactions
WHERE idempotency_key = '$IdempotencyKey'
ORDER BY updated_at DESC;
"
}
function pgtokens {
    param(
        [int]$Limit = 20
    )

    & $PSQL $env:PGURL -c "
SELECT
    id,
    merchant_id,
    payment_id,
    idempotency_key,
    token_preview,
    payment_method,
    masked_value,
    expires_at,
    created_at
FROM payment_tokens
ORDER BY id DESC
LIMIT $Limit;
"
}

function pgtokenpay {
    param(
        [Parameter(Mandatory=$true)]
        [string]$PaymentId
    )

    & $PSQL $env:PGURL -c "
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
WHERE payment_id = '$PaymentId'
ORDER BY id DESC;
"
}

function pgnotifications {
    param(
        [int]$Limit = 20
    )

    & $PSQL $env:PGURL -c "
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
LIMIT $Limit;
"
}

function pglastpaylogs {
    & $PSQL $env:PGURL -c "
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
"
}


function pgpayment {
    param(
        [Parameter(Mandatory=$true)]
        [string]$PaymentId,
        [string]$MerchantId = "merchant_12345"
    )

    & $PSQL $env:PGURL -c "
SELECT
    merchant_id,
    payment_id,
    idempotency_key,
    status,
    payload_json,
    updated_at,
    created_at
FROM payment_transactions
WHERE merchant_id = '$MerchantId'
  AND payment_id = '$PaymentId'
ORDER BY updated_at DESC
LIMIT 1;
"
}
