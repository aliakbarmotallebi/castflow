package app

import (
	"context"
	"fmt"
	"os"

	"castflow/internal/adapter/ffmpeg"
	"castflow/internal/adapter/postgres"
	"castflow/internal/adapter/queue"
	"castflow/internal/adapter/storage"
	"castflow/internal/adapter/webhook"
	"castflow/internal/application"
	"castflow/internal/config"
	"castflow/internal/domain"
	"github.com/google/uuid"
)

// Bootstrap wires shared dependencies for API and worker processes.
type Bootstrap struct {
	Cfg              *config.Config
	Repo             domain.VideoRepository
	Renditions       domain.RenditionRepository
	Store            domain.ObjectStorage
	URLBuilder       *domain.URLBuilder
	UploadWriter     domain.VideoUploadWriter
	Scheduler        domain.TranscodeScheduler
	JobQueue         domain.JobQueue
	QueueRunner      *queue.Asynq
	OutboxRelay      domain.OutboxRelay
	Webhook          domain.WebhookNotifier
	ProcessVideo     *application.ProcessVideo
	RetranscodeVideo *application.RetranscodeVideo
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
	renditionRepo := postgres.NewRenditionRepository(db)
	uploadWriter := postgres.NewUploadWriter(db)
	scheduler := postgres.NewTranscodeScheduler(db)
	outboxRepo := postgres.NewOutboxRepository(db)
	urlBuilder := domain.NewURLBuilder(cfg.CDNBaseURL, cfg.PlayerBaseURL, cfg.Transcode.PlayerQualities)
	webhookNotifier := webhook.NewNotifier(cfg.Playback.WebhookURL, cfg.Playback.WebhookSecret)

	transcoder := ffmpeg.NewTranscoder(
		cfg.Transcode.FFmpegPath,
		cfg.Transcode.FFprobePath,
		cfg.Transcode.HLSSegmentSeconds,
		cfg.Transcode.TooltipMaxFrames,
		cfg.Transcode.TooltipCols,
	)
	processUC := application.NewProcessVideo(
		repo, renditionRepo, store, transcoder, urlBuilder, webhookNotifier,
		cfg.Playback, cfg.Transcode, cfg.Transcode.TempDir,
	)
	retranscodeUC := application.NewRetranscodeVideo(repo, scheduler)

	jobQueue, err := queue.NewAsynq(cfg.RedisURL, cfg.Worker.Concurrency)
	if err != nil {
		return nil, fmt.Errorf("queue: %w", err)
	}

	processor := &queue.TranscodeProcessor{
		ProcessFn: func(ctx context.Context, videoID uuid.UUID, opts domain.TranscodeJobOptions) error {
			return processUC.Execute(ctx, videoID, opts)
		},
	}
	jobQueue.RegisterProcessor(processor)

	outboxRelay := queue.NewOutboxRelay(outboxRepo, jobQueue, cfg.Outbox.PollInterval, cfg.Outbox.BatchSize)

	return &Bootstrap{
		Cfg:              cfg,
		Repo:             repo,
		Renditions:       renditionRepo,
		Store:            store,
		URLBuilder:       urlBuilder,
		UploadWriter:     uploadWriter,
		Scheduler:        scheduler,
		JobQueue:         jobQueue,
		QueueRunner:      jobQueue,
		OutboxRelay:      outboxRelay,
		Webhook:          webhookNotifier,
		ProcessVideo:     processUC,
		RetranscodeVideo: retranscodeUC,
	}, nil
}
