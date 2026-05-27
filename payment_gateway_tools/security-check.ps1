param(
    [switch]$InstallMissingTools,
    [switch]$RequireGovulncheck,
    [switch]$SkipGovulncheck,
    [switch]$SkipSecretScan,
    [switch]$RunGitleaks,
    [string[]]$GoTestPackages = @("./...")
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$ProjectRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$Failures = 0

function Invoke-Step {
    param(
        [Parameter(Mandatory=$true)]
        [string]$Name,

        [Parameter(Mandatory=$true)]
        [scriptblock]$Action
    )

    Write-Host ""
    Write-Host "==> $Name" -ForegroundColor Cyan
    try {
        & $Action
        Write-Host "OK: $Name" -ForegroundColor Green
    }
    catch {
        $script:Failures++
        Write-Host "FAILED: $Name" -ForegroundColor Red
        Write-Host $_.Exception.Message -ForegroundColor Red
    }
}

function Invoke-Native {
    param(
        [Parameter(Mandatory=$true)]
        [string]$File,

        [Parameter(Mandatory=$false)]
        [string[]]$Arguments = @()
    )

    & $File @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$File $($Arguments -join ' ') exited with code $LASTEXITCODE"
    }
}

function Get-GoBin {
    $goPath = (& go env GOPATH).Trim()
    if ([string]::IsNullOrWhiteSpace($goPath)) {
        throw "go env GOPATH returned empty value"
    }
    return Join-Path $goPath "bin"
}

function Ensure-Govulncheck {
    $cmd = Get-Command govulncheck -ErrorAction SilentlyContinue
    if ($cmd) {
        return $cmd.Source
    }

    if ($InstallMissingTools) {
        Invoke-Native "go" @("install", "golang.org/x/vuln/cmd/govulncheck@latest")
        $goBin = Get-GoBin
        if ($env:PATH -notlike "*$goBin*") {
            $env:PATH = "$goBin;$env:PATH"
        }
        $cmd = Get-Command govulncheck -ErrorAction SilentlyContinue
        if ($cmd) {
            return $cmd.Source
        }
    }

    if ($RequireGovulncheck) {
        throw "govulncheck is not installed. Run with -InstallMissingTools or install golang.org/x/vuln/cmd/govulncheck."
    }

    Write-Host "WARN: govulncheck is not installed; skipping vulnerability database check." -ForegroundColor Yellow
    return ""
}

function Invoke-TrackedSecretScan {
    $patterns = @(
        @{ Name = "Gateway generated secret key"; Pattern = "pg_sk_(test|live)_[a-fA-F0-9]{32,}" },
        @{ Name = "PostgreSQL URL with inline password"; Pattern = "postgres(?:ql)?://[A-Za-z0-9_.~-]+:[^@\s<>{}\[\]\(\)""']+@[A-Za-z0-9_.-]+" },
        @{ Name = "Private key block"; Pattern = "-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----" }
    )

    $trackedFiles = & git ls-files
    if ($LASTEXITCODE -ne 0) {
        throw "git ls-files failed"
    }

    $findings = New-Object System.Collections.Generic.List[string]
    foreach ($file in $trackedFiles) {
        if ([string]::IsNullOrWhiteSpace($file)) {
            continue
        }
        if ($file -like "*.md") {
            continue
        }
        if ($file -like "*.sum") {
            continue
        }
        if (-not (Test-Path $file -PathType Leaf)) {
            continue
        }

        $lineNumber = 0
        foreach ($line in Get-Content -LiteralPath $file -ErrorAction Stop) {
            $lineNumber++
            foreach ($pattern in $patterns) {
                if ($line -match $pattern.Pattern) {
                    $findings.Add(("{0}:{1}: {2}" -f $file, $lineNumber, $pattern.Name))
                }
            }
        }
    }

    if ($findings.Count -gt 0) {
        $message = "Potential secrets found in tracked files:`n" + ($findings -join "`n")
        throw $message
    }
}

function Invoke-OptionalGitleaks {
    $cmd = Get-Command gitleaks -ErrorAction SilentlyContinue
    if (-not $cmd) {
        Write-Host "WARN: gitleaks is not installed; optional gitleaks scan skipped." -ForegroundColor Yellow
        return
    }

    Invoke-Native $cmd.Source @("detect", "--redact", "--no-banner")
}

Push-Location $ProjectRoot
try {
    Invoke-Step "Go unit tests" {
        Invoke-Native "go" (@("test") + $GoTestPackages)
    }

    Invoke-Step "Go vet" {
        Invoke-Native "go" @("vet", "./...")
    }

    if (-not $SkipGovulncheck) {
        Invoke-Step "govulncheck" {
            $govulncheck = Ensure-Govulncheck
            if (-not [string]::IsNullOrWhiteSpace($govulncheck)) {
                Invoke-Native $govulncheck @("./...")
            }
        }
    }

    if (-not $SkipSecretScan) {
        Invoke-Step "tracked file secret scan" {
            Invoke-TrackedSecretScan
        }
    }

    if ($RunGitleaks) {
        Invoke-Step "optional gitleaks scan" {
            Invoke-OptionalGitleaks
        }
    }
}
finally {
    Pop-Location
}

Write-Host ""
if ($Failures -gt 0) {
    Write-Host "Security checks finished with $Failures failure(s)." -ForegroundColor Red
    exit 1
}

Write-Host "Security checks passed." -ForegroundColor Green
