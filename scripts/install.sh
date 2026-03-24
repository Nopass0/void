#!/usr/bin/env bash
# =============================================================================
# VoidDB — Linux/macOS installer & launcher
# =============================================================================
# Usage:
#   chmod +x scripts/install.sh
#   ./scripts/install.sh [--no-build] [--data-dir /path] [--port 7700]
#
# Environment overrides:
#   VOID_PORT, VOID_DATA_DIR, VOID_JWT_SECRET, VOID_ADMIN_PASSWORD
# =============================================================================
set -euo pipefail

# ── Colours ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

info()    { echo -e "${CYAN}[INFO]${NC} $*"; }
success() { echo -e "${GREEN}[OK]${NC}   $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC} $*"; }
error()   { echo -e "${RED}[ERR]${NC}  $*"; exit 1; }

# ── Defaults ──────────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
NO_BUILD=false
INSTALL_SERVICE=false

# ── Argument parsing ──────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case $1 in
    --no-build)         NO_BUILD=true ;;
    --install-service)  INSTALL_SERVICE=true ;;
    --data-dir)         export VOID_DATA_DIR="$2"; shift ;;
    --port)             export VOID_PORT="$2"; shift ;;
    --domain)           export VOID_DOMAIN="$2"; shift ;;
    --email)            export VOID_ACME_EMAIL="$2"; shift ;;
    -h|--help)
      echo "Usage: $0 [--no-build] [--install-service] [--data-dir DIR] [--port PORT] [--domain DOMAIN] [--email EMAIL]"
      exit 0 ;;
    *) warn "Unknown argument: $1" ;;
  esac
  shift
done

echo ""
echo -e "${BOLD}╔══════════════════════════════════════╗${NC}"
echo -e "${BOLD}║    VoidDB Installer  (Linux/macOS)   ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════╝${NC}"
echo ""

cd "$ROOT_DIR"

# ── System checks ─────────────────────────────────────────────────────────────
info "Checking system requirements…"

check_cmd() {
  command -v "$1" &>/dev/null || error "$1 is required but not found. Please install it first."
}

check_cmd go
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
info "Go version: $GO_VERSION"
[[ "${GO_VERSION%%.*}" -ge 1 && "${GO_VERSION#*.}" -ge 22 ]] 2>/dev/null || \
  warn "Go 1.22+ recommended (found $GO_VERSION)"

# ── Directories ───────────────────────────────────────────────────────────────
DATA_DIR="${VOID_DATA_DIR:-$ROOT_DIR/data}"
BLOB_DIR="${VOID_BLOB_DIR:-$ROOT_DIR/blob}"
BACKUP_DIR="${VOID_BACKUP_DIR:-$ROOT_DIR/backups}"
LOG_DIR="${VOID_LOG_DIR:-$ROOT_DIR/logs}"

for d in "$DATA_DIR" "$BLOB_DIR" "$BACKUP_DIR" "$LOG_DIR"; do
  mkdir -p "$d"
  success "Directory ready: $d"
done

# ── .env file ─────────────────────────────────────────────────────────────────
ENV_FILE="$ROOT_DIR/.env"
if [[ ! -f "$ENV_FILE" ]]; then
  info "Creating .env from .env.example…"
  cp "$ROOT_DIR/.env.example" "$ENV_FILE"

  # Generate a random JWT secret.
  JWT_SECRET=$(tr -dc 'A-Za-z0-9!@#$%^&*' </dev/urandom | head -c 48 2>/dev/null || \
               python3 -c "import secrets; print(secrets.token_urlsafe(36))" 2>/dev/null || \
               cat /proc/sys/kernel/random/uuid 2>/dev/null | tr -d '-')
  sed -i.bak "s|change-me-to-a-random-32-char-string|$JWT_SECRET|g" "$ENV_FILE" && rm -f "$ENV_FILE.bak"
  success ".env created with random JWT secret"
else
  warn ".env already exists — skipping"
fi

# Source .env
set -o allexport
source "$ENV_FILE" 2>/dev/null || true
set +o allexport

# ── config.yaml ───────────────────────────────────────────────────────────────
if [[ ! -f "$ROOT_DIR/config.yaml" ]]; then
  cp "$ROOT_DIR/.env.example" "$ROOT_DIR/.env.example.bak" 2>/dev/null || true
fi

# ── Go dependencies ───────────────────────────────────────────────────────────
info "Downloading Go modules..."
export GOPROXY="${GOPROXY:-https://goproxy.cn,https://goproxy.io,direct}"
export GONOSUMDB="${GONOSUMDB:-*}"
go mod download
success "Go modules ready"

# ── Build ─────────────────────────────────────────────────────────────────────
if [[ "$NO_BUILD" == "false" ]]; then
  info "Building VoidDB binary..."
  CGO_ENABLED=0 go build -mod=mod \
    -ldflags="-s -w -X main.version=$(git describe --tags --always 2>/dev/null || echo dev)" \
    -o "$ROOT_DIR/voiddb" \
    ./cmd/voiddb
  success "Binary built: $ROOT_DIR/voiddb"
fi

# ── systemd service (optional) ────────────────────────────────────────────────
if [[ "$INSTALL_SERVICE" == "true" ]]; then
  if command -v systemctl &>/dev/null; then
    info "Installing systemd service…"
    cat > /tmp/voiddb.service <<EOF
[Unit]
Description=VoidDB — High-performance document database
After=network.target

[Service]
Type=simple
User=${SUDO_USER:-$USER}
WorkingDirectory=$ROOT_DIR
ExecStart=$ROOT_DIR/voiddb -config $ROOT_DIR/config.yaml
Restart=on-failure
RestartSec=5
StandardOutput=append:$LOG_DIR/voiddb.log
StandardError=append:$LOG_DIR/voiddb.error.log
EnvironmentFile=$ENV_FILE

[Install]
WantedBy=multi-user.target
EOF
    sudo mv /tmp/voiddb.service /etc/systemd/system/voiddb.service
    sudo systemctl daemon-reload
    sudo systemctl enable voiddb
    sudo systemctl start voiddb
    success "systemd service installed and started"
    info "Manage with: sudo systemctl {start|stop|restart|status} voiddb"
  else
    warn "systemd not available — skipping service install"
  fi
fi

# ── Summary ───────────────────────────────────────────────────────────────────
VOID_PORT="${VOID_PORT:-7700}"
echo ""
echo -e "${GREEN}${BOLD}✓ VoidDB is ready!${NC}"
echo ""
echo -e "  Binary  : ${CYAN}$ROOT_DIR/voiddb${NC}"
echo -e "  Data    : ${CYAN}$DATA_DIR${NC}"
echo -e "  Blob    : ${CYAN}$BLOB_DIR${NC}"
echo -e "  API     : ${CYAN}http://localhost:$VOID_PORT${NC}"
echo -e "  Health  : ${CYAN}http://localhost:$VOID_PORT/health${NC}"
echo ""
echo -e "  Start   : ${BOLD}./scripts/run.sh${NC}"
echo -e "  Backup  : ${BOLD}./scripts/backup.sh${NC}"
echo -e "  Test    : ${BOLD}./scripts/test.sh${NC}"
echo ""
