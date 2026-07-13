# Castflow — مستند فنی کامل

> **Castflow** یک پلتفرم VOD (Video-on-Demand) خودمیزبان است که با Go نوشته شده. ویدیوها را آپلود می‌کند، با FFmpeg به HLS/DASH تبدیل می‌کند، و لینک‌های پخش را روی دامنهٔ اختصاصی شما ارائه می‌دهد.

---

## فهرست مطالب

1. [معرفی و اهداف](#1-معرفی-و-اهداف)
2. [ویژگی‌ها](#2-ویژگی‌ها)
3. [پشتهٔ فناوری](#3-پشتهٔ-فناوری)
4. [معماری سیستم](#4-معماری-سیستم)
5. [لایه‌های Clean Architecture](#5-لایه‌های-clean-architecture)
6. [جریان داده و چرخهٔ عمر ویدیو](#6-جریان-داده-و-چرخهٔ-عمر-ویدیو)
7. [الگوی Outbox و صف کارها](#7-الگوی-outbox-و-صف-کارها)
8. [پایپلاین Transcode (FFmpeg)](#8-پایپلاین-transcode-ffmpeg)
9. [ذخیره‌سازی (Storage)](#9-ذخیره‌سازی-storage)
10. [ساخت URL و Playback Variant](#10-ساخت-url-و-playback-variant)
11. [پلیر تعبیه‌شده (Embedded Player)](#11-پلیر-تعبیه‌شده-embedded-player)
12. [API REST](#12-api-rest)
13. [اسکیمای پایگاه داده](#13-اسکیمای-پایگاه-داده)
14. [پیکربندی (Environment Variables)](#14-پیکربندی-environment-variables)
15. [استقرار با Docker Compose](#15-استقرار-با-docker-compose)
16. [SSL / HTTPS](#16-ssl--https)
17. [استقرار Production](#17-استقرار-production)
18. [مانیتورینگ و دیباگ](#18-مانیتورینگ-و-دیباگ)
19. [ساختار پروژه](#19-ساختار-پروژه)
20. [دستورات Makefile](#20-دستورات-makefile)
21. [تصمیمات طراحی](#21-تصمیمات-طراحی)
22. [نقاط توسعهٔ آینده](#22-نقاط-توسعهٔ-آینده)

---

## 1. معرفی و اهداف

Castflow جایگزینی self-hosted برای سرویس‌های VOD ابری (مانند ArvanCloud VOD) است. هدف اصلی:

- **کنترل کامل** روی زیرساخت، داده و هزینه
- **دامنهٔ اختصاصی** برای API، CDN، و Player
- **Transcode چند کیفیت** (Adaptive Streaming) با HLS و DASH
- **استقرار ساده** با Docker Compose و Makefile
- **معماری تمیز** برای نگهداری و توسعهٔ بلندمدت

### سناریوهای استفاده

| سناریو | توضیح |
|--------|-------|
| آموزش آنلاین | آپلود ویدیوهای درس، embed در LMS |
| پلتفرم داخلی | VOD سازمانی بدون وابستگی به CDN خارجی |
| توسعهٔ محلی | تست pipeline transcode قبل از production |

---

## 2. ویژگی‌ها

| دسته | قابلیت |
|------|--------|
| **API** | آپلود، لیست، وضعیت، لینک‌های پخش، حذف |
| **Transcode** | HLS چند bitrate + DASH + thumbnail + tooltip sprite (VTT+PNG) |
| **Player** | صفحهٔ embed با Video.js، پشتیبانی RTL/فارسی |
| **صف کار** | Asynq روی Redis + Asynqmon UI |
| **ذخیره‌سازی** | Local filesystem یا S3-compatible (RustFS, AWS S3, …) |
| **Proxy** | Nginx با TLS اختیاری (Let's Encrypt) |
| **Reliability** | Transactional Outbox برای enqueue مطمئن jobها |

---

## 3. پشتهٔ فناوری

| لایه | فناوری | نسخه |
|------|--------|------|
| زبان | Go | 1.24+ |
| HTTP Router | chi | v5 |
| Database | PostgreSQL | 16 |
| Queue | Redis + Asynq | Redis 7 |
| Transcode | FFmpeg + FFprobe | (در Docker image) |
| Object Storage | minio-go (S3 API) | v7 |
| DB Driver | pgx | v5 |
| Reverse Proxy | Nginx | 1.27 |
| S3 Storage (اختیاری) | RustFS | latest |
| Queue UI | Asynqmon | latest |
| Player | Video.js | 8.x |

### وابستگی‌های Go (go.mod)

```
github.com/go-chi/chi/v5
github.com/google/uuid
github.com/hibiken/asynq
github.com/jackc/pgx/v5
github.com/minio/minio-go/v7
```

---

## 4. معماری سیستم

### نمای کلی

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   Client    │────▶│    Nginx     │────▶│  Castflow   │
│ (API/Admin) │     │  :80 / :443  │     │  API :8080  │
└─────────────┘     └──────────────┘     └──────┬──────┘
                                                  │
                    ┌─────────────────────────────┼─────────────────────────────┐
                    │                             │                             │
             ┌──────▼──────┐              ┌───────▼───────┐            ┌────────▼────────┐
             │ PostgreSQL  │              │ Redis/Asynq   │            │ Object Storage  │
             │ (metadata)  │              │ queue:castflow│            │ local / S3      │
             └─────────────┘              └───────┬───────┘            └────────┬────────┘
                                                  │                             │
                                           ┌──────▼───────┐                     │
                                           │ castflow-    │─────────────────────┘
                                           │ worker       │
                                           │ (FFmpeg)     │
                                           └──────────────┘
                                                  │
                                           ┌──────▼───────┐
                                           │  Asynqmon    │
                                           │  :3000       │
                                           └──────────────┘
```

### دو باینری اجرایی

| باینری | مسیر | نقش |
|--------|------|-----|
| **castflow** | `cmd/castflow/main.go` | HTTP API + Outbox Relay + Worker اختیاری |
| **worker** | `cmd/worker/main.go` | فقط مصرف‌کنندهٔ صف transcode |

در Docker:
- سرویس `castflow`: API + embedded worker (fallback)
- سرویس `castflow-worker`: worker اختصاصی transcode

---

## 5. لایه‌های Clean Architecture

قانون وابستگی: **Domain ← Application ← Adapters**. Domain هیچ import از لایه‌های بیرونی ندارد.

### 5.1 Domain (`internal/domain`)

منطق کسب‌وکار خالص، بدون وابستگی خارجی.

| فایل | مسئولیت |
|------|---------|
| `video.go` | Entity `Video`، وضعیت‌ها، transitionها |
| `playback.go` | DTOهای `PlaybackLinks`، `PlayerConfig` |
| `ports.go` | Interfaceهای Repository و Infrastructure |
| `url_builder.go` | ساخت URL عمومی + کلیدهای storage |
| `variant.go` | شناسهٔ variant برای versioning خروجی transcode |
| `outbox.go` | Event types و payload outbox |
| `errors.go` | خطاهای دامنه (`ErrNotFound`, `ErrInvalidInput`, …) |

#### وضعیت‌های ویدیو (`VideoStatus`)

```
uploading → uploaded → processing → ready
                              ↘ error
```

| وضعیت | معنی |
|-------|------|
| `uploading` | در حال آپلود (entity اولیه) |
| `uploaded` | فایل origin ذخیره شد |
| `processing` | job transcode در صف/در حال اجرا |
| `ready` | transcode کامل، لینک‌ها قابل استفاده |
| `error` | خطا در transcode (پیام در `error_message`) |

#### Ports (Interfaceها)

```go
VideoRepository      // CRUD متادیتای ویدیو
ObjectStorage        // آپلود/دانلود/حذف فایل
Transcoder           // FFmpeg pipeline
JobQueue             // enqueue transcode
VideoUploadWriter    // ذخیرهٔ اتمیک metadata + outbox
OutboxRelay          // publish outbox → queue
```

### 5.2 Application (`internal/application`)

Use caseها — orchestration بین domain و ports.

| Use Case | فایل/کلاس | توضیح |
|----------|-----------|-------|
| `UploadVideo` | `video.go` | آپلود origin، ذخیره metadata، enqueue |
| `ProcessVideo` | `video.go` | دانلود origin، FFmpeg، آپلود artifacts، config.json |
| `GetVideoLinks` | `video.go` | ساخت تمام URLهای پخش |
| `ListVideos` | `video.go` | لیست paginated |
| `GetVideo` | `video.go` | متادیتای یک ویدیو |
| `DeleteVideo` | `video.go` | حذف DB + storage prefix |

### 5.3 Adapters (`internal/adapter`)

پیاده‌سازی infrastructure.

| Adapter | Port | فناوری |
|---------|------|--------|
| `postgres/` | `VideoRepository`, `VideoUploadWriter`, Outbox | PostgreSQL + pgx |
| `storage/` | `ObjectStorage` | Local FS / S3 (minio-go) |
| `ffmpeg/` | `Transcoder` | FFmpeg CLI |
| `queue/` | `JobQueue`, `OutboxRelay` | Asynq + Redis |
| `http/` | Delivery | chi router |

### 5.4 Composition Root (`internal/app`)

| فایل | نقش |
|------|-----|
| `bootstrap.go` | Wire کردن تمام dependencyها |
| `app.go` | HTTP server + outbox relay + embedded worker |
| `worker.go` | فقط worker process |

---

## 6. جریان داده و چرخهٔ عمر ویدیو

### 6.1 آپلود

```
Client ──POST /api/v1/videos/upload──▶ HTTP Handler
                                           │
                                           ▼
                                    UploadVideo.Execute()
                                           │
                    ┌──────────────────────┼──────────────────────┐
                    │                      │                      │
                    ▼                      ▼                      ▼
            storage.Upload()      video.MarkUploaded()    video.MarkProcessing()
            v/{id}/origin.mp4                              │
                                                           ▼
                                              UploadWriter.AcceptUpload()
                                              (TX: INSERT video + outbox)
```

### 6.2 Outbox → Queue → Process

```
OutboxRelay (poll هر 1s)
    │
    ▼
PublishBatch → EnqueueTranscode → Redis/Asynq
    │
    ▼
Worker (castflow-worker یا embedded)
    │
    ▼
ProcessVideo.Execute()
    ├── Download origin از storage
    ├── FFmpeg: thumbnail, tooltip, HLS, DASH
    ├── Upload artifacts
    ├── Upload config.json
    └── video.MarkReady() + UPDATE DB
```

### 6.3 دریافت لینک‌های پخش

```
Client ──GET /api/v1/videos/{id}/links──▶ GetVideoLinks
                                               │
                                               ▼
                                    URLBuilder.BuildLinksForVideo()
                                               │
                                               ▼
                              hlsUrl, dashUrl, playerUrl, iframe, ...
```

---

## 7. الگوی Outbox و صف کارها

### چرا Outbox؟

بدون outbox، اگر بعد از `INSERT video` خطا در enqueue رخ دهد، ویدیو بدون job transcode باقی می‌ماند. Outbox تضمین می‌کند:

1. **ذخیرهٔ metadata** و **ثبت event** در یک transaction
2. Relay جداگانه event را به Asynq publish می‌کند
3. در صورت خطا، `attempts` افزایش می‌یابد و retry می‌شود

### Asynq

| پارامتر | مقدار |
|---------|-------|
| Queue name | `castflow` (ثابت در کد) |
| Task type | `transcode` |
| Max retry | 3 |
| Timeout | 2 ساعت |
| Concurrency | `CASTFLOW_WORKER_CONCURRENCY` (پیش‌فرض: 2) |

### Payload task

```json
{ "videoId": "550e8400-e29b-41d4-a716-446655440000" }
```

### Redis keys (Asynq)

| Key pattern | معنی |
|-------------|------|
| `asynq:{castflow}:pending` | job در انتظار worker |
| `asynq:{castflow}:active` | transcode در حال اجرا |
| `asynq:{castflow}:retry` | failed، retry می‌شود |
| `asynq:{castflow}:archived` | permanently failed |

---

## 8. پایپلاین Transcode (FFmpeg)

### مراحل (`internal/adapter/ffmpeg/transcoder.go`)

| مرحله | ابزار | خروجی |
|-------|-------|-------|
| Thumbnail | `ffmpeg -ss {sec} -vframes 1` | `thumbnail.jpg` |
| Tooltip frames | `ffmpeg fps=1/interval scale=160x90` | فریم‌های JPG |
| Tooltip sprite | `ffmpeg tile=cols×rows` | `tooltip.png` |
| Tooltip VTT | Go (WebVTT writer) | `tooltip.vtt` |
| HLS per quality | libx264 + AAC, segment 6s | `hls/{variant}/{quality}/playlist.m3u8` |
| HLS Master | Go template | `hls/{variant}/master.m3u8` |
| DASH | `ffmpeg -f dash` | `dash/{variant}/manifest.mpd` |

### کیفیت‌های پیش‌فرض

| نام | Resolution | Video Bitrate | Audio Bitrate |
|-----|------------|---------------|---------------|
| 360p | 640×360 | 800k | 96k |
| 720p | 1280×720 | 2500k | 128k |
| 1080p | 1920×1080 | 5000k | 128k |

همچنین 144p, 240p, 480p در presets موجودند و با `CASTFLOW_QUALITIES` قابل تنظیم.

### فیلتر scale

```
scale=W:H:force_original_aspect_ratio=decrease,
pad=W:H:(ow-iw)/2:(oh-ih)/2
```

Aspect ratio حفظ می‌شود و letterbox/pillarbox اضافه می‌شود.

### Tooltip

- هر `CASTFLOW_TOOLTIP_INTERVAL_SEC` ثانیه (پیش‌فرض: 5) یک فریم
- حداکثر `CASTFLOW_TOOLTIP_MAX_FRAMES` فریم (پیش‌فرض: 60)
- Sprite در grid با `CASTFLOW_TOOLTIP_COLS` ستون (پیش‌فرض: 10)
- VTT با `#xywh=` برای hover preview در player

---

## 9. ذخیره‌سازی (Storage)

### Layout فایل‌ها

```
v/{video_id}/
├── origin.mp4
├── config.json
├── thumbnail.jpg
├── tooltip.png
├── tooltip.vtt
├── hls/
│   └── {playback_variant}/
│       ├── master.m3u8
│       ├── 360p/
│       │   ├── playlist.m3u8
│       │   └── seg_00001.ts ...
│       ├── 720p/...
│       └── 1080p/...
└── dash/
    └── {playback_variant}/
        ├── manifest.mpd
        ├── init-*.m4s
        └── chunk-*.m4s
```

### Driverها

| Driver | Env | کاربرد |
|--------|-----|--------|
| `local` | `CASTFLOW_STORAGE_DRIVER=local` | Dev، single-node Docker |
| `s3` | `CASTFLOW_STORAGE_DRIVER=s3` | Production، RustFS، AWS S3 |

### Local Storage

- مسیر: `CASTFLOW_STORAGE_LOCAL_DIR` (Docker: `/app/data/storage`)
- سرو HTTP: `/media/` توسط `MediaServer` در API
- Volume مشترک بین `castflow` و `castflow-worker`

### S3 Storage

- Client: minio-go
- Bucket auto-create با `EnsureBucket()`
- Public URL از `CASTFLOW_CDN_BASE_URL` ساخته می‌شود (نه endpoint داخلی S3)

---

## 10. ساخت URL و Playback Variant

### URL Builder (`internal/domain/url_builder.go`)

دو base URL قابل تنظیم:

| متغیر | نقش | پیش‌فرض (از BASE_URL) |
|-------|-----|----------------------|
| `CASTFLOW_CDN_BASE_URL` | media assets | `{BASE}/media` |
| `CASTFLOW_PLAYER_BASE_URL` | player page | `{BASE}/player` |
| `CASTFLOW_API_BASE_URL` | API | `{BASE}` |

### مثال خروجی (video `abc-123`)

```
https://cdn.example.com/v/abc-123/hls/default/a3f2b1c4/master.m3u8
https://cdn.example.com/v/abc-123/dash/default/a3f2b1c4/manifest.mpd
https://cdn.example.com/v/abc-123/config.json
https://player.example.com/index.html?config=https%3A%2F%2Fcdn...
```

### Playback Profile + Revision

#### ایدهٔ طراحی

به‌جای embed کردن bitrate یا ladder کامل داخل URL (مثل `h_,360_800,720_2500,k__…`)، مسیر خروجی به دو بخش تقسیم شده:

| بخش | نقش | پایدار؟ |
|-----|-----|---------|
| **profile** | نام معنادار پروفایل transcode | بله — تا وقتی config عوض نشود |
| **revision** | fingerprint کوتاه تنظیمات مؤثر | فقط وقتی ladder/encoder عوض شود |

این مدل سه مشکل را حل می‌کند:

1. **CDN cache** — تغییر ladder → revision جدید → URL جدید → نیازی به purge دستی نیست
2. **لینک‌های embed** — ویدیوهای قدیمی روی revision قبلی خودشان می‌مانند
3. **چند خروجی** — یک ویدیو هم `default` (دسکتاپ) هم `mobile` (موبایل) دارد

#### مسیر Storage و URL

```
v/{videoId}/
├── origin.mp4
├── config.json              ← شامل renditions[] + primary
├── thumbnail.jpg            ← مشترک بین profileها
├── tooltip.vtt / .png       ← مشترک
├── hls/
│   ├── default/
│   │   └── {revision}/
│   │       ├── master.m3u8
│   │       ├── 360p/playlist.m3u8 + segments
│   │       └── 720p/...
│   └── mobile/
│       └── {revision}/
│           └── ...
└── dash/
    ├── default/{revision}/manifest.mpd
    └── mobile/{revision}/manifest.mpd
```

URL عمومی:

```
v/{videoId}/hls/{profile}/{revision}/master.m3u8
v/{videoId}/dash/{profile}/{revision}/manifest.mpd
```

مثال:

```
v/abc/hls/default/a3f2b1c4/master.m3u8
v/abc/hls/mobile/deadbeef/master.m3u8
```

| بخش | معنی | مثال |
|-----|------|------|
| **profile** | پروفایل هدف (از env) | `default`, `mobile`, `cinema` |
| **revision** | 8 کاراکتر اول SHA256 تنظیمات | `a3f2b1c4` |

#### ساخت revision (کد)

تابع `domain.BuildRevision()` در `internal/domain/variant.go`:

```go
// ورودی hash:
payload := {
  profile:         "default",
  qualities:       [{name, width, height, videoBitrate, audioBitrate}, ...],  // مرتب‌شده بر height
  hlsSegmentSec:   6,
  thumbnailAtSec:  1.0,
  tooltipIntervalSec: 5.0
}
revision = sha256(json(payload))[:8]   // مثلاً "a3f2b1c4"
variantPath = profile + "/" + revision  // "default/a3f2b1c4"
```

نکات مهم:

- **bitrate داخل URL نیست** — فقط داخل hash؛ پس URL کوتاه و خوانا می‌ماند
- **ترتیب qualities نرمال می‌شود** (sort by height) — `720p,360p` و `360p,720p` revision یکسان می‌سازند
- تغییر `CASTFLOW_HLS_SEGMENT_SECONDS` یا tooltip settings → revision جدید
- تغییر bitrate یک quality → revision جدید (بدون شکستن URL قدیمی)

#### تعریف profileها (config)

فایل: `internal/config/profiles.go`

| Env | نقش |
|-----|-----|
| `CASTFLOW_PLAYBACK_PROFILE` | پروفایل primary (در API و player پیش‌فرض) |
| `CASTFLOW_PLAYBACK_PROFILES` | لیست profileهایی که روی آپلود transcode می‌شوند |
| `CASTFLOW_PROFILE_{NAME}_QUALITIES` | ladder هر profile (مثلاً `CASTFLOW_PROFILE_MOBILE_QUALITIES`) |
| `CASTFLOW_PROFILE_{NAME}_PLAYER_QUALITIES` | کیفیت‌های نمایش در player |

پیش‌فرض `mobile` اگر env نباشد: `144p,240p,360p,480p`

اگر `CASTFLOW_PLAYBACK_PROFILES` خالی باشد، فقط `CASTFLOW_PLAYBACK_PROFILE` transcode می‌شود.

#### جریان transcode

```
Upload → outbox (transcode job)
    → Worker: ProcessVideo.Execute(videoId, {profiles, force})
        → برای هر profile در CASTFLOW_PLAYBACK_PROFILES:
            1. revision = BuildRevision(profile + ladder + settings)
            2. اگر !force && rendition همین revision ready است → skip
            3. INSERT/UPDATE video_renditions (status=processing)
            4. FFmpeg → hls/{profile}/{revision}/ + dash/{profile}/{revision}/
            5. آپلود artifacts (thumbnail/tooltip فقط یک‌بار — profile اول)
            6. UPDATE rendition → ready
            7. Webhook: rendition.ready (اگر CASTFLOW_WEBHOOK_URL ست باشد)
        → آپلود config.json با renditions[] + primary
        → UPDATE videos.playback_variant = "{primary}/{revision}"
        → UPDATE videos.status = ready
```

#### پایگاه داده — `video_renditions`

Migration: `migrations/004_video_renditions.sql`

| ستون | نوع | توضیح |
|------|-----|-------|
| `video_id` | UUID FK | ویدیوی والد |
| `profile` | TEXT | مثلاً `default` |
| `revision` | TEXT | مثلاً `a3f2b1c4` |
| `status` | TEXT | `processing` \| `ready` \| `error` |
| `qualities` | TEXT | `"360p,720p,1080p"` |
| `duration_sec` | INT | مدت بعد از transcode |

Constraint: `UNIQUE (video_id, profile, revision)` — یک revision مشخص فقط یک‌بار ثبت می‌شود.

فیلد legacy `videos.playback_variant` همچنان primary profile path را نگه می‌دارد (`default/a3f2b1c4`) برای سازگاری با کد/API قدیمی.

#### config.json (خروجی player)

`URLBuilder.BuildPlayerConfig()` ساختار زیر را می‌سازد:

```json
{
  "primary": "default",
  "renditions": [
    {
      "profile": "default",
      "revision": "a3f2b1c4",
      "qualities": ["360p", "720p", "1080p"],
      "source": [
        { "src": ".../hls/default/a3f2b1c4/master.m3u8", "type": "application/x-mpegURL" },
        { "src": ".../dash/default/a3f2b1c4/manifest.mpd", "type": "application/dash+xml" }
      ],
      "poster": ".../thumbnail.jpg",
      "thumbnail": ".../tooltip.vtt"
    },
    {
      "profile": "mobile",
      "revision": "deadbeef",
      "qualities": ["144p", "240p", "360p", "480p"],
      "source": [ ".../hls/mobile/deadbeef/master.m3u8", ... ]
    }
  ],
  "source": [ "... primary profile HLS/DASH ..." ]
}
```

Player **URL را حدس نمی‌زند** — فقط `source` از config می‌خواند.

انتخاب profile در player (`web/player/index.html`):

| شرایط | profile انتخاب‌شده |
|--------|-------------------|
| `?profile=mobile` در URL | همان (override) |
| دستگاه موبایل + rendition `mobile` موجود | `mobile` |
| در غیر این صورت | `primary` از config |

#### CDN / Cache

| مسیر | Cache پیشنهادی |
|------|----------------|
| `/hls/{profile}/{revision}/*` | `max-age=31536000, immutable` — revision جدید = URL جدید |
| `/config.json` | کوتاه (`max-age=60`) — بعد از re-transcode به‌روز می‌شود |
| `/thumbnail.jpg`, `/tooltip.vtt` | متوسط (`max-age=86400`) |

#### سازگاری عقب‌رو (ویدیوهای قدیمی)

ویدیوهای transcode‌شده **قبل** از این تغییر:

- مسیر قدیمی: `v/{id}/hls/{old-variant}/master.m3u8` (hash طولانی)
- `playback_variant` در DB همان مسیر legacy را دارد
- `video_renditions` خالی است تا re-transcode شود

برای migrate: `POST /api/v1/videos/{id}/retranscode` با `{"force": true}`

#### Webhook — `rendition.ready`

اگر `CASTFLOW_WEBHOOK_URL` تنظیم شده باشد، بعد از ready شدن هر profile:

```json
{
  "event": "rendition.ready",
  "videoId": "uuid",
  "title": "My Lecture",
  "profile": "default",
  "revision": "a3f2b1c4",
  "durationSec": 421,
  "rendition": {
    "profile": "default",
    "revision": "a3f2b1c4",
    "hlsUrl": "...",
    "dashUrl": "...",
    "qualities": ["360p","720p","1080p"]
  }
}
```

Header اختیاری: `X-Castflow-Signature: sha256=<hmac>` با `CASTFLOW_WEBHOOK_SECRET`

### Env پروفایل‌ها

```env
CASTFLOW_PLAYBACK_PROFILE=default
CASTFLOW_PLAYBACK_PROFILES=default,mobile
CASTFLOW_PROFILE_MOBILE_QUALITIES=144p,240p,360p,480p
CASTFLOW_PROFILE_MOBILE_PLAYER_QUALITIES=360p,480p
CASTFLOW_WEBHOOK_URL=https://example.com/hooks/castflow
CASTFLOW_WEBHOOK_SECRET=your-webhook-secret
```

### API — renditions و re-transcode

**GET /api/v1/videos/{id}/links** — فیلدهای جدید:

```json
{
  "primary": "default",
  "renditions": [
    {
      "profile": "default",
      "revision": "a3f2b1c4",
      "status": "ready",
      "hlsUrl": "...",
      "dashUrl": "...",
      "qualities": ["360p","720p","1080p"]
    }
  ],
  "links": { "...": "primary profile links (backward compat)" }
}
```

**POST /api/v1/videos/{id}/retranscode**

```json
{ "profiles": ["default"], "force": false }
```

---

## 11. پلیر تعبیه‌شده (Embedded Player)

مسیر: `web/player/index.html`

### فناوری

- **Video.js 8.x** — پخش HLS/DASH
- **Tailwind CSS** — UI
- RTL/فارسی (`lang="fa" dir="rtl"`)
- Sprite tooltip preview روی progress bar

### config.json

Player از query param `?config=` URL فایل config را می‌خواند:

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
    { "src": ".../hls/.../master.m3u8", "type": "application/x-mpegURL" },
    { "src": ".../dash/.../manifest.mpd", "type": "application/dash+xml" }
  ],
  "poster": ".../thumbnail.jpg",
  "thumbnail": ".../tooltip.vtt",
  "qualities": ["360p", "720p", "1080p"]
}
```

### iFrame Embed

API در `links.iframe` کد embed responsive (16:9) برمی‌گرداند.

---

## 12. API REST

**Base URL:** `http://localhost` (Docker/nginx) یا `CASTFLOW_API_BASE_URL`

**Authentication:** Header `X-API-Key: <key>` (یا query param `api_key`)

### Endpoints

| Method | Path | Auth | توضیح |
|--------|------|------|-------|
| GET | `/health` | ❌ | Liveness probe |
| GET | `/ready` | ❌ | Readiness probe |
| GET | `/api/v1/videos` | ✅ | لیست ویدیوها |
| POST | `/api/v1/videos/upload` | ✅ | آپلود + شروع transcode |
| GET | `/api/v1/videos/{id}` | ✅ | متادیتای ویدیو |
| GET | `/api/v1/videos/{id}/status` | ✅ | وضعیت processing |
| GET | `/api/v1/videos/{id}/links` | ✅ | لینک‌های پخش |
| DELETE | `/api/v1/videos/{id}` | ✅ | حذف ویدیو + artifacts |

### Static Routes

| Path | توضیح |
|------|-------|
| `/media/v/{id}/...` | فایل‌های transcode (local storage) |
| `/player/index.html?config=...` | Player embed |

### Upload

```bash
curl -X POST http://localhost/api/v1/videos/upload \
  -H "X-API-Key: dev-secret-key" \
  -F "title=My Lecture" \
  -F "description=Optional" \
  -F "file=@video.mp4"
```

**Response `202`:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "title": "My Lecture",
  "status": "processing",
  "message": "upload accepted; transcoding started"
}
```

### Links Response

```json
{
  "videoId": "uuid",
  "title": "My Lecture",
  "status": "ready",
  "links": {
    "videoId": "uuid",
    "title": "My Lecture",
    "hlsUrl": "https://.../v/uuid/hls/{variant}/master.m3u8",
    "dashUrl": "https://.../v/uuid/dash/{variant}/manifest.mpd",
    "configUrl": "https://.../v/uuid/config.json",
    "playerUrl": "https://.../player/index.html?config=...",
    "thumbnailUrl": "https://.../v/uuid/thumbnail.jpg",
    "tooltipUrl": "https://.../v/uuid/tooltip.vtt",
    "videoUrl": "https://.../v/uuid/origin.mp4",
    "iframe": "<style>...</style><div>...</div>"
  }
}
```

### Error Format

```json
{ "error": "video not found" }
```

| HTTP | معنی |
|------|------|
| 400 | ورودی نامعتبر |
| 401 | API key نامعتبر |
| 404 | ویدیو پیدا نشد |
| 409 | وضعیت conflict (مثلاً not ready) |
| 500 | خطای داخلی |

---

## 13. اسکیمای پایگاه داده

### جدول `videos`

| ستون | نوع | توضیح |
|------|-----|-------|
| `id` | UUID PK | شناسهٔ ویدیو |
| `title` | TEXT | عنوان |
| `description` | TEXT | توضیحات |
| `status` | video_status ENUM | وضعیت lifecycle |
| `duration_sec` | INT | مدت (ثانیه) |
| `file_size` | BIGINT | حجم origin |
| `content_type` | TEXT | MIME type |
| `origin_key` | TEXT | کلید storage (مثلاً `v/{id}/origin.mp4`) |
| `playback_variant` | TEXT | شناسه variant transcode |
| `error_message` | TEXT | پیام خطا |
| `created_at` | TIMESTAMPTZ | زمان ایجاد |
| `updated_at` | TIMESTAMPTZ | آخرین بروزرسانی |

**Indexes:** `status`, `created_at DESC`

### جدول `outbox_events`

| ستون | نوع | توضیح |
|------|-----|-------|
| `id` | UUID PK | شناسه event |
| `aggregate_id` | UUID | video ID |
| `event_type` | TEXT | مثلاً `transcode` |
| `payload` | JSONB | `{ "videoId": "..." }` |
| `created_at` | TIMESTAMPTZ | زمان ایجاد |
| `published_at` | TIMESTAMPTZ NULL | NULL = pending |
| `attempts` | INT | تعداد تلاش publish |

**Index:** partial روی `created_at WHERE published_at IS NULL`

### Migrations

```
migrations/
├── 001_init.sql          — videos table
├── 002_outbox.sql        — outbox_events table
└── 003_playback_variant.sql — playback_variant column
```

---

## 14. پیکربندی (Environment Variables)

### URLها

| متغیر | پیش‌فرض | توضیح |
|-------|---------|-------|
| `CASTFLOW_BASE_URL` | `http://localhost:8080` | URL پایه (Docker: `http://localhost`) |
| `CASTFLOW_API_BASE_URL` | = BASE | override API |
| `CASTFLOW_CDN_BASE_URL` | = BASE/media | override CDN |
| `CASTFLOW_PLAYER_BASE_URL` | = BASE/player | override Player |

### اپلیکیشن

| متغیر | پیش‌فرض | توضیح |
|-------|---------|-------|
| `CASTFLOW_HTTP_ADDR` | `:8080` | آدرس bind |
| `CASTFLOW_API_KEY` | `dev-secret-key` | کلید API |
| `CASTFLOW_LOG_LEVEL` | `info` | debug/info/warn/error |
| `CASTFLOW_DATABASE_URL` | postgres://... | PostgreSQL DSN |
| `CASTFLOW_REDIS_URL` | redis://localhost:6379/0 | Redis DSN |

### Storage

| متغیر | پیش‌فرض | توضیح |
|-------|---------|-------|
| `CASTFLOW_STORAGE_DRIVER` | `local` | `local` یا `s3` |
| `CASTFLOW_STORAGE_LOCAL_DIR` | `./data/storage` | مسیر local |
| `CASTFLOW_S3_ENDPOINT` | `localhost:9000` | S3 endpoint |
| `CASTFLOW_S3_BUCKET` | `castflow-vod` | نام bucket |
| `CASTFLOW_S3_REGION` | `us-east-1` | region |
| `CASTFLOW_S3_ACCESS_KEY` | `minioadmin` | access key |
| `CASTFLOW_S3_SECRET_KEY` | `minioadmin` | secret key |
| `CASTFLOW_S3_USE_SSL` | `false` | HTTPS برای S3 |

### Transcode

| متغیر | پیش‌فرض | توضیح |
|-------|---------|-------|
| `CASTFLOW_FFMPEG_PATH` | `ffmpeg` | مسیر ffmpeg |
| `CASTFLOW_FFPROBE_PATH` | `ffprobe` | مسیر ffprobe |
| `CASTFLOW_QUALITIES` | `360p,720p,1080p` | کیفیت‌های transcode |
| `CASTFLOW_PLAYER_QUALITIES` | (همه) | کیفیت‌های نمایش در player |
| `CASTFLOW_HLS_SEGMENT_SECONDS` | `6` | طول segment HLS |
| `CASTFLOW_THUMBNAIL_AT_SEC` | `1` | ثانیهٔ thumbnail |
| `CASTFLOW_TOOLTIP_INTERVAL_SEC` | `5` | فاصله tooltip frames |
| `CASTFLOW_TOOLTIP_MAX_FRAMES` | `60` | حداکثر فریم tooltip |
| `CASTFLOW_TOOLTIP_COLS` | `10` | ستون sprite |
| `CASTFLOW_TEMP_DIR` | `./data/tmp` | temp transcode |

### Worker & Outbox

| متغیر | پیش‌فرض | توضیح |
|-------|---------|-------|
| `CASTFLOW_WORKER_CONCURRENCY` | `2` | job همزمان per worker |
| `CASTFLOW_ENABLE_EMBEDDED_WORKER` | `true` | worker در API process |
| `CASTFLOW_OUTBOX_POLL_MS` | `1000` | interval relay |
| `CASTFLOW_OUTBOX_BATCH_SIZE` | `50` | batch size relay |

---

## 15. استقرار با Docker Compose

### سرویس‌ها

| سرویس | Image | Port (host) | نقش |
|-------|-------|-------------|-----|
| `nginx` | nginx:1.27-alpine | 80, 443 | Reverse proxy |
| `castflow` | castflow:latest (build) | — | API |
| `castflow-worker` | castflow:latest | — | Transcode worker |
| `postgres` | postgres:16-alpine | — | Metadata DB |
| `redis` | redis:7-alpine | — | Asynq backend |
| `asynqmon` | hibiken/asynqmon | 3000 | Queue dashboard |
| `rustfs` | rustfs/rustfs | 9000, 9001 | S3 storage (اختیاری) |

### Volumes

| Volume | محتوا |
|--------|-------|
| `pgdata` | PostgreSQL data |
| `castflow_storage` | فایل‌های ویدیo (local driver) |
| `castflow_tmp` | temp transcode |
| `rustfsdata` | RustFS S3 data |

### Quick Start

```bash
cd castflow
make install-build   # اولین بار: build + start + migrate
# make install       # دفعات بعد: فقط start + migrate
```

فایل `.env` از `deploy/.env.docker.example` کپی می‌شود.

### URLهای محلی

| URL | توضیح |
|-----|-------|
| http://localhost/health | Health check |
| http://localhost/api/v1/videos | API |
| http://localhost/player/index.html | Player |
| http://localhost:3000 | Asynqmon |
| http://localhost:9001 | RustFS console |

---

## 16. SSL / HTTPS

گواهی‌ها در `deploy/nginx/ssl/` قرار می‌گیرند. **نیازی به rebuild Docker image نیست** — فقط nginx reload.

| دستور | توضیح |
|-------|-------|
| `make ssl` | راهنمای setup |
| `make ssl-certbot DOMAIN=example.com EMAIL=you@example.com` | Let's Encrypt |
| `make ssl-install-certs DOMAIN=example.com` | کپی certbot certs |
| `make ssl-init` | Self-signed (dev) |
| `make ssl-enable` | فعال‌سازی HTTPS config |
| `make ssl-reload` | nginx -t + reload |

### Production HTTPS

```bash
make install-build
make ssl-certbot DOMAIN=example.com EMAIL=you@example.com
# در .env:
# CASTFLOW_BASE_URL=https://example.com
make docker-restart
```

---

## 17. استقرار Production

### DNS

**Single domain (ساده‌ترین):**
```
example.com → A record → server IP
```

**Multi-domain (اختیاری):**
```
api.example.com   → Castflow API
cdn.example.com   → CDN / S3 origin
player.example.com → Player
```

### Firewall

| Port | پروتکل | کاربرد |
|------|--------|--------|
| 80 | TCP | HTTP + ACME |
| 443 | TCP | HTTPS |
| 3000 | TCP | Asynqmon (محدود کنید!) |
| 9001 | TCP | RustFS console (اختیاری) |

### Scale Workers

```bash
docker compose -f deploy/docker-compose.yml up -d --scale castflow-worker=3
```

`CASTFLOW_WORKER_CONCURRENCY` را per replica تنظیم کنید (CPU/RAM).

### Asynqmon در Production

- **بدون authentication** — از SSH tunnel استفاده کنید:

```bash
ssh -L 3000:127.0.0.1:3000 user@your-server
# سپس: http://localhost:3000
```

---

## 18. مانیتورینگ و دیباگ

### دستورات سریع

```bash
make docker-ps          # وضعیت containerها
make docker-health      # HTTP health check
make docker-logs        # لاگ API
make docker-logs-worker # لاگ worker
make docker-logs-nginx  # لاگ nginx
```

### Verbose Logs

```env
CASTFLOW_LOG_LEVEL=debug
```
سپس: `make docker-restart`

### بررسی صف Redis

```bash
docker compose -f deploy/docker-compose.yml exec -T redis redis-cli <<'EOF'
LLEN asynq:{castflow}:pending
LLEN asynq:{castflow}:active
ZCARD asynq:{castflow}:retry
ZCARD asynq:{castflow}:archived
EOF
```

### Query DB

```sql
SELECT id, title, status, error_message FROM videos ORDER BY created_at DESC LIMIT 5;
SELECT * FROM outbox WHERE published_at IS NULL LIMIT 10;
```

### مشکلات رایج

| علامت | علت احتمالی | راه‌حل |
|-------|-------------|--------|
| Connection refused :443 | HTTPS فعال نیست | `make ssl-enable` |
| Connection refused :8080 | API expose نشده | از nginx :80 استفاده کنید |
| 401/403 | API key اشتباه | `.env` → `CASTFLOW_API_KEY` |
| آپلود OK، transcode نه | worker down | `make docker-logs-worker` |
| `.env` اعمال نشده | container قدیمی | `make docker-restart` |
| تغییر کد اعمال نشده | image rebuild نشده | `make install-build` |

---

## 19. ساختار پروژه

```
castflow/
├── cmd/
│   ├── castflow/main.go       # Entry point API
│   └── worker/main.go         # Entry point Worker
├── internal/
│   ├── domain/                # Entities, ports, URL builder
│   ├── application/           # Use cases
│   ├── adapter/
│   │   ├── http/              # REST handler + static servers
│   │   ├── postgres/          # Repository + outbox
│   │   ├── storage/           # Local + S3
│   │   ├── ffmpeg/            # Transcoder
│   │   └── queue/             # Asynq + outbox relay
│   ├── config/                # Env config + URL resolution
│   └── app/                   # Bootstrap + App + Worker
├── migrations/                # SQL migrations
├── deploy/
│   ├── docker-compose.yml
│   ├── Dockerfile
│   ├── .env.docker.example
│   └── nginx/                 # Reverse proxy + TLS
├── web/player/                # Embedded player (HTML/JS)
├── api/openapi.yaml           # OpenAPI spec
├── docs/                      # مستندات تکمیلی (EN)
│   ├── ARCHITECTURE.md
│   ├── API.md
│   ├── DEPLOYMENT.md
│   └── DEBUG.md
├── Makefile
├── go.mod
└── README.md
```

---

## 20. دستورات Makefile

| دستور | توضیح |
|-------|-------|
| `make install` | Start + migrate (بدون rebuild) |
| `make install-build` | Build image + start + migrate |
| `make uninstall` | Stop + حذف volumes |
| `make docker-up` | Start همه containerها |
| `make docker-down` | Stop (حفظ data) |
| `make docker-restart` | Recreate API, worker, nginx |
| `make docker-migrate` | Apply SQL migrations |
| `make docker-logs` | Follow API logs |
| `make docker-logs-worker` | Follow worker logs |
| `make docker-health` | Health checks |
| `make docker-shell SERVICE=castflow` | Shell در container |
| `make build` | Build binaries محلی |
| `make test` | Run tests |
| `make ssl-*` | SSL/TLS management |

---

## 21. تصمیمات طراحی

| تصمیم | دلیل |
|-------|------|
| FFmpeg CLI (نه libav) | ساده‌تر، universal، debug آسان |
| Asynq روی Redis | Queue mature در Go، UI (Asynqmon)، retry/concurrency |
| chi (نه gin) | سبک، سازگار با stdlib |
| Local storage پیش‌فرض | تجربه dev بدون dependency |
| Split API/Worker در Docker | scale transcode مستقل از HTTP |
| Transactional Outbox | enqueue مطمئن بعد از commit DB |
| Playback Variant | versioning URL بدون شکستن ویدیوهای قدیمی |
| Nginx جلوی API | TLS، upload size، production-ready |

---

## 22. نقاط توسعهٔ آینده

- Webhook روی status `ready`
- Auth پیشرفته: JWT, OAuth
- Storage providerهای بیشتر (Bunny, Cloudflare R2)
- Queue name قابل تنظیم از env
- Speech-to-text / زیرنویس
- DRM packaging
- Configurable player themes

---

## مستندات تکمیلی (انگلیسی)

| فایل | موضوع |
|------|-------|
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | معماری (EN) |
| [docs/API.md](docs/API.md) | مرجع API (EN) |
| [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) | استقرار production (EN) |
| [docs/DEBUG.md](docs/DEBUG.md) | دیباگ Docker (EN) |
| [api/openapi.yaml](api/openapi.yaml) | OpenAPI specification |

---

## License

MIT
