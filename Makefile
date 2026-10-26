SHELL := /bin/bash
COMPOSE := docker compose
PYTHON ?= python3
WORKER_VENV := worker/.venv

.DEFAULT_GOAL := help

.PHONY: help up down logs ps rebuild clean smoke test test-backend test-frontend test-worker lint fmt venv

help: ## Zeigt alle Targets
	@grep -hE '^[a-zA-Z_.-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-14s %s\n", $$1, $$2}'

up: ## Stack starten (docker compose up -d)
	@test -f .env || cp .env.example .env
	$(COMPOSE) up -d
	@echo "Frontend: http://localhost:$${FRONTEND_PORT:-3000}"

down: ## Stack stoppen (Volumes bleiben erhalten)
	$(COMPOSE) down

logs: ## Logs aller Services folgen
	$(COMPOSE) logs -f --tail=100

ps: ## Status der Services
	$(COMPOSE) ps

rebuild: ## Images ohne Cache neu bauen und starten
	$(COMPOSE) build --no-cache
	$(COMPOSE) up -d

clean: ## Stack inkl. Volumes entfernen (Datenverlust!)
	$(COMPOSE) down -v --remove-orphans

smoke: ## HTTP-Smoke-Test gegen den laufenden Stack
	./scripts/smoke-test.sh

test: test-backend test-frontend test-worker ## Alle Tests lokal (Go, Frontend, Worker)
	@echo "Alle Tests erfolgreich."

test-backend: ## go vet + go test -race
	cd backend && go vet ./... && go test -race ./...

test-frontend: ## npm ci + tsc + vite build
	cd frontend && npm ci --no-audit --no-fund && npx tsc --noEmit && npm run build

test-worker: venv ## ruff check + ruff format --check + pytest
	cd worker && .venv/bin/ruff check . && .venv/bin/ruff format --check . && .venv/bin/pytest

venv: $(WORKER_VENV)/bin/python ## Python-Venv fuer den Worker anlegen/aktualisieren

$(WORKER_VENV)/bin/python: worker/requirements.txt worker/requirements-dev.txt
	$(PYTHON) -m venv $(WORKER_VENV)
	$(WORKER_VENV)/bin/pip install --quiet --upgrade pip
	$(WORKER_VENV)/bin/pip install --quiet -r worker/requirements-dev.txt

lint: ## go vet + golangci-lint + ruff
	cd backend && go vet ./...
	cd backend && if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run; else echo "golangci-lint nicht installiert - uebersprungen"; fi
	$(MAKE) test-worker

fmt: ## Code formatieren (gofmt, ruff format)
	cd backend && gofmt -w ./cmd ./internal
	cd worker && .venv/bin/ruff format . && .venv/bin/ruff check --fix .
