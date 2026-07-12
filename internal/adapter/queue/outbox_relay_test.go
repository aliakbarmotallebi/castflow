package queue_test

import (
	"context"
	"encoding/json"
	"testing"

	"castflow/internal/adapter/postgres"
	"castflow/internal/adapter/queue"
	"castflow/internal/domain"
	"github.com/google/uuid"
)

type recordingQueue struct {
	enqueued []uuid.UUID
}

func (q *recordingQueue) EnqueueTranscode(_ context.Context, videoID uuid.UUID) error {
	q.enqueued = append(q.enqueued, videoID)
	return nil
}

func TestOutboxRelayPublishEvent(t *testing.T) {
	videoID := uuid.New()
	payload, err := json.Marshal(domain.TranscodeOutboxPayload{VideoID: videoID.String()})
	if err != nil {
		t.Fatal(err)
	}

	recorder := &recordingQueue{}
	relay := queue.NewOutboxRelay(postgres.NewOutboxRepository(nil), recorder, 0, 0)

	event := domain.OutboxEvent{
		EventType: domain.OutboxEventTranscode,
		Payload:   payload,
	}
	if err := relay.PublishEvent(context.Background(), event); err != nil {
		t.Fatalf("publish event: %v", err)
	}
	if len(recorder.enqueued) != 1 || recorder.enqueued[0] != videoID {
		t.Fatalf("expected one enqueue for %s, got %v", videoID, recorder.enqueued)
	}
}
