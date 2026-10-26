SHELL := /bin/bash
COMPOSE := docker compose

.DEFAULT_GOAL := help

.PHONY: help up down logs ps rebuild clean test fmt

help: ## Zeigt alle Targets
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'

up: ## Stack bauen und starten
	test -f .env || cp .env.example .env
	$(COMPOSE) up --build -d
	@echo "Frontend: http://localhost:$${FRONTEND_PORT:-3000}"

down: ## Stack stoppen (Volumes bleiben erhalten)
	$(COMPOSE) down

logs: ## Logs aller Services folgen
	$(COMPOSE) logs -f --tail=100

ps: ## Status der Services
	$(COMPOSE) ps

rebuild: ## Images ohne Cache neu bauen
	$(COMPOSE) build --no-cache
	$(COMPOSE) up -d

clean: ## Stack inkl. Volumes entfernen (Datenverlust!)
	$(COMPOSE) down -v --remove-orphans

test: ## Smoke-Test gegen laufenden Stack
	./scripts/smoke-test.sh

fmt: ## Go formatieren
	cd backend && gofmt -w ./cmd ./internal
