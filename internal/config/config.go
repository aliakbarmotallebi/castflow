package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"castflow/internal/domain"
)

// Config holds all runtime configuration (12-factor via env).
type Config struct {
	HTTPAddr      string
	APIKey        string
	DatabaseURL   string
	RedisURL      string
	CDNBaseURL    string
	PlayerBaseURL string
	APIBaseURL    string
	Storage       StorageConfig
	Transcode     TranscodeConfig
	Worker        WorkerConfig
	Outbox        OutboxConfig
	LogLevel      string
}

type StorageConfig struct {
	Driver    string
	Endpoint  string
	Bucket    string
	Region    string
	AccessKey string
	SecretKey string
	UseSSL    bool
	LocalDir  string
	PublicURL string
}

type TranscodeConfig struct {
	FFmpegPath         string
	FFprobePath        string
	Qualities          []domain.QualityProfile
	HLSSegmentSeconds  int
	ThumbnailAtSec     float64
	TooltipIntervalSec float64
	TempDir            string
}

type WorkerConfig struct {
	Concurrency    int
	EnableEmbedded bool
}

type OutboxConfig struct {
	PollInterval time.Duration
	BatchSize    int
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cdn := env("CASTFLOW_CDN_BASE_URL", "http://localhost:8080/media")
	cfg := &Config{
		HTTPAddr:      env("CASTFLOW_HTTP_ADDR", ":8080"),
		APIKey:        env("CASTFLOW_API_KEY", "dev-secret-key"),
		DatabaseURL:   env("CASTFLOW_DATABASE_URL", "postgres://castflow:castflow@localhost:5432/castflow?sslmode=disable"),
		RedisURL:      env("CASTFLOW_REDIS_URL", "redis://localhost:6379/0"),
		CDNBaseURL:    cdn,
		PlayerBaseURL: env("CASTFLOW_PLAYER_BASE_URL", "http://localhost:8080/player"),
		APIBaseURL:    env("CASTFLOW_API_BASE_URL", "http://localhost:8080"),
		LogLevel:      env("CASTFLOW_LOG_LEVEL", "info"),
		Storage: StorageConfig{
			Driver:    env("CASTFLOW_STORAGE_DRIVER", "local"),
			Endpoint:  env("CASTFLOW_S3_ENDPOINT", "localhost:9000"),
			Bucket:    env("CASTFLOW_S3_BUCKET", "castflow-vod"),
			Region:    env("CASTFLOW_S3_REGION", "us-east-1"),
			AccessKey: env("CASTFLOW_S3_ACCESS_KEY", "minioadmin"),
			SecretKey: env("CASTFLOW_S3_SECRET_KEY", "minioadmin"),
			UseSSL:    envBool("CASTFLOW_S3_USE_SSL", false),
			LocalDir:  env("CASTFLOW_STORAGE_LOCAL_DIR", "./data/storage"),
			PublicURL: cdn,
		},
		Transcode: TranscodeConfig{
			FFmpegPath:         env("CASTFLOW_FFMPEG_PATH", "ffmpeg"),
			FFprobePath:        env("CASTFLOW_FFPROBE_PATH", "ffprobe"),
			HLSSegmentSeconds:  envInt("CASTFLOW_HLS_SEGMENT_SECONDS", 6),
			ThumbnailAtSec:     envFloat("CASTFLOW_THUMBNAIL_AT_SEC", 1),
			TooltipIntervalSec: envFloat("CASTFLOW_TOOLTIP_INTERVAL_SEC", 5),
			TempDir:            env("CASTFLOW_TEMP_DIR", "./data/tmp"),
			Qualities:          defaultQualities(),
		},
		Worker: WorkerConfig{
			Concurrency:    envInt("CASTFLOW_WORKER_CONCURRENCY", 2),
			EnableEmbedded: envBool("CASTFLOW_ENABLE_EMBEDDED_WORKER", true),
		},
		Outbox: OutboxConfig{
			PollInterval: time.Duration(envInt("CASTFLOW_OUTBOX_POLL_MS", 1000)) * time.Millisecond,
			BatchSize:    envInt("CASTFLOW_OUTBOX_BATCH_SIZE", 50),
		},
	}
	return cfg, nil
}

func defaultQualities() []domain.QualityProfile {
	raw := env("CASTFLOW_QUALITIES", "360p,720p,1080p")
	presets := map[string]domain.QualityProfile{
		"144p":  {Name: "144p", Width: 256, Height: 144, VideoBitrate: "200k", AudioBitrate: "64k"},
		"240p":  {Name: "240p", Width: 426, Height: 240, VideoBitrate: "400k", AudioBitrate: "64k"},
		"360p":  {Name: "360p", Width: 640, Height: 360, VideoBitrate: "800k", AudioBitrate: "96k"},
		"480p":  {Name: "480p", Width: 854, Height: 480, VideoBitrate: "1200k", AudioBitrate: "128k"},
		"720p":  {Name: "720p", Width: 1280, Height: 720, VideoBitrate: "2500k", AudioBitrate: "128k"},
		"1080p": {Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: "5000k", AudioBitrate: "128k"},
	}
	var out []domain.QualityProfile
	for _, name := range strings.Split(raw, ",") {
		name = strings.TrimSpace(name)
		if q, ok := presets[name]; ok {
			out = append(out, q)
		}
	}
	if len(out) == 0 {
		return []domain.QualityProfile{presets["360p"], presets["720p"], presets["1080p"]}
	}
	return out
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}
