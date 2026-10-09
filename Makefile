SHELL := /usr/bin/env bash
.SHELLFLAGS := -Eeuo pipefail -c
.DEFAULT_GOAL := help

ROOT_DIR := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
COMPOSE := docker compose
DEV_PROJECT ?= avatar-id-dev
DEV := $(COMPOSE) --project-name "$(DEV_PROJECT)" -f docker-compose.yml
PROD := $(COMPOSE) -f docker-compose.yml -f docker-compose.prod.yml
ADMIN := $(COMPOSE) -f docker-compose.yml -f docker-compose.prod.yml -f docker-compose.admin.yml

BACKUP_DIR ?= $(ROOT_DIR)/backups
BACKUP_RETENTION_DAYS ?= 30
DOCKER_PRUNE_UNTIL ?= 168h
URL ?=
LOGIN ?=
FILE ?=
SERVICE ?= app
TAIL ?= 200

.PHONY: help doctor \
	dev-up dev-build dev-stop dev-down dev-restart dev-logs dev-ps \
	check config build rebuild pull up start stop down restart recreate update deploy \
	logs logs-app logs-db ps status health wait migrate backup backup-prune restore docker-usage docker-clean \
	db-shell app-shell set-password enable-user disable-user grant-admin revoke-admin security-status configure-integrations verify \
	catalog-prepare catalog-prepare-auchan catalog-import test test-go test-frontend security clean verify-manifest manifest release

help: ## Показать команды управления проектом
	@awk 'BEGIN {FS = ":.*## "; printf "identity workspace — запуск и управление проектом\n\n"} /^[a-zA-Z0-9_.-]+:.*## / {printf "  %-24s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@printf '\nОсновной production-сценарий:\n'
	@printf '  make doctor\n'
	@printf '  make check\n'
	@printf '  make deploy\n'
	@printf '  make logs\n'
	@printf '\nMakefile не устанавливает пакеты, не меняет firewall/reverse proxy и не управляет чужими проектами.\n'

# ---------------------------- Environment checks ----------------------------

doctor: ## Проверить наличие Docker/Compose и доступ к Docker daemon
	@command -v docker >/dev/null 2>&1 || { echo 'Docker не установлен или отсутствует в PATH.' >&2; exit 1; }
	@docker compose version >/dev/null 2>&1 || { echo 'Docker Compose plugin недоступен.' >&2; exit 1; }
	@docker info >/dev/null 2>&1 || { echo 'Нет доступа к Docker daemon. Проверьте права текущего пользователя.' >&2; exit 1; }
	@test -f "$(ROOT_DIR)/docker-compose.yml" -a -f "$(ROOT_DIR)/docker-compose.prod.yml"
	@printf 'Docker и Compose доступны. Системные настройки не изменялись.\n'

check: doctor ## Проверить production .env, secrets и итоговую Compose-конфигурацию
	@cd "$(ROOT_DIR)" && ./deploy/check-production-files.sh
	@cd "$(ROOT_DIR)" && $(PROD) config --quiet
	@printf 'Production-конфигурация корректна.\n'

config: ## Показать итоговую production Compose-конфигурацию
	@cd "$(ROOT_DIR)" && $(PROD) config

# ---------------------------- Development ----------------------------

dev-up: doctor ## Запустить development-контейнеры
	@cd "$(ROOT_DIR)" && $(DEV) up -d

dev-build: doctor ## Собрать и запустить development-контейнеры
	@cd "$(ROOT_DIR)" && $(DEV) up -d --build

dev-stop: ## Остановить development-контейнеры без удаления контейнеров и данных
	@cd "$(ROOT_DIR)" && $(DEV) stop

dev-down: ## Удалить только контейнеры/сеть development-проекта, сохранив volume базы
	@cd "$(ROOT_DIR)" && $(DEV) down --remove-orphans

dev-restart: ## Перезапустить development-контейнеры
	@cd "$(ROOT_DIR)" && $(DEV) restart

dev-logs: ## Смотреть development-логи; SERVICE=app|db, TAIL=200
	@cd "$(ROOT_DIR)" && $(DEV) logs -f --tail="$(TAIL)" "$(SERVICE)"

dev-ps: ## Показать development-контейнеры
	@cd "$(ROOT_DIR)" && $(DEV) ps

# ---------------------------- Production lifecycle ----------------------------

pull: doctor ## Обновить только базовый образ PostgreSQL
	@cd "$(ROOT_DIR)" && $(PROD) pull db

build: check ## Собрать production image приложения с использованием кэша
	@cd "$(ROOT_DIR)" && $(PROD) build app

rebuild: check ## Полностью пересобрать production image без кэша
	@cd "$(ROOT_DIR)" && $(PROD) build --no-cache app

up start: check ## Запустить production-контейнеры в фоне
	@cd "$(ROOT_DIR)" && $(PROD) up -d --remove-orphans
	@$(MAKE) wait

stop: ## Остановить production-контейнеры без их удаления
	@cd "$(ROOT_DIR)" && $(PROD) stop

down: ## Удалить только контейнеры/сеть identity workspace, сохранив PostgreSQL volume
	@cd "$(ROOT_DIR)" && $(PROD) down --remove-orphans

restart: ## Перезапустить уже созданные production-контейнеры
	@cd "$(ROOT_DIR)" && $(PROD) restart
	@$(MAKE) wait

recreate: check ## Пересоздать контейнеры identity workspace без удаления данных
	@cd "$(ROOT_DIR)" && $(PROD) up -d --force-recreate --remove-orphans
	@$(MAKE) wait

migrate: check ## Запустить PostgreSQL и применить только новые миграции
	@cd "$(ROOT_DIR)" && $(PROD) up -d db
	@cd "$(ROOT_DIR)" && $(PROD) run --rm app migrate

security-status: check ## Проверить production-пароли активных аккаунтов
	@cd "$(ROOT_DIR)" && $(PROD) run --rm app security-status

deploy: ## Первый/ручной deploy: build → migrations → security gate → start
	@$(MAKE) build
	@$(MAKE) migrate
	@$(MAKE) security-status
	@$(MAKE) up
	@printf 'identity workspace развернут. Reverse proxy и HTTPS настраиваются отдельно от Makefile.\n'

update: check ## Безопасное обновление без перезапуска PostgreSQL
	@cd "$(ROOT_DIR)" && $(PROD) build app
	@cd "$(ROOT_DIR)" && $(PROD) up -d db
	@cd "$(ROOT_DIR)" && ./deploy/backup.sh "$(BACKUP_DIR)"
	@cd "$(ROOT_DIR)" && $(PROD) run --rm app predeploy
	@cd "$(ROOT_DIR)" && $(PROD) up -d --no-deps app
	@$(MAKE) wait
	@printf 'identity workspace обновлён, PostgreSQL volume сохранён.\n'

# ---------------------------- Runtime operations ----------------------------

logs: ## Смотреть логи; SERVICE=app|db, TAIL=200
	@cd "$(ROOT_DIR)" && $(PROD) logs -f --tail="$(TAIL)" "$(SERVICE)"

logs-app: ## Смотреть логи backend/frontend-контейнера
	@cd "$(ROOT_DIR)" && $(PROD) logs -f --tail="$(TAIL)" app

logs-db: ## Смотреть логи PostgreSQL
	@cd "$(ROOT_DIR)" && $(PROD) logs -f --tail="$(TAIL)" db

ps: ## Показать production-контейнеры и health status
	@cd "$(ROOT_DIR)" && $(PROD) ps

health: ## Проверить /healthz изнутри контейнера приложения
	@cd "$(ROOT_DIR)" && $(PROD) exec -T app wget -q -O - http://127.0.0.1:8080/healthz
	@printf '\n'

wait: ## Дождаться готовности приложения
	@cd "$(ROOT_DIR)" && for attempt in $$(seq 1 45); do \
		if $(PROD) exec -T app wget -q -O - http://127.0.0.1:8080/healthz 2>/dev/null | grep -qx ok; then \
			echo 'identity workspace готов.'; exit 0; \
		fi; \
		sleep 2; \
	done; \
	echo 'Приложение не стало готово за 90 секунд. Выполните make logs-app.' >&2; exit 1

status: ## Показать контейнеры и выполнить внутреннюю health-проверку
	@$(MAKE) ps
	@$(MAKE) health

docker-usage: doctor ## Показать, сколько места занимают Docker images, containers, volumes и build cache
	@docker system df

docker-clean: check ## Безопасно очистить старые images/containers проекта и build cache; DOCKER_PRUNE_UNTIL=168h
	@cd "$(ROOT_DIR)" && ./deploy/docker-cleanup.sh "$(DOCKER_PRUNE_UNTIL)"

app-shell: ## Открыть shell внутри контейнера приложения
	@cd "$(ROOT_DIR)" && $(PROD) exec app sh

db-shell: ## Открыть psql внутри PostgreSQL
	@cd "$(ROOT_DIR)" && $(PROD) exec db sh -lc 'exec psql --username="$$POSTGRES_USER" --dbname="$$POSTGRES_DB"'

# ---------------------------- Data and users ----------------------------

catalog-prepare: ## Локально подготовить российский каталог OFF; FILE=/path/products.csv.gz
	@test -n "$(FILE)" || { echo 'Укажите FILE=/path/to/en.openfoodfacts.org.products.csv.gz' >&2; exit 2; }
	@mkdir -p "$(ROOT_DIR)/data/catalog"
	@cd "$(ROOT_DIR)/backend" && GOCACHE=/tmp/avatar-go-cache go run ./cmd/catalog-tool prepare-openfoodfacts \
		-input "$(abspath $(FILE))" -output "$(ROOT_DIR)/data/catalog/openfoodfacts-ru.jsonl.gz"

catalog-prepare-auchan: ## Подготовить каталог АШАН с объединением дублей; FILE=/path/auchan_catalog.json
	@test -n "$(FILE)" -a -f "$(FILE)" || { echo 'Укажите существующий FILE=/path/to/auchan_catalog.json' >&2; exit 2; }
	@mkdir -p "$(ROOT_DIR)/data/catalog"
	@cd "$(ROOT_DIR)/backend" && GOCACHE=/tmp/avatar-go-cache go run ./cmd/catalog-tool prepare-auchan \
		-input "$(abspath $(FILE))" -output "$(ROOT_DIR)/data/catalog/auchan-2026-09-07.jsonl.gz"

catalog-import: check ## Импортировать готовый каталог в production; FILE=./data/catalog/openfoodfacts-ru.jsonl.gz
	@test -n "$(FILE)" -a -f "$(FILE)" || { echo 'Укажите существующий FILE=/path/to/catalog.jsonl.gz' >&2; exit 2; }
	@cd "$(ROOT_DIR)" && $(PROD) run --rm --no-deps \
		-v "$(abspath $(FILE)):/tmp/food-catalog.jsonl.gz:ro" app import-food-catalog /tmp/food-catalog.jsonl.gz

backup: check ## Создать и проверить backup; BACKUP_DIR=./backups
	@cd "$(ROOT_DIR)" && $(PROD) up -d db
	@cd "$(ROOT_DIR)" && ./deploy/backup.sh "$(BACKUP_DIR)"

backup-prune: check ## Создать backup и удалить старше N дней; BACKUP_RETENTION_DAYS=30
	@cd "$(ROOT_DIR)" && $(PROD) up -d db
	@cd "$(ROOT_DIR)" && ./deploy/backup-and-prune.sh "$(BACKUP_DIR)" "$(BACKUP_RETENTION_DAYS)"

restore: ## Восстановить backup; FILE=/path/file.dump CONFIRM_RESTORE=RESTORE_AVATAR_ID
	@test -n "$(FILE)" || { echo 'Укажите FILE=/path/to/backup.dump' >&2; exit 2; }
	@cd "$(ROOT_DIR)" && BACKUP_DIR="$(BACKUP_DIR)" CONFIRM_RESTORE="$${CONFIRM_RESTORE:-}" ./deploy/restore.sh "$(FILE)"

set-password: ## Интерактивно сменить пароль; LOGIN=avatar01
	@test -n "$(LOGIN)" || { echo 'Укажите LOGIN=avatar01' >&2; exit 2; }
	@cd "$(ROOT_DIR)" && ./deploy/manage-user.sh set-password "$(LOGIN)"

enable-user: ## Включить пользователя; LOGIN=avatar01
	@test -n "$(LOGIN)" || { echo 'Укажите LOGIN=avatar01' >&2; exit 2; }
	@cd "$(ROOT_DIR)" && ./deploy/manage-user.sh enable-user "$(LOGIN)"

disable-user: ## Отключить пользователя и отозвать сессии; LOGIN=avatar15
	@test -n "$(LOGIN)" || { echo 'Укажите LOGIN=avatar15' >&2; exit 2; }
	@cd "$(ROOT_DIR)" && ./deploy/manage-user.sh disable-user "$(LOGIN)"

grant-admin: ## Выдать доступ к модерации каталога; LOGIN=avatar01
	@test -n "$(LOGIN)" || { echo 'Укажите LOGIN=avatar01' >&2; exit 2; }
	@cd "$(ROOT_DIR)" && ./deploy/manage-user.sh grant-admin "$(LOGIN)"

revoke-admin: ## Отозвать доступ к модерации каталога; LOGIN=avatar02
	@test -n "$(LOGIN)" || { echo 'Укажите LOGIN=avatar02' >&2; exit 2; }
	@cd "$(ROOT_DIR)" && ./deploy/manage-user.sh revoke-admin "$(LOGIN)"

configure-integrations: ## Интерактивно сохранить ключи FatSecret
	@cd "$(ROOT_DIR)" && ./deploy/configure-integrations.sh

verify: ## Проверить опубликованный HTTPS-сайт; URL=https://app.example.com
	@test -n "$(URL)" || { echo 'Укажите URL=https://app.example.com' >&2; exit 2; }
	@cd "$(ROOT_DIR)" && ./deploy/verify-production.sh "$(URL)"

# ---------------------------- Source checks and release ----------------------------

test: test-go test-frontend ## Запустить backend и frontend проверки

test-go: ## gofmt, Go tests, race, vet и build
	@cd "$(ROOT_DIR)/backend" && test -z "$$(gofmt -l .)"
	@cd "$(ROOT_DIR)/backend" && go test ./...
	@cd "$(ROOT_DIR)/backend" && go test -race ./internal/...
	@cd "$(ROOT_DIR)/backend" && go vet ./...
	@cd "$(ROOT_DIR)/backend" && go build ./...

test-frontend: ## Чистая установка frontend, audit и production build
	@cd "$(ROOT_DIR)/frontend" && npm ci --ignore-scripts --no-fund
	@cd "$(ROOT_DIR)/frontend" && npm audit --audit-level=high
	@cd "$(ROOT_DIR)/frontend" && npm run build

security: ## Запустить доступные статические security-проверки
	@cd "$(ROOT_DIR)" && ./deploy/security-checks.sh

clean: ## Удалить только локальные build-артефакты, не затрагивая данные
	@rm -rf "$(ROOT_DIR)/frontend/dist" "$(ROOT_DIR)/frontend/node_modules" \
		"$(ROOT_DIR)/frontend/tsconfig.tsbuildinfo" \
		"$(ROOT_DIR)/desktop/dist" "$(ROOT_DIR)/desktop/node_modules" \
		"$(ROOT_DIR)/backend/avatar-id-server"

verify-manifest: ## Проверить SHA-256 файлов релиза
	@cd "$(ROOT_DIR)" && sha256sum -c RELEASE_MANIFEST_SHA256.txt

manifest: ## Пересоздать RELEASE_MANIFEST_SHA256.txt
	@cd "$(ROOT_DIR)" && ./deploy/create-release-manifest.sh

release: manifest ## Создать ZIP рядом с каталогом проекта; RELEASE_NAME=avatar-id-release.zip
	@cd "$(ROOT_DIR)" && ./deploy/create-release-zip.sh "$${RELEASE_NAME:-avatar-id-release.zip}"
