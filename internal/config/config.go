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
	Playback      PlaybackConfig
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
	PlayerQualities    []string
	HLSSegmentSeconds  int
	ThumbnailAtSec     float64
	TooltipIntervalSec float64
	TooltipMaxFrames   int
	TooltipCols        int
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
	api, cdn, player := resolvePublicURLs()
	qualities := defaultQualities()
	playback := loadPlaybackConfig(qualities)
	cfg := &Config{
		HTTPAddr:      env("CASTFLOW_HTTP_ADDR", ":8080"),
		APIKey:        env("CASTFLOW_API_KEY", "dev-secret-key"),
		DatabaseURL:   env("CASTFLOW_DATABASE_URL", "postgres://castflow:castflow@localhost:5432/castflow?sslmode=disable"),
		RedisURL:      env("CASTFLOW_REDIS_URL", "redis://localhost:6379/0"),
		CDNBaseURL:    cdn,
		PlayerBaseURL: player,
		APIBaseURL:    api,
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
			TooltipMaxFrames:   envInt("CASTFLOW_TOOLTIP_MAX_FRAMES", 60),
			TooltipCols:        envInt("CASTFLOW_TOOLTIP_COLS", 10),
			TempDir:            env("CASTFLOW_TEMP_DIR", "./data/tmp"),
			Qualities:          qualities,
			PlayerQualities:    defaultPlayerQualities(qualities),
		},
		Playback: playback,
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
	return parseQualityList(raw, qualityPresets())
}

// defaultPlayerQualities returns quality labels shown in the player menu.
// CASTFLOW_PLAYER_QUALITIES overrides; when unset, all transcoded qualities are listed.
func defaultPlayerQualities(transcoded []domain.QualityProfile) []string {
	raw := env("CASTFLOW_PLAYER_QUALITIES", "")
	if raw == "" {
		out := make([]string, len(transcoded))
		for i, q := range transcoded {
			out[i] = q.Name
		}
		return out
	}
	presets := qualityPresetNames()
	var out []string
	for _, name := range strings.Split(raw, ",") {
		name = strings.TrimSpace(name)
		if presets[name] {
			out = append(out, name)
		}
	}
	if len(out) == 0 {
		out = make([]string, len(transcoded))
		for i, q := range transcoded {
			out[i] = q.Name
		}
	}
	return out
}

func qualityPresetNames() map[string]bool {
	return map[string]bool{
		"144p": true, "240p": true, "360p": true,
		"480p": true, "720p": true, "1080p": true,
	}
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
