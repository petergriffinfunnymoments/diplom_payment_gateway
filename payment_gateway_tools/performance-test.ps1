param(
    [string[]]$Package = @("./..."),
    [string]$Benchmark = ".",
    [string]$Benchtime = "3s",
    [int]$Count = 1,
    [switch]$CpuProfile,
    [switch]$MemProfile
)

$ErrorActionPreference = "Stop"

$argsList = @("test")
$argsList += $Package
$argsList += @(
    "-run", "^$",
    "-bench", $Benchmark,
    "-benchmem",
    "-benchtime", $Benchtime,
    "-count", $Count
)

if ($CpuProfile) {
    $argsList += @("-cpuprofile", "cpu.prof")
}

if ($MemProfile) {
    $argsList += @("-memprofile", "mem.prof")
}

Write-Host "Running performance benchmarks..." -ForegroundColor Cyan
Write-Host ("go " + ($argsList -join " ")) -ForegroundColor DarkGray

& go @argsList
exit $LASTEXITCODE
