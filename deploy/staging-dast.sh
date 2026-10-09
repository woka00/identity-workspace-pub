#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

BASE_URL="${1:-}"
if [[ ! "$BASE_URL" =~ ^https://[^/]+$ ]]; then
  echo "Usage: $0 https://staging-avatar.example.com" >&2
  exit 2
fi
if ! command -v docker >/dev/null 2>&1; then
  echo "Docker is required to run the OWASP ZAP container." >&2
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPORT_DIR="$ROOT_DIR/SECURITY_TEST_RESULTS/zap"
mkdir -p "$REPORT_DIR"
chmod 700 "$ROOT_DIR/SECURITY_TEST_RESULTS" "$REPORT_DIR"

# Passive/non-destructive baseline scan. Run against staging first. The report
# remains outside the release archive because SECURITY_TEST_RESULTS is ignored.
docker run --rm \
  -v "$REPORT_DIR:/zap/wrk/:rw" \
  ghcr.io/zaproxy/zaproxy:stable \
  zap-baseline.py -t "$BASE_URL" -m 5 \
  -r zap-baseline.html -J zap-baseline.json -w zap-baseline.md

printf 'ZAP baseline reports: %s\n' "$REPORT_DIR"
printf '%s\n' 'Authenticated API/IDOR tests still require a dedicated staging account and manual proxy setup.'

if [[ "${AVATAR_ALLOW_ACTIVE_ZAP:-}" == "STAGING_ONLY_I_HAVE_A_BACKUP" ]]; then
  printf '%s\n' 'Running active ZAP scan. This can send attack payloads and must never target production data.'
  docker run --rm \
    -v "$REPORT_DIR:/zap/wrk/:rw" \
    ghcr.io/zaproxy/zaproxy:stable \
    zap-full-scan.py -t "$BASE_URL" -m 10 \
    -r zap-full.html -J zap-full.json -w zap-full.md
else
  printf '%s\n' 'Active scan skipped. To run it on disposable staging only:'
  printf '%s\n' 'AVATAR_ALLOW_ACTIVE_ZAP=STAGING_ONLY_I_HAVE_A_BACKUP ./deploy/staging-dast.sh https://staging.example.com'
fi
