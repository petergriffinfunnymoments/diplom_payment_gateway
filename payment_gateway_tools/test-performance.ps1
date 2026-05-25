param(
    [string[]]$Package = @(
        "./internal/orchestrator/simple",
        "./internal/subsystems/validator",
        "./internal/subsystems/antifraud",
        "./internal/subsystems/storage"
    ),
    [string]$Benchmark = ".",
    [string]$Benchtime = "3s",
    [int]$Count = 1,
    [switch]$CpuProfile,
    [switch]$MemProfile
)

$ErrorActionPreference = "Stop"

$params = @{
    Package = $Package
    Benchmark = $Benchmark
    Benchtime = $Benchtime
    Count = $Count
}

if ($CpuProfile) {
    $params.CpuProfile = $true
}

if ($MemProfile) {
    $params.MemProfile = $true
}

& (Join-Path $PSScriptRoot "performance-test.ps1") @params
exit $LASTEXITCODE
