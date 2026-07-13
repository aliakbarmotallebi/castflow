package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"castflow/internal/domain"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// VideoRepository implements domain.VideoRepository with PostgreSQL.
type VideoRepository struct {
	db *sql.DB
}

func NewVideoRepository(db *sql.DB) *VideoRepository {
	return &VideoRepository{db: db}
}

func Open(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	return db, nil
}

func (r *VideoRepository) Save(ctx context.Context, v *domain.Video) error {
	return saveVideo(ctx, r.db, v)
}

func (r *VideoRepository) Update(ctx context.Context, v *domain.Video) error {
	return updateVideo(ctx, r.db, v)
}

func (r *VideoRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Video, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, title, description, status, duration_sec, file_size, content_type, origin_key, playback_variant, error_message, created_at, updated_at
		FROM videos WHERE id=$1`, id)
	v, err := scanVideo(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return v, err
}

func (r *VideoRepository) List(ctx context.Context, limit, offset int) ([]*domain.Video, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM videos`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, title, description, status, duration_sec, file_size, content_type, origin_key, playback_variant, error_message, created_at, updated_at
		FROM videos ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list []*domain.Video
	for rows.Next() {
		v, err := scanVideo(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, v)
	}
	return list, total, rows.Err()
}

func (r *VideoRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM videos WHERE id=$1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *VideoRepository) FindByStatus(ctx context.Context, status domain.VideoStatus, limit int) ([]*domain.Video, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, title, description, status, duration_sec, file_size, content_type, origin_key, playback_variant, error_message, created_at, updated_at
		FROM videos WHERE status=$1 ORDER BY created_at ASC LIMIT $2`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*domain.Video
	for rows.Next() {
		v, err := scanVideo(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanVideo(row scannable) (*domain.Video, error) {
	var v domain.Video
	err := row.Scan(&v.ID, &v.Title, &v.Description, &v.Status, &v.DurationSec, &v.FileSize, &v.ContentType, &v.OriginKey, &v.PlaybackVariant, &v.ErrorMessage, &v.CreatedAt, &v.UpdatedAt)
	return &v, err
}
