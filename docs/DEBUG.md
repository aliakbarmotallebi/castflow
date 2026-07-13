# Docker debugging guide

How to inspect, troubleshoot, and debug the Castflow Docker stack.

## Quick checklist

```bash
make docker-ps          # container status + health
make docker-health      # HTTP checks through nginx
make docker-logs        # API logs (follow)
make docker-logs-worker # transcode worker logs
make docker-logs-nginx  # reverse proxy logs
```

If `make install` fails on health check:

1. `make docker-ps` — are `postgres`, `redis`, `castflow`, `nginx` up?
2. `make docker-logs` — API crash on startup?
3. Check `.env` — `CASTFLOW_BASE_URL` must match how you reach nginx (`http://localhost`, not `:8080`)
4. `make docker-health` — see which hop fails (nginx vs castflow)

## Container layout

```
Host :80 / :443
    └── nginx
            └── castflow:8080  (API + /player/ + /media/)
                    ├── postgres:5432
                    └── redis:6379

castflow-worker  → postgres, redis (FFmpeg jobs)
asynqmon :3000   → redis
rustfs :9000/9001
```

Castflow is **not** exposed on host port 8080. Always test through nginx unless you `docker exec` into the network.

## Makefile debug commands

| Command | Description |
|---------|-------------|
| `make docker-ps` | Status of all services |
| `make docker-health` | Curl `/health` via public URL + internal castflow |
| `make docker-logs` | Follow `castflow` (API) logs |
| `make docker-logs-worker` | Follow `castflow-worker` logs |
| `make docker-logs-nginx` | Follow `nginx` logs |
| `make docker-logs-all` | Follow all service logs |
| `make docker-shell SERVICE=castflow` | Shell inside a container |
| `make docker-restart` | Recreate API, worker, nginx (after `.env` change) |

Raw compose (project dir = repo root):

```bash
docker compose -f deploy/docker-compose.yml ps
docker compose -f deploy/docker-compose.yml logs -f castflow castflow-worker
```

## Verbose application logs

Set in `.env`:

```env
CASTFLOW_LOG_LEVEL=debug
```

Restart app containers (no image rebuild):

```bash
make docker-restart
make docker-logs
```

Levels: `debug`, `info` (default), `warn`, `error`. Logs are JSON on stdout.

## Service-by-service

### nginx

```bash
make docker-logs-nginx
make docker-shell SERVICE=nginx
# inside container:
nginx -t
cat /etc/nginx/conf.d/castflow.conf
ls -la /etc/nginx/ssl/
```

Test config without rebuild:

```bash
make ssl-reload    # runs nginx -t then reload
```

**HTTPS not working (connection refused on :443):**

- Certs missing → `make ssl-enable` failed; place certs in `deploy/nginx/ssl/` or run `make ssl-install-certs DOMAIN=…`
- Only HTTP works until `castflow-ssl.conf` exists

**Upload fails / 413:**

- `client_max_body_size 2G` is in `deploy/nginx/nginx.conf`
- Large uploads also need `proxy_read_timeout` (already 300s in `castflow.conf`)

### castflow (API)

```bash
make docker-logs
make docker-shell SERVICE=castflow
```

Inside the container:

```sh
wget -qO- http://127.0.0.1:8080/health
ls -la /app/data/storage /app/data/tmp
ffmpeg -version
```

Bypass nginx from the host (diagnose proxy vs app):

```bash
docker compose -f deploy/docker-compose.yml exec castflow wget -qO- http://127.0.0.1:8080/health
```

### castflow-worker (transcode)

```bash
make docker-logs-worker
```

Check the queue UI: http://localhost:3000 (Asynqmon).

Common issues:

- Job stuck in **retry** → FFmpeg error; read worker logs
- No jobs at all → Redis URL wrong or outbox not publishing (`make docker-logs` for outbox relay on API)

### postgres

```bash
docker compose -f deploy/docker-compose.yml exec postgres psql -U castflow -d castflow
```

Useful queries:

```sql
\dt
SELECT id, title, status, created_at FROM videos ORDER BY created_at DESC LIMIT 10;
SELECT * FROM outbox WHERE published_at IS NULL LIMIT 10;
```

Re-run migrations:

```bash
make docker-migrate
```

### redis

```bash
docker compose -f deploy/docker-compose.yml exec redis redis-cli
```

```redis
PING
KEYS asynq:*
```

### rustfs (S3)

Console: http://localhost:9001  
API: http://localhost:9000

Only relevant when `CASTFLOW_STORAGE_DRIVER=s3` in `.env`.

## End-to-end HTTP debug

```bash
# 1. Public URL (through nginx)
curl -v "$(grep CASTFLOW_BASE_URL .env | cut -d= -f2)/health"

# 2. Or use make
make docker-health

# 3. Upload test
curl -v -X POST "$CASTFLOW_BASE_URL/api/v1/videos/upload" \
  -H "X-API-Key: dev-secret-key" \
  -F "title=Debug" \
  -F "file=@small.mp4"
```

With `CASTFLOW_BASE_URL` from `.env`:

```bash
export $(grep -v '^#' .env | xargs)
curl -v "$CASTFLOW_BASE_URL/health"
```

## Common problems

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| `Connection refused` on `:443` | HTTPS not enabled | `make ssl-install-certs` or `make ssl-enable` |
| `Connection refused` on `:8080` | API not exposed on host | Use `http://localhost` (nginx :80) |
| `make install` health check fails | Wrong `CASTFLOW_BASE_URL` | Set `http://localhost` (or your domain) |
| `401` / `403` on API | Wrong API key | Match `X-API-Key` with `CASTFLOW_API_KEY` in `.env` |
| Upload OK, no transcode | Worker down or queue issue | `make docker-logs-worker`, check Asynqmon |
| `ssl-certbot` permission error on copy | Certbot wrote root-owned files | `make ssl-install-certs DOMAIN=…` |
| Changes to `.env` not reflected | Containers use old env | `make docker-restart` |
| Code changes not applied | Image not rebuilt | `make install-build` |

## Reset / clean slate

```bash
make docker-down      # stop, keep volumes
make uninstall        # stop + delete volumes (data loss)
make install-build    # fresh start
```

## Local Go debug (outside Docker)

Run API on the host against Docker Postgres/Redis — expose DB/Redis temporarily or use SSH tunnel. For most cases, prefer logs inside Docker:

```bash
make build
CASTFLOW_DATABASE_URL=... CASTFLOW_REDIS_URL=... ./bin/castflow
```

## Related docs

- [Deployment](DEPLOYMENT.md) — SSL, firewall, production
- [API Reference](API.md) — endpoints and auth
