<#
.SYNOPSIS
    VoidDB backup and restore tool for Windows.
.DESCRIPTION
    Exports databases to .void archive files and restores them.
.PARAMETER Action
    backup | restore | list | schedule
.PARAMETER Db
    Target database name (optional, backs up all if omitted).
.PARAMETER Out
    Output file path for backup (default: .\backups\voiddb_<ts>.void).
.PARAMETER In
    Input .void file for restore.
.EXAMPLE
    .\scripts\backup.ps1 backup
    .\scripts\backup.ps1 backup -Db mydb -Out C:\backups\mydb.void
    .\scripts\backup.ps1 restore -In .\backups\voiddb_all_20240101.void
    .\scripts\backup.ps1 list
    .\scripts\backup.ps1 schedule
#>
param(
    [Parameter(Position=0)]
    [ValidateSet("backup","restore","list","schedule")]
    [string]$Action = "backup",
    [string]$Db     = "",
    [string]$Out    = "",
    [string]$In     = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$ROOT       = Split-Path $PSScriptRoot -Parent
$BackupDir  = Join-Path $ROOT "backups"
$TokenFile  = Join-Path $ROOT ".void_token"

if (-not (Test-Path $BackupDir)) { New-Item -ItemType Directory -Path $BackupDir | Out-Null }

# Source .env.
$envFile = Join-Path $ROOT ".env"
if (Test-Path $envFile) {
    Get-Content $envFile | Where-Object { $_ -match '^\s*[^#].*=.*' } | ForEach-Object {
        $p = $_ -split '=', 2
        [System.Environment]::SetEnvironmentVariable($p[0].Trim(), $p[1].Trim(), 'Process')
    }
}

$VOID_URL  = if ($env:VOID_URL)  { $env:VOID_URL  } else { "http://localhost:$($env:VOID_PORT ?? '7700')" }
$ADMIN_PWD = if ($env:VOID_ADMIN_PASSWORD) { $env:VOID_ADMIN_PASSWORD } else { "admin" }

function Write-OK   { param($m) Write-Host "  [OK]  $m" -ForegroundColor Green }
function Write-Info { param($m) Write-Host "  [>>]  $m" -ForegroundColor Cyan  }
function Write-Warn { param($m) Write-Host "  [!!]  $m" -ForegroundColor Yellow }

# ── Auth ──────────────────────────────────────────────────────────────────────
function Get-VoidToken {
    if (Test-Path $TokenFile) { return (Get-Content $TokenFile -Raw).Trim() }
    $body = @{ username = "admin"; password = $ADMIN_PWD } | ConvertTo-Json
    $resp = Invoke-RestMethod -Uri "$VOID_URL/v1/auth/login" -Method Post `
                -Body $body -ContentType "application/json"
    $resp.access_token | Set-Content $TokenFile -Encoding UTF8
    return $resp.access_token
}

function Invoke-Void {
    param([string]$Method, [string]$Path, [object]$Body = $null)
    $tok = Get-VoidToken
    $headers = @{ Authorization = "Bearer $tok" }
    $params  = @{ Uri = "$VOID_URL$Path"; Method = $Method; Headers = $headers }
    if ($Body) {
        $params.Body = ($Body | ConvertTo-Json -Depth 10)
        $params.ContentType = "application/json"
    }
    try {
        Invoke-RestMethod @params
    } catch {
        $_.Exception.Message
    }
}

# ── BACKUP ────────────────────────────────────────────────────────────────────
function Start-Backup {
    $ts = Get-Date -Format "yyyyMMdd_HHmmss"
    $suffix = if ($Db) { $Db } else { "all" }
    $outFile = if ($Out) { $Out } else { Join-Path $BackupDir "voiddb_${suffix}_${ts}.void" }

    Write-Info "Starting backup → $outFile"
    $tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) "void_backup_$ts"
    New-Item -ItemType Directory -Path $tmpDir | Out-Null

    try {
        $dbs = (Invoke-Void GET "/v1/databases").databases
        if ($Db) { $dbs = $dbs | Where-Object { $_ -eq $Db } }

        $manifest = @{
            void_version = "1.0.0"
            created_at   = (Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ")
            databases    = @($dbs)
            format       = "void-backup-v1"
        }
        $manifest | ConvertTo-Json | Set-Content (Join-Path $tmpDir "manifest.json") -Encoding UTF8

        foreach ($db in $dbs) {
            Write-Info "  Exporting: $db"
            $dbDir = Join-Path $tmpDir $db
            New-Item -ItemType Directory -Path $dbDir | Out-Null

            $cols = (Invoke-Void GET "/v1/databases/$db/collections").collections
            foreach ($col in $cols) {
                Write-Info "    Collection: $col"
                $page = 0; $pageSize = 1000; $allDocs = @()
                do {
                    $skip   = $page * $pageSize
                    $body   = @{ limit = $pageSize; skip = $skip }
                    $result = Invoke-Void POST "/v1/databases/$db/$col/query" $body
                    $docs   = $result.results
                    $allDocs += $docs
                    $page++
                } while ($docs.Count -eq $pageSize)

                @{ collection = $col; documents = $allDocs } |
                    ConvertTo-Json -Depth 20 |
                    Set-Content (Join-Path $dbDir "$col.json") -Encoding UTF8
                Write-OK "    Exported $($allDocs.Count) docs from $col"
            }
        }

        # Package as .void (zip archive).
        Add-Type -AssemblyName System.IO.Compression.FileSystem
        [System.IO.Compression.ZipFile]::CreateFromDirectory($tmpDir, $outFile)
        $size = (Get-Item $outFile).Length / 1KB
        Write-OK "Backup complete: $outFile ($([math]::Round($size, 1)) KB)"
    } finally {
        Remove-Item $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# ── RESTORE ───────────────────────────────────────────────────────────────────
function Start-Restore {
    if (-not $In -or -not (Test-Path $In)) {
        Write-Host "Restore requires -In <path>.void" -ForegroundColor Red; exit 1
    }
    $ts     = Get-Date -Format "yyyyMMdd_HHmmss"
    $tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) "void_restore_$ts"
    New-Item -ItemType Directory -Path $tmpDir | Out-Null

    try {
        Write-Info "Extracting: $In"
        Add-Type -AssemblyName System.IO.Compression.FileSystem
        [System.IO.Compression.ZipFile]::ExtractToDirectory($In, $tmpDir)

        $manifestPath = Join-Path $tmpDir "manifest.json"
        if (Test-Path $manifestPath) {
            $manifest = Get-Content $manifestPath | ConvertFrom-Json
            Write-Info "Backup from: $($manifest.created_at)"
        }

        foreach ($dbDir in (Get-ChildItem $tmpDir -Directory)) {
            $db = $dbDir.Name
            if ($Db -and $db -ne $Db) { continue }
            Write-Info "Restoring: $db"
            Invoke-Void POST "/v1/databases" @{ name = $db } | Out-Null
            foreach ($colFile in (Get-ChildItem $dbDir.FullName -Filter "*.json")) {
                $col  = $colFile.BaseName
                $data = Get-Content $colFile.FullName | ConvertFrom-Json
                Invoke-Void POST "/v1/databases/$db/collections" @{ name = $col } | Out-Null
                $count = 0
                foreach ($doc in $data.documents) {
                    $docHash = @{}
                    $doc.PSObject.Properties | ForEach-Object { $docHash[$_.Name] = $_.Value }
                    Invoke-Void POST "/v1/databases/$db/$col" $docHash | Out-Null
                    $count++
                }
                Write-OK "  Restored $count docs → $db/$col"
            }
        }
    } finally {
        Remove-Item $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# ── LIST ──────────────────────────────────────────────────────────────────────
function Show-List {
    Write-Info "Backups in $BackupDir:"
    $files = Get-ChildItem $BackupDir -Filter "*.void" -ErrorAction SilentlyContinue |
             Sort-Object LastWriteTime -Descending | Select-Object -First 20
    if ($files) {
        $files | Format-Table -AutoSize @{L="File";E={$_.Name}}, @{L="Size";E={"$([math]::Round($_.Length/1KB,1)) KB"}}, LastWriteTime
    } else { Write-Info "No backups found." }
}

# ── SCHEDULE (Task Scheduler) ─────────────────────────────────────────────────
function Register-Backup {
    $taskName   = "VoidDB Daily Backup"
    $scriptPath = Join-Path $ROOT "scripts\backup.ps1"
    $action     = New-ScheduledTaskAction -Execute "PowerShell.exe" `
                    -Argument "-NonInteractive -File `"$scriptPath`" backup"
    $trigger    = New-ScheduledTaskTrigger -Daily -At "02:00"
    $settings   = New-ScheduledTaskSettingsSet -StartWhenAvailable
    Register-ScheduledTask -TaskName $taskName -Action $action `
        -Trigger $trigger -Settings $settings -Force | Out-Null
    Write-OK "Scheduled task '$taskName' registered (daily at 02:00)"
    Write-Info "Manage with Task Scheduler or: Get-ScheduledTask -TaskName '$taskName'"
}

# ── Dispatch ──────────────────────────────────────────────────────────────────
switch ($Action) {
    "backup"   { Start-Backup }
    "restore"  { Start-Restore }
    "list"     { Show-List }
    "schedule" { Register-Backup }
}
