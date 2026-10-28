#!/bin/sh
# pm-backup: consistent backup of a PrintMarshal data directory.
#
# Produces <OUT_DIR>/printmarshal-backup-<UTC stamp>.tar.gz (mode 0600) containing
#   printmarshal.db   consistent snapshot of the SQLite database
#   secret.key        the credential encryption key (when the server manages it)
#   firmware/         uploaded firmware packages (when present)
#   printmarshal.json the config file passed with -c (optional)
#   MANIFEST          what was taken, how, and from where
#
# The database is copied with the SQLite online-backup API (`sqlite3 .backup`),
# which is safe while the server is running and WAL mode is active.  Without
# the sqlite3 CLI the script refuses unless --cold is given, in which case the
# server MUST be stopped and the db + -wal + -shm files are copied verbatim.
#
# THE ARCHIVE CONTAINS THE ENCRYPTION KEY.  Treat it like a password store:
# keep it off shared drives, encrypt it at rest (age/gpg), restrict who can
# read it.  If the key is supplied through PM_SECRET_KEY (platform secret)
# instead of data/secret.key, back that secret up separately -- a database
# without its key has unrecoverable device credentials.
#
# Usage: backup.sh [-d DATA_DIR] [-o OUT_DIR] [-c CONFIG_FILE] [--cold] [--keep N]
#   -d DATA_DIR    data directory (default: $PM_DATA_DIR, else /var/lib/printmarshal)
#   -o OUT_DIR     where to write the archive (default: $PM_BACKUP_DIR, else .)
#   -c FILE        include this config file in the archive
#   --cold         allow a plain file copy when sqlite3 is missing (server stopped!)
#   --keep N       delete all but the newest N archives in OUT_DIR afterwards
#
# Examples
#   systemd:   sudo -u printmarshal backup.sh -o /var/backups/printmarshal --keep 14
#   docker:    docker exec printmarshal pm-backup -o /data/backups && docker cp printmarshal:/data/backups .
#   k8s:       kubectl -n printmarshal exec deploy/printmarshal -- pm-backup -o /data/backups
#              kubectl -n printmarshal cp printmarshal/<pod>:/data/backups ./backups
set -eu

DATA_DIR="${PM_DATA_DIR:-/var/lib/printmarshal}"
OUT_DIR="${PM_BACKUP_DIR:-.}"
CONFIG=""
COLD=0
KEEP=0

usage() { sed -n '2,36p' "$0" | sed 's/^# \{0,1\}//'; }
die() { echo "backup: $*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    -d) DATA_DIR=$2; shift 2 ;;
    -o) OUT_DIR=$2; shift 2 ;;
    -c) CONFIG=$2; shift 2 ;;
    --cold) COLD=1; shift ;;
    --keep) KEEP=$2; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; die "unknown argument: $1" ;;
  esac
done

DB="$DATA_DIR/printmarshal.db"
[ -f "$DB" ] || die "no database at $DB (set -d or PM_DATA_DIR)"
[ -n "$CONFIG" ] && [ ! -f "$CONFIG" ] && die "config file not found: $CONFIG"
mkdir -p "$OUT_DIR"

STAMP=$(date -u +%Y%m%d-%H%M%SZ)
NAME="printmarshal-backup-$STAMP"
TMP=$(mktemp -d "${TMPDIR:-/tmp}/pm-backup.XXXXXX")
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT INT TERM
WORK="$TMP/$NAME"
mkdir -p "$WORK"
umask 077

# 1. Database
if command -v sqlite3 >/dev/null 2>&1; then
  sqlite3 "$DB" ".backup '$WORK/printmarshal.db'"
  CHECK=$(sqlite3 "$WORK/printmarshal.db" "PRAGMA integrity_check;" | head -n 1)
  [ "$CHECK" = "ok" ] || die "integrity check of the snapshot failed: $CHECK"
  METHOD="sqlite3 online backup (integrity_check ok)"
else
  [ "$COLD" -eq 1 ] || die "sqlite3 not found: install it, or stop the server and re-run with --cold"
  if command -v fuser >/dev/null 2>&1 && fuser "$DB" >/dev/null 2>&1; then
    die "database is open by another process; stop the server before a --cold copy"
  fi
  cp -p "$DB" "$WORK/"
  for suffix in -wal -shm; do
    [ -f "$DB$suffix" ] && cp -p "$DB$suffix" "$WORK/"
  done
  METHOD="cold file copy (db + wal + shm)"
fi

# 2. Encryption key
if [ -f "$DATA_DIR/secret.key" ]; then
  cp -p "$DATA_DIR/secret.key" "$WORK/"
  KEYNOTE="secret.key included"
else
  KEYNOTE="secret.key NOT present: the key is supplied via PM_SECRET_KEY -- back up that secret separately"
  echo "backup: warning: $KEYNOTE" >&2
fi

# 3. Firmware packages and optional config
[ -d "$DATA_DIR/firmware" ] && cp -Rp "$DATA_DIR/firmware" "$WORK/"
[ -n "$CONFIG" ] && cp -p "$CONFIG" "$WORK/printmarshal.json"

# 4. Manifest
{
  echo "printmarshal backup"
  echo "created:   $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "host:      $(hostname 2>/dev/null || echo unknown)"
  echo "data_dir:  $DATA_DIR"
  echo "method:    $METHOD"
  echo "key:       $KEYNOTE"
  command -v printmarshal >/dev/null 2>&1 && echo "version:   $(printmarshal -version 2>/dev/null || true)"
  echo "contents:"
  (cd "$WORK" && find . -type f ! -name MANIFEST | sort | sed 's/^/  /')
} > "$WORK/MANIFEST"

# 5. Archive
ARCHIVE="$OUT_DIR/$NAME.tar.gz"
tar -C "$TMP" -czf "$ARCHIVE" "$NAME"
chmod 600 "$ARCHIVE"

# 6. Retention
if [ "$KEEP" -gt 0 ]; then
  ls -1t "$OUT_DIR"/printmarshal-backup-*.tar.gz 2>/dev/null | tail -n +$((KEEP + 1)) | while IFS= read -r old; do
    rm -f "$old"
  done
fi

echo "backup: wrote $ARCHIVE ($METHOD)"
