#!/usr/bin/env bash
# Krovara — daily Postgres backup → Hetzner Storage Box (ADR/PROD-ARCHITECTURE).
#
# Dumps the DB from the running postgres container, gzips it, ships it to a
# Storage Box over SSH/rsync, and prunes old copies (local + remote). Wire it as
# a daily cron on the VPS:
#
#   # /etc/cron.d/krovara-backup  (runs 03:17 UTC daily)
#   17 3 * * * deploy KROVARA_DIR=/opt/krovara /opt/krovara/scripts/backup-pg.sh >> /var/log/krovara-backup.log 2>&1
#
# Required env (set in the cron line or a sourced file):
#   STORAGEBOX_USER   e.g. u123456
#   STORAGEBOX_HOST   e.g. u123456.your-storagebox.de
#   STORAGEBOX_DIR    remote dir, e.g. krovara/pg   (created if missing)
# Optional:
#   KROVARA_DIR       repo dir (default /opt/krovara)
#   PG_SERVICE        compose service name (default postgres)
#   LOCAL_KEEP_DAYS   local retention (default 7)
#   REMOTE_KEEP       remote copies to keep (default 30)
#   SSH_KEY           ssh identity (default ~/.ssh/id_ed25519)
set -euo pipefail

KROVARA_DIR="${KROVARA_DIR:-/opt/krovara}"
PG_SERVICE="${PG_SERVICE:-postgres}"
LOCAL_KEEP_DAYS="${LOCAL_KEEP_DAYS:-7}"
REMOTE_KEEP="${REMOTE_KEEP:-30}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"
BACKUP_DIR="${BACKUP_DIR:-$KROVARA_DIR/backups}"

: "${STORAGEBOX_USER:?set STORAGEBOX_USER}"
: "${STORAGEBOX_HOST:?set STORAGEBOX_HOST}"
: "${STORAGEBOX_DIR:?set STORAGEBOX_DIR}"

cd "$KROVARA_DIR"
mkdir -p "$BACKUP_DIR"

# Read POSTGRES_USER/DB from the prod .env (fallback to defaults).
PGUSER="$(grep -E '^POSTGRES_USER=' .env | cut -d= -f2-)"; PGUSER="${PGUSER:-krovara}"
PGDB="$(grep -E '^POSTGRES_DB=' .env | cut -d= -f2-)"; PGDB="${PGDB:-krovara}"

STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT="$BACKUP_DIR/krovara-${STAMP}.sql.gz"

echo "==> Dumping $PGDB → $OUT"
# pg_dump runs inside the container; stream straight to a gzip on the host.
docker compose exec -T "$PG_SERVICE" pg_dump -U "$PGUSER" -d "$PGDB" --no-owner --clean --if-exists \
  | gzip -9 > "$OUT"

SIZE="$(du -h "$OUT" | cut -f1)"
echo "    wrote $SIZE"

echo "==> Shipping to Storage Box"
SSH_OPTS=(-p 23 -i "$SSH_KEY" -o StrictHostKeyChecking=accept-new)
# Storage Box exposes SFTP on port 23. Ensure the remote dir exists, then rsync.
ssh "${SSH_OPTS[@]}" "${STORAGEBOX_USER}@${STORAGEBOX_HOST}" "mkdir -p ${STORAGEBOX_DIR}" || true
rsync -e "ssh ${SSH_OPTS[*]}" -av "$OUT" \
  "${STORAGEBOX_USER}@${STORAGEBOX_HOST}:${STORAGEBOX_DIR}/"

echo "==> Pruning local backups older than ${LOCAL_KEEP_DAYS}d"
find "$BACKUP_DIR" -name 'krovara-*.sql.gz' -mtime "+${LOCAL_KEEP_DAYS}" -delete

echo "==> Pruning remote to the newest ${REMOTE_KEEP}"
# List remote dumps oldest-first, delete all but the newest REMOTE_KEEP.
ssh "${SSH_OPTS[@]}" "${STORAGEBOX_USER}@${STORAGEBOX_HOST}" \
  "ls -1t ${STORAGEBOX_DIR}/krovara-*.sql.gz 2>/dev/null | tail -n +$((REMOTE_KEEP + 1)) | xargs -r rm -f" || true

echo "==> Backup OK: $OUT"
