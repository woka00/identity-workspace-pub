#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
NAME="${1:-avatar-id-release.zip}"
[[ "$NAME" =~ ^[A-Za-z0-9._-]+\.zip$ ]] || { echo 'Некорректное имя ZIP.' >&2; exit 2; }
DEST="$(dirname "$ROOT_DIR")/$NAME"
cd "$ROOT_DIR"
./deploy/create-release-manifest.sh
rm -f "$DEST" "$DEST.sha256"
zip -q -r "$DEST" . \
  -x '.git/*' '.env' 'secrets/*' 'backups/*' 'SECURITY_TEST_RESULTS/*' \
     'frontend/node_modules/*' 'frontend/dist/*' \
     'desktop/node_modules/*' 'desktop/dist/*' \
     '*.tsbuildinfo' '*.log' '*.zip'
sha256sum "$DEST" > "$DEST.sha256"
printf 'Release: %s\nChecksum: %s\n' "$DEST" "$DEST.sha256"
