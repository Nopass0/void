<#
.SYNOPSIS
    VoidDB -- Windows installer
.DESCRIPTION
    Installs dependencies, builds the VoidDB binary, and optionally registers
    it as a Windows service using NSSM.
.PARAMETER NoBuild
    Skip Go build (use if binary already exists).
.PARAMETER DataDir
    Path to the data directory (default: .\data).
.PARAMETER Port
    API server port (default: 7700).
.PARAMETER Domain
    Public domain for TLS (e.g. void.example.com).
.PARAMETER InstallService
    Register VoidDB as a Windows service (requires NSSM and admin rights).
.EXAMPLE
    .\scripts\install.ps1
    .\scripts\install.ps1 -Port 8080 -DataDir C:\voiddata
    .\scripts\install.ps1 -Domain void.example.com -InstallService
#>
param(
    [switch]$NoBuild,
    [string]$DataDir   = "",
    [string]$BlobDir   = "",
    [int]   $Port      = 0,
    [string]$Domain    = "",
    [string]$AcmeEmail = "",
    [switch]$InstallService
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$ROOT = Split-Path $PSScriptRoot -Parent

# ── Helpers ───────────────────────────────────────────────────────────────────
function Write-OK   { param($m) Write-Host "  [OK]  $m" -ForegroundColor Green  }
function Write-Info { param($m) Write-Host "  [>>]  $m" -ForegroundColor Cyan   }
function Write-Warn { param($m) Write-Host "  [!!]  $m" -ForegroundColor Yellow }
function Write-Fail { param($m) Write-Host "  [XX]  $m" -ForegroundColor Red; exit 1 }

Write-Host ""
Write-Host "+======================================+" -ForegroundColor Cyan
Write-Host "|   VoidDB Installer  (Windows/PS1)    |" -ForegroundColor Cyan
Write-Host "+======================================+" -ForegroundColor Cyan
Write-Host ""

# ── Check Go ──────────────────────────────────────────────────────────────────
Write-Info "Checking Go installation..."
try {
    $goVer = (go version 2>&1)
    Write-OK "Go found: $goVer"
} catch {
    Write-Fail "Go not found. Install from https://go.dev/dl/ and re-run."
}

# ── Resolve paths ─────────────────────────────────────────────────────────────
if (-not $DataDir) { $DataDir = Join-Path $ROOT "data" }
if (-not $BlobDir) { $BlobDir = Join-Path $ROOT "blob" }
$BackupDir = Join-Path $ROOT "backups"
$LogDir    = Join-Path $ROOT "logs"

foreach ($d in @($DataDir, $BlobDir, $BackupDir, $LogDir)) {
    if (-not (Test-Path $d)) {
        New-Item -ItemType Directory -Path $d -Force | Out-Null
        Write-OK "Created: $d"
    }
}

# ── .env ──────────────────────────────────────────────────────────────────────
$envFile = Join-Path $ROOT ".env"
if (-not (Test-Path $envFile)) {
    $example = Join-Path $ROOT ".env.example"
    if (Test-Path $example) {
        Copy-Item $example $envFile
    } else {
        # Create minimal .env from scratch.
        @(
            "VOID_HOST=0.0.0.0"
            "VOID_PORT=7700"
            "VOID_DATA_DIR=./data"
            "VOID_BLOB_DIR=./blob"
            "VOID_JWT_SECRET=change-me-to-a-random-32-char-string"
            "VOID_ADMIN_PASSWORD=admin"
            "VOID_LOG_LEVEL=info"
            "NEXT_PUBLIC_API_URL=http://localhost:7700"
        ) | Set-Content $envFile -Encoding UTF8
    }
    # Generate random JWT secret.
    $bytes  = New-Object byte[] 36
    [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
    $secret = [Convert]::ToBase64String($bytes) -replace '[/+=]', 'x'
    (Get-Content $envFile) -replace 'change-me-to-a-random-32-char-string', $secret |
        Set-Content $envFile -Encoding UTF8
    Write-OK ".env created with random JWT secret"
} else {
    Write-Warn ".env already exists -- skipping"
}

# Apply CLI overrides to .env.
$envContent = Get-Content $envFile -Raw
if ($Port -gt 0) {
    $envContent = $envContent -replace '(?m)^VOID_PORT=.*$', "VOID_PORT=$Port"
}
if ($DataDir) {
    $safePath = $DataDir -replace '\\', '/'
    $envContent = $envContent -replace '(?m)^VOID_DATA_DIR=.*$', "VOID_DATA_DIR=$safePath"
}
if ($Domain) {
    $envContent = $envContent -replace '(?m)^#?VOID_DOMAIN=.*$', "VOID_DOMAIN=$Domain"
    if (-not ($envContent -match 'VOID_DOMAIN=')) {
        $envContent += "`nVOID_DOMAIN=$Domain"
    }
}
if ($AcmeEmail) {
    $envContent = $envContent -replace '(?m)^#?VOID_ACME_EMAIL=.*$', "VOID_ACME_EMAIL=$AcmeEmail"
    if (-not ($envContent -match 'VOID_ACME_EMAIL=')) {
        $envContent += "`nVOID_ACME_EMAIL=$AcmeEmail"
    }
}
$envContent | Set-Content $envFile -Encoding UTF8 -NoNewline

# ── Go modules ────────────────────────────────────────────────────────────────
Write-Info "Downloading Go modules..."
Push-Location $ROOT
# Use fallback proxies in case proxy.golang.org is unreachable.
$env:GOPROXY   = "https://goproxy.cn,https://goproxy.io,direct"
$env:GONOSUMDB = "*"
go mod download
if ($LASTEXITCODE -ne 0) { Write-Fail "go mod download failed" }
Write-OK "Go modules ready"

# ── Build ─────────────────────────────────────────────────────────────────────
if (-not $NoBuild) {
    Write-Info "Building VoidDB binary..."
    $gitDesc = "dev"
    try { $gitDesc = (git describe --tags --always 2>$null) } catch {}
    $env:CGO_ENABLED = "0"
    go build -mod=mod -ldflags "-s -w -X main.version=$gitDesc" `
             -o "$ROOT\voiddb.exe" `
             .\cmd\voiddb
    if ($LASTEXITCODE -ne 0) { Write-Fail "Build failed" }
    Write-OK "Binary: $ROOT\voiddb.exe"
}
Pop-Location

# ── Windows Service via NSSM ──────────────────────────────────────────────────
if ($InstallService) {
    $nssm = Get-Command nssm -ErrorAction SilentlyContinue
    if (-not $nssm) {
        Write-Warn "NSSM not found. Install with: winget install nssm  or  choco install nssm"
    } else {
        Write-Info "Registering Windows service 'VoidDB'..."
        $exe = Join-Path $ROOT "voiddb.exe"
        $cfg = Join-Path $ROOT "config.yaml"
        nssm install VoidDB $exe "-config" $cfg
        nssm set    VoidDB AppDirectory $ROOT
        nssm set    VoidDB AppEnvironmentExtra "VOID_DATA_DIR=$DataDir"
        nssm set    VoidDB AppStdout (Join-Path $LogDir "voiddb.log")
        nssm set    VoidDB AppStderr (Join-Path $LogDir "voiddb.error.log")
        nssm start  VoidDB
        Write-OK "Service 'VoidDB' installed and started"
        Write-Info "Manage with: nssm {start|stop|restart|status} VoidDB"
    }
}

# ── Summary ───────────────────────────────────────────────────────────────────
$p = if ($Port -gt 0) { $Port } else { 7700 }
Write-Host ""
Write-Host "  VoidDB is ready!" -ForegroundColor Green
Write-Host ""
Write-Host "  Binary  : $ROOT\voiddb.exe"          -ForegroundColor Cyan
Write-Host "  API     : http://localhost:$p"        -ForegroundColor Cyan
Write-Host "  Health  : http://localhost:$p/health" -ForegroundColor Cyan
Write-Host ""
Write-Host "  Start   : .\scripts\run.ps1"         -ForegroundColor White
Write-Host "  Backup  : .\scripts\backup.ps1 backup" -ForegroundColor White
Write-Host "  Test    : .\scripts\test.ps1"         -ForegroundColor White
Write-Host ""
