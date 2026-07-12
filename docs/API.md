# Castflow API Reference

Base URL: `http://localhost:8080` (or `CASTFLOW_API_BASE_URL`)

Authentication: header `X-API-Key: <key>` on all `/api/v1/*` routes.

---

## Health

### `GET /health`

Liveness probe.

```json
{ "status": "ok" }
```

### `GET /ready`

Readiness probe.

```json
{ "status": "ready" }
```

---

## Videos

### `GET /api/v1/videos`

List videos (newest first).

**Query:** `limit` (default 20), `offset` (default 0)

**Response `200`:**

```json
{
  "items": [
    {
      "id": "uuid",
      "title": "My Video",
      "description": "",
      "status": "ready",
      "durationSec": 421,
      "fileSize": 16654321,
      "contentType": "video/mp4",
      "createdAt": "2026-07-12T12:00:00Z",
      "updatedAt": "2026-07-12T12:05:00Z"
    }
  ],
  "total": 1
}
```

---

### `POST /api/v1/videos/upload`

Upload a video and start transcoding.

**Content-Type:** `multipart/form-data`

| Field | Required | Description |
|-------|----------|-------------|
| `file` | yes | Video file (mp4, mov, mkv, ...) |
| `title` | no | Defaults to filename |
| `description` | no | Optional description |

**Response `202`:**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "title": "My Lecture",
  "status": "processing",
  "message": "upload accepted; transcoding started"
}
```

---

### `GET /api/v1/videos/{id}`

Get video metadata.

**Response `200`:** same shape as list item.

**Errors:** `404` not found

---

### `GET /api/v1/videos/{id}/status`

Poll processing status.

**Response `200`:**

```json
{
  "id": "uuid",
  "status": "processing"
}
```

When ready:

```json
{
  "id": "uuid",
  "status": "ready",
  "durationSec": 421
}
```

On error:

```json
{
  "id": "uuid",
  "status": "error",
  "error": "ffmpeg failed: ..."
}
```

**Status values:** `uploading` | `uploaded` | `processing` | `ready` | `error`

---

### `GET /api/v1/videos/{id}/links`

Get all playback URLs (equivalent to ArvanCloud "copy video links").

**Response `200`:**

```json
{
  "videoId": "uuid",
  "title": "My Lecture",
  "status": "ready",
  "links": {
    "videoId": "uuid",
    "title": "My Lecture",
    "hlsUrl": "https://cdn.example.com/v/uuid/hls/master.m3u8",
    "dashUrl": "https://cdn.example.com/v/uuid/dash/manifest.mpd",
    "configUrl": "https://cdn.example.com/v/uuid/config.json",
    "playerUrl": "https://player.example.com/index.html?config=...",
    "thumbnailUrl": "https://cdn.example.com/v/uuid/thumbnail.jpg",
    "tooltipUrl": "https://cdn.example.com/v/uuid/tooltip.vtt",
    "videoUrl": "https://cdn.example.com/v/uuid/origin.mp4",
    "iframe": "<style>...</style><div>...</div>"
  }
}
```

---

### `DELETE /api/v1/videos/{id}`

Delete video and all storage artifacts.

**Response:** `204 No Content`

---

## Player config (`config.json`)

Served at CDN path `v/{id}/config.json`:

```json
{
  "title": "My Lecture",
  "description": null,
  "mediaid": "uuid",
  "behavior": {
    "type": "video",
    "mode": "static",
    "preload": "auto",
    "autostart": false,
    "repeat": false,
    "mute": false
  },
  "appearance": {
    "lang": "fa",
    "controls": true,
    "displaytitle": true,
    "displaydescription": false
  },
  "source": [
    { "src": "https://cdn.example.com/v/uuid/hls/master.m3u8", "type": "application/x-mpegURL" },
    { "src": "https://cdn.example.com/v/uuid/dash/manifest.mpd", "type": "application/dash+xml" }
  ],
  "poster": "https://cdn.example.com/v/uuid/thumbnail.jpg",
  "thumbnail": "https://cdn.example.com/v/uuid/tooltip.vtt"
}
```

---

## Static routes

| Path | Description |
|------|-------------|
| `/media/v/{id}/...` | Transcoded assets (dev mode) |
| `/player/index.html?config=...` | Embedded player |

---

## Error format

```json
{ "error": "video not found" }
```

| HTTP | Meaning |
|------|---------|
| 400 | Invalid input |
| 401 | Missing/invalid API key |
| 404 | Video not found |
| 500 | Internal error |

---

## cURL examples

```bash
# Upload
curl -X POST http://localhost:8080/api/v1/videos/upload \
  -H "X-API-Key: dev-secret-key" \
  -F "title=NLP Lecture" \
  -F "file=@lecture.mp4"

# Poll status
curl http://localhost:8080/api/v1/videos/{id}/status \
  -H "X-API-Key: dev-secret-key"

# Get links
curl http://localhost:8080/api/v1/videos/{id}/links \
  -H "X-API-Key: dev-secret-key"
```
