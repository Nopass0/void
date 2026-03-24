<#
.SYNOPSIS
    Start VoidDB server and optional admin panel on Windows.
.PARAMETER Config
    Path to config.yaml (default: .\config.yaml).
.PARAMETER Dev
    Watch for source changes and auto-rebuild (requires 'air').
.PARAMETER WithAdmin
    Start the admin panel together with the server.
.PARAMETER AdminProd
    Start the admin panel in production mode.
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
if (-not $Config) {
    $Config = Join-Path $ROOT "config.yaml"
}

$envFile = Join-Path $ROOT ".env"
if (Test-Path $envFile) {
    Get-Content $envFile | Where-Object { $_ -match '^\s*[^#].*=.*' } | ForEach-Object {
        $parts = $_ -split '=', 2
        [System.Environment]::SetEnvironmentVariable($parts[0].Trim(), $parts[1].Trim(), "Process")
    }
}

$PORT = if ($env:VOID_PORT) { $env:VOID_PORT } else { "7700" }
$ADMIN_PORT = if ($env:VOID_ADMIN_PORT) { $env:VOID_ADMIN_PORT } else { "3000" }
$PKG = ""
if (Get-Command bun -ErrorAction SilentlyContinue) {
    $PKG = "bun"
} elseif (Get-Command npm -ErrorAction SilentlyContinue) {
    $PKG = "npm"
}

function Get-PortProcess {
    param([int]$Port)

    $conn = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue |
        Select-Object -First 1
    if (-not $conn) {
        return $null
    }

    return Get-Process -Id $conn.OwningProcess -ErrorAction SilentlyContinue
}

function Test-BinaryNeedsBuild {
    param([string]$Path)

    if (-not (Test-Path $Path)) {
        return $true
    }

    $binTime = (Get-Item $Path).LastWriteTimeUtc
    $sourceRoots = @(
        (Join-Path $ROOT "cmd"),
        (Join-Path $ROOT "internal")
    )

    $newerSource = Get-ChildItem -Path $sourceRoots -Recurse -Filter *.go -File -ErrorAction SilentlyContinue |
        Where-Object { $_.LastWriteTimeUtc -gt $binTime } |
        Select-Object -First 1

    return [bool]$newerSource
}

function Ensure-Binaries {
    $serverBin = Join-Path $ROOT "voiddb.exe"
    $cliBin = Join-Path $ROOT "voidcli.exe"
    $needsServer = Test-BinaryNeedsBuild -Path $serverBin
    $needsCLI = Test-BinaryNeedsBuild -Path $cliBin

    if (-not $needsServer -and -not $needsCLI) {
        return $serverBin
    }

    Write-Host "[run] Building VoidDB..." -ForegroundColor Cyan
    Push-Location $ROOT
    try {
        $env:CGO_ENABLED = "0"
        if (-not $env:GOPROXY) {
            $env:GOPROXY = "https://goproxy.cn,https://goproxy.io,direct"
        }
        if (-not $env:GONOSUMDB) {
            $env:GONOSUMDB = "*"
        }

        go build -mod=mod -o $serverBin .\cmd\voiddb
        if ($LASTEXITCODE -ne 0) {
            throw "server build failed"
        }

        go build -mod=mod -o $cliBin .\cmd\voidcli
        if ($LASTEXITCODE -ne 0) {
            throw "cli build failed"
        }
    } catch {
        Write-Host "[run] Build failed: $_" -ForegroundColor Red
        exit 1
    } finally {
        Pop-Location
    }

    Write-Host "[run] Build OK" -ForegroundColor Green
    return $serverBin
}

function Start-AdminPanel {
    param([bool]$Prod = $false)

    if (-not $PKG) {
        Write-Host "[admin] Bun/Node not found - cannot start admin panel" -ForegroundColor Yellow
        return
    }

    $adminDir = Join-Path $ROOT "admin"
    if (-not (Test-Path $adminDir)) {
        Write-Host "[admin] Admin directory not found: $adminDir" -ForegroundColor Yellow
        return
    }

    if (-not (Test-Path (Join-Path $adminDir "node_modules"))) {
        Write-Host "[admin] Installing dependencies..." -ForegroundColor Cyan
        Push-Location $adminDir
        try {
            & $PKG install
        } finally {
            Pop-Location
        }
    }

    "NEXT_PUBLIC_API_URL=http://localhost:$PORT" |
        Set-Content (Join-Path $adminDir ".env.local") -Encoding UTF8

    $existingAdmin = Get-PortProcess -Port ([int]$ADMIN_PORT)
    if ($existingAdmin) {
        Write-Host ""
        Write-Host "[admin] Port $ADMIN_PORT is already in use by: $($existingAdmin.Name) (PID $($existingAdmin.Id))" -ForegroundColor Yellow
        Write-Host "  [1] Stop the existing process and restart admin (default)"
        Write-Host "  [2] Leave it running (skip admin start)"
        Write-Host "  [3] Exit"
        Write-Host ""
        $choice = (Read-Host "  Choice [1]").Trim()
        if (-not $choice) {
            $choice = "1"
        }

        if ($choice -eq "3") {
            exit 0
        }

        if ($choice -eq "2") {
            Write-Host "[admin] Admin already running at http://localhost:$ADMIN_PORT" -ForegroundColor Green
            return
        }

        Write-Host "[admin] Stopping PID $($existingAdmin.Id)..." -ForegroundColor Cyan
        Stop-Process -Id $existingAdmin.Id -Force -ErrorAction SilentlyContinue
        Start-Sleep -Milliseconds 800
        Write-Host "[admin] Stopped." -ForegroundColor Green
    }

    Write-Host "[admin] Starting admin panel at http://localhost:$ADMIN_PORT" -ForegroundColor Green
    Push-Location $adminDir
    try {
        if ($Prod) {
            if (-not (Test-Path ".next")) {
                Write-Host "[admin] Building..." -ForegroundColor Cyan
                & $PKG run build
            }
            & $PKG run start -- --port $ADMIN_PORT
        } else {
            & $PKG run dev -- --port $ADMIN_PORT
        }
    } finally {
        Pop-Location
    }
}

function Start-ServerJob {
    param(
        [string]$Binary,
        [string]$ConfigPath
    )

    return Start-Job -ScriptBlock {
        param($b, $c)
        & $b -config $c
    } -ArgumentList $Binary, $ConfigPath
}

if (-not $AdminOnly) {
    $existing = Get-PortProcess -Port ([int]$PORT)
    if ($existing) {
        Write-Host ""
        Write-Host "[run] Port $PORT is already in use by: $($existing.Name) (PID $($existing.Id))" -ForegroundColor Yellow
        Write-Host "  [1] Stop the existing process and restart (default)"
        Write-Host "  [2] Leave it running (skip server start)"
        Write-Host "  [3] Exit"
        Write-Host ""
        $choice = (Read-Host "  Choice [1]").Trim()
        if (-not $choice) {
            $choice = "1"
        }

        if ($choice -eq "3") {
            exit 0
        }

        if ($choice -eq "2") {
            Write-Host "[run] Server already running at http://localhost:$PORT" -ForegroundColor Green
            if ($WithAdmin -or $AdminProd) {
                Start-AdminPanel -Prod:$AdminProd
            }
            exit 0
        }

        Write-Host "[run] Stopping PID $($existing.Id)..." -ForegroundColor Cyan
        Stop-Process -Id $existing.Id -Force -ErrorAction SilentlyContinue
        Start-Sleep -Milliseconds 800
        Write-Host "[run] Stopped." -ForegroundColor Green
    }
}

if ($AdminOnly) {
    Start-AdminPanel -Prod:$AdminProd
    exit 0
}

$bin = Ensure-Binaries

if ($Dev) {
    if (Get-Command air -ErrorAction SilentlyContinue) {
        Write-Host "[run] Starting in DEV mode (air hot-reload)..." -ForegroundColor Yellow
        if ($WithAdmin -or $AdminProd) {
            $adminJob = Start-Job -ScriptBlock {
                param($root, $pkg, $port, $prod)
                if (-not $pkg) {
                    return
                }

                Set-Location (Join-Path $root "admin")
                "NEXT_PUBLIC_API_URL=http://localhost:$port" | Set-Content ".env.local" -Encoding UTF8
                if ($prod) {
                    if (-not (Test-Path ".next")) {
                        & $pkg run build
                    }
                    & $pkg run start -- --port 3000
                } else {
                    & $pkg run dev -- --port 3000
                }
            } -ArgumentList $ROOT, $PKG, $PORT, [bool]$AdminProd
            Write-Host "[admin] Admin panel starting (job $($adminJob.Id))..." -ForegroundColor Green
        }

        Push-Location $ROOT
        try {
            air
        } finally {
            Pop-Location
        }
    } else {
        Write-Warning "[run] 'air' not found -- running normally"
        & $bin -config $Config
    }
    exit 0
}

if ($WithAdmin -or $AdminProd) {
    Write-Host "[run] Starting VoidDB server in background..." -ForegroundColor Cyan
    $srvJob = Start-ServerJob -Binary $bin -ConfigPath $Config
    Write-Host "[run] Server starting (job $($srvJob.Id))..." -ForegroundColor Green
    Write-Host "[run] API: http://localhost:$PORT" -ForegroundColor Cyan
    Start-Sleep -Seconds 1
    Start-AdminPanel -Prod:$AdminProd
    exit 0
}

if ($PKG -and -not $env:VOID_NO_PROMPT) {
    Write-Host ""
    Write-Host "[run] VoidDB server starting at http://localhost:$PORT" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "  Also start admin panel?" -ForegroundColor White
    Write-Host "    [1] No - server only (default)"
    Write-Host "    [2] Yes - dev mode"
    Write-Host "    [3] Yes - production"
    Write-Host ""
    $choice = (Read-Host "  Choice [1]").Trim()

    if ($choice -eq "2") {
        $srvJob = Start-ServerJob -Binary $bin -ConfigPath $Config
        Write-Host "[run] Server job $($srvJob.Id) started" -ForegroundColor Green
        Start-Sleep -Seconds 1
        Start-AdminPanel -Prod:$false
    } elseif ($choice -eq "3") {
        $srvJob = Start-ServerJob -Binary $bin -ConfigPath $Config
        Write-Host "[run] Server job $($srvJob.Id) started" -ForegroundColor Green
        Start-Sleep -Seconds 1
        Start-AdminPanel -Prod:$true
    } else {
        Write-Host "[run] Starting VoidDB..." -ForegroundColor Cyan
        & $bin -config $Config
    }
} else {
    Write-Host "[run] Starting VoidDB..." -ForegroundColor Cyan
    & $bin -config $Config
}
