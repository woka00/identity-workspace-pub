#!/usr/bin/env bash
set -Eeuo pipefail

BASE_URL="${1:-}"
if [[ ! "$BASE_URL" =~ ^https://[^/]+$ ]]; then
  echo "Usage: $0 https://avatar.example.com" >&2
  exit 2
fi
HTTP_URL="http://${BASE_URL#https://}"
failures=0

pass() { printf '[PASS] %s\n' "$1"; }
fail() { printf '[FAIL] %s\n' "$1" >&2; failures=$((failures + 1)); }

health="$(curl --fail --silent --show-error --max-time 15 "$BASE_URL/healthz" || true)"
[[ "$health" == $'ok' ]] && pass "HTTPS health endpoint" || fail "HTTPS health endpoint"

redirect_headers="$(curl --silent --show-error --max-time 15 --dump-header - --output /dev/null "$HTTP_URL/" || true)"
if grep -Eqi '^HTTP/[^ ]+ 30[178] ' <<<"$redirect_headers" && grep -Eqi "^location: ${BASE_URL//./\\.}(/|$)" <<<"$redirect_headers"; then
  pass "HTTP redirects to HTTPS"
else
  fail "HTTP does not redirect to the expected HTTPS origin"
fi

headers="$(curl --silent --show-error --max-time 15 --dump-header - --output /dev/null "$BASE_URL/" || true)"
for header in \
  'strict-transport-security:' \
  'content-security-policy:' \
  'x-content-type-options: nosniff' \
  'x-frame-options: DENY' \
  'referrer-policy:'; do
  grep -Fqi "$header" <<<"$headers" && pass "Header $header" || fail "Missing/incorrect header $header"
done

state_code="$(curl --silent --output /dev/null --write-out '%{http_code}' --max-time 15 "$BASE_URL/api/state" || true)"
[[ "$state_code" == "401" ]] && pass "Unauthenticated API is denied" || fail "Unauthenticated /api/state returned $state_code"

csrf_code="$(curl --silent --output /dev/null --write-out '%{http_code}' --max-time 15 -X POST "$BASE_URL/api/auth/logout" || true)"
[[ "$csrf_code" == "403" ]] && pass "Unsafe request without CSRF marker is denied" || fail "CSRF probe returned $csrf_code"

cors_headers="$(curl --silent --show-error --max-time 15 --dump-header - --output /dev/null \
  -X OPTIONS -H 'Origin: https://attacker.invalid' \
  -H 'Access-Control-Request-Method: POST' "$BASE_URL/api/tasks" || true)"
if grep -Eqi '^HTTP/[^ ]+ 403 ' <<<"$cors_headers" && ! grep -Fqi 'access-control-allow-origin:' <<<"$cors_headers"; then
  pass "Cross-origin preflight is denied"
else
  fail "Cross-origin preflight was not safely denied"
fi

if (( failures > 0 )); then
  printf '%d production verification check(s) failed.\n' "$failures" >&2
  exit 1
fi
printf 'All non-destructive production checks passed.\n'
