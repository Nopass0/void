<#
.SYNOPSIS
    Start VoidDB server (and optionally the admin panel) on Windows.
.PARAMETER Config
    Path to config.yaml (default: .\config.yaml).
.PARAMETER Dev
    Watch for source changes and auto-rebuild (requires 'air').
.PARAMETER WithAdmin
    Also start the admin panel in dev mode after the server.
.PARAMETER AdminProd
    Start the admin panel in production mode (requires built assets).
.PARAMETER AdminOnly
    Start only the admin panel, not the server.
.EXAMPLE
    .\scripts\run.ps1
    .\scripts\run.ps1 -WithAdmin
    .\scripts\run.ps1 -AdminProd
    .\scripts\run.ps1 -AdminOnly
#>
param(
    [string]$Config = "",
    [switch]$Dev,
    [switch]$WithAdmin,
    [switch]$AdminProd,
    [switch]$AdminOnly
)

$ROOT = Split-Path $PSScriptRoot -Parent
if (-not $Config) { $Config = Join-Path $ROOT "config.yaml" }

# ── Load .env ─────────────────────────────────────────────────────────────────
$envFile = Join-Path $ROOT ".env"
if (Test-Path $envFile) {
    Get-Content $envFile | Where-Object { $_ -match '^\s*[^#].*=.*' } | ForEach-Object {
        $parts = $_ -split '=', 2
        [System.Environment]::SetEnvironmentVariable($parts[0].Trim(), $parts[1].Trim(), 'Process')
    }
}
$PORT = if ($env:VOID_PORT) { $env:VOID_PORT } else { "7700" }

# ── Detect package manager ────────────────────────────────────────────────────
$PKG = ""
if (Get-Command bun  -ErrorAction SilentlyContinue) { $PKG = "bun" }
elseif (Get-Command npm -ErrorAction SilentlyContinue) { $PKG = "npm" }

# ── Auto-build server binary ──────────────────────────────────────────────────
$bin = Join-Path $ROOT "voiddb.exe"

if (-not $AdminOnly) {
    if (-not (Test-Path $bin)) {
        Write-Host "[run] Building VoidDB..." -ForegroundColor Cyan
        Push-Location $ROOT
        $env:CGO_ENABLED = "0"
        $env:GOPROXY     = "https://goproxy.cn,https://goproxy.io,direct"
        $env:GONOSUMDB   = "*"
        go build -mod=mod -o $bin .\cmd\voiddb
        if ($LASTEXITCODE -ne 0) {
            Write-Host "[run] Build failed." -ForegroundColor Red
            Pop-Location; exit 1
        }
        # Also build voidcli if not present.
        $cli = Join-Path $ROOT "voidcli.exe"
        if (-not (Test-Path $cli)) {
            go build -mod=mod -o $cli .\cmd\voidcli
        }
        Pop-Location
        Write-Host "[run] Build OK" -ForegroundColor Green
    }
}

# ── Admin panel helper ────────────────────────────────────────────────────────
function Start-AdminPanel {
    param([bool]$Prod = $false)
    if (-not $PKG) {
        Write-Host "[admin] Bun/Node not found - cannot start admin panel" -ForegroundColor Yellow
        return
    }
    $adminDir = Join-Path $ROOT "admin"
    if (-not (Test-Path (Join-Path $adminDir "node_modules"))) {
        Write-Host "[admin] Installing dependencies..." -ForegroundColor Cyan
        Push-Location $adminDir
        & $PKG install
        Pop-Location
    }
    # Write .env.local.
    "NEXT_PUBLIC_API_URL=http://localhost:$PORT" |
        Set-Content (Join-Path $adminDir ".env.local") -Encoding UTF8

    Write-Host "[admin] Starting admin panel at http://localhost:3000" -ForegroundColor Green
    Push-Location $adminDir
    if ($Prod) {
        if (-not (Test-Path ".next")) {
            Write-Host "[admin] Building..." -ForegroundColor Cyan
            & $PKG run build
        }
        & $PKG run start
    } else {
        & $PKG run dev
    }
    Pop-Location
}

# ── Start modes ───────────────────────────────────────────────────────────────
if ($AdminOnly) {
    Start-AdminPanel -Prod:$AdminProd
    exit 0
}

if ($Dev) {
    if (Get-Command air -ErrorAction SilentlyContinue) {
        Write-Host "[run] Starting in DEV mode (air hot-reload)..." -ForegroundColor Yellow
        if ($WithAdmin -or $AdminProd) {
            # Start admin in background job, server in foreground.
            $adminJob = Start-Job -ScriptBlock {
                param($r, $pkg, $p)
                Set-Location (Join-Path $r "admin")
                "NEXT_PUBLIC_API_URL=http://localhost:$p" | Set-Content ".env.local"
                & $pkg run dev
            } -ArgumentList $ROOT, $PKG, $PORT
            Write-Host "[admin] Admin panel starting (job $($adminJob.Id))..." -ForegroundColor Green
        }
        Push-Location $ROOT
        air
        Pop-Location
    } else {
        Write-Warning "[run] 'air' not found -- running normally"
        & $bin -config $Config
    }
    exit 0
}

# Normal mode: server in background, admin in foreground (or vice versa).
if ($WithAdmin -or $AdminProd) {
    Write-Host "[run] Starting VoidDB server in background..." -ForegroundColor Cyan
    $srvJob = Start-Job -ScriptBlock {
        param($b, $c)
        & $b -config $c
    } -ArgumentList $bin, $Config
    Write-Host "[run] Server starting (job $($srvJob.Id))..." -ForegroundColor Green
    Write-Host "[run] API: http://localhost:$PORT" -ForegroundColor Cyan
    Start-Sleep -Seconds 1
    Start-AdminPanel -Prod:$AdminProd
} else {
    # Interactive: ask if user wants admin panel.
    if ($PKG -and -not $env:VOID_NO_PROMPT) {
        Write-Host ""
        Write-Host "[run] VoidDB server starting at http://localhost:$PORT" -ForegroundColor Cyan
        Write-Host ""
        Write-Host "  Also start admin panel?" -ForegroundColor White
        Write-Host "    [1] No - server only (default)"
        Write-Host "    [2] Yes - dev mode   (hot-reload, requires Bun/Node)"
        Write-Host "    [3] Yes - production (requires built assets)"
        Write-Host ""
        $choice = (Read-Host "  Choice [1]").Trim()
        if ($choice -eq "2") {
            $srvJob = Start-Job -ScriptBlock { param($b, $c); & $b -config $c } -ArgumentList $bin, $Config
            Write-Host "[run] Server job $($srvJob.Id) started" -ForegroundColor Green
            Start-Sleep 1
            Start-AdminPanel -Prod:$false
        } elseif ($choice -eq "3") {
            $srvJob = Start-Job -ScriptBlock { param($b, $c); & $b -config $c } -ArgumentList $bin, $Config
            Write-Host "[run] Server job $($srvJob.Id) started" -ForegroundColor Green
            Start-Sleep 1
            Start-AdminPanel -Prod:$true
        } else {
            Write-Host "[run] Starting VoidDB..." -ForegroundColor Cyan
            & $bin -config $Config
        }
    } else {
        Write-Host "[run] Starting VoidDB..." -ForegroundColor Cyan
        & $bin -config $Config
    }
}
