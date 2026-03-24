#!/usr/bin/env bash
# =============================================================================
# VoidDB — Start server (Linux/macOS)
# =============================================================================
# Usage:
#   ./scripts/run.sh [--dev] [--config path/to/config.yaml]
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

DEV_MODE=false
CONFIG="$ROOT_DIR/config.yaml"

while [[ $# -gt 0 ]]; do
  case $1 in
    --dev)     DEV_MODE=true ;;
    --config)  CONFIG="$2"; shift ;;
    *) ;;
  esac
  shift
done

# Source environment.
[[ -f "$ROOT_DIR/.env" ]] && source "$ROOT_DIR/.env" 2>/dev/null || true

cd "$ROOT_DIR"

# Auto-build if binary is missing or source is newer.
if [[ ! -f "$ROOT_DIR/voiddb" ]] || \
   find ./cmd ./internal -name "*.go" -newer "$ROOT_DIR/voiddb" 2>/dev/null | grep -q .; then
  echo "[run] Building VoidDB…"
  CGO_ENABLED=0 go build -o "$ROOT_DIR/voiddb" ./cmd/voiddb
fi

if [[ "$DEV_MODE" == "true" ]]; then
  echo "[run] Starting VoidDB in DEV mode (live rebuild on file change)…"
  # Requires 'air' (go install github.com/air-verse/air@latest)
  if command -v air &>/dev/null; then
    air -c .air.toml 2>/dev/null || air
  else
    echo "[run] 'air' not found — running normally"
    exec "$ROOT_DIR/voiddb" -config "$CONFIG"
  fi
else
  echo "[run] Starting VoidDB…"
  exec "$ROOT_DIR/voiddb" -config "$CONFIG"
fi
