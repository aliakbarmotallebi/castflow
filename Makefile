# ─── Castflow Makefile ────────────────────────────────────────────────────────
# Docker:  make install          (start without rebuild)
#          make install-build    (first time / after code changes)

COMPOSE_FILE := deploy/docker-compose.yml
NGINX_SSL_DIR := deploy/nginx/ssl
NGINX_SSL_CONF := deploy/nginx/conf.d/castflow-ssl.conf
NGINX_SSL_EXAMPLE := deploy/nginx/conf.d/castflow-ssl.conf.example

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

CASTFLOW_BASE_URL ?= http://localhost
CASTFLOW_API_BASE_URL ?= $(CASTFLOW_BASE_URL)
CASTFLOW_API_KEY  ?= dev-secret-key

.DEFAULT_GOAL := help

.PHONY: help install install-build uninstall \
        docker-build docker-up docker-down docker-restart docker-logs docker-ps \
        docker-migrate docker-stop docker-clean env \
        ssl ssl-init ssl-enable ssl-reload nginx-reload \
        build test tidy lint

help:
	@echo ""
	@echo "Castflow — VOD Platform"
	@echo ""
	@echo "  make install        Start + migrate (no rebuild)"
	@echo "  make install-build  Build image + start + migrate"
	@echo "  make uninstall      Stop and remove volumes"
	@echo "  make docker-logs    Follow castflow logs"
	@echo "  make docker-ps      Container status"
	@echo ""
	@echo "  SSL (certs in deploy/nginx/ssl/, no image rebuild):"
	@echo "  make ssl            Show SSL setup instructions"
	@echo "  make ssl-init       Generate self-signed certs (dev)"
	@echo "  make ssl-enable     Enable HTTPS after placing certs"
	@echo "  make ssl-reload     Reload nginx after cert change"
	@echo ""
	@echo "  URL: $(CASTFLOW_API_BASE_URL)"
	@echo ""

install: env docker-up docker-migrate
	@echo ""
	@echo "✓ Castflow is running (via nginx :80)"
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
	@echo "HTTPS: place certs in deploy/nginx/ssl/ then run: make ssl-enable"
	@echo ""

install-build: env docker-build install

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
	@echo "Waiting for castflow (via nginx)..."
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
		curl -sf "$(CASTFLOW_API_BASE_URL)/health" >/dev/null 2>&1 && break; \
		sleep 2; \
	done
	@curl -sf "$(CASTFLOW_API_BASE_URL)/health" >/dev/null 2>&1 || \
		(echo "✗ castflow unhealthy — run: make docker-logs" && exit 1)

docker-down:
	$(COMPOSE) down

docker-stop: docker-down

docker-restart:
	$(COMPOSE) up -d --force-recreate castflow castflow-worker nginx

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

ssl:
	@echo ""
	@echo "Place your TLS certs (no Docker rebuild needed):"
	@echo "  $(NGINX_SSL_DIR)/fullchain.pem"
	@echo "  $(NGINX_SSL_DIR)/privkey.pem"
	@echo ""
	@echo "Then enable HTTPS and reload nginx:"
	@echo "  make ssl-enable"
	@echo "  make ssl-reload"
	@echo ""
	@echo "For local dev with self-signed certs:"
	@echo "  make ssl-init"
	@echo ""

ssl-init:
	@mkdir -p $(NGINX_SSL_DIR)
	@if [ -f "$(NGINX_SSL_DIR)/fullchain.pem" ]; then \
		echo "✓ Certs already exist — use make ssl-reload to apply changes"; \
	else \
		openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
			-keyout "$(NGINX_SSL_DIR)/privkey.pem" \
			-out "$(NGINX_SSL_DIR)/fullchain.pem" \
			-subj "/CN=localhost"; \
		echo "✓ Self-signed certs created in $(NGINX_SSL_DIR)"; \
	fi
	@$(MAKE) ssl-enable
	@$(MAKE) ssl-reload

ssl-enable:
	@if [ ! -f "$(NGINX_SSL_DIR)/fullchain.pem" ] || [ ! -f "$(NGINX_SSL_DIR)/privkey.pem" ]; then \
		echo "✗ Missing certs in $(NGINX_SSL_DIR)/"; \
		echo "  Run: make ssl"; \
		exit 1; \
	fi
	@cp "$(NGINX_SSL_EXAMPLE)" "$(NGINX_SSL_CONF)"
	@echo "✓ HTTPS config enabled ($(NGINX_SSL_CONF))"

ssl-reload: nginx-reload

nginx-reload:
	@$(COMPOSE) up -d nginx
	@$(COMPOSE) exec nginx nginx -t
	@$(COMPOSE) exec nginx nginx -s reload
	@echo "✓ nginx reloaded"

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
