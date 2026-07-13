# Castflow

Self-hosted VOD platform written in Go. Upload videos, transcode to HLS/DASH, and serve playback links on **your own domain**.

## Features

- REST API for video upload, status, and link generation
- FFmpeg transcoding (multi-bitrate HLS + DASH)
- Thumbnail, tooltip sprite (VTT + PNG)
- Custom CDN and player URLs (`config.json`, iFrame embed)
- Clean Architecture (domain → application → adapters)
- Docker Compose for local/production deploy
- **Nginx** reverse proxy with optional TLS (Let's Encrypt or your own certs)
- Job queue: **Asynq** on Redis (with **Asynqmon** UI)
- Storage: local filesystem or S3-compatible (RustFS, AWS S3, …)

## Quick start

```bash
cd castflow
make install-build   # first time (build image + start + migrate)
# make install       # later runs — start + migrate only, no rebuild
```

Creates `.env` from `deploy/.env.docker.example`. Edit `.env` for app settings; Postgres/Redis stay in `docker-compose.yml`.

Traffic goes through **nginx** on port **80** (and **443** after SSL is enabled). The API container is internal only.

| URL | Description |
|-----|-------------|
| http://localhost/health | Health check |
| http://localhost/api/v1/videos | List videos |
| http://localhost/player/index.html | Embedded player |
| http://localhost:3000 | Asynqmon (queue UI) |
| http://localhost:9001 | RustFS console (storage UI) |

```bash
make docker-logs    # follow API logs
make docker-down    # stop (keep data)
make uninstall      # stop + remove volumes
```

### `.env` essentials

```env
# Dev — one URL, CDN/player derived automatically
CASTFLOW_BASE_URL=http://localhost
CASTFLOW_API_KEY=dev-secret-key

# Production (single domain with HTTPS):
# CASTFLOW_BASE_URL=https://example.com

# Production (separate domains):
# CASTFLOW_API_BASE_URL=https://api.example.com
# CASTFLOW_CDN_BASE_URL=https://cdn.example.com
# CASTFLOW_PLAYER_BASE_URL=https://player.example.com
```

See `deploy/.env.docker.example` for storage, FFmpeg, worker, and outbox options.

## Stack overview

| Service | Role | Exposed port |
|---------|------|--------------|
| `nginx` | Reverse proxy (HTTP/HTTPS) | 80, 443 |
| `castflow` | HTTP API + player + media | — (internal) |
| `asynqmon` | Queue dashboard | 3000 |
| `rustfs` | S3 storage API | 9000 |
| `rustfs` | Storage console UI | 9001 |
| `castflow-worker` | FFmpeg transcode | — |
| Postgres / Redis | Internal only | — |

**Queue:** Asynq queue `castflow`, task type `transcode`. Monitor via [Asynqmon or CLI](#monitor-transcode-queue).

**Storage:** Docker defaults to **local** volumes (`CASTFLOW_STORAGE_DRIVER=local`). RustFS runs in the stack but is only used when you switch to `CASTFLOW_STORAGE_DRIVER=s3` — see `deploy/.env.docker.example`.

Transcoding runs via the Asynq queue: dedicated `castflow-worker` plus an embedded consumer in `castflow` (enabled in `docker-compose.yml` so jobs still process if the worker container is down).

## Makefile commands

| Command | Description |
|---------|-------------|
| `make install` | Start + migrate (no image rebuild) |
| `make install-build` | Build image + start + migrate (first time / after code changes) |
| `make uninstall` | Remove stack and volumes |
| `make docker-up` | Start all containers |
| `make docker-restart` | Recreate API, worker, and nginx |
| `make docker-migrate` | Apply SQL migrations |
| `make docker-logs` | Follow API logs |
| `make docker-health` | Health checks (nginx + castflow) |
| `make build` | Build binaries locally |

See [Docker debugging](docs/DEBUG.md) for logs, shells, and troubleshooting.

### SSL / HTTPS

Certs live in `deploy/nginx/ssl/`. Enabling or renewing TLS only reloads nginx — **no Docker image rebuild**.

| Command | Description |
|---------|-------------|
| `make ssl` | Show SSL setup instructions |
| `make ssl-certbot DOMAIN=example.com EMAIL=you@example.com` | Let's Encrypt via webroot (production) |
| `make ssl-install-certs DOMAIN=example.com` | Copy existing certbot certs into nginx (after a partial run) |
| `make ssl-init` | Self-signed certs for local HTTPS |
| `make ssl-enable` | Enable HTTPS config after placing certs |
| `make ssl-reload` | Test and reload nginx |

**Production (Let's Encrypt):**

```bash
make install-build
make ssl-certbot DOMAIN=example.com EMAIL=you@example.com
# set CASTFLOW_BASE_URL=https://example.com in .env
make docker-restart
```

**Manual certs:** copy `fullchain.pem` and `privkey.pem` to `deploy/nginx/ssl/`, then `make ssl-enable && make ssl-reload`.

See [Deployment](docs/DEPLOYMENT.md) for firewall, renewal, and multi-domain setups.

## Upload a video

```bash
curl -X POST http://localhost/api/v1/videos/upload \
  -H "X-API-Key: dev-secret-key" \
  -F "title=My Lecture" \
  -F "file=@video.mp4"
```

Check transcode progress in Asynqmon: http://localhost:3000

## Monitor transcode queue

Asynqmon is **not** proxied through nginx — it listens on host port **3000** only. On a VPS, that port is often blocked by the provider firewall or iptables/UFW.

### Asynqmon (web UI)

| Environment | URL |
|-------------|-----|
| Local | http://localhost:3000 |
| Production (direct) | `http://<server-ip>:3000` — only if port 3000 is open in firewall |
| Production (SSH tunnel) | see below |

**SSH tunnel** (recommended on production — no need to open port 3000 publicly):

```bash
ssh -L 3000:127.0.0.1:3000 user@your-server
```

Keep the session open, then open **http://localhost:3000** in your browser.

Verify Asynqmon is running on the server:

```bash
curl -s http://127.0.0.1:3000/ | head -1
docker compose -f deploy/docker-compose.yml ps asynqmon
```

Asynqmon has **no authentication** — do not expose `:3000` to the public internet unless you add nginx basic auth or restrict by IP.

### CLI (no GUI)

**Video status via API:**

```bash
curl -s "$CASTFLOW_BASE_URL/api/v1/videos/" -H "X-API-Key: dev-secret-key"
```

Status values: `processing` | `ready` | `error`

**Asynq queue in Redis** (queue name `castflow`, task type `transcode`):

```bash
docker compose -f deploy/docker-compose.yml exec -T redis redis-cli <<'EOF'
SMEMBERS asynq:queues
LLEN asynq:{castflow}:pending
LLEN asynq:{castflow}:active
ZCARD asynq:{castflow}:retry
ZCARD asynq:{castflow}:archived
EOF
```

| Redis key length | Meaning |
|------------------|---------|
| `pending > 0` | Job waiting for a worker |
| `active > 0` | Transcode in progress |
| `retry > 0` | Failed, will retry |
| `archived > 0` | Permanently failed |

**Database:**

```bash
docker compose -f deploy/docker-compose.yml exec -T postgres psql -U castflow -d castflow -c \
  "SELECT id, title, status, error_message FROM videos ORDER BY created_at DESC LIMIT 5;"
```

**Worker logs:**

```bash
make docker-logs-worker
make docker-logs
```

**All-in-one check** (from repo root):

```bash
echo "=== API ===" && \
curl -s "$(grep ^CASTFLOW_BASE_URL .env | cut -d= -f2)/api/v1/videos/" \
  -H "X-API-Key: $(grep ^CASTFLOW_API_KEY .env | cut -d= -f2)" && \
echo -e "\n\n=== Redis queue ===" && \
docker compose -f deploy/docker-compose.yml exec -T redis redis-cli <<'EOF'
LLEN asynq:{castflow}:pending
LLEN asynq:{castflow}:active
ZCARD asynq:{castflow}:retry
ZCARD asynq:{castflow}:archived
EOF
```

See [Docker debugging](docs/DEBUG.md) for more troubleshooting.

## Get playback links

```bash
curl http://localhost/api/v1/videos/{id}/links \
  -H "X-API-Key: dev-secret-key"
```

Response includes `hlsUrl`, `dashUrl`, `playerUrl`, `configUrl`, `thumbnailUrl`, `tooltipUrl`, `videoUrl`, and `iframe`.

## Custom domain (production)

Single domain (simplest — built-in nginx handles API, player, and media):

```env
CASTFLOW_BASE_URL=https://example.com
```

Separate API / CDN / player domains — uncomment in `.env` (see `deploy/.env.docker.example`):

```env
CASTFLOW_API_BASE_URL=https://api.example.com
CASTFLOW_CDN_BASE_URL=https://cdn.example.com
CASTFLOW_PLAYER_BASE_URL=https://player.example.com
```

For a separate CDN origin (S3 / RustFS), extend `deploy/nginx/conf.d/`. See [Deployment](docs/DEPLOYMENT.md).

## Requirements

- Go 1.24+
- FFmpeg + FFprobe
- PostgreSQL 16
- Redis 7 (Asynq backend)

## Documentation

- [Architecture](docs/ARCHITECTURE.md)
- [Deployment](docs/DEPLOYMENT.md)
- [Docker debugging](docs/DEBUG.md)
- [API Reference](docs/API.md)

## Project structure

```
castflow/
├── cmd/
│   ├── castflow/          # API entry point
│   └── worker/            # Transcode worker entry point
├── internal/
│   ├── domain/            # Entities, ports, URL builder
│   ├── application/       # Use cases
│   ├── adapter/           # HTTP, Postgres, Storage, FFmpeg, Queue
│   ├── config/
│   └── app/               # Wiring / bootstrap
├── migrations/
├── deploy/
│   ├── docker-compose.yml
│   ├── Dockerfile
│   └── nginx/             # Reverse proxy + TLS (conf.d, ssl/, certbot/)
├── web/player/            # Embedded player
└── docs/
```

## License

MIT
