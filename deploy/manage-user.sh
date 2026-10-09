#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
COMPOSE=(docker compose -f "$ROOT_DIR/docker-compose.yml" -f "$ROOT_DIR/docker-compose.prod.yml")
ADMIN_COMPOSE=(docker compose -f "$ROOT_DIR/docker-compose.yml" -f "$ROOT_DIR/docker-compose.prod.yml" -f "$ROOT_DIR/docker-compose.admin.yml")
ACTION="${1:-}"
LOGIN="${2:-}"

usage() {
  echo "Usage: $0 {set-password|enable-user|disable-user|grant-admin|revoke-admin} avatar01" >&2
  exit 2
}
[[ "$ACTION" =~ ^(set-password|enable-user|disable-user|grant-admin|revoke-admin)$ ]] || usage
[[ "$LOGIN" =~ ^avatar(0[1-9]|1[0-5])$ ]] || { echo 'Допустимы только avatar01 ... avatar15' >&2; exit 2; }
cd "$ROOT_DIR"
./deploy/check-production-files.sh >/dev/null

"${COMPOSE[@]}" up -d db

if [[ "$ACTION" == 'set-password' ]]; then
  [[ -t 0 ]] || { echo 'Для безопасного ввода пароля требуется интерактивный терминал.' >&2; exit 1; }
  read -rsp "Новый пароль $LOGIN (не менее 15 символов): " password; echo
  read -rsp 'Повторите пароль: ' confirm; echo
  [[ "$password" == "$confirm" ]] || { echo 'Пароли не совпадают.' >&2; exit 1; }
  (( ${#password} >= 15 )) || { echo 'Пароль должен содержать не менее 15 символов.' >&2; exit 1; }
  printf '%s' "$password" > secrets/admin_password
  unset password confirm
  chmod 600 secrets/admin_password
  chown 10001:10001 secrets/admin_password
  trap 'shred -u "$ROOT_DIR/secrets/admin_password" 2>/dev/null || rm -f "$ROOT_DIR/secrets/admin_password"' EXIT
  "${ADMIN_COMPOSE[@]}" run --rm app set-password "$LOGIN"
else
  "${COMPOSE[@]}" run --rm app "$ACTION" "$LOGIN"
fi
