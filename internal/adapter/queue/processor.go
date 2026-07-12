package queue

import (
	"context"
	"fmt"

	"castflow/internal/domain"
	"github.com/google/uuid"
)

// TranscodeProcessor bridges queue to application use case.
type TranscodeProcessor struct {
	ProcessFn func(ctx context.Context, videoID uuid.UUID) error
}

func (p *TranscodeProcessor) Process(ctx context.Context, videoID uuid.UUID) error {
	if p.ProcessFn == nil {
		return fmt.Errorf("transcode processor not configured")
	}
	return p.ProcessFn(ctx, videoID)
}

// NoopQueue enqueues by storing video IDs in memory (tests / no-redis dev).
type NoopQueue struct {
	onEnqueue func(videoID uuid.UUID)
}

func NewNoopQueue(onEnqueue func(videoID uuid.UUID)) *NoopQueue {
	return &NoopQueue{onEnqueue: onEnqueue}
}

func (q *NoopQueue) EnqueueTranscode(_ context.Context, videoID uuid.UUID) error {
	if q.onEnqueue != nil {
		q.onEnqueue(videoID)
	}
	return nil
}

var _ domain.JobQueue = (*NoopQueue)(nil)
