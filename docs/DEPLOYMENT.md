# Production deployment with custom domain

## DNS

| Record | Points to |
|--------|-----------|
| `api.example.com` | Castflow API server |
| `cdn.example.com` | Nginx → S3 bucket (RustFS, AWS S3, …) |
| `player.example.com` | Nginx → Castflow `/player` or static |

## Environment (`.env`)

Postgres and Redis URLs are set in `deploy/docker-compose.yml` for Docker — do not duplicate them in `.env`.

```env
# Dev
CASTFLOW_BASE_URL=http://localhost:8080

# Production (separate domains)
CASTFLOW_API_BASE_URL=https://api.example.com
CASTFLOW_CDN_BASE_URL=https://cdn.example.com
CASTFLOW_PLAYER_BASE_URL=https://player.example.com

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

## Docker Compose services

| Service | Purpose |
|---------|---------|
| `castflow` | HTTP API |
| `castflow-worker` | FFmpeg transcode worker |
| `redis` | Asynq backend (`maxmemory-policy noeviction`) |
| `asynqmon` | Queue dashboard — restrict network access in production |
| `rustfs` | Optional S3-compatible object storage |
| `postgres` | Video metadata |

API and worker share `castflow_storage` and `castflow_tmp` volumes when using local storage.

## Nginx (CDN)

```nginx
server {
    listen 443 ssl http2;
    server_name cdn.example.com;

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

server {
    listen 443 ssl http2;
    server_name player.example.com;

    location / {
        proxy_pass http://castflow:8080/player/;
    }
}

server {
    listen 443 ssl http2;
    server_name api.example.com;

    client_max_body_size 2G;

    location / {
        proxy_pass http://castflow:8080;
        proxy_read_timeout 300s;
    }
}
```

Replace `rustfs:9000` with your S3 endpoint. For AWS S3, use the bucket virtual-host URL or CloudFront instead of this proxy block.

## Asynqmon (production)

The default Docker setup exposes Asynqmon without authentication. Restrict network access to `:3000` or put it behind a VPN / internal ingress with auth in front.

## Scaling workers

Run multiple `castflow-worker` replicas — they all consume from the same Asynq queue (`castflow`):

```bash
docker compose -f deploy/docker-compose.yml up -d --scale castflow-worker=3
```

Increase `CASTFLOW_WORKER_CONCURRENCY` per replica for parallel FFmpeg jobs (watch CPU/RAM).

## Docker production stack

```bash
make install
# or manually:
docker compose -f deploy/docker-compose.yml up -d
psql $CASTFLOW_DATABASE_URL -f migrations/001_init.sql
```

## Local ports (dev stack)

| Service | URL |
|---------|-----|
| API | http://localhost:8080 |
| Asynqmon | http://localhost:3000 |
| Redis | localhost:6380 |
| RustFS S3 | http://localhost:9000 |
| RustFS Console | http://localhost:9001 |
| Postgres | localhost:5433 |

## Generated links (example)

After `GET /api/v1/videos/{id}/links`:

```
hlsUrl:      https://cdn.example.com/v/{id}/hls/master.m3u8
dashUrl:     https://cdn.example.com/v/{id}/dash/manifest.mpd
configUrl:   https://cdn.example.com/v/{id}/config.json
playerUrl:   https://player.example.com/index.html?config=...
thumbnailUrl: https://cdn.example.com/v/{id}/thumbnail.jpg
tooltipUrl:  https://cdn.example.com/v/{id}/tooltip.vtt
videoUrl:    https://cdn.example.com/v/{id}/origin.mp4
iframe:      <embed code>
```
