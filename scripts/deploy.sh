#!/usr/bin/env bash
# =============================================================================
# VoidDB — One-Command Deploy Script
# =============================================================================
# Deploys VoidDB to a fresh Linux server in a single command:
#
#   curl -sSL https://raw.githubusercontent.com/voiddb/void/main/scripts/deploy.sh | bash
#
# With domain + auto-HTTPS via Caddy:
#
#   curl -sSL https://raw.githubusercontent.com/voiddb/void/main/scripts/deploy.sh | bash -s -- \
#     --domain db.example.com
#
# Flags:
#   --domain DOMAIN    Enable HTTPS via Caddy reverse proxy
#   --port   PORT      API port (default: 7700)
#   --dir    PATH      Install directory (default: /opt/voiddb)
# =============================================================================
set -euo pipefail

# ── Colours ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'
info()    { echo -e "${CYAN}[INFO]${NC} $*"; }
ok()      { echo -e "${GREEN}[OK]${NC}   $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC} $*"; }
fail()    { echo -e "${RED}[ERR]${NC}  $*" >&2; exit 1; }

# ── Defaults ──────────────────────────────────────────────────────────────────
DOMAIN=""
PORT=7700
INSTALL_DIR="/opt/voiddb"
REPO="https://github.com/voiddb/void.git"

# ── Argument parsing ──────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case $1 in
    --domain) DOMAIN="$2"; shift ;;
    --port)   PORT="$2"; shift ;;
    --dir)    INSTALL_DIR="$2"; shift ;;
    -h|--help)
      echo "Usage: deploy.sh [--domain DOMAIN] [--port PORT] [--dir PATH]"
      exit 0 ;;
    *) warn "Unknown arg: $1" ;;
  esac
  shift
done

# ── Word list for random credentials ─────────────────────────────────────────
WORDS=(
  alpha bravo charlie delta echo foxtrot golf hotel india juliet kilo lima
  mike nova oscar papa quebec romeo sierra tango ultra victor whiskey xray
  yankee zulu amber azure blaze cedar coral dusk ember flame frost glow
  haven ivory jade karma lotus maple nexus oasis pearl quartz raven sage
  spark terra umbra vivid wren xenon yield zenith arctic breeze cliff dawn
  eagle flare grove hydra ivory jewel knight lunar mango north onyx prism
  quest ridge solar titan unity vapor willow binary cipher delta enigma
  falcon gamma helix ignite jolt kinetic laser matrix nebula optic photon
  radiant swift turbo vertex warp quantum
)
WORD_COUNT=${#WORDS[@]}

rand_word() { echo "${WORDS[$((RANDOM % WORD_COUNT))]}"; }

gen_passphrase() {
  echo "$(rand_word)-$(rand_word)-$(rand_word)-$(rand_word)"
}

# ── Banner ────────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}╔══════════════════════════════════════╗${NC}"
echo -e "${BOLD}║   VoidDB One-Command Deploy          ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════╝${NC}"
echo ""

# ── 1. System checks ─────────────────────────────────────────────────────────
info "Checking system..."
[[ $EUID -eq 0 ]] || fail "Run as root: sudo bash deploy.sh ..."
command -v git &>/dev/null || { info "Installing git..."; apt-get update -qq && apt-get install -y -qq git; }
ok "git ready"

# ── 2. Install Go if missing ─────────────────────────────────────────────────
if ! command -v go &>/dev/null; then
  info "Installing Go 1.22..."
  GO_TAR="go1.22.5.linux-amd64.tar.gz"
  curl -fsSL "https://go.dev/dl/$GO_TAR" -o "/tmp/$GO_TAR"
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "/tmp/$GO_TAR"
  export PATH="/usr/local/go/bin:$PATH"
  echo 'export PATH="/usr/local/go/bin:$PATH"' >> /etc/profile.d/go.sh
  rm -f "/tmp/$GO_TAR"
  ok "Go $(go version | awk '{print $3}') installed"
else
  ok "Go $(go version | awk '{print $3}') found"
fi

# ── 3. Clone / update repository ─────────────────────────────────────────────
if [[ -d "$INSTALL_DIR/.git" ]]; then
  info "Updating existing installation..."
  cd "$INSTALL_DIR"
  git pull --ff-only || warn "git pull failed, using existing"
else
  info "Cloning VoidDB to $INSTALL_DIR..."
  git clone --depth 1 "$REPO" "$INSTALL_DIR"
  cd "$INSTALL_DIR"
fi
ok "Source ready at $INSTALL_DIR"

# ── 4. Build binaries ────────────────────────────────────────────────────────
info "Building VoidDB..."
export GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}"
GIT_DESC=$(git describe --tags --always 2>/dev/null || echo "dev")
CGO_ENABLED=0 go build -mod=mod \
  -ldflags="-s -w -X main.version=$GIT_DESC" \
  -o "$INSTALL_DIR/voiddb" ./cmd/voiddb
ok "voiddb binary built"

if [[ -d "$INSTALL_DIR/cmd/voidcli" ]]; then
  CGO_ENABLED=0 go build -mod=mod \
    -ldflags="-s -w" \
    -o "$INSTALL_DIR/voidcli" ./cmd/voidcli
  ok "voidcli binary built"
fi

# ── 5. Generate credentials ──────────────────────────────────────────────────
ADMIN_USER="$(gen_passphrase)"
ADMIN_PASS="$(gen_passphrase)"
JWT_SECRET="$(head -c 48 /dev/urandom | base64 | tr -d '=/+' | head -c 48)"

ok "Generated admin credentials"

# ── 6. Create directories ────────────────────────────────────────────────────
DATA_DIR="$INSTALL_DIR/data"
BLOB_DIR="$INSTALL_DIR/blob"
BACKUP_DIR="$INSTALL_DIR/backups"
LOG_DIR="$INSTALL_DIR/logs"

mkdir -p "$DATA_DIR" "$BLOB_DIR" "$BACKUP_DIR" "$LOG_DIR"
ok "Directories created"

# ── 7. Write configuration ───────────────────────────────────────────────────
cat > "$INSTALL_DIR/.env" <<EOF
VOID_HOST=0.0.0.0
VOID_PORT=$PORT
VOID_DATA_DIR=$DATA_DIR
VOID_BLOB_DIR=$BLOB_DIR
VOID_JWT_SECRET=$JWT_SECRET
VOID_ADMIN_USER=$ADMIN_USER
VOID_ADMIN_PASSWORD=$ADMIN_PASS
VOID_LOG_LEVEL=info
EOF

cat > "$INSTALL_DIR/config.yaml" <<EOF
server:
  host: "0.0.0.0"
  port: $PORT
  read_timeout: "30s"
  write_timeout: "60s"
  cors_origins: ["*"]

engine:
  data_dir: "$DATA_DIR"
  memtable_size: 67108864
  block_cache_size: 268435456
  bloom_false_positive_rate: 0.01
  compaction_workers: 2
  sync_wal: false
  max_levels: 7
  level_size_multiplier: 10

auth:
  jwt_secret: "$JWT_SECRET"
  token_expiry: "24h"
  refresh_expiry: "168h"
  admin_user: "$ADMIN_USER"
  admin_password: "$ADMIN_PASS"

blob:
  storage_dir: "$BLOB_DIR"
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

backup:
  dir: "$BACKUP_DIR"
  retain: 14
EOF
ok "Configuration written"

# ── 8. Install systemd service ────────────────────────────────────────────────
info "Setting up systemd service..."
cat > /etc/systemd/system/voiddb.service <<EOF
[Unit]
Description=VoidDB High-Performance Document Database
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/voiddb -config $INSTALL_DIR/config.yaml
Restart=on-failure
RestartSec=5s
EnvironmentFile=-$INSTALL_DIR/.env
NoNewPrivileges=yes
PrivateTmp=yes
LimitNOFILE=65536
StandardOutput=journal
StandardError=journal
SyslogIdentifier=voiddb

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable voiddb
systemctl restart voiddb
sleep 2

if systemctl is-active --quiet voiddb; then
  ok "VoidDB service started"
else
  warn "Service may not have started — check: journalctl -u voiddb"
fi

# ── 9. Domain + Caddy (optional) ─────────────────────────────────────────────
if [[ -n "$DOMAIN" ]]; then
  info "Setting up Caddy for $DOMAIN..."

  if ! command -v caddy &>/dev/null; then
    # Install Caddy
    apt-get install -y -qq debian-keyring debian-archive-keyring apt-transport-https curl 2>/dev/null || true
    curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg 2>/dev/null
    curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | tee /etc/apt/sources.list.d/caddy-stable.list >/dev/null
    apt-get update -qq
    apt-get install -y -qq caddy
    ok "Caddy installed"
  fi

  cat > /etc/caddy/Caddyfile <<EOF
$DOMAIN {
    # VoidDB API
    reverse_proxy localhost:$PORT

    # Enable compression
    encode gzip zstd

    # Security headers
    header {
        X-Content-Type-Options nosniff
        X-Frame-Options DENY
        Referrer-Policy strict-origin-when-cross-origin
    }
}
EOF

  systemctl enable caddy
  systemctl restart caddy
  sleep 2

  if systemctl is-active --quiet caddy; then
    ok "Caddy started — SSL will be provisioned automatically"
  else
    warn "Caddy may not have started — check: journalctl -u caddy"
  fi
fi

# ── 10. Build admin panel (if Node/Bun available) ────────────────────────────
ADMIN_URL="http://localhost:$PORT"
if [[ -n "$DOMAIN" ]]; then
  ADMIN_URL="https://$DOMAIN"
fi

if command -v node &>/dev/null || command -v bun &>/dev/null; then
  if [[ -d "$INSTALL_DIR/admin" ]]; then
    info "Building admin panel..."
    cd "$INSTALL_DIR/admin"
    echo "NEXT_PUBLIC_API_URL=$ADMIN_URL" > .env.local

    PKG="npm"
    command -v bun &>/dev/null && PKG="bun"
    $PKG install --silent 2>/dev/null || $PKG install
    $PKG run build 2>/dev/null
    ok "Admin panel built"
    cd "$INSTALL_DIR"
  fi
else
  warn "Node.js/Bun not found — admin panel not built"
  info "Install Node.js: curl -fsSL https://deb.nodesource.com/setup_20.x | bash - && apt-get install -y nodejs"
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}${BOLD}╔════════════════════════════════════════╗${NC}"
echo -e "${GREEN}${BOLD}║    VoidDB Deployed Successfully!       ║${NC}"
echo -e "${GREEN}${BOLD}╠════════════════════════════════════════╣${NC}"
if [[ -n "$DOMAIN" ]]; then
echo -e "${GREEN}${BOLD}║${NC}  URL:    ${CYAN}https://$DOMAIN${NC}"
else
echo -e "${GREEN}${BOLD}║${NC}  URL:    ${CYAN}http://localhost:$PORT${NC}"
fi
echo -e "${GREEN}${BOLD}║${NC}  Login:  ${YELLOW}$ADMIN_USER${NC}"
echo -e "${GREEN}${BOLD}║${NC}  Pass:   ${YELLOW}$ADMIN_PASS${NC}"
echo -e "${GREEN}${BOLD}╚════════════════════════════════════════╝${NC}"
echo ""
echo -e "  ${BOLD}Save these credentials!${NC} They are stored in:"
echo -e "  ${CYAN}$INSTALL_DIR/.env${NC}"
echo ""
echo -e "  ${BOLD}Management commands:${NC}"
echo "    sudo systemctl status voiddb"
echo "    sudo systemctl restart voiddb"
echo "    sudo journalctl -u voiddb -f"
if [[ -n "$DOMAIN" ]]; then
echo "    sudo systemctl status caddy"
fi
echo ""
