#!/usr/bin/env bash
# =============================================================================
# VoidDB — Backup & Restore (Linux/macOS)
# =============================================================================
# Usage:
#   ./scripts/backup.sh backup [--db mydb] [--out /path/to/file.void]
#   ./scripts/backup.sh restore /path/to/file.void [--db mydb]
#   ./scripts/backup.sh list
#   ./scripts/backup.sh schedule  (add cron job, daily at 02:00)
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

[[ -f "$ROOT_DIR/.env" ]] && source "$ROOT_DIR/.env" 2>/dev/null || true

VOID_URL="${VOID_URL:-http://localhost:${VOID_PORT:-7700}}"
BACKUP_DIR="${VOID_BACKUP_DIR:-$ROOT_DIR/backups}"
TOKEN_FILE="$ROOT_DIR/.void_token"

# ── Colours ───────────────────────────────────────────────────────────────────
GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[1;33m'; NC='\033[0m'
info()    { echo -e "${CYAN}[backup]${NC} $*"; }
success() { echo -e "${GREEN}[ok]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[warn]${NC}  $*"; }

mkdir -p "$BACKUP_DIR"

# ── Auth helper ───────────────────────────────────────────────────────────────
get_token() {
  if [[ -f "$TOKEN_FILE" ]]; then
    cat "$TOKEN_FILE"
    return
  fi
  ADMIN_PASS="${VOID_ADMIN_PASSWORD:-admin}"
  TOKEN=$(curl -sf -X POST "$VOID_URL/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"admin\",\"password\":\"$ADMIN_PASS\"}" | \
    python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])" 2>/dev/null || \
    grep -o '"access_token":"[^"]*"' | cut -d'"' -f4)
  echo "$TOKEN" > "$TOKEN_FILE"
  chmod 600 "$TOKEN_FILE"
  echo "$TOKEN"
}

# ── BACKUP ────────────────────────────────────────────────────────────────────
do_backup() {
  local db_filter=""
  local out_file=""

  while [[ $# -gt 0 ]]; do
    case $1 in
      --db)  db_filter="$2"; shift ;;
      --out) out_file="$2"; shift ;;
    esac
    shift
  done

  TIMESTAMP=$(date +%Y%m%d_%H%M%S)
  if [[ -z "$out_file" ]]; then
    out_file="$BACKUP_DIR/voiddb_${db_filter:-all}_${TIMESTAMP}.void"
  fi

  TOKEN=$(get_token)
  info "Starting backup → $out_file"

  # Collect databases to back up.
  DBS=$(curl -sf "$VOID_URL/v1/databases" \
    -H "Authorization: Bearer $TOKEN" | \
    python3 -c "import sys,json; [print(d) for d in json.load(sys.stdin).get('databases',[])]" 2>/dev/null)

  if [[ -n "$db_filter" ]]; then
    DBS=$(echo "$DBS" | grep "^$db_filter$" || true)
    [[ -z "$DBS" ]] && { warn "Database '$db_filter' not found"; exit 1; }
  fi

  TMP_DIR=$(mktemp -d)
  trap "rm -rf $TMP_DIR" EXIT

  # Write .void archive (tar + gzip of JSON exports)
  for db in $DBS; do
    info "  Exporting database: $db"
    DB_DIR="$TMP_DIR/$db"
    mkdir -p "$DB_DIR"

    # Export each collection.
    COLS=$(curl -sf "$VOID_URL/v1/databases/$db/collections" \
      -H "Authorization: Bearer $TOKEN" | \
      python3 -c "import sys,json; [print(c) for c in json.load(sys.stdin).get('collections',[])]" 2>/dev/null)

    for col in $COLS; do
      info "    Collection: $col"
      # Query all documents (no limit → paginate).
      PAGE=0
      PAGE_SIZE=1000
      COL_FILE="$DB_DIR/${col}.json"
      echo '{"collection":"'"$col"'","documents":[' > "$COL_FILE"
      FIRST=true
      while true; do
        SKIP=$((PAGE * PAGE_SIZE))
        RESULT=$(curl -sf -X POST "$VOID_URL/v1/databases/$db/$col/query" \
          -H "Authorization: Bearer $TOKEN" \
          -H "Content-Type: application/json" \
          -d "{\"limit\":$PAGE_SIZE,\"skip\":$SKIP}")
        COUNT=$(echo "$RESULT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('results',[])))" 2>/dev/null || echo 0)
        if [[ "$COUNT" -eq 0 ]]; then break; fi

        DOCS=$(echo "$RESULT" | python3 -c "import sys,json; d=json.load(sys.stdin); [print((',' if i>0 else '')+json.dumps(doc)) for i,doc in enumerate(d.get('results',[]))]" 2>/dev/null)
        [[ "$FIRST" == "false" ]] && echo "," >> "$COL_FILE"
        echo -n "$DOCS" >> "$COL_FILE"
        FIRST=false
        [[ "$COUNT" -lt "$PAGE_SIZE" ]] && break
        PAGE=$((PAGE + 1))
      done
      echo ']}' >> "$COL_FILE"
      success "    Exported $col"
    done

    # Export blob objects for this db (if bucket exists).
    BLOB_BUCKET="$db"
    BLOB_OBJS=$(curl -sf "$VOID_URL/s3/$BLOB_BUCKET" \
      -H "Authorization: Bearer $TOKEN" 2>/dev/null | \
      grep -o '<Key>[^<]*</Key>' | sed 's/<[^>]*>//g' || true)
    if [[ -n "$BLOB_OBJS" ]]; then
      mkdir -p "$DB_DIR/_blobs"
      while IFS= read -r key; do
        [[ -z "$key" ]] && continue
        curl -sf "$VOID_URL/s3/$BLOB_BUCKET/$key" \
          -H "Authorization: Bearer $TOKEN" \
          -o "$DB_DIR/_blobs/$key" 2>/dev/null || true
      done <<< "$BLOB_OBJS"
    fi
  done

  # Write manifest.
  cat > "$TMP_DIR/manifest.json" <<EOF
{
  "void_version": "1.0.0",
  "created_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "databases": [$(echo "$DBS" | python3 -c "import sys; dbs=[l.strip() for l in sys.stdin if l.strip()]; print(','.join('\"'+d+'\"' for d in dbs))" 2>/dev/null)],
  "format": "void-backup-v1"
}
EOF

  # Package into .void file (tar.gz with .void extension).
  tar -czf "$out_file" -C "$TMP_DIR" .
  local size
  size=$(du -sh "$out_file" | cut -f1)
  success "Backup complete: $out_file ($size)"
}

# ── RESTORE ───────────────────────────────────────────────────────────────────
do_restore() {
  local in_file="$1"; shift
  local db_filter=""
  while [[ $# -gt 0 ]]; do
    case $1 in --db) db_filter="$2"; shift ;; esac
    shift
  done

  [[ ! -f "$in_file" ]] && { echo "File not found: $in_file"; exit 1; }

  TOKEN=$(get_token)
  TMP_DIR=$(mktemp -d)
  trap "rm -rf $TMP_DIR" EXIT

  info "Extracting backup: $in_file"
  tar -xzf "$in_file" -C "$TMP_DIR"

  MANIFEST="$TMP_DIR/manifest.json"
  [[ -f "$MANIFEST" ]] && cat "$MANIFEST"

  for db_dir in "$TMP_DIR"/*/; do
    db=$(basename "$db_dir")
    [[ "$db" == "manifest.json" ]] && continue
    [[ -n "$db_filter" && "$db" != "$db_filter" ]] && continue

    info "Restoring database: $db"
    curl -sf -X POST "$VOID_URL/v1/databases" \
      -H "Authorization: Bearer $TOKEN" \
      -H "Content-Type: application/json" \
      -d "{\"name\":\"$db\"}" >/dev/null 2>&1 || true

    for col_file in "$db_dir"*.json; do
      [[ ! -f "$col_file" ]] && continue
      col=$(basename "$col_file" .json)
      info "  Restoring collection: $col"
      curl -sf -X POST "$VOID_URL/v1/databases/$db/collections" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{\"name\":\"$col\"}" >/dev/null 2>&1 || true

      # Re-insert documents.
      python3 - "$col_file" <<'PYEOF'
import json, sys, urllib.request, os

col_file = sys.argv[1]
token = open(os.path.expanduser('~/.void_token') if os.path.exists(os.path.expanduser('~/.void_token')) else '/tmp/.void_token').read().strip() if False else os.environ.get('VOID_TOKEN','')
void_url = os.environ.get('VOID_URL', 'http://localhost:7700')

with open(col_file) as f:
    data = json.load(f)

col_name = data['collection']
db_name = os.path.basename(os.path.dirname(col_file))

for doc in data.get('documents', []):
    doc_id = doc.get('_id')
    payload = {k: v for k, v in doc.items() if k != '_id'}
    if doc_id:
        payload['_id'] = doc_id
    req = urllib.request.Request(
        f'{void_url}/v1/databases/{db_name}/{col_name}',
        data=json.dumps(payload).encode(),
        headers={'Content-Type': 'application/json', 'Authorization': f'Bearer {token}'},
        method='POST'
    )
    try:
        urllib.request.urlopen(req)
    except Exception as e:
        print(f'  warn: {e}', file=sys.stderr)
print(f'  Restored {len(data.get("documents",[]))} documents into {col_name}')
PYEOF
    done

    # Restore blobs.
    if [[ -d "$db_dir/_blobs" ]]; then
      for blob_file in "$db_dir/_blobs/"*; do
        [[ ! -f "$blob_file" ]] && continue
        key=$(basename "$blob_file")
        curl -sf -X PUT "$VOID_URL/s3/$db/$key" \
          -H "Authorization: Bearer $TOKEN" \
          -H "Content-Type: application/octet-stream" \
          --data-binary "@$blob_file" >/dev/null 2>&1 || true
      done
    fi
    success "Restored: $db"
  done
}

# ── LIST ──────────────────────────────────────────────────────────────────────
do_list() {
  info "Backups in $BACKUP_DIR:"
  if ls "$BACKUP_DIR"/*.void 2>/dev/null | head -20 | while read -r f; do
    size=$(du -sh "$f" | cut -f1)
    date=$(stat -c %y "$f" 2>/dev/null || stat -f %Sm "$f" 2>/dev/null || echo "unknown")
    echo "  $(basename "$f")  [$size]  $date"
  done; then true; else info "No backups found"; fi
}

# ── SCHEDULE ──────────────────────────────────────────────────────────────────
do_schedule() {
  CRON_CMD="0 2 * * * $ROOT_DIR/scripts/backup.sh backup >> $ROOT_DIR/logs/backup.log 2>&1"
  (crontab -l 2>/dev/null | grep -v "backup.sh"; echo "$CRON_CMD") | crontab -
  success "Daily backup scheduled at 02:00 (cron)"
  info "View with: crontab -l"
}

# ── Dispatch ──────────────────────────────────────────────────────────────────
CMD="${1:-backup}"; shift || true
case "$CMD" in
  backup)   do_backup "$@" ;;
  restore)  do_restore "$@" ;;
  list)     do_list ;;
  schedule) do_schedule ;;
  *)        echo "Usage: $0 {backup|restore|list|schedule} [options]"; exit 1 ;;
esac
