SHELL := /usr/bin/env bash
.SHELLFLAGS := -Eeuo pipefail -c
.DEFAULT_GOAL := help

COMPOSE := docker compose

.PHONY: help doctor config dev up build stop down restart logs ps health wait migrate test test-go test-frontend test-docker clean

help: ## Show available commands
	@awk 'BEGIN {FS = ":.*## "; printf "Identity Workspace — local development\n\n"} /^[a-zA-Z0-9_.-]+:.*## / {printf "  %-16s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

doctor: ## Check Docker and Docker Compose
	@command -v docker >/dev/null 2>&1 || { echo 'Docker is not installed.' >&2; exit 1; }
	@docker compose version >/dev/null 2>&1 || { echo 'Docker Compose is unavailable.' >&2; exit 1; }
	@docker info >/dev/null 2>&1 || { echo 'Docker daemon is unavailable.' >&2; exit 1; }

config: doctor ## Validate the local Compose configuration
	@$(COMPOSE) config --quiet

dev up: doctor ## Build and start the local stack
	@$(COMPOSE) up -d --build
	@$(MAKE) wait

build: doctor ## Build the application image
	@$(COMPOSE) build app

stop: ## Stop containers without deleting them
	@$(COMPOSE) stop

down: ## Remove local containers and network, preserving the database volume
	@$(COMPOSE) down --remove-orphans

restart: doctor ## Restart the local stack
	@$(COMPOSE) restart
	@$(MAKE) wait

logs: ## Follow application and database logs
	@$(COMPOSE) logs -f --tail=200

ps: ## Show local containers
	@$(COMPOSE) ps

health: ## Check the application health endpoint
	@$(COMPOSE) exec -T app wget -q -O - http://127.0.0.1:8080/healthz
	@printf '\n'

wait: ## Wait until the application is healthy
	@for attempt in $$(seq 1 45); do \
		if $(COMPOSE) exec -T app wget -q -O - http://127.0.0.1:8080/healthz 2>/dev/null | grep -qx ok; then \
			echo 'Identity Workspace is ready at http://localhost:$${APP_PORT:-8080}.'; exit 0; \
		fi; \
		sleep 2; \
	done; \
	echo 'Application did not become healthy in time.' >&2; exit 1

migrate: doctor ## Apply pending migrations to the local database
	@$(COMPOSE) up -d db
	@$(COMPOSE) run --rm app migrate

test: test-go test-frontend ## Run backend and frontend checks

test-go: ## Run Go formatting, tests, race detector, vet and build
	@cd backend && test -z "$$(gofmt -l .)"
	@cd backend && go test ./...
	@cd backend && go test -race ./internal/...
	@cd backend && go vet ./...
	@cd backend && go build ./...

test-frontend: ## Install frontend dependencies and create a production build
	@cd frontend && npm ci --ignore-scripts --no-fund
	@cd frontend && npm run build

test-docker: doctor ## Build the same multi-stage image used by Compose
	@docker build -t identity-workspace-local-check .

clean: ## Remove only generated local build artifacts
	@rm -rf frontend/dist frontend/node_modules frontend/tsconfig.tsbuildinfo backend/identity-workspace-server
