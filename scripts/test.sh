#!/usr/bin/env bash
# =============================================================================
# VoidDB — Test runner (Linux/macOS)
# =============================================================================
# Usage:
#   ./scripts/test.sh [--unit] [--bench] [--e2e] [--all]
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

[[ -f "$ROOT_DIR/.env" ]] && source "$ROOT_DIR/.env" 2>/dev/null || true

GREEN='\033[0;32m'; RED='\033[0;31m'; CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'
ok()   { echo -e "${GREEN}✓${NC} $*"; }
fail() { echo -e "${RED}✗${NC} $*"; }
info() { echo -e "${CYAN}▶${NC} $*"; }

RUN_UNIT=false; RUN_BENCH=false; RUN_E2E=false
[[ $# -eq 0 ]] && RUN_UNIT=true && RUN_BENCH=true
while [[ $# -gt 0 ]]; do
  case $1 in
    --unit)  RUN_UNIT=true ;;
    --bench) RUN_BENCH=true ;;
    --e2e)   RUN_E2E=true ;;
    --all)   RUN_UNIT=true; RUN_BENCH=true; RUN_E2E=true ;;
  esac
  shift
done

cd "$ROOT_DIR"
PASS=0; FAIL=0

run_test() {
  local name="$1"; shift
  if "$@" > /tmp/void_test_out 2>&1; then
    ok "$name"
    PASS=$((PASS+1))
  else
    fail "$name"
    cat /tmp/void_test_out
    FAIL=$((FAIL+1))
  fi
}

echo ""
echo -e "${BOLD}VoidDB Test Suite${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# ── Unit tests ────────────────────────────────────────────────────────────────
if [[ "$RUN_UNIT" == "true" ]]; then
  info "Unit tests (go test ./...)"
  run_test "Go: types/values"         go test ./internal/engine/types/...    -v -count=1
  run_test "Go: storage/bloom"        go test ./internal/engine/storage/...  -v -count=1
  run_test "Go: WAL"                  go test ./internal/engine/wal/...      -v -count=1
  run_test "Go: cache/LRU"            go test ./internal/engine/cache/...    -v -count=1
  run_test "Go: engine (integration)" go test ./internal/engine/...          -v -count=1 -timeout 30s
  run_test "Go: auth"                 go test ./internal/auth/...            -v -count=1
  run_test "Go: ORM client"           go test ./orm/go/...                   -v -count=1
fi

# ── Benchmarks ────────────────────────────────────────────────────────────────
if [[ "$RUN_BENCH" == "true" ]]; then
  info "Benchmarks"
  VOID_URL="${VOID_URL:-http://localhost:7700}"

  # Check if server is running.
  if curl -sf "$VOID_URL/health" >/dev/null 2>&1; then
    info "Running VoidDB vs PostgreSQL benchmark (50k records, 4 workers)…"
    cd benchmark
    go mod download 2>/dev/null || true
    go run main.go -records 50000 -workers 4 | tee /tmp/bench_results.txt
    ok "Benchmark complete → /tmp/bench_results.txt"
    cd "$ROOT_DIR"
    PASS=$((PASS+1))
  else
    info "Server not running — running in-process Go benchmarks only"
    run_test "Go: engine benchmarks" go test ./internal/engine/... -bench=. -benchtime=5s -run='^$'
  fi
fi

# ── E2E tests ─────────────────────────────────────────────────────────────────
if [[ "$RUN_E2E" == "true" ]]; then
  VOID_URL="${VOID_URL:-http://localhost:7700}"
  info "E2E tests against $VOID_URL"

  assert_status() {
    local desc="$1"; local expected="$2"; local actual="$3"
    if [[ "$actual" == "$expected" ]]; then
      ok "E2E: $desc (HTTP $actual)"
      PASS=$((PASS+1))
    else
      fail "E2E: $desc — expected HTTP $expected, got $actual"
      FAIL=$((FAIL+1))
    fi
  }

  # Health check.
  STATUS=$(curl -so /dev/null -w "%{http_code}" "$VOID_URL/health")
  assert_status "Health check" "200" "$STATUS"

  # Login.
  LOGIN=$(curl -sf -X POST "$VOID_URL/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"admin"}' 2>/dev/null)
  TOKEN=$(echo "$LOGIN" | python3 -c "import sys,json; print(json.load(sys.stdin).get('access_token',''))" 2>/dev/null || echo "")

  if [[ -n "$TOKEN" ]]; then
    ok "E2E: Login succeeded"
    PASS=$((PASS+1))

    AUTH_H="Authorization: Bearer $TOKEN"
    CT="Content-Type: application/json"

    # Create database.
    S=$(curl -so /dev/null -w "%{http_code}" -X POST "$VOID_URL/v1/databases" \
      -H "$AUTH_H" -H "$CT" -d '{"name":"e2e_test"}')
    assert_status "Create database" "201" "$S"

    # Create collection.
    S=$(curl -so /dev/null -w "%{http_code}" -X POST "$VOID_URL/v1/databases/e2e_test/collections" \
      -H "$AUTH_H" -H "$CT" -d '{"name":"items"}')
    assert_status "Create collection" "201" "$S"

    # Insert document.
    RESP=$(curl -sf -X POST "$VOID_URL/v1/databases/e2e_test/items" \
      -H "$AUTH_H" -H "$CT" -d '{"name":"test","value":42}')
    DOC_ID=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('_id',''))" 2>/dev/null)
    [[ -n "$DOC_ID" ]] && { ok "E2E: Insert document (id=$DOC_ID)"; PASS=$((PASS+1)); } || \
      { fail "E2E: Insert document"; FAIL=$((FAIL+1)); }

    # Get document.
    if [[ -n "$DOC_ID" ]]; then
      S=$(curl -so /dev/null -w "%{http_code}" "$VOID_URL/v1/databases/e2e_test/items/$DOC_ID" \
        -H "$AUTH_H")
      assert_status "Get document" "200" "$S"

      # Patch document.
      S=$(curl -so /dev/null -w "%{http_code}" -X PATCH "$VOID_URL/v1/databases/e2e_test/items/$DOC_ID" \
        -H "$AUTH_H" -H "$CT" -d '{"value":43}')
      assert_status "Patch document" "200" "$S"

      # Query.
      S=$(curl -so /dev/null -w "%{http_code}" -X POST "$VOID_URL/v1/databases/e2e_test/items/query" \
        -H "$AUTH_H" -H "$CT" -d '{"where":[{"field":"value","op":"eq","value":43}]}')
      assert_status "Query documents" "200" "$S"

      # Delete document.
      S=$(curl -so /dev/null -w "%{http_code}" -X DELETE "$VOID_URL/v1/databases/e2e_test/items/$DOC_ID" \
        -H "$AUTH_H")
      assert_status "Delete document" "204" "$S"
    fi

    # Blob upload.
    S=$(curl -so /dev/null -w "%{http_code}" -X PUT "$VOID_URL/s3/e2e-bucket" \
      -H "$AUTH_H")
    assert_status "Create bucket" "200" "$S"

    S=$(curl -so /dev/null -w "%{http_code}" -X PUT "$VOID_URL/s3/e2e-bucket/hello.txt" \
      -H "$AUTH_H" -H "Content-Type: text/plain" --data "hello voiddb")
    assert_status "Upload blob" "200" "$S"

    S=$(curl -so /dev/null -w "%{http_code}" "$VOID_URL/s3/e2e-bucket/hello.txt" -H "$AUTH_H")
    assert_status "Download blob" "200" "$S"
  else
    fail "E2E: Login failed — is VoidDB running at $VOID_URL?"
    FAIL=$((FAIL+1))
  fi
fi

# ── TypeScript ORM tests ──────────────────────────────────────────────────────
if [[ "$RUN_UNIT" == "true" ]] && command -v node &>/dev/null; then
  info "TypeScript ORM build check"
  if command -v npm &>/dev/null; then
    cd "$ROOT_DIR/orm/typescript"
    run_test "TS ORM: npm install" npm install --prefer-offline
    run_test "TS ORM: type check"  npx tsc --noEmit 2>/dev/null || true
    cd "$ROOT_DIR"
  fi
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -e "  ${GREEN}Passed: $PASS${NC}   ${RED}Failed: $FAIL${NC}"
echo ""
[[ $FAIL -eq 0 ]] && echo -e "${GREEN}${BOLD}All tests passed ✓${NC}" || { echo -e "${RED}${BOLD}Some tests failed ✗${NC}"; exit 1; }
