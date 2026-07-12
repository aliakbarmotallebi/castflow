package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"castflow/internal/domain"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const (
	// Name is the Asynq queue name (visible in Asynqmon).
	Name = "castflow"
	// TaskTranscode is the task type for video transcoding.
	TaskTranscode = "transcode"
)

type transcodePayload struct {
	VideoID string `json:"videoId"`
}

// Asynq implements domain.JobQueue and runs an Asynq worker server.
type Asynq struct {
	client    *asynq.Client
	server    *asynq.Server
	processor *TranscodeProcessor
}

func NewAsynq(redisURL string, concurrency int) (*Asynq, error) {
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	if concurrency <= 0 {
		concurrency = 2
	}

	return &Asynq{
		client: asynq.NewClient(opt),
		server: asynq.NewServer(opt, asynq.Config{
			Concurrency: concurrency,
			Queues: map[string]int{
				Name: 10,
			},
		}),
	}, nil
}

func (a *Asynq) RegisterProcessor(processor *TranscodeProcessor) {
	a.processor = processor
}

func (a *Asynq) EnqueueTranscode(ctx context.Context, videoID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	payload, err := json.Marshal(transcodePayload{VideoID: videoID.String()})
	if err != nil {
		return fmt.Errorf("marshal transcode payload: %w", err)
	}
	task := asynq.NewTask(TaskTranscode, payload)
	_, err = a.client.EnqueueContext(ctx, task,
		asynq.Queue(Name),
		asynq.MaxRetry(3),
		asynq.Timeout(2*time.Hour),
	)
	return err
}

func (a *Asynq) handleTranscode(ctx context.Context, t *asynq.Task) error {
	if a.processor == nil {
		return fmt.Errorf("transcode processor not configured")
	}
	var p transcodePayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	id, err := uuid.Parse(p.VideoID)
	if err != nil {
		return fmt.Errorf("%w: invalid videoId: %v", asynq.SkipRetry, err)
	}
	return a.processor.Process(ctx, id)
}

func (a *Asynq) Run(ctx context.Context) error {
	if a.processor == nil {
		return fmt.Errorf("transcode processor not configured")
	}

	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskTranscode, a.handleTranscode)

	go func() {
		<-ctx.Done()
		a.server.Shutdown()
	}()

	if err := a.server.Run(mux); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func (a *Asynq) Stop() {
	a.server.Shutdown()
}

func (a *Asynq) Close() error {
	a.server.Shutdown()
	return a.client.Close()
}

var _ domain.JobQueue = (*Asynq)(nil)
