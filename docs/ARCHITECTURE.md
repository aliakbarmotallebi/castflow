# Castflow — Architecture

## Overview

Castflow is a **self-hosted Video-on-Demand control plane**. It accepts video uploads, transcodes them with FFmpeg into adaptive streaming packages (HLS + DASH), generates player configuration and preview assets, and exposes all playback URLs on configurable custom domains.

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   Client    │────▶│  Castflow    │────▶│  PostgreSQL │
│ (API/Admin) │     │  API Server  │     │  (metadata)  │
└─────────────┘     └──────┬───────┘     └─────────────┘
                           │ enqueue
                    ┌──────▼───────┐     ┌─────────────┐
                    │ Redis/Asynq  │────▶│   Worker    │
                    │ queue:castflow│    │  (FFmpeg)   │
                    └──────┬───────┘     └──────┬──────┘
                           │                    │
                    ┌──────▼───────┐             │
                    │   Asynqmon   │             │
                    │  (queue UI)  │             │
                    └──────────────┘             │
                           ┌────────────────────▼────────────────────┐
                           │  Object Storage (local / RustFS / S3)   │
                           │  v/{id}/origin.mp4, hls/, dash/, ...    │
                           └────────────────────┬────────────────────┘
                                                │
                           ┌────────────────────▼────────────────────┐
                           │  CDN (Nginx) → cdn.example.com          │
                           │  Player      → player.example.com       │
                           └─────────────────────────────────────────┘
```

## Clean Architecture layers

### Domain (`internal/domain`)

Pure business logic. No external dependencies.

| Package | Responsibility |
|---------|----------------|
| `video.go` | `Video` entity, status transitions |
| `playback.go` | `PlaybackLinks`, `PlayerConfig` DTOs |
| `ports.go` | Repository and infrastructure interfaces |
| `url_builder.go` | URL generation for custom domains |
| `errors.go` | Domain errors |

**Dependency rule:** domain imports nothing from application or adapters.

### Application (`internal/application`)

Use cases orchestrate domain + ports.

| Use case | Description |
|----------|-------------|
| `UploadVideo` | Store origin file, save metadata, enqueue transcode |
| `ProcessVideo` | Download origin, FFmpeg pipeline, upload artifacts, write config.json |
| `GetVideoLinks` | Build all public URLs for a video |
| `ListVideos` | Paginated listing |
| `GetVideo` | Single video metadata |
| `DeleteVideo` | Remove DB row + storage prefix |

### Adapters (`internal/adapter`)

Infrastructure implementations of domain ports.

| Adapter | Port | Technology |
|---------|------|------------|
| `postgres` | `VideoRepository` | PostgreSQL + pgx |
| `storage` | `ObjectStorage` | Local FS / S3-compatible (minio-go client) |
| `ffmpeg` | `Transcoder` | FFmpeg CLI |
| `queue` | `JobQueue` | Redis + Asynq (`hibiken/asynq`) |
| `http` | Delivery | chi router |

### Composition root (`internal/app`)

Wires all dependencies. Two entry points:

| Binary | Role |
|--------|------|
| `cmd/castflow` | HTTP API; optional embedded worker (`CASTFLOW_ENABLE_EMBEDDED_WORKER`) |
| `cmd/worker` | Transcode worker only (Docker: `castflow-worker` service) |

## Job queue

- **Backend:** Redis (required — Asynq uses Redis for task storage)
- **Library:** `github.com/hibiken/asynq`
- **Queue name:** `castflow` (hardcoded in `internal/adapter/queue`)
- **Task type:** `transcode`
- **Monitoring:** Asynqmon at `:3000` (Docker service `asynqmon`)

## Video lifecycle

```
uploading → uploaded → processing → ready
                              ↘ error
```

1. **Upload** — Client POSTs multipart file. Origin saved to `v/{id}/origin.mp4`.
2. **Enqueue** — Asynq task pushed to Redis queue `castflow`.
3. **Process** — Worker runs FFmpeg: HLS variants, DASH manifest, thumbnail, tooltip sprite.
4. **Publish** — Artifacts uploaded to storage. `config.json` written with CDN URLs.
5. **Ready** — `GET /videos/{id}/links` returns HLS, DASH, player, iFrame, etc.

## Storage layout

```
v/{video_id}/
├── origin.mp4
├── config.json
├── thumbnail.jpg
├── tooltip.png
├── tooltip.vtt
├── hls/
│   ├── master.m3u8
│   ├── 360p/playlist.m3u8 + segments
│   ├── 720p/...
│   └── 1080p/...
└── dash/
    ├── manifest.mpd
    └── chunk-*.m4s
```

## URL generation

`domain.URLBuilder` uses two configurable base URLs:

- `CASTFLOW_CDN_BASE_URL` — media assets
- `CASTFLOW_PLAYER_BASE_URL` — embedded player page

Example output for video `abc-123`:

```
https://cdn.example.com/v/abc-123/hls/master.m3u8
https://cdn.example.com/v/abc-123/config.json
https://player.example.com/index.html?config=...
```

## Transcode pipeline

| Step | Tool | Output |
|------|------|--------|
| Thumbnail | `ffmpeg -ss 1 -vframes 1` | `thumbnail.jpg` |
| Tooltip | `ffmpeg fps=1/5` + tile filter | `tooltip.png` + `tooltip.vtt` |
| HLS | per-quality encode + segment | `hls/{quality}/playlist.m3u8` |
| Master | Go template | `hls/master.m3u8` |
| DASH | `ffmpeg -f dash` | `dash/manifest.mpd` |

Default qualities: 360p, 720p, 1080p (configurable via `CASTFLOW_QUALITIES`).

## Deployment modes

### Single node (dev)

- Storage: local `./data/storage`
- CDN: `http://localhost:8080/media`
- Player: `http://localhost:8080/player`
- Queue UI: `http://localhost:3000`
- Worker: embedded in `make run`, or separate `make run-worker`

### Docker Compose

- API container: worker disabled (`CASTFLOW_ENABLE_EMBEDDED_WORKER=false`)
- `castflow-worker` container: runs `/app/worker`
- Shared volumes: `castflow_storage`, `castflow_tmp` (API and worker must share storage)

### Production

- Storage: RustFS, AWS S3, or any S3-compatible backend
- CDN: Nginx → `cdn.example.com` → object storage bucket
- Player: Nginx → `player.example.com` → Castflow or static
- API: `api.example.com` → Castflow
- Scale workers: run multiple `castflow-worker` replicas (same Redis queue)

## Design decisions

| Decision | Rationale |
|----------|-----------|
| FFmpeg CLI vs libav | Simpler ops, universal, easy to debug |
| Asynq on Redis | Mature Go-native queue, Asynqmon UI, retries and concurrency |
| chi vs gin | Lightweight, stdlib-compatible |
| Local storage default | Zero-deps dev experience |
| Split API / worker in Docker | Scale transcode independently from HTTP |

## Extension points

- Add `StorageProvider` implementations (Bunny, Cloudflare R2)
- Webhook on `ready` status
- API auth: JWT, OAuth
- Configurable queue name via env
- Speech-to-text subtitle pipeline
- DRM packaging
