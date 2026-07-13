# Production deployment with custom domain

## Overview

The Docker stack includes **nginx** as the public entry point:

- **Port 80** — HTTP (proxies all traffic to `castflow:8080`)
- **Port 443** — HTTPS (enabled after TLS certs are installed)

The `castflow` API container is **not** exposed on the host. All requests go through nginx.

```
Client → nginx (:80 / :443) → castflow (:8080)
```

Nginx config lives in `deploy/nginx/`:

| Path | Purpose |
|------|---------|
| `nginx.conf` | Main nginx config (`client_max_body_size 2G` for uploads) |
| `conf.d/castflow.conf` | HTTP reverse proxy + ACME webroot |
| `conf.d/castflow-ssl.conf` | HTTPS block (created by `make ssl-enable`) |
| `conf.d/castflow-ssl.conf.example` | HTTPS template |
| `ssl/fullchain.pem` | TLS certificate (not in git) |
| `ssl/privkey.pem` | TLS private key (not in git) |
| `certbot/www/` | Let's Encrypt HTTP-01 challenge files |
| `certbot/conf/` | Let's Encrypt issued certs (not in git) |

## DNS

**Single domain (recommended for small deployments):**

| Record | Points to |
|--------|-----------|
| `example.com` | Your server IP |

**Multi-domain (optional):**

| Record | Points to |
|--------|-----------|
| `api.example.com` | Castflow server |
| `cdn.example.com` | CDN / S3 origin |
| `player.example.com` | Castflow `/player` |

## Environment (`.env`)

Postgres and Redis URLs are set in `deploy/docker-compose.yml` for Docker — do not duplicate them in `.env`.

```env
# Dev (via nginx on port 80)
CASTFLOW_BASE_URL=http://localhost

# Production — single domain with HTTPS
CASTFLOW_BASE_URL=https://example.com

# Production — separate domains (optional)
# CASTFLOW_API_BASE_URL=https://api.example.com
# CASTFLOW_CDN_BASE_URL=https://cdn.example.com
# CASTFLOW_PLAYER_BASE_URL=https://player.example.com

# S3-compatible storage (RustFS in Docker: use rustfs:9000)
CASTFLOW_STORAGE_DRIVER=s3
CASTFLOW_S3_ENDPOINT=rustfs:9000
CASTFLOW_S3_BUCKET=castflow-vod
CASTFLOW_S3_ACCESS_KEY=your-access-key
CASTFLOW_S3_SECRET_KEY=your-secret-key
CASTFLOW_S3_USE_SSL=false

CASTFLOW_ENABLE_EMBEDDED_WORKER=false
CASTFLOW_WORKER_CONCURRENCY=2
```

After changing `.env`, restart the app containers so URL settings take effect:

```bash
make docker-restart
```

## First deploy

```bash
git clone https://github.com/your-org/castflow.git
cd castflow
make install-build
```

## HTTPS / TLS

TLS is optional. HTTP on port 80 works immediately after `make install`. HTTPS requires certs in `deploy/nginx/ssl/`.

**No Docker rebuild is needed** for SSL — only nginx is reloaded.

### Option A — Let's Encrypt (production)

Requirements:

- Domain DNS points to the server
- Port **80** open (ACME HTTP-01 challenge)
- Port **443** open (HTTPS traffic)

```bash
make ssl-certbot DOMAIN=example.com EMAIL=you@example.com
```

This will:

1. Start nginx (if not running)
2. Run certbot with webroot challenge (`/.well-known/acme-challenge/`)
3. Copy certs into `deploy/nginx/ssl/` (via a root-owned volume workaround)
4. Enable `castflow-ssl.conf` and reload nginx

Then set in `.env`:

```env
CASTFLOW_BASE_URL=https://example.com
```

```bash
make docker-restart
curl https://example.com/health
```

**If certbot succeeded but copy failed** (permission error on first run):

```bash
git pull   # get ssl-install-certs fix
make ssl-install-certs DOMAIN=example.com
```

### Option B — Your own certificates

Place files manually:

```
deploy/nginx/ssl/fullchain.pem
deploy/nginx/ssl/privkey.pem
```

```bash
make ssl-enable
make ssl-reload
```

### Option C — Self-signed (local dev)

```bash
make ssl-init
```

Browsers will show a certificate warning — acceptable for local testing only.

### Certificate renewal

Let's Encrypt certs expire every 90 days. Renew and reinstall:

```bash
docker run --rm \
  -v "$(pwd)/deploy/nginx/certbot/www:/var/www/certbot" \
  -v "$(pwd)/deploy/nginx/certbot/conf:/etc/letsencrypt" \
  certbot/certbot renew

make ssl-install-certs DOMAIN=example.com
```

Consider a monthly cron job for `renew` + `ssl-install-certs`.

### SSL Makefile reference

| Command | Description |
|---------|-------------|
| `make ssl` | Print setup instructions |
| `make ssl-certbot DOMAIN=… [EMAIL=…]` | Issue + install Let's Encrypt cert |
| `make ssl-install-certs DOMAIN=…` | Copy certbot certs to nginx ssl dir |
| `make ssl-enable` | Activate HTTPS nginx config |
| `make ssl-reload` | `nginx -t` + reload |
| `make ssl-init` | Self-signed cert for dev |

## Docker Compose services

| Service | Purpose | Host port |
|---------|---------|-----------|
| `nginx` | Reverse proxy (HTTP/HTTPS) | 80, 443 |
| `castflow` | HTTP API + player + media | — |
| `castflow-worker` | FFmpeg transcode worker | — |
| `redis` | Asynq backend (`maxmemory-policy noeviction`) | — |
| `asynqmon` | Queue dashboard | 3000 |
| `rustfs` | Optional S3-compatible object storage | 9000, 9001 |
| `postgres` | Video metadata | — |

API and worker share `castflow_storage` and `castflow_tmp` volumes when using local storage.

## Makefile — install vs rebuild

| Command | When to use |
|---------|-------------|
| `make install` | Start stack + migrate (no image rebuild) |
| `make install-build` | First deploy or after Go code changes |
| `make docker-restart` | After `.env` changes or cert updates |

## Nginx — multi-domain CDN (advanced)

The default `deploy/nginx/conf.d/castflow.conf` proxies everything to Castflow (API, `/player/`, `/media/`). For a separate CDN origin backed by S3/RustFS, add server blocks like:

```nginx
server {
    listen 443 ssl;
    http2 on;
    server_name cdn.example.com;

    ssl_certificate     /etc/nginx/ssl/fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/privkey.pem;

    location /v/ {
        proxy_pass http://rustfs:9000/castflow-vod/v/;
        proxy_set_header Host rustfs:9000;
        add_header Access-Control-Allow-Origin * always;
        add_header Cache-Control "public, max-age=31536000" always;

        types {
            application/vnd.apple.mpegurl m3u8;
            video/mp2t ts;
            application/dash+xml mpd;
            video/iso.segment m4s;
            text/vtt vtt;
        }
    }
}
```

Replace `rustfs:9000` with your S3 endpoint. For AWS S3, use the bucket virtual-host URL or CloudFront instead of this proxy block.

After editing nginx config:

```bash
make ssl-reload
```

## Firewall

Open these ports on the server:

| Port | Protocol | Purpose |
|------|----------|---------|
| 80 | TCP | HTTP + ACME challenge |
| 443 | TCP | HTTPS |
| 3000 | TCP | Asynqmon (restrict in production) |
| 9001 | TCP | RustFS console (optional, restrict) |

## Asynqmon (production)

The default Docker setup exposes Asynqmon without authentication. Restrict network access to `:3000` or put it behind a VPN / internal ingress with auth in front.

## Scaling workers

Run multiple `castflow-worker` replicas — they all consume from the same Asynq queue (`castflow`):

```bash
docker compose -f deploy/docker-compose.yml up -d --scale castflow-worker=3
```

Increase `CASTFLOW_WORKER_CONCURRENCY` per replica for parallel FFmpeg jobs (watch CPU/RAM).

## Local ports (dev stack)

| Service | URL |
|---------|-----|
| API (via nginx) | http://localhost |
| Asynqmon | http://localhost:3000 |
| RustFS S3 | http://localhost:9000 |
| RustFS Console | http://localhost:9001 |

Postgres and Redis are **not** exposed on the host — containers connect via the Docker network (`postgres:5432`, `redis:6379`).

## Upload example (production)

```bash
curl -X POST "https://example.com/api/v1/videos/upload" \
  -H "X-API-Key: your-api-key" \
  -F "title=My Lecture" \
  -F "file=@video.mp4" \
  --max-time 600
```

## Generated links (example)

After `GET /api/v1/videos/{id}/links`:

```
hlsUrl:       https://cdn.example.com/v/{id}/hls/master.m3u8
dashUrl:      https://cdn.example.com/v/{id}/dash/manifest.mpd
configUrl:    https://cdn.example.com/v/{id}/config.json
playerUrl:    https://player.example.com/index.html?config=...
thumbnailUrl: https://cdn.example.com/v/{id}/thumbnail.jpg
tooltipUrl:   https://cdn.example.com/v/{id}/tooltip.vtt
videoUrl:     https://cdn.example.com/v/{id}/origin.mp4
iframe:       <embed code>
```

With a single domain and local storage, CDN/player URLs are derived from `CASTFLOW_BASE_URL` (`/media`, `/player`).

## Debugging

See [Docker debugging guide](DEBUG.md) for logs, health checks, shells, and common fixes.
