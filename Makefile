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
        docker-logs-worker docker-logs-nginx docker-logs-all docker-health docker-shell \
        docker-migrate docker-stop docker-clean env \
        ssl ssl-init ssl-enable ssl-certbot ssl-install-certs ssl-reload nginx-reload \
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
	@echo "  make docker-health  HTTP health checks"
	@echo "  See docs/DEBUG.md for full Docker debugging guide"
	@echo ""
	@echo "  SSL (certs in deploy/nginx/ssl/, no image rebuild):"
	@echo "  make ssl            Show SSL setup instructions"
	@echo "  make ssl-certbot DOMAIN=example.com   Let's Encrypt (production)"
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
	$(COMPOSE) build castflow castflow-worker

docker-up:
	$(COMPOSE) up -d
	@echo "Waiting for castflow (via nginx)..."
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
		curl -sf "$(CASTFLOW_API_BASE_URL)/health" >/dev/null 2>&1 && break; \
		sleep 2; \
	done
	@curl -sf "$(CASTFLOW_API_BASE_URL)/health" >/dev/null 2>&1 || \
		(echo "✗ castflow unhealthy — run: make docker-logs" && exit 1)
	@$(COMPOSE) ps --status running -q castflow-worker 2>/dev/null | grep -q . || \
		(echo "⚠ castflow-worker not running — transcode relies on embedded worker in castflow" && true)

docker-down:
	$(COMPOSE) down

docker-stop: docker-down

docker-restart:
	$(COMPOSE) up -d --force-recreate castflow castflow-worker nginx

docker-logs:
	$(COMPOSE) logs -f castflow

docker-logs-worker:
	$(COMPOSE) logs -f castflow-worker

docker-logs-nginx:
	$(COMPOSE) logs -f nginx

docker-logs-all:
	$(COMPOSE) logs -f

docker-health:
	@echo "── Public (via nginx) ──"
	@curl -sf "$(CASTFLOW_API_BASE_URL)/health" && echo " OK  $(CASTFLOW_API_BASE_URL)/health" || \
		(echo " FAIL $(CASTFLOW_API_BASE_URL)/health" && exit 1)
	@echo "── castflow (internal) ──"
	@$(COMPOSE) exec -T castflow wget -qO- http://127.0.0.1:8080/health && echo " OK  castflow:8080/health" || \
		echo " FAIL castflow:8080/health"
	@echo "── services ──"
	@$(COMPOSE) ps

docker-shell:
	@test -n "$(SERVICE)" || (echo "Usage: make docker-shell SERVICE=castflow|castflow-worker|nginx|postgres|redis" && exit 1)
	$(COMPOSE) exec $(SERVICE) sh

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
	@echo "Production (Let's Encrypt):"
	@echo "  make ssl-certbot DOMAIN=your-domain.com EMAIL=you@example.com"
	@echo ""
	@echo "Or place your own certs:"
	@echo "  $(NGINX_SSL_DIR)/fullchain.pem"
	@echo "  $(NGINX_SSL_DIR)/privkey.pem"
	@echo "  make ssl-enable && make ssl-reload"
	@echo ""
	@echo "Local dev (self-signed):"
	@echo "  make ssl-init"
	@echo ""

ssl-certbot:
	@test -n "$(DOMAIN)" || (echo "Usage: make ssl-certbot DOMAIN=example.com [EMAIL=you@example.com]" && exit 1)
	@mkdir -p deploy/nginx/certbot/www deploy/nginx/ssl
	@$(COMPOSE) up -d nginx
	@docker run --rm \
		-v "$(CURDIR)/deploy/nginx/certbot/www:/var/www/certbot" \
		-v "$(CURDIR)/deploy/nginx/certbot/conf:/etc/letsencrypt" \
		certbot/certbot certonly --webroot -w /var/www/certbot \
		-d "$(DOMAIN)" \
		$(if $(EMAIL),--email "$(EMAIL)",--register-unsafely-without-email) \
		--agree-tos --no-eff-email
	@$(MAKE) ssl-install-certs DOMAIN="$(DOMAIN)"

ssl-install-certs:
	@test -n "$(DOMAIN)" || (echo "Usage: make ssl-install-certs DOMAIN=example.com" && exit 1)
	@docker run --rm \
		-v "$(CURDIR)/deploy/nginx/certbot/conf:/etc/letsencrypt:ro" \
		-v "$(CURDIR)/$(NGINX_SSL_DIR):/out" \
		alpine:3.20 sh -c '\
			cp "/etc/letsencrypt/live/$(DOMAIN)/fullchain.pem" /out/fullchain.pem && \
			cp "/etc/letsencrypt/live/$(DOMAIN)/privkey.pem" /out/privkey.pem && \
			chmod 644 /out/fullchain.pem && \
			chmod 600 /out/privkey.pem'
	@echo "✓ Certs installed for $(DOMAIN)"
	@$(MAKE) ssl-enable
	@$(MAKE) ssl-reload
	@echo ""
	@echo "Update .env:"
	@echo "  CASTFLOW_BASE_URL=https://$(DOMAIN)"
	@echo "Then: make docker-restart"
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
