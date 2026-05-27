param(
    [string[]]$Package = @(
        "./internal/orchestrator/simple",
        "./internal/httpapi/webhooks",
        "./internal/httpapi/reports",
        "./internal/subsystems/storage"
    )
)

$ErrorActionPreference = "Stop"

$ProjectRoot = Resolve-Path (Join-Path $PSScriptRoot "..")

Push-Location $ProjectRoot
try {
    $argsList = @("test") + $Package
    Write-Host "Running component integration tests..." -ForegroundColor Cyan
    Write-Host ("go " + ($argsList -join " ")) -ForegroundColor DarkGray

    & go @argsList
    exit $LASTEXITCODE
}
finally {
    Pop-Location
}
