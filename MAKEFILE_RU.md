# Makefile AVATAR.ID

Этот Makefile предназначен только для запуска и управления самим AVATAR.ID. Он **не**:

- устанавливает или переустанавливает Docker;
- выполняет `apt upgrade`;
- меняет UFW, SSH, Fail2ban, Nginx, Caddy, Apache или Traefik;
- получает TLS-сертификаты;
- перезапускает системные службы;
- удаляет Docker volumes;
- управляет контейнерами других Compose-проектов.

Compose project name задаётся в production-файле `.env` через `COMPOSE_PROJECT_NAME`. На VPS с другими проектами используйте уникальное значение, например `avatar-id-prod`, и отдельный свободный `APP_PORT`, например `18080`.

## Начальная проверка

```bash
make doctor
make check
```

`doctor` только проверяет Docker/Compose и доступ текущего пользователя к Docker daemon. `check` проверяет `.env`, secret-файлы и итоговую Compose-конфигурацию.

## Первый запуск

```bash
make deploy
```

Порядок действий:

1. сборка image приложения;
2. запуск PostgreSQL;
3. применение только новых миграций;
4. проверка production-паролей;
5. запуск контейнеров;
6. ожидание успешного `/healthz`.

Makefile не настраивает домен и reverse proxy. Nginx/Caddy/Traefik должен проксировать домен на `127.0.0.1:APP_PORT`.

## Обычное обновление

```bash
make update
```

Команда один раз проверяет production-конфигурацию, собирает новый image без остановки работающего приложения, создаёт и проверяет свежий backup, применяет новые миграции вместе с security gate и обновляет только контейнер приложения. Контейнер PostgreSQL не перезапускается, его volume не удаляется.

## Управление

```bash
make up                 # запустить
make stop               # остановить контейнеры, не удаляя их
make down               # удалить контейнеры/сеть, сохранить volume
make restart            # перезапустить
make recreate           # пересоздать контейнеры без удаления данных
make ps                  # состояние контейнеров
make status              # состояние + /healthz
make logs                # логи app
make logs SERVICE=db     # логи PostgreSQL
make logs TAIL=500       # другое количество строк
make app-shell           # shell в app
make db-shell            # psql
make docker-usage        # посмотреть расход диска Docker
make docker-clean        # безопасно убрать старые артефакты сборок
```

`make down` намеренно не содержит `-v`. Команд, удаляющих PostgreSQL volume, в Makefile нет.

## Очистка диска Docker

```bash
make docker-usage
make docker-clean
make docker-clean DOCKER_PRUNE_UNTIL=720h
```

По умолчанию удаляются только остановленные контейнеры и потерявшие тег images
текущего production Compose-проекта, а также неиспользуемый build cache старше
7 дней. Запущенные контейнеры, используемые и тегированные images, сети и все
Docker volumes сохраняются. Build cache общий для Docker daemon: его очистка
не влияет на работающие приложения, но следующая сборка другого проекта может
занять больше времени.

## Резервные копии

По умолчанию backup хранится в `./backups`:

```bash
make backup
make backup BACKUP_DIR=/srv/backups/avatar-id
make backup-prune BACKUP_DIR=/srv/backups/avatar-id BACKUP_RETENTION_DAYS=30
```

Восстановление требует явного подтверждения:

```bash
CONFIRM_RESTORE=RESTORE_AVATAR_ID \
make restore FILE=/srv/backups/avatar-id/avatar-id-YYYYMMDDTHHMMSSZ.dump
```

Перед восстановлением автоматически создаётся backup текущего состояния.

## Пользователи и интеграции

```bash
make set-password LOGIN=avatar01
make enable-user LOGIN=avatar02
make disable-user LOGIN=avatar15
make grant-admin LOGIN=avatar01
make revoke-admin LOGIN=avatar02
make security-status
make configure-integrations
```

## Проверки и релиз

```bash
make test
make security
make verify URL=https://avatar.example.com
make manifest
make release RELEASE_NAME=avatar-id-release.zip
```

## Development

```bash
make dev-build
make dev-logs
make dev-stop
make dev-down
```

Development и production используют разные наборы Compose-файлов, но оба сохраняют volume базы при остановке.
