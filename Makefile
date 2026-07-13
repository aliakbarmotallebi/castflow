# ─── Castflow Makefile ────────────────────────────────────────────────────────
# Docker:  make install

COMPOSE_FILE := deploy/docker-compose.yml

ifeq ($(shell docker compose version >/dev/null 2>&1 && echo ok),ok)
  COMPOSE := docker compose -f $(COMPOSE_FILE)
else ifneq ($(shell command -v docker-compose 2>/dev/null),)
  COMPOSE := docker-compose -f $(COMPOSE_FILE)
else
  $(error Docker Compose not found. Install Docker Desktop or docker-compose.)
endif

ifneq (,$(wildcard ./.env))
  include .env
  export
endif

CASTFLOW_BASE_URL ?= http://localhost:8080
CASTFLOW_API_BASE_URL ?= $(CASTFLOW_BASE_URL)
CASTFLOW_API_KEY  ?= dev-secret-key

.DEFAULT_GOAL := help

.PHONY: help install uninstall \
        docker-build docker-up docker-down docker-restart docker-logs docker-ps \
        docker-migrate docker-stop docker-clean env \
        build test tidy lint

help:
	@echo ""
	@echo "Castflow — VOD Platform"
	@echo ""
	@echo "  make install        Build + start + migrate (Docker)"
	@echo "  make uninstall      Stop and remove volumes"
	@echo "  make docker-logs    Follow castflow logs"
	@echo "  make docker-ps      Container status"
	@echo ""
	@echo "  URL: $(CASTFLOW_API_BASE_URL)"
	@echo ""

install: env docker-build docker-up docker-migrate
	@echo ""
	@echo "✓ Castflow is running"
	@echo "  API:      $(CASTFLOW_API_BASE_URL)"
	@echo "  Health:   $(CASTFLOW_API_BASE_URL)/health"
	@echo "  Player:   $(CASTFLOW_API_BASE_URL)/player/index.html"
	@echo "  Asynqmon:       http://localhost:3000"
	@echo "  RustFS console: http://localhost:9001"
	@echo "  API key:  $(CASTFLOW_API_KEY)"
	@echo ""
	@echo "Upload:"
	@echo '  curl -X POST $(CASTFLOW_API_BASE_URL)/api/v1/videos/upload \'
	@echo '    -H "X-API-Key: $(CASTFLOW_API_KEY)" \'
	@echo '    -F "title=Test" -F "file=@video.mp4"'
	@echo ""

uninstall: docker-down docker-clean
	@echo "✓ Castflow removed"

env:
	@if [ ! -f .env ]; then \
		cp deploy/.env.docker.example .env; \
		echo "✓ Created .env"; \
	else \
		echo "✓ .env exists"; \
	fi

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
		(echo "✗ castflow unhealthy — run: make docker-logs" && exit 1)

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

build:
	@mkdir -p bin
	go build -o bin/castflow ./cmd/castflow
	go build -o bin/worker ./cmd/worker

test:
	go test ./...

tidy:
	go mod tidy

lint:
	go vet ./...

migrate: docker-migrate
