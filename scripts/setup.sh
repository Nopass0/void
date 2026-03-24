#!/usr/bin/env bash
# =============================================================================
# VoidDB Interactive Setup Wizard — Linux / macOS
# =============================================================================
# Usage:
#   chmod +x scripts/setup.sh && ./scripts/setup.sh
#   ./scripts/setup.sh --silent   # use all defaults, no prompts
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SILENT=false

for arg in "$@"; do
  case $arg in
    --silent|-s) SILENT=true ;;
  esac
done

# ── Colours ───────────────────────────────────────────────────────────────────
R='\033[0;31m'; G='\033[0;32m'; Y='\033[1;33m'
C='\033[0;36m'; W='\033[1;37m'; D='\033[0;90m'; NC='\033[0m'
step()   { echo -e "${C}  [>>]${NC} $*"; }
ok()     { echo -e "${G}  [OK]${NC} $*"; }
warn()   { echo -e "${Y}  [!!]${NC} $*"; }
info()   { echo -e "${D}       $*${NC}"; }
fail()   { echo -e "${R}  [XX]${NC} $*" >&2; exit 1; }
header() { echo -e "\n${C}+--------------------------------------------------+${NC}"; \
           echo -e "${W}   $*${NC}"; \
           echo -e "${C}+--------------------------------------------------+${NC}"; }

ask() {
  local prompt="$1" default="${2:-}" ans
  if $SILENT; then echo "$default"; return; fi
  [[ -n $default ]] && prompt="$prompt [$default]"
  read -rp "  $prompt: " ans
  echo "${ans:-$default}"
}

ask_secret() {
  local prompt="$1" default="${2:-}" ans
  if $SILENT; then echo "$default"; return; fi
  [[ -n $default ]] && prompt="$prompt [keep current]"
  read -rsp "  $prompt: " ans; echo ""
  echo "${ans:-$default}"
}

ask_yn() {
  local prompt="$1" default="${2:-n}"
  if $SILENT; then [[ $default == y ]]; return; fi
  local hint; [[ $default == y ]] && hint="Y/n" || hint="y/N"
  read -rp "  $prompt [$hint]: " ans
  ans="${ans:-$default}"
  [[ ${ans,,} == y* ]]
}

# ── Banner ────────────────────────────────────────────────────────────────────
clear
echo -e "\n${C}  ======================================================${NC}"
echo -e "${W}         V O I D D B   S E T U P   W I Z A R D       ${NC}"
echo -e "${C}  ======================================================${NC}"
echo -e "${D}  High-performance LSM-tree document database${NC}\n"

# ── Step 1: Prerequisites ─────────────────────────────────────────────────────
header "Step 1: Checking prerequisites"

command -v go   >/dev/null 2>&1 && ok "Go: $(go version)" || fail "Go not found. Install: https://go.dev/dl/"
HAS_BUN=false; HAS_NODE=false
command -v bun  >/dev/null 2>&1 && { HAS_BUN=true;  ok  "Bun: $(bun --version)"; } || true
command -v node >/dev/null 2>&1 && { HAS_NODE=true; ok  "Node: $(node --version)"; } || true
$HAS_BUN || $HAS_NODE || warn "Bun/Node not found. Admin panel unavailable."
command -v git  >/dev/null 2>&1 && ok "Git found" || warn "Git not found"

# Detect OS for service installer.
OS="linux"
[[ "$(uname)" == "Darwin" ]] && OS="macos"
ok "OS: $OS"

# ── Step 2: Server config ─────────────────────────────────────────────────────
header "Step 2: Server configuration"

API_PORT=$(ask "API port" "7700")
DATA_DIR=$(ask "Data directory" "$ROOT/data")
BLOB_DIR=$(ask "Blob directory" "$ROOT/blob")
BACKUP_DIR=$(ask "Backup directory" "$ROOT/backups")

# ── Step 3: Security ──────────────────────────────────────────────────────────
header "Step 3: Security"

EXISTING_SECRET=""
[[ -f "$ROOT/.env" ]] && EXISTING_SECRET=$(grep "^VOID_JWT_SECRET=" "$ROOT/.env" | cut -d= -f2-)
[[ -z $EXISTING_SECRET ]] && EXISTING_SECRET=$(LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 48 || true)

JWT_SECRET=$(ask_secret "JWT secret (auto-generated if empty)" "$EXISTING_SECRET")
[[ -z $JWT_SECRET ]] && JWT_SECRET=$(LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 48)
ADMIN_PASS=$(ask_secret "Admin password" "admin")
warn "Change the admin password before exposing to the internet!"

# ── Step 4: TLS ───────────────────────────────────────────────────────────────
header "Step 4: TLS / Domain (optional — off | file | acme)"
TLS_MODE=$(ask "TLS mode" "off")
DOMAIN="" ACME_EMAIL="" CERT_FILE="" KEY_FILE=""

if [[ $TLS_MODE == "acme" ]]; then
  DOMAIN=$(ask "Your domain (e.g. void.example.com)" "")
  ACME_EMAIL=$(ask "Let's Encrypt email" "")
elif [[ $TLS_MODE == "file" ]]; then
  DOMAIN=$(ask "Your domain" "")
  CERT_FILE=$(ask "Certificate PEM path" "")
  KEY_FILE=$(ask "Private key PEM path" "")
fi

# ── Step 5: Write config ──────────────────────────────────────────────────────
header "Step 5: Writing configuration"

mkdir -p "$DATA_DIR" "$BLOB_DIR" "$BACKUP_DIR" "$ROOT/logs"

cat > "$ROOT/.env" <<EOF
VOID_HOST=0.0.0.0
VOID_PORT=$API_PORT
VOID_DATA_DIR=$DATA_DIR
VOID_BLOB_DIR=$BLOB_DIR
VOID_JWT_SECRET=$JWT_SECRET
VOID_ADMIN_PASSWORD=$ADMIN_PASS
VOID_LOG_LEVEL=info
NEXT_PUBLIC_API_URL=http://localhost:$API_PORT
EOF
[[ $TLS_MODE != "off" ]] && echo "VOID_TLS_MODE=$TLS_MODE"     >> "$ROOT/.env"
[[ -n $DOMAIN      ]]    && echo "VOID_DOMAIN=$DOMAIN"          >> "$ROOT/.env"
[[ -n $ACME_EMAIL  ]]    && echo "VOID_ACME_EMAIL=$ACME_EMAIL"  >> "$ROOT/.env"
[[ -n $CERT_FILE   ]]    && echo "VOID_TLS_CERT=$CERT_FILE"     >> "$ROOT/.env"
[[ -n $KEY_FILE    ]]    && echo "VOID_TLS_KEY=$KEY_FILE"       >> "$ROOT/.env"
ok ".env written"

cat > "$ROOT/config.yaml" <<EOF
server:
  host: "0.0.0.0"
  port: $API_PORT
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

tls:
  mode: "$TLS_MODE"
  cert_file: "$CERT_FILE"
  key_file:  "$KEY_FILE"
  domain:    "$DOMAIN"
  acme_email: "$ACME_EMAIL"
  redirect_http: $([ "$TLS_MODE" != "off" ] && echo "true" || echo "false")
  http_src_port: 80
  https_port: 443

backup:
  dir: "$BACKUP_DIR"
  retain: 14
EOF
ok "config.yaml written"

# ── Step 6: Build binaries ────────────────────────────────────────────────────
header "Step 6: Building binaries"

cd "$ROOT"
export GOPROXY="${GOPROXY:-https://goproxy.cn,https://goproxy.io,direct}"
export GONOSUMDB="${GONOSUMDB:-*}"
GIT_DESC=$(git describe --tags --always 2>/dev/null || echo "dev")

step "Building voiddb..."
CGO_ENABLED=0 go build -mod=mod \
  -ldflags="-s -w -X main.version=$GIT_DESC" \
  -o "$ROOT/voiddb" ./cmd/voiddb
ok "voiddb built"

step "Building voidcli..."
CGO_ENABLED=0 go build -mod=mod \
  -ldflags="-s -w -X main.version=$GIT_DESC" \
  -o "$ROOT/voidcli" ./cmd/voidcli
ok "voidcli built"

# ── Step 7: Install CLI to PATH ───────────────────────────────────────────────
header "Step 7: Installing CLI commands to PATH"

INSTALL_DIR=""
if ask_yn "Install voiddb + voidcli to /usr/local/bin (requires sudo)?" "n"; then
  INSTALL_DIR="/usr/local/bin"
  sudo cp "$ROOT/voiddb"  "$INSTALL_DIR/voiddb"
  sudo cp "$ROOT/voidcli" "$INSTALL_DIR/voidcli"
  sudo chmod +x "$INSTALL_DIR/voiddb" "$INSTALL_DIR/voidcli"
  ok "Installed to $INSTALL_DIR"
elif ask_yn "Install to ~/.local/bin (no sudo, must be in PATH)?" "y"; then
  INSTALL_DIR="$HOME/.local/bin"
  mkdir -p "$INSTALL_DIR"
  cp "$ROOT/voiddb"  "$INSTALL_DIR/voiddb"
  cp "$ROOT/voidcli" "$INSTALL_DIR/voidcli"
  chmod +x "$INSTALL_DIR/voiddb" "$INSTALL_DIR/voidcli"
  ok "Installed to $INSTALL_DIR"
  # Add to shell rc if not already there.
  for RC in "$HOME/.bashrc" "$HOME/.zshrc"; do
    if [[ -f $RC ]] && ! grep -q "$INSTALL_DIR" "$RC"; then
      echo "export PATH=\"\$PATH:$INSTALL_DIR\"" >> "$RC"
      info "Added $INSTALL_DIR to $RC"
    fi
  done
else
  info "Skipped. Run manually: $ROOT/voiddb and $ROOT/voidcli"
fi

# ── Step 8: System service (autostart) ───────────────────────────────────────
header "Step 8: System service (autostart on boot)"

INSTALLED_SVC=false
BIN_PATH="${INSTALL_DIR:-$ROOT}/voiddb"

if [[ $OS == "linux" ]] && command -v systemctl >/dev/null 2>&1; then
  if ask_yn "Install as systemd service (autostart)?" "y"; then
    SVC_FILE="/etc/systemd/system/voiddb.service"
    cat > /tmp/voiddb.service <<EOF
[Unit]
Description=VoidDB High-Performance Document Database
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=$(whoami)
WorkingDirectory=$ROOT
ExecStart=$ROOT/voiddb -config $ROOT/config.yaml
Restart=on-failure
RestartSec=5s
EnvironmentFile=-$ROOT/.env
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=$DATA_DIR $BLOB_DIR $BACKUP_DIR $ROOT/logs
LimitNOFILE=65536
StandardOutput=journal
StandardError=journal
SyslogIdentifier=voiddb

[Install]
WantedBy=multi-user.target
EOF
    sudo mv /tmp/voiddb.service "$SVC_FILE"
    sudo systemctl daemon-reload
    sudo systemctl enable  voiddb
    sudo systemctl restart voiddb
    INSTALLED_SVC=true
    ok "Service enabled and started (sudo systemctl status voiddb)"
  fi
elif [[ $OS == "macos" ]]; then
  PLIST="$HOME/Library/LaunchAgents/com.voiddb.server.plist"
  if ask_yn "Install as launchd service (autostart on login)?" "y"; then
    cat > "$PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>             <string>com.voiddb.server</string>
  <key>ProgramArguments</key>  <array><string>$ROOT/voiddb</string><string>-config</string><string>$ROOT/config.yaml</string></array>
  <key>WorkingDirectory</key>  <string>$ROOT</string>
  <key>RunAtLoad</key>         <true/>
  <key>KeepAlive</key>         <true/>
  <key>StandardOutPath</key>   <string>$ROOT/logs/voiddb.log</string>
  <key>StandardErrorPath</key> <string>$ROOT/logs/voiddb.error.log</string>
</dict>
</plist>
EOF
    launchctl unload "$PLIST" 2>/dev/null || true
    launchctl load   "$PLIST"
    INSTALLED_SVC=true
    ok "LaunchAgent installed and started"
  fi
else
  warn "Autostart setup not supported on this OS. Start manually."
fi

# ── Step 9: Admin panel ───────────────────────────────────────────────────────
header "Step 9: Admin panel"

if ! $HAS_BUN && ! $HAS_NODE; then
  warn "No Node.js/Bun — skipping admin panel"
else
  if ask_yn "Set up the admin panel?" "y"; then
    cd "$ROOT/admin"
    cat > .env.local <<EOF
NEXT_PUBLIC_API_URL=http://localhost:$API_PORT
EOF
    step "Installing admin dependencies..."
    if $HAS_BUN; then bun install; else npm install; fi
    ok "Dependencies installed"

    echo ""
    echo "  Admin panel mode:"
    echo "    [1] Development (hot-reload, port 3000)"
    echo "    [2] Production  (build + start)"
    MODE=$(ask "Choose" "1")

    if [[ $MODE == "2" ]]; then
      step "Building production admin panel..."
      if $HAS_BUN; then bun run build; else npm run build; fi
      ok "Built. Start with: cd admin && bun start"
      if ask_yn "Start production admin now?" "y"; then
        if $HAS_BUN; then bun run start & else npm run start & fi
        ADMIN_PID=$!
        ok "Admin panel started (pid $ADMIN_PID) at http://localhost:3000"
      fi
    else
      if ask_yn "Start admin panel in dev mode now?" "y"; then
        if $HAS_BUN; then bun run dev & else npm run dev & fi
        ADMIN_PID=$!
        ok "Dev admin started (pid $ADMIN_PID) at http://localhost:3000"
      else
        info "Start later: cd admin && bun dev"
      fi
    fi
    cd "$ROOT"
  fi
fi

# ── Start server ──────────────────────────────────────────────────────────────
if ! $INSTALLED_SVC; then
  if ask_yn "Start VoidDB server now?" "y"; then
    step "Starting VoidDB..."
    nohup "$ROOT/voiddb" -config "$ROOT/config.yaml" \
      > "$ROOT/logs/voiddb.log" 2>&1 &
    SERVER_PID=$!
    sleep 1
    if kill -0 $SERVER_PID 2>/dev/null; then
      ok "VoidDB started (pid $SERVER_PID)"
    else
      warn "Server may have failed - check $ROOT/logs/voiddb.log"
    fi
  fi
fi

# ── Summary ───────────────────────────────────────────────────────────────────
header "Setup complete!"

PROTO="http"; [[ $TLS_MODE != "off" ]] && PROTO="https"
HOST_DISPLAY="${DOMAIN:-localhost}"

echo ""
echo -e "${G}  VoidDB is ready!${NC}"
echo ""
echo -e "${C}  API      : $PROTO://$HOST_DISPLAY:$API_PORT${NC}"
echo -e "${C}  Health   : $PROTO://$HOST_DISPLAY:$API_PORT/health${NC}"
echo -e "${C}  Admin UI : http://localhost:3000${NC}"
echo ""
echo -e "${W}  CLI commands:${NC}"
echo "    voidcli status"
echo "    voidcli login"
echo "    voidcli db list"
echo "    voidcli db create myapp"
echo "    voidcli col create myapp users"
echo "    voidcli doc insert myapp users '{\"name\":\"Alice\"}'"
echo ""
echo -e "${W}  Scripts:${NC}"
echo "    ./scripts/run.sh           -- start server"
echo "    ./scripts/backup.sh backup -- backup all databases"
echo "    ./scripts/test.sh          -- run tests"
echo ""
