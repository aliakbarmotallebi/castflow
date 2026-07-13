package queue

import (
	"context"
	"fmt"

	"castflow/internal/domain"
	"github.com/google/uuid"
)

// TranscodeProcessor bridges queue to application use case.
type TranscodeProcessor struct {
	ProcessFn func(ctx context.Context, videoID uuid.UUID, opts domain.TranscodeJobOptions) error
}

func (p *TranscodeProcessor) Process(ctx context.Context, videoID uuid.UUID, opts domain.TranscodeJobOptions) error {
	if p.ProcessFn == nil {
		return fmt.Errorf("transcode processor not configured")
	}
	return p.ProcessFn(ctx, videoID, opts)
}

// NoopQueue enqueues by storing video IDs in memory (tests / no-redis dev).
type NoopQueue struct {
	onEnqueue func(videoID uuid.UUID, opts domain.TranscodeJobOptions)
}

func NewNoopQueue(onEnqueue func(videoID uuid.UUID, opts domain.TranscodeJobOptions)) *NoopQueue {
	return &NoopQueue{onEnqueue: onEnqueue}
}

func (q *NoopQueue) EnqueueTranscode(_ context.Context, videoID uuid.UUID, opts domain.TranscodeJobOptions) error {
	if q.onEnqueue != nil {
		q.onEnqueue(videoID, opts)
	}
	return nil
}

var _ domain.JobQueue = (*NoopQueue)(nil)
