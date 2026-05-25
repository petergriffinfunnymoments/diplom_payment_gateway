param(
    [switch]$InstallMissingTools,
    [switch]$RequireGovulncheck,
    [switch]$SkipGovulncheck,
    [switch]$SkipSecretScan,
    [switch]$RunGitleaks,
    [string[]]$GoTestPackages = @(
        "./internal/dto",
        "./internal/httpapi/security",
        "./internal/httpapi/reports",
        "./internal/orchestrator/simple",
        "./internal/subsystems/logging",
        "./internal/subsystems/merchantauth",
        "./internal/subsystems/secrets",
        "./internal/subsystems/validator"
    )
)

$ErrorActionPreference = "Stop"

$params = @{
    GoTestPackages = $GoTestPackages
}

if ($InstallMissingTools) {
    $params.InstallMissingTools = $true
}

if ($RequireGovulncheck) {
    $params.RequireGovulncheck = $true
}

if ($SkipGovulncheck) {
    $params.SkipGovulncheck = $true
}

if ($SkipSecretScan) {
    $params.SkipSecretScan = $true
}

if ($RunGitleaks) {
    $params.RunGitleaks = $true
}

Write-Host "Running security tests and checks..." -ForegroundColor Cyan
& (Join-Path $PSScriptRoot "security-check.ps1") @params
exit $LASTEXITCODE
