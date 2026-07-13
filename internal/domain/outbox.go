package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// OutboxEventType identifies a background job stored in the outbox.
type OutboxEventType string

const (
	OutboxEventTranscode OutboxEventType = "transcode"
)

// OutboxEvent is a pending or published message for the job relay.
type OutboxEvent struct {
	ID          uuid.UUID
	AggregateID uuid.UUID
	EventType   OutboxEventType
	Payload     json.RawMessage
	CreatedAt   time.Time
	PublishedAt *time.Time
	Attempts    int
}

// TranscodeOutboxPayload is the JSON body for transcode events.
type TranscodeOutboxPayload struct {
	VideoID  string   `json:"videoId"`
	Profiles []string `json:"profiles,omitempty"`
	Force    bool     `json:"force,omitempty"`
}
