package app

import (
	"context"
	"fmt"
	"os"

	"castflow/internal/adapter/ffmpeg"
	"castflow/internal/adapter/postgres"
	"castflow/internal/adapter/queue"
	"castflow/internal/adapter/storage"
	"castflow/internal/application"
	"castflow/internal/config"
	"castflow/internal/domain"
	"github.com/google/uuid"
)

// Bootstrap wires shared dependencies for API and worker processes.
type Bootstrap struct {
	Cfg         *config.Config
	Repo          domain.VideoRepository
	Store         domain.ObjectStorage
	URLBuilder    *domain.URLBuilder
	UploadWriter  domain.VideoUploadWriter
	JobQueue      domain.JobQueue
	QueueRunner *queue.Asynq
	OutboxRelay domain.OutboxRelay
}

func NewBootstrap(cfg *config.Config) (*Bootstrap, error) {
	if err := os.MkdirAll(cfg.Transcode.TempDir, 0o755); err != nil {
		return nil, fmt.Errorf("temp dir: %w", err)
	}
	if cfg.Storage.Driver == "local" || cfg.Storage.Driver == "" {
		if err := os.MkdirAll(cfg.Storage.LocalDir, 0o755); err != nil {
			return nil, fmt.Errorf("storage dir: %w", err)
		}
	}

	db, err := postgres.Open(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}

	store, err := storage.NewFromConfig(cfg.Storage)
	if err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}
	if err := store.EnsureBucket(context.Background()); err != nil {
		return nil, fmt.Errorf("storage bucket: %w", err)
	}

	repo := postgres.NewVideoRepository(db)
	uploadWriter := postgres.NewUploadWriter(db)
	outboxRepo := postgres.NewOutboxRepository(db)
	urlBuilder := domain.NewURLBuilder(cfg.CDNBaseURL, cfg.PlayerBaseURL, cfg.Transcode.PlayerQualities)
	transcoder := ffmpeg.NewTranscoder(
		cfg.Transcode.FFmpegPath,
		cfg.Transcode.FFprobePath,
		cfg.Transcode.HLSSegmentSeconds,
		cfg.Transcode.TooltipMaxFrames,
		cfg.Transcode.TooltipCols,
	)
	processUC := application.NewProcessVideo(repo, store, transcoder, urlBuilder, cfg.Transcode.TempDir)

	jobQueue, err := queue.NewAsynq(cfg.RedisURL, cfg.Worker.Concurrency)
	if err != nil {
		return nil, fmt.Errorf("queue: %w", err)
	}

	processor := &queue.TranscodeProcessor{
		ProcessFn: func(ctx context.Context, videoID uuid.UUID) error {
			return processUC.Execute(ctx, videoID, cfg.Transcode.Qualities, cfg.Transcode.ThumbnailAtSec, cfg.Transcode.TooltipIntervalSec)
		},
	}
	jobQueue.RegisterProcessor(processor)

	outboxRelay := queue.NewOutboxRelay(outboxRepo, jobQueue, cfg.Outbox.PollInterval, cfg.Outbox.BatchSize)

	return &Bootstrap{
		Cfg:          cfg,
		Repo:         repo,
		Store:        store,
		URLBuilder:   urlBuilder,
		UploadWriter: uploadWriter,
		JobQueue:     jobQueue,
		QueueRunner:  jobQueue,
		OutboxRelay:  outboxRelay,
	}, nil
}
