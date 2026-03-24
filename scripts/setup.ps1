<#
.SYNOPSIS
    VoidDB Interactive Setup Wizard for Windows.
.DESCRIPTION
    Configures, builds, installs, and starts VoidDB interactively.
    - Collects server settings (port, dirs, domain, TLS)
    - Builds voiddb.exe and voidcli.exe
    - Installs CLI commands to PATH (user scope)
    - Registers Windows service with autostart (optional, requires NSSM)
    - Sets up and starts the admin panel (dev or production mode)
.EXAMPLE
    .\scripts\setup.ps1
    .\scripts\setup.ps1 -Silent   # use defaults without prompting
#>
param(
    [switch]$Silent,
    [switch]$SkipBuild,
    [switch]$SkipService
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$ROOT = Split-Path $PSScriptRoot -Parent

# ── Colour helpers ────────────────────────────────────────────────────────────
function Write-Header { param($t)
    Write-Host ""
    Write-Host "+--------------------------------------------------+" -ForegroundColor DarkCyan
    Write-Host "|  $t" -ForegroundColor Cyan
    Write-Host "+--------------------------------------------------+" -ForegroundColor DarkCyan
}
function Write-Step  { param($t) Write-Host "  [>>] $t" -ForegroundColor Cyan }
function Write-OK    { param($t) Write-Host "  [OK] $t" -ForegroundColor Green }
function Write-Warn  { param($t) Write-Host "  [!!] $t" -ForegroundColor Yellow }
function Write-Fail  { param($t) Write-Host "  [XX] $t" -ForegroundColor Red; exit 1 }
function Write-Info  { param($t) Write-Host "       $t" -ForegroundColor Gray }

function Ask {
    param([string]$Prompt, [string]$Default = "")
    if ($Silent) { return $Default }
    $hint = if ($Default) { " [$Default]" } else { "" }
    $ans  = Read-Host "$Prompt$hint"
    return if ($ans -eq "") { $Default } else { $ans }
}

function AskSecret {
    param([string]$Prompt, [string]$Default = "")
    if ($Silent) { return $Default }
    $hint = if ($Default) { " [press Enter to keep]" } else { "" }
    $sec  = Read-Host "$Prompt$hint" -AsSecureString
    $plain = [Runtime.InteropServices.Marshal]::PtrToStringAuto(
                [Runtime.InteropServices.Marshal]::SecureStringToBSTR($sec))
    return if ($plain -eq "") { $Default } else { $plain }
}

function AskYN {
    param([string]$Prompt, [bool]$Default = $false)
    if ($Silent) { return $Default }
    $hint = if ($Default) { " [Y/n]" } else { " [y/N]" }
    $ans  = (Read-Host "$Prompt$hint").Trim().ToLower()
    if ($ans -eq "")  { return $Default }
    return ($ans -eq "y" -or $ans -eq "yes")
}

# ── Banner ────────────────────────────────────────────────────────────────────
Clear-Host
Write-Host ""
Write-Host "  ======================================================" -ForegroundColor Cyan
Write-Host "           V O I D D B   S E T U P   W I Z A R D       " -ForegroundColor White
Write-Host "  ======================================================" -ForegroundColor Cyan
Write-Host "  High-performance LSM-tree document database"            -ForegroundColor Gray
Write-Host ""

# ── Step 1: Check prerequisites ───────────────────────────────────────────────
Write-Header "Step 1: Checking prerequisites"

# Go
try { $goVer = (go version 2>&1); Write-OK "Go: $goVer" }
catch { Write-Fail "Go not found. Download: https://go.dev/dl/" }

# Bun / Node (optional, for admin panel)
$hasBun  = $null -ne (Get-Command bun  -ErrorAction SilentlyContinue)
$hasNode = $null -ne (Get-Command node -ErrorAction SilentlyContinue)
if ($hasBun)       { Write-OK  "Bun: $(bun --version 2>&1)" }
elseif ($hasNode)  { Write-OK  "Node.js: $(node --version 2>&1)" }
else               { Write-Warn "Bun/Node not found. Admin panel won't be available." }

# Git
$hasGit = $null -ne (Get-Command git -ErrorAction SilentlyContinue)
if ($hasGit) { Write-OK "Git found" }

# NSSM (for Windows service)
$hasNSSM = $null -ne (Get-Command nssm -ErrorAction SilentlyContinue)
if ($hasNSSM) { Write-OK "NSSM found (service install available)" }
else          { Write-Warn "NSSM not found. Install with: winget install nssm" }

# ── Step 2: Server configuration ──────────────────────────────────────────────
Write-Header "Step 2: Server configuration"

$apiPort   = [int](Ask "API port"  "7700")
$dataDir   = Ask "Data directory" (Join-Path $ROOT "data")
$blobDir   = Ask "Blob directory" (Join-Path $ROOT "blob")
$backupDir = Ask "Backup directory" (Join-Path $ROOT "backups")

# ── Step 3: Security ──────────────────────────────────────────────────────────
Write-Header "Step 3: Security (press Enter to auto-generate)"

$existingEnv = Join-Path $ROOT ".env"
$existingSecret = ""
if (Test-Path $existingEnv) {
    $existingSecret = (Get-Content $existingEnv | Where-Object { $_ -match "^VOID_JWT_SECRET=" }) -replace "^VOID_JWT_SECRET=", ""
}
if (-not $existingSecret) {
    $bytes = New-Object byte[] 36
    [Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
    $existingSecret = [Convert]::ToBase64String($bytes) -replace '[/+=]', 'x'
}

$jwtSecret = AskSecret "JWT secret" $existingSecret
if (-not $jwtSecret) {
    $bytes = New-Object byte[] 36
    [Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
    $jwtSecret = [Convert]::ToBase64String($bytes) -replace '[/+=]', 'x'
    Write-Info "Generated: $jwtSecret"
}
$adminPass = AskSecret "Admin password" "admin"
Write-Warn "Remember to change the admin password before exposing to the internet!"

# ── Step 4: TLS / Domain ──────────────────────────────────────────────────────
Write-Header "Step 4: TLS / Domain (optional)"
Write-Info "Options: off | file (your cert) | acme (Let's Encrypt)"
$tlsMode = (Ask "TLS mode" "off").ToLower()
$domain = ""
$acmeEmail = ""
$certFile = ""
$keyFile = ""

if ($tlsMode -eq "acme") {
    $domain    = Ask "Your domain (e.g. void.example.com)" ""
    $acmeEmail = Ask "Let's Encrypt email" ""
} elseif ($tlsMode -eq "file") {
    $domain   = Ask "Your domain" ""
    $certFile = Ask "Certificate PEM file path" ""
    $keyFile  = Ask "Private key PEM file path" ""
}

# ── Step 5: Write .env ────────────────────────────────────────────────────────
Write-Header "Step 5: Writing configuration"

$envLines = @(
    "VOID_HOST=0.0.0.0"
    "VOID_PORT=$apiPort"
    "VOID_DATA_DIR=$($dataDir -replace '\\','/')"
    "VOID_BLOB_DIR=$($blobDir -replace '\\','/')"
    "VOID_JWT_SECRET=$jwtSecret"
    "VOID_ADMIN_PASSWORD=$adminPass"
    "VOID_LOG_LEVEL=info"
    "NEXT_PUBLIC_API_URL=http://localhost:$apiPort"
)
if ($tlsMode -ne "off") {
    $envLines += "VOID_TLS_MODE=$tlsMode"
}
if ($domain)    { $envLines += "VOID_DOMAIN=$domain" }
if ($acmeEmail) { $envLines += "VOID_ACME_EMAIL=$acmeEmail" }
if ($certFile)  { $envLines += "VOID_TLS_CERT=$certFile" }
if ($keyFile)   { $envLines += "VOID_TLS_KEY=$keyFile" }

$envLines | Set-Content (Join-Path $ROOT ".env") -Encoding UTF8
Write-OK ".env written"

# Write config.yaml.
$cfgContent = @"
server:
  host: "0.0.0.0"
  port: $apiPort
  read_timeout: "30s"
  write_timeout: "60s"
  cors_origins: ["*"]

engine:
  data_dir: "$($dataDir -replace '\\','/')"
  memtable_size: 67108864
  block_cache_size: 268435456
  bloom_false_positive_rate: 0.01
  compaction_workers: 2
  sync_wal: false
  max_levels: 7
  level_size_multiplier: 10

auth:
  jwt_secret: "$jwtSecret"
  token_expiry: "24h"
  refresh_expiry: "168h"
  admin_password: "$adminPass"

blob:
  storage_dir: "$($blobDir -replace '\\','/')"
  max_object_size: 5368709120
  enable_s3_api: true
  s3_region: "void-1"

log:
  level: "info"
  format: "console"
  output_path: "stdout"

admin:
  enabled: true
  static_dir: "./admin/out"

tls:
  mode: "$tlsMode"
  cert_file: "$certFile"
  key_file: "$keyFile"
  domain: "$domain"
  acme_email: "$acmeEmail"
  redirect_http: $(if ($tlsMode -ne "off") { "true" } else { "false" })
  http_src_port: 80
  https_port: 443

backup:
  dir: "$($backupDir -replace '\\','/')"
  retain: 14
"@
$cfgContent | Set-Content (Join-Path $ROOT "config.yaml") -Encoding UTF8
Write-OK "config.yaml written"

# Create directories.
foreach ($d in @($dataDir, $blobDir, $backupDir, (Join-Path $ROOT "logs"))) {
    if (-not (Test-Path $d)) {
        New-Item -ItemType Directory -Path $d -Force | Out-Null
        Write-OK "Created: $d"
    }
}

# ── Step 6: Build binaries ────────────────────────────────────────────────────
Write-Header "Step 6: Building binaries"

if (-not $SkipBuild) {
    Push-Location $ROOT
    $env:CGO_ENABLED = "0"
    $env:GOPROXY    = "https://goproxy.cn,https://goproxy.io,direct"
    $env:GONOSUMDB  = "*"

    $gitDesc = "dev"
    try { $gitDesc = (git describe --tags --always 2>$null) } catch {}

    Write-Step "Building voiddb.exe..."
    go build -mod=mod -ldflags "-s -w -X main.version=$gitDesc" `
             -o "$ROOT\voiddb.exe" .\cmd\voiddb
    if ($LASTEXITCODE -ne 0) { Write-Fail "voiddb build failed" }
    Write-OK "voiddb.exe built"

    Write-Step "Building voidcli.exe..."
    go build -mod=mod -ldflags "-s -w -X main.version=$gitDesc" `
             -o "$ROOT\voidcli.exe" .\cmd\voidcli
    if ($LASTEXITCODE -ne 0) { Write-Fail "voidcli build failed" }
    Write-OK "voidcli.exe built"

    Pop-Location
} else {
    Write-Warn "Skipping build (-SkipBuild)"
}

# ── Step 7: Install CLI to PATH ───────────────────────────────────────────────
Write-Header "Step 7: Installing CLI commands to PATH"

$installCLI = AskYN "Add voiddb and voidcli to your user PATH?" $true
if ($installCLI) {
    $userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    if ($userPath -notlike "*$ROOT*") {
        [Environment]::SetEnvironmentVariable("PATH", "$userPath;$ROOT", "User")
        $env:PATH += ";$ROOT"
        Write-OK "Added $ROOT to user PATH"
    } else {
        Write-OK "Already in PATH"
    }
    Write-Info "Open a new terminal and run: voidcli status"
}

# ── Step 8: Windows Service (autostart) ───────────────────────────────────────
Write-Header "Step 8: Windows Service (autostart)"

$installSvc = if (-not $SkipService -and $hasNSSM) {
    AskYN "Install VoidDB as a Windows service (autostart on boot)?" $false
} else {
    if (-not $hasNSSM) { Write-Warn "NSSM not found - skipping service install" }
    $false
}

if ($installSvc) {
    $svcName = "VoidDB"
    $exe     = Join-Path $ROOT "voiddb.exe"
    $cfg     = Join-Path $ROOT "config.yaml"
    $logDir  = Join-Path $ROOT "logs"

    # Remove existing service if present.
    $existing = Get-Service -Name $svcName -ErrorAction SilentlyContinue
    if ($existing) {
        Write-Step "Removing old service..."
        nssm stop  $svcName 2>$null
        nssm remove $svcName confirm
    }

    Write-Step "Registering service '$svcName'..."
    nssm install  $svcName $exe "-config" $cfg
    nssm set      $svcName AppDirectory $ROOT
    nssm set      $svcName DisplayName  "VoidDB Database Server"
    nssm set      $svcName Description  "High-performance LSM-tree document database"
    nssm set      $svcName Start        SERVICE_AUTO_START
    nssm set      $svcName AppStdout    (Join-Path $logDir "voiddb.log")
    nssm set      $svcName AppStderr    (Join-Path $logDir "voiddb.error.log")
    nssm set      $svcName AppRotateFiles 1
    nssm set      $svcName AppRotateOnline 1
    nssm set      $svcName AppRotateSeconds 86400

    # Pass .env vars to service.
    $envVars = "VOID_DATA_DIR=$($dataDir -replace '\\','/')"
    nssm set $svcName AppEnvironmentExtra $envVars

    nssm start $svcName
    if ($LASTEXITCODE -eq 0) { Write-OK "Service '$svcName' started and set to autostart" }
    else                      { Write-Warn "Service registered but may not have started yet" }
} else {
    Write-Info "Skipping service install. Start manually: .\scripts\run.ps1"
}

# ── Step 9: Admin panel ───────────────────────────────────────────────────────
Write-Header "Step 9: Admin panel"

if (-not ($hasBun -or $hasNode)) {
    Write-Warn "Node.js/Bun not available - skipping admin panel"
} else {
    $startAdmin = AskYN "Set up the admin panel?" $true
    if ($startAdmin) {
        Push-Location (Join-Path $ROOT "admin")

        Write-Step "Installing admin dependencies..."
        if ($hasBun) {
            bun install
        } else {
            npm install
        }

        $adminMode = (Ask "Admin mode: [1] Dev (hot-reload)  [2] Production build" "1").Trim()

        # Write .env.local for Next.js.
        $nextEnv = @(
            "NEXT_PUBLIC_API_URL=http://localhost:$apiPort"
        )
        $nextEnv | Set-Content (Join-Path $ROOT "admin" ".env.local") -Encoding UTF8

        if ($adminMode -eq "2") {
            Write-Step "Building admin panel for production..."
            if ($hasBun) { bun run build } else { npm run build }
            if ($LASTEXITCODE -eq 0) {
                Write-OK "Admin panel built"
                $startNow = AskYN "Start admin panel now (port 3000)?" $true
                if ($startNow) {
                    Pop-Location
                    Push-Location (Join-Path $ROOT "admin")
                    Write-OK "Admin panel starting at http://localhost:3000"
                    Write-Info "Press Ctrl+C to stop"
                    if ($hasBun) { bun run start } else { npm run start }
                }
            } else {
                Write-Warn "Build failed"
            }
        } else {
            $startNow = AskYN "Start admin panel in dev mode now (port 3000)?" $true
            if ($startNow) {
                Write-OK "Admin panel starting at http://localhost:3000"
                Write-Info "Press Ctrl+C to stop"
                if ($hasBun) { bun run dev } else { npm run dev }
            } else {
                Write-Info "Start later with: cd admin && bun dev"
            }
        }
        Pop-Location
    }
}

# ── Summary ───────────────────────────────────────────────────────────────────
Write-Header "Setup complete!"

$proto = if ($tlsMode -ne "off") { "https" } else { "http" }
$host2 = if ($domain) { $domain } else { "localhost" }

Write-Host ""
Write-Host "  VoidDB is ready!" -ForegroundColor Green
Write-Host ""
Write-Host "  API      : $proto://$host2`:$apiPort"       -ForegroundColor Cyan
Write-Host "  Health   : $proto://$host2`:$apiPort/health" -ForegroundColor Cyan
Write-Host "  Admin UI : http://localhost:3000"            -ForegroundColor Cyan
Write-Host ""
Write-Host "  CLI commands (open a new terminal first):"    -ForegroundColor White
Write-Host "    voidcli status"                             -ForegroundColor White
Write-Host "    voidcli login"                              -ForegroundColor White
Write-Host "    voidcli db list"                            -ForegroundColor White
Write-Host "    voidcli db create myapp"                    -ForegroundColor White
Write-Host "    voidcli col create myapp users"             -ForegroundColor White
Write-Host "    voidcli doc insert myapp users '{`"name`":`"Alice`"}'"  -ForegroundColor White
Write-Host ""
Write-Host "  Scripts:"                                     -ForegroundColor White
Write-Host "    .\scripts\run.ps1          -- start server" -ForegroundColor Gray
Write-Host "    .\scripts\backup.ps1 backup -- backup all"  -ForegroundColor Gray
Write-Host "    .\scripts\test.ps1          -- run tests"   -ForegroundColor Gray
Write-Host ""
