# ─── Castflow Makefile ────────────────────────────────────────────────────────
# Full Docker install:  make install
# Local dev (Go):       make docker-deps && make migrate-local && make run

COMPOSE_FILE := deploy/docker-compose.yml

# Support both "docker compose" (v2) and "docker-compose" (v1)
ifeq ($(shell docker compose version >/dev/null 2>&1 && echo ok),ok)
  COMPOSE := docker compose -f $(COMPOSE_FILE)
else ifneq ($(shell command -v docker-compose 2>/dev/null),)
  COMPOSE := docker-compose -f $(COMPOSE_FILE)
else
  $(error Docker Compose not found. Install Docker Desktop or docker-compose.)
endif

PROJECT      := castflow
IMAGE        := castflow:latest

.DEFAULT_GOAL := help

.PHONY: help install uninstall \
        docker-build docker-up docker-down docker-restart docker-logs docker-ps \
        docker-migrate docker-deps docker-stop docker-clean env \
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
	@echo "    make docker-deps    Start Postgres + Redis + RustFS + Asynqmon"
	@echo "    make migrate-local  Migrate DB on localhost:5433"
	@echo "    make run            Run API locally with go run"
	@echo "    make run-worker     Run transcode worker only"
	@echo ""
	@echo "  Build & test"
	@echo "    make build          Compile binary to bin/castflow"
	@echo "    make test           Run unit tests"
	@echo "    make lint           Run go vet"
	@echo ""

# ─── Docker install (one command) ─────────────────────────────────────────────

install: env docker-build docker-up docker-migrate
	@echo ""
	@echo "✓ Castflow is running"
	@echo "  API:    http://localhost:8080"
	@echo "  Health: http://localhost:8080/health"
	@echo "  Player: http://localhost:8080/player/index.html"
	@echo "  API key: $$(grep CASTFLOW_API_KEY .env | cut -d= -f2)"
	@echo ""
	@echo "  Asynqmon   → http://localhost:3000"
	@echo ""
	@echo "Upload:"
	@echo '  curl -X POST http://localhost:8080/api/v1/videos/upload \'
	@echo '    -H "X-API-Key: dev-secret-key" \'
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

# ─── Docker targets ───────────────────────────────────────────────────────────

docker-build:
	$(COMPOSE) build castflow

docker-up:
	$(COMPOSE) up -d
	@echo "Waiting for castflow..."
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
		curl -sf http://localhost:8080/health >/dev/null 2>&1 && break; \
		sleep 2; \
	done
	@curl -sf http://localhost:8080/health >/dev/null 2>&1 || \
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
	@echo "✓ Postgres   → localhost:5433"
	@echo "✓ Redis      → localhost:6380 (Asynq backend)"
	@echo "✓ Asynqmon   → http://localhost:3000"
	@echo "✓ RustFS     → localhost:9000 (console :9001)"

# ─── Local development ────────────────────────────────────────────────────────

migrate-local:
	@if [ -f .env ]; then set -a && . ./.env && set +a; fi; \
	psql "$${CASTFLOW_DATABASE_URL:-postgres://castflow:castflow@localhost:5433/castflow?sslmode=disable}" \
		-f migrations/001_init.sql && \
	psql "$${CASTFLOW_DATABASE_URL:-postgres://castflow:castflow@localhost:5433/castflow?sslmode=disable}" \
		-f migrations/002_outbox.sql

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
