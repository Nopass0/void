#!/usr/bin/env bash
# =============================================================================
# VoidDB -- Start server (Linux/macOS)
# =============================================================================
# Usage:
#   ./scripts/run.sh [--dev] [--config path] [--with-admin] [--admin-only]
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

DEV_MODE=false
CONFIG="$ROOT_DIR/config.yaml"
WITH_ADMIN=false
ADMIN_ONLY=false
ADMIN_PROD=false
SRV_PID=""
ADMIN_PID=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dev) DEV_MODE=true ;;
    --config) CONFIG="$2"; shift ;;
    --with-admin) WITH_ADMIN=true ;;
    --admin-only) ADMIN_ONLY=true ;;
    --admin-prod) ADMIN_PROD=true; WITH_ADMIN=true ;;
    *) ;;
  esac
  shift
done

if [[ -f "$ROOT_DIR/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT_DIR/.env"
  set +a
fi

PORT="${VOID_PORT:-7700}"
PKG=""
if command -v bun >/dev/null 2>&1; then
  PKG="bun"
elif command -v npm >/dev/null 2>&1; then
  PKG="npm"
fi

port_pid() {
  local port="$1"

  if command -v lsof >/dev/null 2>&1; then
    lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null | head -1
    return 0
  fi

  if command -v ss >/dev/null 2>&1; then
    ss -tlnp 2>/dev/null | awk -v p=":$port" '
      $4 ~ p {
        if (match($0, /pid=([0-9]+)/, a)) {
          print a[1]
          exit
        }
      }
    '
  fi
}

needs_build() {
  [[ ! -f "$ROOT_DIR/voiddb" ]] && return 0
  find "$ROOT_DIR/cmd" "$ROOT_DIR/internal" -name "*.go" -newer "$ROOT_DIR/voiddb" -print -quit 2>/dev/null | grep -q .
}

ensure_binaries() {
  if ! needs_build && [[ -f "$ROOT_DIR/voidcli" ]]; then
    return
  fi

  echo "[run] Building VoidDB..."
  (
    cd "$ROOT_DIR"
    export GOPROXY="${GOPROXY:-https://goproxy.cn,https://goproxy.io,direct}"
    export GONOSUMDB="${GONOSUMDB:-*}"
    CGO_ENABLED=0 go build -mod=mod -o "$ROOT_DIR/voiddb" ./cmd/voiddb
    CGO_ENABLED=0 go build -mod=mod -o "$ROOT_DIR/voidcli" ./cmd/voidcli
  )
  echo "[run] Build OK"
}

start_admin() {
  local admin_dir="$ROOT_DIR/admin"

  if [[ -z "$PKG" ]]; then
    echo "[admin] No Node/Bun found"
    return 0
  fi

  if [[ ! -d "$admin_dir" ]]; then
    echo "[admin] Admin directory not found: $admin_dir"
    return 0
  fi

  pushd "$admin_dir" >/dev/null
  if [[ ! -d node_modules ]]; then
    echo "[admin] Installing deps..."
    "$PKG" install
  fi

  printf 'NEXT_PUBLIC_API_URL=http://localhost:%s\n' "$PORT" > .env.local
  echo "[admin] Starting admin panel at http://localhost:3000"
  if $ADMIN_PROD; then
    [[ -d .next ]] || "$PKG" run build
    "$PKG" run start
  else
    "$PKG" run dev
  fi
  popd >/dev/null
}

start_admin_bg() {
  local admin_dir="$ROOT_DIR/admin"

  if [[ -z "$PKG" ]]; then
    echo "[admin] No Node/Bun found"
    return 0
  fi

  if [[ ! -d "$admin_dir" ]]; then
    echo "[admin] Admin directory not found: $admin_dir"
    return 0
  fi

  (
    cd "$admin_dir"
    if [[ ! -d node_modules ]]; then
      echo "[admin] Installing deps..."
      "$PKG" install
    fi

    printf 'NEXT_PUBLIC_API_URL=http://localhost:%s\n' "$PORT" > .env.local
    echo "[admin] Starting admin panel at http://localhost:3000"
    if $ADMIN_PROD; then
      [[ -d .next ]] || "$PKG" run build
      "$PKG" run start
    else
      "$PKG" run dev
    fi
  ) &
  ADMIN_PID=$!
  echo "[admin] PID $ADMIN_PID"
}

start_server_bg() {
  "$ROOT_DIR/voiddb" -config "$CONFIG" &
  SRV_PID=$!
  echo "[run] Server PID $SRV_PID at http://localhost:$PORT"
}

cleanup_children() {
  kill ${SRV_PID:-} ${ADMIN_PID:-} 2>/dev/null || true
}

if ! $ADMIN_ONLY; then
  EXISTING_PID="$(port_pid "$PORT" || true)"
  if [[ -n "$EXISTING_PID" ]]; then
    EXISTING_NAME="$(ps -p "$EXISTING_PID" -o comm= 2>/dev/null || echo "unknown")"
    echo ""
    echo "[run] Port $PORT already in use by: $EXISTING_NAME (PID $EXISTING_PID)"
    echo "  [1] Stop the existing process and restart (default)"
    echo "  [2] Leave it running (skip server start)"
    echo "  [3] Exit"
    echo ""
    read -rp "  Choice [1]: " CHOICE || CHOICE="1"
    CHOICE="${CHOICE:-1}"

    if [[ "$CHOICE" == "3" ]]; then
      exit 0
    fi

    if [[ "$CHOICE" == "2" ]]; then
      echo "[run] Server already running at http://localhost:$PORT"
      if $WITH_ADMIN; then
        start_admin
      fi
      exit 0
    fi

    echo "[run] Stopping PID $EXISTING_PID..."
    kill "$EXISTING_PID" 2>/dev/null || true
    sleep 1
    echo "[run] Stopped."
  fi
fi

if $ADMIN_ONLY; then
  start_admin
  exit 0
fi

ensure_binaries

if $DEV_MODE; then
  echo "[run] Starting VoidDB in DEV mode..."
  if $WITH_ADMIN; then
    start_admin_bg
    trap cleanup_children EXIT INT TERM
  fi

  if command -v air >/dev/null 2>&1; then
    (
      cd "$ROOT_DIR"
      if [[ -f .air.toml ]]; then
        air -c .air.toml
      else
        air
      fi
    )
  else
    echo "[run] 'air' not found -- running normally"
    "$ROOT_DIR/voiddb" -config "$CONFIG"
  fi
  exit 0
fi

if $WITH_ADMIN; then
  echo "[run] Starting VoidDB in background..."
  start_server_bg
  sleep 1
  start_admin_bg
  echo "[run] Press Ctrl+C to stop everything"
  trap cleanup_children EXIT INT TERM
  wait "$SRV_PID"
  exit 0
fi

if [[ -n "$PKG" && -z "${VOID_NO_PROMPT:-}" ]]; then
  echo ""
  echo "[run] VoidDB server starting at http://localhost:$PORT"
  echo ""
  echo "  Also start admin panel?"
  echo "    [1] No - server only (default)"
  echo "    [2] Yes - dev mode"
  echo "    [3] Yes - production"
  echo ""
  read -rp "  Choice [1]: " CHOICE || CHOICE="1"
  CHOICE="${CHOICE:-1}"

  if [[ "$CHOICE" == "2" || "$CHOICE" == "3" ]]; then
    if [[ "$CHOICE" == "3" ]]; then
      ADMIN_PROD=true
    fi
    start_server_bg
    sleep 1
    start_admin_bg
    trap cleanup_children EXIT INT TERM
    wait "$SRV_PID"
    exit 0
  fi
fi

echo "[run] Starting VoidDB at http://localhost:$PORT"
exec "$ROOT_DIR/voiddb" -config "$CONFIG"
