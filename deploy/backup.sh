#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST_DIR="${1:-$ROOT_DIR/backups}"
COMPOSE=(docker compose -f "$ROOT_DIR/docker-compose.yml" -f "$ROOT_DIR/docker-compose.prod.yml")
TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"
BACKUP="$DEST_DIR/avatar-id-$TIMESTAMP.dump"

mkdir -p "$DEST_DIR"
chmod 700 "$DEST_DIR"

# Custom format supports pg_restore inspection and selective restore. pg_dump
# creates a transactionally consistent snapshot while the service is online.
"${COMPOSE[@]}" exec -T db sh -ec \
  'exec pg_dump --format=custom --compress=6 --no-owner --no-privileges --username="$POSTGRES_USER" --dbname="$POSTGRES_DB"' \
  > "$BACKUP"

if [[ ! -s "$BACKUP" ]]; then
  echo "Backup is empty: $BACKUP" >&2
  rm -f "$BACKUP"
  exit 1
fi

"${COMPOSE[@]}" exec -T db pg_restore --list < "$BACKUP" >/dev/null
sha256sum "$BACKUP" > "$BACKUP.sha256"
chmod 600 "$BACKUP" "$BACKUP.sha256"
printf 'Backup created and structurally checked: %s\n' "$BACKUP"
printf 'Important: test a full restore into a disposable database before production.\n'
