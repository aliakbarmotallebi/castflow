# ─── Castflow Makefile ────────────────────────────────────────────────────────
# Full Docker install:  make install
# Local dev (Go):       make env-local && make docker-deps && make migrate-local && make run

COMPOSE_FILE := deploy/docker-compose.yml

# Support both "docker compose" (v2) and "docker-compose" (v1)
ifeq ($(shell docker compose version >/dev/null 2>&1 && echo ok),ok)
  COMPOSE := docker compose -f $(COMPOSE_FILE)
else ifneq ($(shell command -v docker-compose 2>/dev/null),)
  COMPOSE := docker-compose -f $(COMPOSE_FILE)
else
  $(error Docker Compose not found. Install Docker Desktop or docker-compose.)
endif

# Load address overrides from .env (if present)
ifneq (,$(wildcard ./.env))
  include .env
  export
endif

# Defaults — override in .env
CASTFLOW_PUBLIC_SCHEME     ?= http
CASTFLOW_PUBLIC_HOST       ?= localhost
CASTFLOW_API_PORT          ?= 8080
CASTFLOW_API_BASE_URL      ?= $(CASTFLOW_PUBLIC_SCHEME)://$(CASTFLOW_PUBLIC_HOST):$(CASTFLOW_API_PORT)
CASTFLOW_PLAYER_BASE_URL   ?= $(CASTFLOW_API_BASE_URL)/player
CASTFLOW_ASYNQMON_PORT     ?= 3000
CASTFLOW_ASYNQMON_URL      ?= http://localhost:$(CASTFLOW_ASYNQMON_PORT)
CASTFLOW_POSTGRES_PORT     ?= 5433
CASTFLOW_REDIS_PORT        ?= 6380
CASTFLOW_RUSTFS_PORT       ?= 9000
CASTFLOW_RUSTFS_CONSOLE_PORT ?= 9001
CASTFLOW_RUSTFS_URL        ?= http://localhost:$(CASTFLOW_RUSTFS_PORT)
CASTFLOW_RUSTFS_CONSOLE_URL ?= http://localhost:$(CASTFLOW_RUSTFS_CONSOLE_PORT)
CASTFLOW_API_KEY           ?= dev-secret-key
CASTFLOW_DATABASE_URL      ?= postgres://castflow:castflow@localhost:$(CASTFLOW_POSTGRES_PORT)/castflow?sslmode=disable

PROJECT := castflow
IMAGE   := castflow:latest

.DEFAULT_GOAL := help

.PHONY: help install uninstall \
        docker-build docker-up docker-down docker-restart docker-logs docker-ps \
        docker-migrate docker-deps docker-stop docker-clean env env-local \
        run run-worker build build-worker test tidy migrate-local lint

# ─── Help ─────────────────────────────────────────────────────────────────────

help:
	@echo ""
	@echo "Castflow — VOD Platform"
	@echo ""
	@echo "  Docker (recommended)"
	@echo "    make install        Build image, start stack, run migrations"
	@echo "    make uninstall      Stop stack and remove volumes"
	@echo "    make docker-up      Start all containers"
	@echo "    make docker-down    Stop containers (keep volumes)"
	@echo "    make docker-restart Rebuild and restart castflow"
	@echo "    make docker-logs    Follow castflow logs"
	@echo "    make docker-ps      Show container status"
	@echo "    make docker-migrate Run DB migrations"
	@echo ""
	@echo "  Local development (Go on host)"
	@echo "    make env-local      Create .env from .env.example"
	@echo "    make docker-deps    Start Postgres + Redis + RustFS + Asynqmon"
	@echo "    make migrate-local  Migrate DB using CASTFLOW_DATABASE_URL"
	@echo "    make run            Run API locally with go run"
	@echo "    make run-worker     Run transcode worker only"
	@echo ""
	@echo "  Build & test"
	@echo "    make build          Compile binary to bin/castflow"
	@echo "    make test           Run unit tests"
	@echo "    make lint           Run go vet"
	@echo ""
	@echo "  Addresses (set in .env):"
	@echo "    API:      $(CASTFLOW_API_BASE_URL)"
	@echo "    Player:   $(CASTFLOW_PLAYER_BASE_URL)"
	@echo "    Asynqmon: $(CASTFLOW_ASYNQMON_URL)"
	@echo ""

# ─── Docker install (one command) ─────────────────────────────────────────────

install: env docker-build docker-up docker-migrate
	@echo ""
	@echo "✓ Castflow is running"
	@echo "  API:    $(CASTFLOW_API_BASE_URL)"
	@echo "  Health: $(CASTFLOW_API_BASE_URL)/health"
	@echo "  Player: $(CASTFLOW_PLAYER_BASE_URL)/index.html"
	@echo "  API key: $(CASTFLOW_API_KEY)"
	@echo ""
	@echo "  Asynqmon → $(CASTFLOW_ASYNQMON_URL)"
	@echo ""
	@echo "Upload:"
	@echo '  curl -X POST $(CASTFLOW_API_BASE_URL)/api/v1/videos/upload \'
	@echo '    -H "X-API-Key: $(CASTFLOW_API_KEY)" \'
	@echo '    -F "title=Test" -F "file=@video.mp4"'
	@echo ""

uninstall: docker-down docker-clean
	@echo "✓ Castflow removed (volumes deleted)"

env:
	@if [ ! -f .env ]; then \
		cp deploy/.env.docker.example .env; \
		echo "✓ Created .env from deploy/.env.docker.example"; \
	else \
		echo "✓ .env already exists"; \
	fi

env-local:
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		echo "✓ Created .env from .env.example"; \
	else \
		echo "✓ .env already exists"; \
	fi

# ─── Docker targets ───────────────────────────────────────────────────────────

docker-build:
	$(COMPOSE) build castflow

docker-up:
	$(COMPOSE) up -d
	@echo "Waiting for castflow..."
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
		curl -sf "$(CASTFLOW_API_BASE_URL)/health" >/dev/null 2>&1 && break; \
		sleep 2; \
	done
	@curl -sf "$(CASTFLOW_API_BASE_URL)/health" >/dev/null 2>&1 || \
		(echo "✗ castflow did not become healthy — run: make docker-logs" && exit 1)

docker-down:
	$(COMPOSE) down

docker-stop: docker-down

docker-restart: docker-build
	$(COMPOSE) up -d --force-recreate castflow

docker-logs:
	$(COMPOSE) logs -f castflow

docker-ps:
	$(COMPOSE) ps

docker-migrate:
	@echo "Running migrations..."
	@$(COMPOSE) exec -T postgres psql -U castflow -d castflow < migrations/001_init.sql
	@$(COMPOSE) exec -T postgres psql -U castflow -d castflow < migrations/002_outbox.sql
	@echo "✓ Migrations applied"

docker-clean:
	$(COMPOSE) down -v --remove-orphans

# Start only infrastructure (for local `make run`)
docker-deps:
	$(COMPOSE) up -d postgres redis rustfs asynqmon
	@echo "✓ Postgres   → localhost:$(CASTFLOW_POSTGRES_PORT)"
	@echo "✓ Redis      → localhost:$(CASTFLOW_REDIS_PORT) (Asynq backend)"
	@echo "✓ Asynqmon   → $(CASTFLOW_ASYNQMON_URL)"
	@echo "✓ RustFS     → $(CASTFLOW_RUSTFS_URL) (console $(CASTFLOW_RUSTFS_CONSOLE_URL))"

# ─── Local development ────────────────────────────────────────────────────────

migrate-local:
	@psql "$(CASTFLOW_DATABASE_URL)" -f migrations/001_init.sql
	@psql "$(CASTFLOW_DATABASE_URL)" -f migrations/002_outbox.sql

run:
	go run ./cmd/castflow

run-worker:
	go run ./cmd/worker

build:
	@mkdir -p bin
	go build -o bin/castflow ./cmd/castflow
	go build -o bin/worker ./cmd/worker

build-worker:
	@mkdir -p bin
	go build -o bin/worker ./cmd/worker

test:
	go test ./...

tidy:
	go mod tidy

lint:
	go vet ./...

# Backward-compatible aliases
docker-up-deps: docker-deps
migrate: docker-migrate
