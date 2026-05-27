param(
    [string[]]$Package = @(
        "./internal/dto",
        "./internal/subsystems/validator",
        "./internal/subsystems/antifraud",
        "./internal/subsystems/adapter",
        "./internal/subsystems/logging",
        "./internal/subsystems/merchantauth",
        "./internal/subsystems/secrets"
    )
)

$ErrorActionPreference = "Stop"

$ProjectRoot = Resolve-Path (Join-Path $PSScriptRoot "..")

Push-Location $ProjectRoot
try {
    $argsList = @("test") + $Package
    Write-Host "Running unit tests..." -ForegroundColor Cyan
    Write-Host ("go " + ($argsList -join " ")) -ForegroundColor DarkGray

    & go @argsList
    exit $LASTEXITCODE
}
finally {
    Pop-Location
}
