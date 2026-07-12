# Castflow

Self-hosted VOD platform written in Go. Upload videos, transcode to HLS/DASH, and serve playback links on **your own domain**.

## Features

- REST API for video upload, status, and link generation
- FFmpeg transcoding (multi-bitrate HLS + DASH)
- Thumbnail, tooltip sprite (VTT + PNG)
- Custom CDN and player URLs (`config.json`, iFrame embed)
- Clean Architecture (domain → application → adapters)
- Docker Compose for local/production deploy
- Job queue: **Asynq** on Redis (with **Asynqmon** UI)
- Storage: local filesystem or S3-compatible (RustFS, AWS S3, …)

## Quick start (Docker — recommended)

```bash
cd castflow
make install
```

This will:
1. Create `.env` from `deploy/.env.docker.example`
2. Build the `castflow` Docker image
3. Start Postgres, Redis, Asynqmon, RustFS, Castflow API, and `castflow-worker`
4. Run database migrations

| URL | Description |
|-----|-------------|
| http://localhost:8080/health | Health check |
| http://localhost:8080/api/v1/videos | List videos |
| http://localhost:8080/player/index.html | Embedded player |
| http://localhost:3000 | Asynqmon (queue UI) |
| http://localhost:9001 | RustFS console (optional S3) |

```bash
make docker-logs    # follow API logs
make docker-down    # stop (keep data)
make uninstall      # stop + remove volumes
```

## Quick start (local Go)

```bash
cp .env.example .env
make docker-deps    # Postgres, Redis, RustFS, Asynqmon
make migrate-local
make run            # API + embedded worker (default)
```

Default API key: `dev-secret-key` (set `CASTFLOW_API_KEY` in `.env` for production).

### Separate worker (local)

```bash
# Terminal 1 — API only
CASTFLOW_ENABLE_EMBEDDED_WORKER=false make run

# Terminal 2 — worker
make run-worker
```

## Stack overview

| Service | Role | Host port |
|---------|------|-----------|
| `castflow` | HTTP API | 8080 |
| `castflow-worker` | FFmpeg transcode jobs | — |
| Redis | Asynq backend | 6380 |
| Asynqmon | Queue dashboard | 3000 |
| RustFS | S3-compatible storage (optional) | 9000 / 9001 |
| Postgres | Video metadata | 5433 |

**Queue:** Asynq queue `castflow`, task type `transcode`. Monitor jobs at http://localhost:3000 (visible after the first upload).

**Storage:** Docker defaults to **local** volumes (`CASTFLOW_STORAGE_DRIVER=local`). RustFS runs in the stack but is only used when you switch to `CASTFLOW_STORAGE_DRIVER=s3` — see `.env.example`.

**Worker modes:**

| Environment | API worker | Transcode runs in |
|-------------|------------|-------------------|
| Docker (`make install`) | disabled | `castflow-worker` container |
| Local (`make run`) | enabled (default) | same process as API |

Set `CASTFLOW_ENABLE_EMBEDDED_WORKER=false` when using `make run-worker` or the Docker worker container.

## Makefile commands

| Command | Description |
|---------|-------------|
| `make install` | Full Docker install (build + up + migrate) |
| `make uninstall` | Remove stack and volumes |
| `make docker-up` | Start all containers |
| `make docker-restart` | Rebuild and restart API |
| `make docker-migrate` | Apply SQL migrations |
| `make docker-deps` | Infra only (for local `make run`) |
| `make run` | Run API locally |
| `make run-worker` | Run transcode worker only |
| `make build` | Build `bin/castflow` and `bin/worker` |

## Upload a video

```bash
curl -X POST http://localhost:8080/api/v1/videos/upload \
  -H "X-API-Key: dev-secret-key" \
  -F "title=My Lecture" \
  -F "file=@video.mp4"
```

Check transcode progress in Asynqmon: http://localhost:3000

## Get playback links

```bash
curl http://localhost:8080/api/v1/videos/{id}/links \
  -H "X-API-Key: dev-secret-key"
```

Response includes `hlsUrl`, `dashUrl`, `playerUrl`, `configUrl`, `thumbnailUrl`, `tooltipUrl`, `videoUrl`, and `iframe`.

## Custom domain (production)

Set in `.env`:

```env
CASTFLOW_CDN_BASE_URL=https://cdn.example.com
CASTFLOW_PLAYER_BASE_URL=https://player.example.com
CASTFLOW_API_BASE_URL=https://api.example.com
```

Point Nginx to your S3 bucket (RustFS, AWS S3, …) for `/v/` paths and to Castflow for `/player/`. See [Deployment](docs/DEPLOYMENT.md).

## Requirements

- Go 1.24+
- FFmpeg + FFprobe
- PostgreSQL 16
- Redis 7 (Asynq backend)

## Documentation

- [Architecture](docs/ARCHITECTURE.md)
- [Deployment](docs/DEPLOYMENT.md)
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
├── web/player/            # Embedded player
└── docs/
```

## License

MIT
