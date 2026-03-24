<#
.SYNOPSIS
    VoidDB test runner for Windows.
.PARAMETER Unit
    Run Go unit tests.
.PARAMETER Bench
    Run benchmarks.
.PARAMETER E2E
    Run end-to-end API tests.
.PARAMETER All
    Run everything.
#>
param(
    [switch]$Unit,
    [switch]$Bench,
    [switch]$E2E,
    [switch]$All
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Continue"

$ROOT = Split-Path $PSScriptRoot -Parent
Push-Location $ROOT

# Default: unit + bench.
if (-not ($Unit -or $Bench -or $E2E -or $All)) { $Unit = $true; $Bench = $true }
if ($All) { $Unit = $true; $Bench = $true; $E2E = $true }

$pass = 0; $fail = 0

function Write-OK   { param($m) Write-Host "  [PASS] $m" -ForegroundColor Green;  $script:pass++ }
function Write-FAIL { param($m) Write-Host "  [FAIL] $m" -ForegroundColor Red;    $script:fail++ }
function Write-Info { param($m) Write-Host "  [>>]   $m" -ForegroundColor Cyan }

Write-Host ""
Write-Host "VoidDB Test Suite" -ForegroundColor White -BackgroundColor DarkBlue
Write-Host ("-" * 50)

# Source .env.
$envFile = Join-Path $ROOT ".env"
if (Test-Path $envFile) {
    Get-Content $envFile | Where-Object { $_ -match '^\s*[^#].*=.*' } | ForEach-Object {
        $p = $_ -split '=', 2
        [System.Environment]::SetEnvironmentVariable($p[0].Trim(), $p[1].Trim(), 'Process')
    }
}
$VOID_URL = if ($env:VOID_URL) { $env:VOID_URL } else { "http://localhost:7700" }

function Invoke-Test {
    param([string]$Name, [scriptblock]$Block)
    try {
        & $Block | Out-Null
        if ($LASTEXITCODE -eq 0 -or $null -eq $LASTEXITCODE) {
            Write-OK $Name
        } else {
            Write-FAIL "$Name (exit $LASTEXITCODE)"
        }
    } catch {
        Write-FAIL "$Name : $_"
    }
}

# ── Unit tests ────────────────────────────────────────────────────────────────
if ($Unit) {
    Write-Info "Go unit tests"
    Invoke-Test "Go: build check"         { go build ./... }
    Invoke-Test "Go: types"               { go test ./internal/engine/types/... -v -count=1 }
    Invoke-Test "Go: storage/bloom"       { go test ./internal/engine/storage/... -v -count=1 }
    Invoke-Test "Go: WAL"                 { go test ./internal/engine/wal/... -v -count=1 }
    Invoke-Test "Go: cache"               { go test ./internal/engine/cache/... -v -count=1 }
    Invoke-Test "Go: engine integration"  { go test ./internal/engine/... -v -count=1 -timeout 30s }
    Invoke-Test "Go: auth"                { go test ./internal/auth/... -v -count=1 }
}

# ── Benchmarks ────────────────────────────────────────────────────────────────
if ($Bench) {
    Write-Info "Benchmarks"
    $health = $null
    try { $health = Invoke-RestMethod "$VOID_URL/health" -TimeoutSec 3 } catch {}
    if ($health) {
        Write-Info "Running VoidDB vs PostgreSQL benchmark (50k records)..."
        Push-Location (Join-Path $ROOT "benchmark")
        try {
            go mod download 2>$null
            go run main.go -records 50000 -workers 4
            if ($LASTEXITCODE -eq 0) { Write-OK "Benchmark completed" } else { Write-FAIL "Benchmark failed" }
        } finally { Pop-Location }
    } else {
        Write-Info "Server offline -- running in-process Go benchmarks"
        Invoke-Test "Go: engine benchmarks" { go test ./internal/engine/... -bench=. -benchtime=5s -run "^$" }
    }
}

# ── E2E tests ─────────────────────────────────────────────────────────────────
if ($E2E) {
    Write-Info "E2E tests against $VOID_URL"

    function Assert-Status {
        param([string]$Desc, [int]$Expected, [int]$Actual)
        if ($Actual -eq $Expected) { Write-OK "E2E: $Desc (HTTP $Actual)" }
        else { Write-FAIL "E2E: $Desc -- expected $Expected, got $Actual" }
    }

    function Invoke-VoidRaw {
        param([string]$Method, [string]$Path, [object]$Body = $null, [string]$Token = "")
        $uri = "$VOID_URL$Path"
        $h   = @{}
        if ($Token) { $h.Authorization = "Bearer $Token" }
        try {
            $params = @{ Uri = $uri; Method = $Method; Headers = $h }
            if ($Body) { $params.Body = ($Body | ConvertTo-Json); $params.ContentType = "application/json" }
            $r = Invoke-WebRequest @params -UseBasicParsing
            return $r
        } catch [System.Net.WebException] {
            return $_.Exception.Response
        }
    }

    $hc = Invoke-VoidRaw GET "/health"
    Assert-Status "Health check" 200 ([int]$hc.StatusCode)

    $lr = Invoke-RestMethod -Uri "$VOID_URL/v1/auth/login" -Method Post `
            -Body (@{username="admin";password="admin"}|ConvertTo-Json) `
            -ContentType "application/json" -ErrorAction SilentlyContinue
    $tok = $lr.access_token

    if ($tok) {
        Write-OK "E2E: Login succeeded"
        $script:pass++

        $r = Invoke-VoidRaw POST "/v1/databases" @{name="e2e_ps"} $tok
        Assert-Status "Create database" 201 ([int]$r.StatusCode)

        $r = Invoke-VoidRaw POST "/v1/databases/e2e_ps/collections" @{name="items"} $tok
        Assert-Status "Create collection" 201 ([int]$r.StatusCode)

        $insert = Invoke-RestMethod -Uri "$VOID_URL/v1/databases/e2e_ps/items" -Method Post `
                    -Body (@{name="ps_test";score=99}|ConvertTo-Json) `
                    -ContentType "application/json" `
                    -Headers @{Authorization="Bearer $tok"} -ErrorAction SilentlyContinue
        $docId = $insert._id
        if ($docId) { Write-OK "E2E: Insert document (id=$docId)"; $script:pass++ }
        else         { Write-FAIL "E2E: Insert document"; $script:fail++ }

        if ($docId) {
            $r = Invoke-VoidRaw GET "/v1/databases/e2e_ps/items/$docId" $null $tok
            Assert-Status "Get document" 200 ([int]$r.StatusCode)

            $r = Invoke-VoidRaw "PATCH" "/v1/databases/e2e_ps/items/$docId" @{score=100} $tok
            Assert-Status "Patch document" 200 ([int]$r.StatusCode)

            $r = Invoke-VoidRaw DELETE "/v1/databases/e2e_ps/items/$docId" $null $tok
            Assert-Status "Delete document" 204 ([int]$r.StatusCode)
        }
    } else {
        Write-FAIL "E2E: Login failed -- is VoidDB running at $VOID_URL?"
    }
}

# ── Summary ───────────────────────────────────────────────────────────────────
Pop-Location
Write-Host ("-" * 50)
Write-Host "  Passed: $pass   Failed: $fail"
Write-Host ""
if ($fail -eq 0) {
    Write-Host "All tests passed." -ForegroundColor Green
} else {
    Write-Host "Some tests failed." -ForegroundColor Red
    exit 1
}
