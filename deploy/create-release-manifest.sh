#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
find . \
  -path './.git' -prune -o \
  -path './secrets' -prune -o \
  -path './backups' -prune -o \
  -path './SECURITY_TEST_RESULTS' -prune -o \
  -path './frontend/node_modules' -prune -o \
  -path './frontend/dist' -prune -o \
  -path './desktop/node_modules' -prune -o \
  -path './desktop/dist' -prune -o \
  -type f ! -name '.env' ! -name 'RELEASE_MANIFEST_SHA256.txt' ! -name '*.zip' ! -name '*.tsbuildinfo' -print0 \
  | sort -z | xargs -0 sha256sum > "$tmp"
mv "$tmp" RELEASE_MANIFEST_SHA256.txt
chmod 644 RELEASE_MANIFEST_SHA256.txt
printf 'Manifest updated: %s files\n' "$(wc -l < RELEASE_MANIFEST_SHA256.txt)"
