SHELL := /bin/bash
COMPOSE := docker compose

# Переменные из .env автоматически экспортируются в рецепты,
# поэтому Go-бэкенду не нужен dotenv-пакет.
-include .env
export

POSTGRES_USER ?= aiadvent
POSTGRES_DB   ?= aiadvent
POSTGRES_PORT ?= 5432
PORT          ?= 8080

.DEFAULT_GOAL := help

.PHONY: help env db-up db-down db-reset db-ps db-logs psql pgadmin \
        be-run be-build be-test be-fmt be-tidy fe-install fe-run fe-build dev \
        mock-llm be-run-mock dev-mock

help: ## Показать список команд
	@echo "AI Advent Challenge — доступные команды:"
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(firstword $(MAKEFILE_LIST)) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

env: ## Создать .env из .env.example, если его ещё нет
	@test -f .env || (cp .env.example .env && echo "✅ Создан .env — впиши в него DEEPSEEK_API_KEY")

# ---------------- инфраструктура ----------------

db-up: ## Поднять PostgreSQL и дождаться статуса healthy
	$(COMPOSE) up -d postgres
	@printf "⏳ Ждём готовности PostgreSQL"
	@until [ "$$($(COMPOSE) ps -q postgres | xargs docker inspect -f '{{.State.Health.Status}}' 2>/dev/null)" = "healthy" ]; do \
		printf "."; sleep 1; \
	done
	@echo " ✅"
	@echo "🐘 PostgreSQL слушает localhost:$(POSTGRES_PORT) (db=$(POSTGRES_DB), user=$(POSTGRES_USER))"

db-down: ## Остановить контейнеры (данные сохраняются в volume)
	$(COMPOSE) down

db-reset: ## Снести контейнеры вместе с данными и поднять заново
	$(COMPOSE) down -v
	$(MAKE) db-up

db-ps: ## Статус контейнеров
	$(COMPOSE) ps

db-logs: ## Логи PostgreSQL
	$(COMPOSE) logs -f postgres

psql: ## Открыть psql внутри контейнера
	$(COMPOSE) exec postgres psql -U $(POSTGRES_USER) -d $(POSTGRES_DB)

pgadmin: ## Поднять pgAdmin на http://localhost:5050
	$(COMPOSE) --profile tools up -d pgadmin

# ---------------- бэкенд ----------------

be-run: ## Запустить Go-бэкенд (порт из .env, по умолчанию 8080)
	cd backend && go run ./cmd/server

be-build: ## Собрать бинарник в backend/bin/server
	cd backend && go build -o bin/server ./cmd/server

be-test: ## Тесты бэкенда
	cd backend && go test ./...

be-fmt: ## gofmt
	cd backend && gofmt -w .

be-tidy: ## go mod tidy
	cd backend && go mod tidy

# ---------------- разработка без ключа DeepSeek ----------------

mock-llm: ## Заглушка DeepSeek на :9099 — проверка UI без ключа и без трат
	cd backend && go run ./cmd/fakellm

be-run-mock: ## Бэкенд, направленный на заглушку вместо реального DeepSeek
	cd backend && DEEPSEEK_BASE_URL=http://localhost:9099 DEEPSEEK_API_KEY=sk-mock go run ./cmd/server

# ---------------- фронтенд ----------------

fe-install: ## Установить зависимости Angular
	cd frontend && pnpm install

fe-run: ## Angular dev-server на http://localhost:4200
	cd frontend && pnpm start

fe-build: ## Production-сборка Angular
	cd frontend && pnpm build

# ---------------- всё вместе ----------------

dev: db-up ## БД + бэкенд + фронтенд одной командой (Ctrl+C останавливает всё)
	@echo "🚀 backend :$(PORT)  |  frontend :4200  |  postgres :$(POSTGRES_PORT)"
	@trap 'kill 0' INT TERM EXIT; \
	( cd backend && go run ./cmd/server ) & \
	( cd frontend && pnpm start ) & \
	wait

dev-mock: db-up ## То же, но с заглушкой вместо DeepSeek (ключ не нужен)
	@echo "🚀 backend :$(PORT) → заглушка :9099  |  frontend :4200"
	@trap 'kill 0' INT TERM EXIT; \
	( cd backend && go run ./cmd/fakellm ) & \
	( cd backend && DEEPSEEK_BASE_URL=http://localhost:9099 DEEPSEEK_API_KEY=sk-mock go run ./cmd/server ) & \
	( cd frontend && pnpm start ) & \
	wait
