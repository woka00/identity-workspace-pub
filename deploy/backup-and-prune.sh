#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
DEST_DIR="${1:-/var/backups/avatar-id}"
RETENTION_DAYS="${2:-30}"
[[ "$RETENTION_DAYS" =~ ^[0-9]+$ ]] || { echo 'Retention days must be numeric.' >&2; exit 2; }

"$ROOT_DIR/deploy/backup.sh" "$DEST_DIR"
find "$DEST_DIR" -maxdepth 1 -type f \( -name 'avatar-id-*.dump' -o -name 'avatar-id-*.dump.sha256' \) \
  -mtime "+$RETENTION_DAYS" -print -delete
