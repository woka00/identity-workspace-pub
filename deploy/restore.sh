#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
BACKUP="${1:-}"
COMPOSE=(docker compose -f "$ROOT_DIR/docker-compose.yml" -f "$ROOT_DIR/docker-compose.prod.yml")

[[ -n "$BACKUP" && -f "$BACKUP" && ! -L "$BACKUP" ]] || { echo 'Укажите существующий обычный .dump файл.' >&2; exit 2; }
[[ "${CONFIRM_RESTORE:-}" == 'RESTORE_AVATAR_ID' ]] || {
  echo 'Восстановление перезапишет текущую БД.' >&2
  echo 'Повторите с CONFIRM_RESTORE=RESTORE_AVATAR_ID.' >&2
  exit 2
}
cd "$ROOT_DIR"
./deploy/check-production-files.sh >/dev/null

if [[ -f "$BACKUP.sha256" ]]; then
  (cd "$(dirname "$BACKUP")" && sha256sum -c "$(basename "$BACKUP.sha256")")
fi
"${COMPOSE[@]}" up -d db
"${COMPOSE[@]}" exec -T db pg_restore --list < "$BACKUP" >/dev/null

printf 'Создаётся обязательный backup текущего состояния перед restore...\n'
./deploy/backup.sh "${BACKUP_DIR:-$ROOT_DIR/backups}"
"${COMPOSE[@]}" stop app >/dev/null 2>&1 || true
trap '"${COMPOSE[@]}" up -d app >/dev/null 2>&1 || true' EXIT

"${COMPOSE[@]}" exec -T db pg_restore \
  --clean --if-exists --exit-on-error --no-owner --no-privileges \
  --username=avatar --dbname=avatarid < "$BACKUP"
"${COMPOSE[@]}" run --rm app migrate
"${COMPOSE[@]}" up -d app
trap - EXIT
printf 'Restore завершён. Выполните make verify URL=https://ваш-домен\n'
