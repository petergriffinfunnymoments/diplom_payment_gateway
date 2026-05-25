# Служебные команды настройки маршрутизации платежей.
# Подключение:
# . .\payment_gateway_tools\pg-tools.ps1
# . .\payment_gateway_tools\payment-route-tools.ps1

function Get-PaymentGatewayDbUrl {
    if ($env:DATABASE_URL -and $env:DATABASE_URL.Trim() -ne "") {
        return $env:DATABASE_URL
    }
    return $env:PGURL
}

function Invoke-PaymentGatewayRouteSql {
    param(
        [Parameter(Mandatory=$true)]
        [string]$Sql
    )

    $dbUrl = Get-PaymentGatewayDbUrl
    if (-not $dbUrl) {
        throw "DATABASE_URL или PGURL не задан. Подключи pg-tools.ps1 или задай DATABASE_URL."
    }

    & $PSQL $dbUrl -c $Sql
}

function pgrouteinit {
    Invoke-PaymentGatewayRouteSql "
CREATE TABLE IF NOT EXISTS merchant_payment_routes (
  id BIGSERIAL PRIMARY KEY,
  merchant_id TEXT NOT NULL,
  payment_method TEXT NOT NULL,
  provider TEXT NOT NULL,
  payment_system TEXT NOT NULL,
  priority INTEGER NOT NULL DEFAULT 100,
  active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (merchant_id, payment_method, provider)
);

CREATE INDEX IF NOT EXISTS idx_merchant_payment_routes_lookup
  ON merchant_payment_routes (merchant_id, payment_method, active, priority);

CREATE INDEX IF NOT EXISTS idx_merchant_payment_routes_provider
  ON merchant_payment_routes (provider);
"
}

function pgrouteadd {
    param(
        [Parameter(Mandatory=$true)]
        [string]$MerchantID,

        [Parameter(Mandatory=$true)]
        [string]$PaymentMethod,

        [Parameter(Mandatory=$true)]
        [ValidateSet("yookassa", "robokassa", "payanyway", "simulated", "digital_ruble")]
        [string]$Provider,

        [int]$Priority = 1,

        [string]$PaymentSystem = ""
    )

    if ($PaymentSystem.Trim() -eq "") {
        $PaymentSystem = $Provider.ToUpperInvariant()
    }

    pgrouteinit

    $merchantIDSql = $MerchantID.Replace("'", "''")
    $paymentMethodSql = $PaymentMethod.Replace("'", "''")
    $providerSql = $Provider.ToLowerInvariant().Replace("'", "''")
    $paymentSystemSql = $PaymentSystem.ToUpperInvariant().Replace("'", "''")

    Invoke-PaymentGatewayRouteSql "
INSERT INTO merchant_payment_routes (
  merchant_id,
  payment_method,
  provider,
  payment_system,
  priority,
  active,
  created_at,
  updated_at
) VALUES (
  '$merchantIDSql',
  '$paymentMethodSql',
  '$providerSql',
  '$paymentSystemSql',
  $Priority,
  TRUE,
  NOW(),
  NOW()
)
ON CONFLICT (merchant_id, payment_method, provider) DO UPDATE
SET
  payment_system = EXCLUDED.payment_system,
  priority = EXCLUDED.priority,
  active = TRUE,
  updated_at = NOW();

SELECT
  id,
  merchant_id,
  payment_method,
  provider,
  payment_system,
  priority,
  active,
  updated_at
FROM merchant_payment_routes
WHERE merchant_id = '$merchantIDSql'
ORDER BY payment_method, priority, id;
"
}

function pgroutes {
    param([int]$Limit = 50)

    Invoke-PaymentGatewayRouteSql "
SELECT
  id,
  merchant_id,
  payment_method,
  provider,
  payment_system,
  priority,
  active,
  updated_at
FROM merchant_payment_routes
ORDER BY merchant_id, payment_method, priority, id
LIMIT $Limit;
"
}

function pgmerchant_routes {
    param(
        [Parameter(Mandatory=$true)]
        [string]$MerchantID
    )

    $merchantIDSql = $MerchantID.Replace("'", "''")
    Invoke-PaymentGatewayRouteSql "
SELECT
  id,
  merchant_id,
  payment_method,
  provider,
  payment_system,
  priority,
  active,
  updated_at
FROM merchant_payment_routes
WHERE merchant_id = '$merchantIDSql'
ORDER BY payment_method, priority, id;
"
}

function pgroutedisable {
    param(
        [Parameter(Mandatory=$true)]
        [string]$MerchantID,

        [Parameter(Mandatory=$true)]
        [string]$PaymentMethod,

        [Parameter(Mandatory=$true)]
        [string]$Provider
    )

    $merchantIDSql = $MerchantID.Replace("'", "''")
    $paymentMethodSql = $PaymentMethod.Replace("'", "''")
    $providerSql = $Provider.ToLowerInvariant().Replace("'", "''")

    Invoke-PaymentGatewayRouteSql "
UPDATE merchant_payment_routes
SET active = FALSE,
    updated_at = NOW()
WHERE merchant_id = '$merchantIDSql'
  AND payment_method = '$paymentMethodSql'
  AND provider = '$providerSql';
"
}

function pgroutedelete {
    param(
        [Parameter(Mandatory=$true)]
        [string]$MerchantID,

        [Parameter(Mandatory=$true)]
        [string]$PaymentMethod,

        [Parameter(Mandatory=$true)]
        [string]$Provider
    )

    $confirm = Read-Host "Удалить маршрут '$MerchantID' / '$PaymentMethod' / '$Provider'? Введите YES"
    if ($confirm -ne "YES") {
        Write-Host "Удаление отменено."
        return
    }

    $merchantIDSql = $MerchantID.Replace("'", "''")
    $paymentMethodSql = $PaymentMethod.Replace("'", "''")
    $providerSql = $Provider.ToLowerInvariant().Replace("'", "''")

    Invoke-PaymentGatewayRouteSql "
DELETE FROM merchant_payment_routes
WHERE merchant_id = '$merchantIDSql'
  AND payment_method = '$paymentMethodSql'
  AND provider = '$providerSql';
"
}
