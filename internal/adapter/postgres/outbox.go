package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"castflow/internal/domain"
	"github.com/google/uuid"
)

// UploadWriter persists video metadata and outbox events in one transaction.
type UploadWriter struct {
	db *sql.DB
}

func NewUploadWriter(db *sql.DB) *UploadWriter {
	return &UploadWriter{db: db}
}

func (w *UploadWriter) AcceptUpload(ctx context.Context, video *domain.Video) error {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := saveVideo(ctx, tx, video); err != nil {
		return fmt.Errorf("save video: %w", err)
	}

	payload, err := json.Marshal(domain.TranscodeOutboxPayload{VideoID: video.ID.String()})
	if err != nil {
		return fmt.Errorf("marshal outbox payload: %w", err)
	}
	if err := insertOutboxEvent(ctx, tx, video.ID, domain.OutboxEventTranscode, payload); err != nil {
		return fmt.Errorf("insert outbox: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

var _ domain.VideoUploadWriter = (*UploadWriter)(nil)

// OutboxRepository reads and publishes pending outbox events.
type OutboxRepository struct {
	db *sql.DB
}

func NewOutboxRepository(db *sql.DB) *OutboxRepository {
	return &OutboxRepository{db: db}
}

func (r *OutboxRepository) PublishBatch(ctx context.Context, limit int, publish func(ctx context.Context, event domain.OutboxEvent) error) (int, error) {
	if limit <= 0 {
		limit = 50
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	events, err := fetchPendingForUpdate(ctx, tx, limit)
	if err != nil {
		return 0, err
	}

	published := 0
	for _, event := range events {
		if err := publish(ctx, event); err != nil {
			if _, incErr := tx.ExecContext(ctx, `UPDATE outbox_events SET attempts = attempts + 1 WHERE id = $1`, event.ID); incErr != nil {
				return published, fmt.Errorf("increment attempts: %w", incErr)
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE outbox_events SET published_at = now() WHERE id = $1`, event.ID); err != nil {
			return published, fmt.Errorf("mark published: %w", err)
		}
		published++
	}

	if err := tx.Commit(); err != nil {
		return published, fmt.Errorf("commit tx: %w", err)
	}
	return published, nil
}

func insertOutboxEvent(ctx context.Context, q querier, aggregateID uuid.UUID, eventType domain.OutboxEventType, payload []byte) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO outbox_events (aggregate_id, event_type, payload)
		VALUES ($1, $2, $3)`,
		aggregateID, eventType, payload,
	)
	return err
}

func fetchPendingForUpdate(ctx context.Context, tx *sql.Tx, limit int) ([]domain.OutboxEvent, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, aggregate_id, event_type, payload, created_at, published_at, attempts
		FROM outbox_events
		WHERE published_at IS NULL
		ORDER BY created_at
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, fmt.Errorf("fetch pending outbox: %w", err)
	}
	defer rows.Close()

	var events []domain.OutboxEvent
	for rows.Next() {
		var event domain.OutboxEvent
		var publishedAt sql.NullTime
		if err := rows.Scan(
			&event.ID,
			&event.AggregateID,
			&event.EventType,
			&event.Payload,
			&event.CreatedAt,
			&publishedAt,
			&event.Attempts,
		); err != nil {
			return nil, err
		}
		if publishedAt.Valid {
			event.PublishedAt = &publishedAt.Time
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func saveVideo(ctx context.Context, q querier, v *domain.Video) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO videos (id, title, description, status, duration_sec, file_size, content_type, origin_key, error_message, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		v.ID, v.Title, v.Description, v.Status, v.DurationSec, v.FileSize, v.ContentType, v.OriginKey, v.ErrorMessage, v.CreatedAt, v.UpdatedAt,
	)
	return err
}

func updateVideo(ctx context.Context, q querier, v *domain.Video) error {
	_, err := q.ExecContext(ctx, `
		UPDATE videos SET title=$2, description=$3, status=$4, duration_sec=$5, file_size=$6,
		content_type=$7, origin_key=$8, error_message=$9, updated_at=$10 WHERE id=$1`,
		v.ID, v.Title, v.Description, v.Status, v.DurationSec, v.FileSize, v.ContentType, v.OriginKey, v.ErrorMessage, time.Now().UTC(),
	)
	return err
}
