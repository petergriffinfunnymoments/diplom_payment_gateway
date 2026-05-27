param(
    [Parameter(Mandatory=$true)]
    [string]$MerchantID,

    [Parameter(Mandatory=$true)]
    [string]$Name,

    [Parameter(Mandatory=$false)]
    [string]$WebhookURL = "",

    [Parameter(Mandatory=$false)]
    [ValidateSet("merchant", "admin", "auditor")]
    [string]$Role = "merchant",

    [Parameter(Mandatory=$false)]
    [string]$DatabaseURL = $env:DATABASE_URL,

    [Parameter(Mandatory=$false)]
    [string]$PsqlPath = $env:PSQL,

    [switch]$RotateKeys
)

$ErrorActionPreference = "Stop"

function Find-Psql {
    param([string]$Candidate)

    if ($Candidate -and (Test-Path $Candidate)) {
        return $Candidate
    }

    $cmd = Get-Command psql -ErrorAction SilentlyContinue
    if ($cmd) {
        return $cmd.Source
    }

    $found = Get-ChildItem "C:\Program Files\PostgreSQL" -Recurse -Filter psql.exe -ErrorAction SilentlyContinue |
        Sort-Object FullName -Descending |
        Select-Object -First 1

    if ($found) {
        return $found.FullName
    }

    throw "psql.exe не найден. Укажи путь через -PsqlPath или добавь PostgreSQL bin в PATH."
}

function Sql-Escape {
    param([string]$Value)
    if ($null -eq $Value) { return "" }
    return $Value.Replace("'", "''")
}

function New-SecretHex {
    param([int]$Bytes = 32)

    $buffer = New-Object byte[] $Bytes

    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        $rng.GetBytes($buffer)
    }
    finally {
        if ($null -ne $rng) {
            $rng.Dispose()
        }
    }

    return ([System.BitConverter]::ToString($buffer)).Replace("-", "").ToLowerInvariant()
}

function Get-Sha256Hex {
    param([string]$Value)
    $sha = [System.Security.Cryptography.SHA256]::Create()
    try {
        $bytes = [System.Text.Encoding]::UTF8.GetBytes($Value)
        $hash = $sha.ComputeHash($bytes)
        return ([System.BitConverter]::ToString($hash)).Replace("-", "").ToLowerInvariant()
    } finally {
        $sha.Dispose()
    }
}

function Get-FirstNonEmpty {
    param([string[]]$Values)
    foreach ($value in $Values) {
        if (-not [string]::IsNullOrWhiteSpace($value)) {
            return $value
        }
    }
    return ""
}

function Get-VaultToken {
    $tokenFile = $env:VAULT_TOKEN_FILE
    if (-not [string]::IsNullOrWhiteSpace($tokenFile)) {
        if (-not (Test-Path $tokenFile)) {
            throw "VAULT_TOKEN_FILE указывает на несуществующий файл: $tokenFile"
        }
        $token = (Get-Content -Path $tokenFile -Raw).Trim()
        if ([string]::IsNullOrWhiteSpace($token)) {
            throw "VAULT_TOKEN_FILE пустой."
        }
        return $token
    }

    if (-not [string]::IsNullOrWhiteSpace($env:VAULT_TOKEN)) {
        return $env:VAULT_TOKEN.Trim()
    }

    throw "Для Vault нужен VAULT_TOKEN или VAULT_TOKEN_FILE."
}

function Protect-MerchantSecret {
    param([string]$SecretKey)

    $provider = (Get-FirstNonEmpty @($env:SECRET_PROTECTOR, $env:SECRET_PROTECTOR_PROVIDER)).Trim().ToLowerInvariant()
    if ([string]::IsNullOrWhiteSpace($provider) -or @("none", "noop", "plain", "plaintext") -contains $provider) {
        return $SecretKey
    }

    if (@("vault", "vault_transit", "vault-transit", "hashicorp_vault", "hashicorp-vault") -notcontains $provider) {
        throw "Неподдерживаемое значение SECRET_PROTECTOR: $provider"
    }

    $vaultAddr = $env:VAULT_ADDR
    if ([string]::IsNullOrWhiteSpace($vaultAddr)) {
        $vaultAddr = "http://127.0.0.1:8200"
    }
    $vaultAddr = $vaultAddr.TrimEnd("/")

    $mount = $env:VAULT_TRANSIT_MOUNT
    if ([string]::IsNullOrWhiteSpace($mount)) {
        $mount = "transit"
    }
    $mount = $mount.Trim("/")

    $keyName = $env:VAULT_TRANSIT_KEY
    if ([string]::IsNullOrWhiteSpace($keyName)) {
        $keyName = "payment-gateway-merchant-secrets"
    }

    $plaintext = [Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($SecretKey))
    $body = @{
        plaintext = $plaintext
    }

    if (-not [string]::IsNullOrWhiteSpace($env:VAULT_TRANSIT_CONTEXT)) {
        $body["context"] = [Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($env:VAULT_TRANSIT_CONTEXT))
    }

    $bodyJson = $body | ConvertTo-Json -Compress
    $keySegment = [System.Uri]::EscapeDataString($keyName.Trim("/"))
    $url = "{0}/v1/{1}/encrypt/{2}" -f $vaultAddr, $mount, $keySegment
    $headers = @{
        "X-Vault-Token" = "$(Get-VaultToken)"
    }
    if (-not [string]::IsNullOrWhiteSpace($env:VAULT_NAMESPACE)) {
        $headers["X-Vault-Namespace"] = $env:VAULT_NAMESPACE
    }

    try {
        $response = Invoke-RestMethod -Method Post -Uri $url -Headers $headers -ContentType "application/json" -Body $bodyJson
    }
    catch {
        throw "Vault Transit encrypt failed: $($_.Exception.Message)"
    }

    if ($null -eq $response.data -or [string]::IsNullOrWhiteSpace($response.data.ciphertext)) {
        throw "Vault Transit вернул пустой ciphertext."
    }

    return $response.data.ciphertext
}

if (-not $DatabaseURL) {
    throw "DATABASE_URL не задан. Передай -DatabaseURL или задай `$env:DATABASE_URL."
}

$PSQL = Find-Psql $PsqlPath

$merchantIDSql = Sql-Escape $MerchantID
$nameSql = Sql-Escape $Name
$webhookURLSql = Sql-Escape $WebhookURL
$roleSql = Sql-Escape $Role

$schemaSql = @"
CREATE TABLE IF NOT EXISTS merchants (
    id BIGSERIAL PRIMARY KEY
);

ALTER TABLE merchants ADD COLUMN IF NOT EXISTS merchant_id TEXT;
ALTER TABLE merchants ADD COLUMN IF NOT EXISTS name TEXT;
ALTER TABLE merchants ADD COLUMN IF NOT EXISTS api_key_hash TEXT;
ALTER TABLE merchants ADD COLUMN IF NOT EXISTS secret_key TEXT;
ALTER TABLE merchants ADD COLUMN IF NOT EXISTS webhook_url TEXT;
ALTER TABLE merchants ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'merchant';
ALTER TABLE merchants ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE merchants ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE merchants ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

CREATE UNIQUE INDEX IF NOT EXISTS idx_merchants_merchant_id_unique
    ON merchants (merchant_id);
"@

& $PSQL $DatabaseURL -v ON_ERROR_STOP=1 -q -c $schemaSql | Out-Null

$existsRaw = & $PSQL $DatabaseURL -t -A -c "SELECT COUNT(*) FROM merchants WHERE merchant_id = '$merchantIDSql';"
$exists = [int]($existsRaw.Trim())

if ($exists -gt 0 -and -not $RotateKeys) {
    Write-Host "Мерчант '$MerchantID' уже существует." -ForegroundColor Yellow
    Write-Host "Чтобы перевыпустить api_key и secret_key, запусти команду с параметром -RotateKeys." -ForegroundColor Yellow
    Write-Host "Текущая запись:" -ForegroundColor Cyan
    & $PSQL $DatabaseURL -c "
SELECT
    merchant_id,
    name,
    role,
    left(api_key_hash, 12) || '...' AS api_key_hash_preview,
    webhook_url,
    active,
    created_at,
    updated_at
FROM merchants
WHERE merchant_id = '$merchantIDSql';
"
    exit 0
}

$apiKey = "pg_pk_test_" + (New-SecretHex 24)
$secretKey = "pg_sk_test_" + (New-SecretHex 32)
$apiKeyHash = Get-Sha256Hex $apiKey
$storedSecretKey = Protect-MerchantSecret $secretKey

$apiKeyHashSql = Sql-Escape $apiKeyHash
$secretKeySql = Sql-Escape $storedSecretKey

if ($exists -gt 0 -and $RotateKeys) {
    $sql = @"
UPDATE merchants
SET
    name = '$nameSql',
    api_key_hash = '$apiKeyHashSql',
    secret_key = '$secretKeySql',
    webhook_url = '$webhookURLSql',
    role = '$roleSql',
    active = TRUE,
    updated_at = NOW()
WHERE merchant_id = '$merchantIDSql';
"@
    & $PSQL $DatabaseURL -v ON_ERROR_STOP=1 -q -c $sql | Out-Null
    Write-Host "Ключи мерчанта перевыпущены." -ForegroundColor Green
} else {
    $sql = @"
INSERT INTO merchants (
    merchant_id,
    name,
    api_key_hash,
    secret_key,
    webhook_url,
    role,
    active,
    created_at,
    updated_at
) VALUES (
    '$merchantIDSql',
    '$nameSql',
    '$apiKeyHashSql',
    '$secretKeySql',
    '$webhookURLSql',
    '$roleSql',
    TRUE,
    NOW(),
    NOW()
);
"@
    & $PSQL $DatabaseURL -v ON_ERROR_STOP=1 -q -c $sql | Out-Null
    Write-Host "Мерчант создан." -ForegroundColor Green
}

Write-Host ""
Write-Host "Данные для подключения интернет-магазина:" -ForegroundColor Cyan
Write-Host "merchant_id: $MerchantID"
Write-Host "role:        $Role"
Write-Host "api_key:     $apiKey"
Write-Host "secret_key:  $secretKey"
Write-Host "webhook_url: $WebhookURL"
Write-Host ""
Write-Host "ВАЖНО: secret_key показывается только сейчас. Сохрани его и передай магазину по защищенному каналу." -ForegroundColor Yellow
Write-Host "В базе хранится hash от api_key, поэтому сам api_key тоже лучше сохранить сразу." -ForegroundColor Yellow
if ($storedSecretKey -like "vault:v*") {
    Write-Host "secret_key сохранён в базе в зашифрованном виде через HashiCorp Vault Transit." -ForegroundColor Green
} else {
    Write-Host "secret_key сохранён в базе открытым текстом. Для PCI DSS-подобного режима задай SECRET_PROTECTOR=vault_transit." -ForegroundColor Yellow
}
