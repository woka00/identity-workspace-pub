#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
failures=0
run() {
  printf '\n== %s ==\n' "$*"
  if "$@"; then
    printf '[PASS]\n'
  else
    printf '[FAIL]\n' >&2
    failures=$((failures + 1))
  fi
}
optional() {
  local tool="$1"; shift
  if command -v "$tool" >/dev/null 2>&1; then
    run "$@"
  else
    printf '\n[SKIP] %s is not installed\n' "$tool"
  fi
}

cd "$ROOT_DIR/backend"
run bash -c 'test -z "$(gofmt -l .)"'
run go test ./internal/...
run go vet ./internal/...
run go test -race ./internal/...
run go test ./...
run go vet ./...
run go build ./...
optional govulncheck govulncheck ./...
optional gosec gosec ./...
optional staticcheck staticcheck ./...

cd "$ROOT_DIR/frontend"
run npm ci --ignore-scripts --no-fund
run npm run build
run npm audit --audit-level=high

cd "$ROOT_DIR"
if command -v docker >/dev/null 2>&1; then
  run docker compose -f docker-compose.yml -f docker-compose.prod.yml config --quiet
  optional trivy trivy config --exit-code 1 --severity HIGH,CRITICAL .
  optional trivy trivy fs --exit-code 1 --severity HIGH,CRITICAL --scanners vuln,secret .
  optional hadolint hadolint Dockerfile
else
  printf '\n[SKIP] docker is not installed\n'
fi

if find . -path './.git' -prune -o -path './frontend/node_modules' -prune -o \
  -path './desktop/node_modules' -prune -o \
  -type f \( -name '.env' -o -name '*.pem' -o -name '*.key' -o -name 'id_rsa*' \) -print | grep -q .; then
  printf '[FAIL] Potential secret/private-key files found in project tree\n' >&2
  failures=$((failures + 1))
else
  printf '\n[PASS] No .env/private-key files in project tree\n'
fi

if grep -RIlE --exclude-dir=.git --exclude-dir=node_modules --exclude='*.md' \
  -- '-----BEGIN ([A-Z0-9 ]+ )?PRIVATE KEY-----' . | grep -q .; then
  printf '[FAIL] Private-key material found in project contents\n' >&2
  failures=$((failures + 1))
else
  printf '[PASS] No private-key PEM markers in project contents\n'
fi

if find . -path './.git' -prune -o -path './frontend/node_modules' -prune -o \
  -path './desktop/node_modules' -prune -o \
  -type f -perm -0002 -print | grep -q .; then
  printf '[FAIL] World-writable files found in project tree\n' >&2
  failures=$((failures + 1))
else
  printf '[PASS] No world-writable project files\n'
fi

optional shellcheck shellcheck deploy/*.sh

if (( failures > 0 )); then
  printf '\n%d check group(s) failed. Review every failure before deployment.\n' "$failures" >&2
  exit 1
fi
printf '\nAll available security checks passed.\n'
