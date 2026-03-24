<#
.SYNOPSIS
    Start the VoidDB server on Windows.
.PARAMETER Config
    Path to config.yaml (default: .\config.yaml).
.PARAMETER Dev
    Watch for source changes and auto-rebuild (requires 'air').
#>
param(
    [string]$Config = "",
    [switch]$Dev
)

$ROOT = Split-Path $PSScriptRoot -Parent
if (-not $Config) { $Config = Join-Path $ROOT "config.yaml" }

# Source .env variables.
$envFile = Join-Path $ROOT ".env"
if (Test-Path $envFile) {
    Get-Content $envFile | Where-Object { $_ -match '^\s*[^#].*=.*' } | ForEach-Object {
        $parts = $_ -split '=', 2
        [System.Environment]::SetEnvironmentVariable($parts[0].Trim(), $parts[1].Trim(), 'Process')
    }
}

$bin = Join-Path $ROOT "voiddb.exe"

# Auto-build if missing.
if (-not (Test-Path $bin)) {
    Write-Host "[run] Building VoidDB..." -ForegroundColor Cyan
    Push-Location $ROOT
    $env:CGO_ENABLED = "0"
    go build -o $bin .\cmd\voiddb
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[run] Build failed." -ForegroundColor Red
        exit 1
    }
    Pop-Location
}

if ($Dev) {
    if (Get-Command air -ErrorAction SilentlyContinue) {
        Write-Host "[run] Starting in DEV mode (air)..." -ForegroundColor Yellow
        Push-Location $ROOT
        air
        Pop-Location
    } else {
        Write-Warning "[run] 'air' not found -- running in normal mode"
        & $bin -config $Config
    }
} else {
    Write-Host "[run] Starting VoidDB..." -ForegroundColor Cyan
    & $bin -config $Config
}
