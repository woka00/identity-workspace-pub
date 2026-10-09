#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"
mkdir -p secrets
chmod 700 secrets
[[ -t 0 ]] || { echo 'Нужен интерактивный терминал.' >&2; exit 1; }

write_secret() {
  local file="$1" label="$2" hidden="${3:-false}" value
  if [[ -s "$file" ]]; then
    read -rp "$label уже заполнен. Заменить? [y/N]: " answer
    [[ "$answer" =~ ^[Yy]$ ]] || return 0
  fi
  if [[ "$hidden" == true ]]; then
    read -rsp "$label (Enter — оставить пустым): " value; echo
  else
    read -rp "$label (Enter — оставить пустым): " value
  fi
  printf '%s' "$value" > "$file"
  unset value
  chmod 600 "$file"
  chown 10001:10001 "$file"
}

write_secret secrets/fatsecret_consumer_key 'FatSecret Consumer Key'
write_secret secrets/fatsecret_consumer_secret 'FatSecret Consumer Secret' true

./deploy/check-production-files.sh
printf 'Интеграции сохранены. Для применения: make prod-restart\n'
