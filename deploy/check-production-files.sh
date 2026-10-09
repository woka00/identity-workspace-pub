#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"

fail() { printf '[FAIL] %s\n' "$*" >&2; exit 1; }
pass() { printf '[PASS] %s\n' "$*"; }

[[ -f .env && ! -L .env ]] || fail '.env отсутствует или является symlink'
mode="$(stat -c '%a' .env)"
(( (8#$mode & 8#077) == 0 )) || fail ".env доступен группе/остальным (mode=$mode); требуется 600"

if grep -Ev '^[[:space:]]*($|#|[A-Z][A-Z0-9_]*=.*)$' .env | grep -q . \
  || grep -Eq '[`$;]' .env \
  || grep -q $'\r' .env; then
  fail '.env содержит неподдерживаемый синтаксис или shell metacharacters'
fi
get_env() {
  local key="$1" fallback="${2:-}" count value
  count="$(grep -cE "^${key}=" .env || true)"
  (( count <= 1 )) || fail "Ключ $key повторяется в .env"
  if (( count == 0 )); then
    printf '%s' "$fallback"
    return
  fi
  value="$(grep -E "^${key}=" .env | cut -d= -f2-)"
  printf '%s' "$value"
}
PUBLIC_URL="$(get_env PUBLIC_URL)"
APP_PORT="$(get_env APP_PORT 8080)"
RUN_MIGRATIONS="$(get_env RUN_MIGRATIONS false)"
COMPOSE_PROJECT_NAME="$(get_env COMPOSE_PROJECT_NAME avatar-id)"

[[ "$PUBLIC_URL" =~ ^https://[A-Za-z0-9.-]+(:[0-9]+)?$ ]] || fail 'PUBLIC_URL должен быть HTTPS origin без path/query'
[[ "$APP_PORT" =~ ^[0-9]{2,5}$ ]] || fail 'APP_PORT должен быть числом'
[[ "$RUN_MIGRATIONS" == 'false' ]] || fail 'RUN_MIGRATIONS в production должен быть false'
[[ "$COMPOSE_PROJECT_NAME" =~ ^[a-z0-9][a-z0-9_-]*$ ]] || fail 'Некорректный COMPOSE_PROJECT_NAME'

required=(
  postgres_password database_url data_encryption_key
  fatsecret_consumer_key fatsecret_consumer_secret
)
for name in "${required[@]}"; do
  file="secrets/$name"
  [[ -f "$file" && ! -L "$file" ]] || fail "$file отсутствует или является symlink"
  mode="$(stat -c '%a' "$file")"
  (( (8#$mode & 8#077) == 0 )) || fail "$file доступен группе/остальным (mode=$mode); требуется 600"
done

app_secrets=(
  database_url data_encryption_key
  fatsecret_consumer_key fatsecret_consumer_secret
)
for name in "${app_secrets[@]}"; do
  owner="$(stat -c '%u:%g' "secrets/$name")"
  [[ "$owner" == '10001:10001' ]] || fail "secrets/$name должен принадлежать UID:GID 10001:10001 (сейчас $owner)"
done

[[ -s secrets/postgres_password ]] || fail 'postgres_password пуст'
[[ -s secrets/database_url ]] || fail 'database_url пуст'
[[ -s secrets/data_encryption_key ]] || fail 'data_encryption_key пуст'

grep -Eq '^postgres://avatar:[0-9a-f]{64}@db:5432/avatarid\?sslmode=disable&connect_timeout=10$' secrets/database_url \
  || fail 'database_url имеет неожиданный формат или небезопасные preview credentials'

python3 - <<'PY' || fail 'DATA_ENCRYPTION_KEY должен быть base64 ровно для 32 байт'
import base64
from pathlib import Path
raw = Path('secrets/data_encryption_key').read_text(encoding='utf-8').strip()
try:
    value = base64.b64decode(raw, validate=True)
except Exception as exc:
    raise SystemExit(str(exc))
if len(value) != 32:
    raise SystemExit(f'expected 32 bytes, got {len(value)}')
PY

for pair in 'fatsecret_consumer_key fatsecret_consumer_secret'; do
  read -r first second <<<"$pair"
  first_size="$(wc -c < "secrets/$first")"
  second_size="$(wc -c < "secrets/$second")"
  if { (( first_size == 0 )) && (( second_size > 0 )); } || { (( first_size > 0 )) && (( second_size == 0 )); }; then
    fail "$first и $second должны быть заполнены вместе или оба оставлены пустыми"
  fi
done

if find secrets -maxdepth 1 -type f -perm /077 -print -quit | grep -q .; then
  fail 'В secrets найдены файлы с избыточными правами'
fi

pass 'production .env и secret-файлы выглядят корректно'
