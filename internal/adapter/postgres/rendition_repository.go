package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"castflow/internal/domain"
	"github.com/google/uuid"
)

// RenditionRepository implements domain.RenditionRepository.
type RenditionRepository struct {
	db *sql.DB
}

func NewRenditionRepository(db *sql.DB) *RenditionRepository {
	return &RenditionRepository{db: db}
}

func (r *RenditionRepository) Save(ctx context.Context, rend *domain.Rendition) error {
	if rend.ID == uuid.Nil {
		rend.ID = uuid.New()
	}
	now := time.Now().UTC()
	if rend.CreatedAt.IsZero() {
		rend.CreatedAt = now
	}
	rend.UpdatedAt = now
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO video_renditions (id, video_id, profile, revision, status, duration_sec, qualities, error_message, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		rend.ID, rend.VideoID, rend.Profile, rend.Revision, rend.Status, rend.DurationSec,
		joinQualities(rend.Qualities), rend.ErrorMessage, rend.CreatedAt, rend.UpdatedAt,
	)
	return err
}

func (r *RenditionRepository) Update(ctx context.Context, rend *domain.Rendition) error {
	rend.UpdatedAt = time.Now().UTC()
	res, err := r.db.ExecContext(ctx, `
		UPDATE video_renditions SET status=$2, duration_sec=$3, qualities=$4, error_message=$5, updated_at=$6
		WHERE id=$1`,
		rend.ID, rend.Status, rend.DurationSec, joinQualities(rend.Qualities), rend.ErrorMessage, rend.UpdatedAt,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *RenditionRepository) FindByVideoID(ctx context.Context, videoID uuid.UUID) ([]*domain.Rendition, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, video_id, profile, revision, status, duration_sec, qualities, error_message, created_at, updated_at
		FROM video_renditions WHERE video_id=$1 ORDER BY created_at ASC`, videoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRenditions(rows)
}

func (r *RenditionRepository) FindLatestByProfile(ctx context.Context, videoID uuid.UUID, profile string) (*domain.Rendition, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, video_id, profile, revision, status, duration_sec, qualities, error_message, created_at, updated_at
		FROM video_renditions WHERE video_id=$1 AND profile=$2
		ORDER BY created_at DESC LIMIT 1`, videoID, profile)
	rend, err := scanRendition(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return rend, err
}

func (r *RenditionRepository) FindByVideoProfileRevision(ctx context.Context, videoID uuid.UUID, profile, revision string) (*domain.Rendition, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, video_id, profile, revision, status, duration_sec, qualities, error_message, created_at, updated_at
		FROM video_renditions WHERE video_id=$1 AND profile=$2 AND revision=$3`,
		videoID, profile, revision)
	rend, err := scanRendition(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return rend, err
}

func (r *RenditionRepository) DeleteByVideoID(ctx context.Context, videoID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM video_renditions WHERE video_id=$1`, videoID)
	return err
}

func joinQualities(q []string) string {
	return strings.Join(q, ",")
}

func splitQualities(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func scanRenditions(rows *sql.Rows) ([]*domain.Rendition, error) {
	var list []*domain.Rendition
	for rows.Next() {
		rend, err := scanRendition(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, rend)
	}
	return list, rows.Err()
}

func scanRendition(row scannable) (*domain.Rendition, error) {
	var rend domain.Rendition
	var qualities string
	err := row.Scan(
		&rend.ID, &rend.VideoID, &rend.Profile, &rend.Revision, &rend.Status,
		&rend.DurationSec, &qualities, &rend.ErrorMessage, &rend.CreatedAt, &rend.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	rend.Qualities = splitQualities(qualities)
	return &rend, nil
}

// TranscodeScheduler schedules transcode jobs through the outbox.
type TranscodeScheduler struct {
	db *sql.DB
}

func NewTranscodeScheduler(db *sql.DB) *TranscodeScheduler {
	return &TranscodeScheduler{db: db}
}

func (s *TranscodeScheduler) ScheduleTranscode(ctx context.Context, videoID uuid.UUID, opts domain.TranscodeJobOptions) error {
	payload, err := marshalTranscodePayload(videoID, opts)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO outbox_events (aggregate_id, event_type, payload)
		VALUES ($1, $2, $3)`,
		videoID, domain.OutboxEventTranscode, payload,
	)
	return err
}

var _ domain.TranscodeScheduler = (*TranscodeScheduler)(nil)
