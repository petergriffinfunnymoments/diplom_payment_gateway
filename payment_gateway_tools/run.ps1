
$ErrorActionPreference = "Stop"

$LocalConfig = Join-Path $PSScriptRoot "run.local.ps1"
if (Test-Path $LocalConfig) {
    . $LocalConfig
} else {
    Write-Host "Local config not found: $LocalConfig" -ForegroundColor Yellow
    Write-Host "Create it from payment_gateway_tools/run.local.example.ps1 and put your real keys there." -ForegroundColor Yellow

    if (-not $env:DATABASE_URL) {
        Write-Host "DATABASE_URL is not set. Load payment_gateway_tools/run.local.ps1 with your local connection string." -ForegroundColor Yellow
    }

    if (-not $env:PAYMENT_RETURN_URL) {
        $PUBLIC_URL = "https://your-localtunnel-url.loca.lt"
        $env:PAYMENT_RETURN_URL = $PUBLIC_URL
        $env:MERCHANT_WEBHOOK_URL = "$PUBLIC_URL/merchant/webhook"
    }

    if (-not $env:MERCHANT_ID) { $env:MERCHANT_ID = "merchant_12345" }
    if (-not $env:MERCHANT_NAME) { $env:MERCHANT_NAME = "Демонстрационный интернет-магазин" }
    if (-not $env:MERCHANT_API_KEY) { $env:MERCHANT_API_KEY = "demo_api_key" }
    if (-not $env:MERCHANT_SECRET_KEY) { $env:MERCHANT_SECRET_KEY = "demo_secret_key" }
}
