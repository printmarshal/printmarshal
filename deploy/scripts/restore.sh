#!/bin/sh
# pm-restore: restore a PrintMarshal data directory from a backup.sh archive.
#
# THE SERVER MUST BE STOPPED (systemctl stop printmarshal / docker stop /
# scale the Deployment to 0).  SQLite cannot have its file swapped underneath
# an open connection, and a stale -wal file next to a restored database would
# corrupt it, so this script also removes printmarshal.db-wal / -shm.
#
# What happens
#   1. The archive is extracted to a temp dir and the database is checked
#      (PRAGMA integrity_check) when sqlite3 is available.
#   2. Everything currently in DATA_DIR is moved to DATA_DIR/pre-restore-<stamp>/
#      (DATA_DIR itself is kept: it is often a mount point).
#   3. printmarshal.db, secret.key (if in the archive) and firmware/ are put in place.
#
# Encryption key: the restored database's credentials can only be read with the
# key they were sealed with.  If the archive has no secret.key the key was
# supplied through PM_SECRET_KEY at backup time -- make sure the same value is
# set before starting the server.  --keep-key leaves an existing DATA_DIR/secret.key
# in place instead of the archived one (use only if you know they are identical).
#
# Usage: restore.sh -f ARCHIVE [-d DATA_DIR] [--keep-key] [--yes]
#   -f ARCHIVE     printmarshal-backup-*.tar.gz produced by backup.sh
#   -d DATA_DIR    target data directory (default: $PM_DATA_DIR, else /var/lib/printmarshal)
#   --keep-key     do not overwrite an existing secret.key
#   --yes          do not ask for confirmation
#
# Run as the service user (or chown DATA_DIR afterwards):
#   sudo systemctl stop printmarshal
#   sudo -u printmarshal restore.sh -f /var/backups/printmarshal/printmarshal-backup-20260101-020000Z.tar.gz
#   sudo systemctl start printmarshal
set -eu

DATA_DIR="${PM_DATA_DIR:-/var/lib/printmarshal}"
ARCHIVE=""
KEEP_KEY=0
YES=0

usage() { sed -n '2,33p' "$0" | sed 's/^# \{0,1\}//'; }
die() { echo "restore: $*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    -f) ARCHIVE=$2; shift 2 ;;
    -d) DATA_DIR=$2; shift 2 ;;
    --keep-key) KEEP_KEY=1; shift ;;
    --yes) YES=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; die "unknown argument: $1" ;;
  esac
done

[ -n "$ARCHIVE" ] || { usage >&2; die "-f ARCHIVE is required"; }
[ -f "$ARCHIVE" ] || die "archive not found: $ARCHIVE"

DB="$DATA_DIR/printmarshal.db"
if [ -f "$DB" ] && command -v fuser >/dev/null 2>&1 && fuser "$DB" >/dev/null 2>&1; then
  die "$DB is open by another process: stop the server first"
fi

TMP=$(mktemp -d "${TMPDIR:-/tmp}/pm-restore.XXXXXX")
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT INT TERM
umask 077

tar -C "$TMP" -xzf "$ARCHIVE"
SRC=$(find "$TMP" -mindepth 1 -maxdepth 1 -type d | head -n 1)
[ -n "$SRC" ] && [ -f "$SRC/printmarshal.db" ] || die "archive does not contain printmarshal.db"

if command -v sqlite3 >/dev/null 2>&1; then
  CHECK=$(sqlite3 "$SRC/printmarshal.db" "PRAGMA integrity_check;" | head -n 1)
  [ "$CHECK" = "ok" ] || die "archived database fails integrity_check: $CHECK"
fi

HAS_KEY=0
[ -f "$SRC/secret.key" ] && HAS_KEY=1

echo "restore: archive   $ARCHIVE"
[ -f "$SRC/MANIFEST" ] && sed 's/^/restore:   /' "$SRC/MANIFEST"
echo "restore: target    $DATA_DIR"
if [ "$HAS_KEY" -eq 0 ]; then
  if [ -z "${PM_SECRET_KEY:-}" ] && [ ! -f "$DATA_DIR/secret.key" ]; then
    echo "restore: WARNING: the archive has no secret.key, PM_SECRET_KEY is not set and $DATA_DIR has no key." >&2
    echo "restore:          Stored device credentials will be UNREADABLE unless the original key is provided." >&2
  else
    echo "restore: note: archive has no secret.key; the existing key / PM_SECRET_KEY will be used."
  fi
fi
if [ "$YES" -ne 1 ]; then
  printf 'restore: existing contents of %s will be moved aside. Continue? [y/N] ' "$DATA_DIR"
  read -r answer
  case "$answer" in y|Y|yes|YES) ;; *) die "aborted" ;; esac
fi

mkdir -p "$DATA_DIR"
STAMP=$(date -u +%Y%m%d-%H%M%SZ)
ASIDE="$DATA_DIR/pre-restore-$STAMP"
mkdir -p "$ASIDE"
for entry in "$DATA_DIR"/* "$DATA_DIR"/.[!.]*; do
  [ -e "$entry" ] || continue
  case "$entry" in "$ASIDE"|"$DATA_DIR"/pre-restore-*) continue ;; esac
  if [ "$KEEP_KEY" -eq 1 ] && [ "$entry" = "$DATA_DIR/secret.key" ]; then continue; fi
  mv "$entry" "$ASIDE/"
done
# Nothing was there (fresh directory): no need to keep an empty aside folder.
rmdir "$ASIDE" 2>/dev/null && ASIDE=""

# Never leave a WAL/SHM from the previous database next to the restored one.
rm -f "$DB-wal" "$DB-shm"
cp -p "$SRC/printmarshal.db" "$DB"
for suffix in -wal -shm; do          # present only in --cold archives
  [ -f "$SRC/printmarshal.db$suffix" ] && cp -p "$SRC/printmarshal.db$suffix" "$DB$suffix"
done
if [ "$HAS_KEY" -eq 1 ] && { [ "$KEEP_KEY" -eq 0 ] || [ ! -f "$DATA_DIR/secret.key" ]; }; then
  cp -p "$SRC/secret.key" "$DATA_DIR/secret.key"
fi
[ -d "$SRC/firmware" ] && cp -Rp "$SRC/firmware" "$DATA_DIR/"
chmod 600 "$DB" 2>/dev/null || true
[ -f "$DATA_DIR/secret.key" ] && chmod 600 "$DATA_DIR/secret.key"

if [ -n "$ASIDE" ]; then
  echo "restore: done. Previous contents kept in $ASIDE (delete when satisfied)."
else
  echo "restore: done."
fi
if [ -f "$SRC/printmarshal.json" ]; then
  cp -p "$SRC/printmarshal.json" "$DATA_DIR/printmarshal.json.from-archive"
  echo "restore: the archive also carries a config file; left at $DATA_DIR/printmarshal.json.from-archive -- move it to your config location by hand if needed."
fi
echo "restore: start the server and check the log for 'printmarshal starting'."
