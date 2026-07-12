package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"castflow/internal/adapter/postgres"
	"castflow/internal/domain"
	"github.com/google/uuid"
)

// OutboxRelay polls the outbox table and enqueues jobs to Asynq.
type OutboxRelay struct {
	outbox    *postgres.OutboxRepository
	queue     domain.JobQueue
	interval  time.Duration
	batchSize int
}

func NewOutboxRelay(outbox *postgres.OutboxRepository, queue domain.JobQueue, interval time.Duration, batchSize int) *OutboxRelay {
	if interval <= 0 {
		interval = time.Second
	}
	if batchSize <= 0 {
		batchSize = 50
	}
	return &OutboxRelay{
		outbox:    outbox,
		queue:     queue,
		interval:  interval,
		batchSize: batchSize,
	}
}

func (r *OutboxRelay) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			r.publishOnce(ctx)
		}
	}
}

func (r *OutboxRelay) publishOnce(ctx context.Context) {
	published, err := r.outbox.PublishBatch(ctx, r.batchSize, r.PublishEvent)
	if err != nil {
		slog.Error("outbox relay batch failed", "err", err)
		return
	}
	if published > 0 {
		slog.Info("outbox relay published", "count", published)
	}
}

func (r *OutboxRelay) PublishEvent(ctx context.Context, event domain.OutboxEvent) error {
	switch event.EventType {
	case domain.OutboxEventTranscode:
		var payload domain.TranscodeOutboxPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("decode transcode payload: %w", err)
		}
		videoID, err := uuid.Parse(payload.VideoID)
		if err != nil {
			return fmt.Errorf("invalid videoId: %w", err)
		}
		return r.queue.EnqueueTranscode(ctx, videoID)
	default:
		return fmt.Errorf("unknown outbox event type: %s", event.EventType)
	}
}

var _ domain.OutboxRelay = (*OutboxRelay)(nil)
