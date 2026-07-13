package domain

import (
	"context"
	"io"

	"github.com/google/uuid"
)

// VideoRepository persists video metadata.
type VideoRepository interface {
	Save(ctx context.Context, video *Video) error
	Update(ctx context.Context, video *Video) error
	FindByID(ctx context.Context, id uuid.UUID) (*Video, error)
	List(ctx context.Context, limit, offset int) ([]*Video, int, error)
	Delete(ctx context.Context, id uuid.UUID) error
	FindByStatus(ctx context.Context, status VideoStatus, limit int) ([]*Video, error)
}

// ObjectStorage stores binary assets (origin, segments, thumbnails).
type ObjectStorage interface {
	Upload(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	DeletePrefix(ctx context.Context, prefix string) error
	PublicURL(key string) string
	EnsureBucket(ctx context.Context) error
}

// Transcoder converts raw video into HLS, DASH, thumbnail and tooltip assets.
type Transcoder interface {
	Process(ctx context.Context, input TranscodeInput) (*TranscodeOutput, error)
	ProbeDuration(ctx context.Context, inputPath string) (int, error)
}

// TranscodeInput holds paths for a transcode job.
type TranscodeInput struct {
	VideoID            string
	InputPath          string
	OutputDir          string
	Variant            string
	Qualities          []QualityProfile
	ThumbnailAtSec     float64
	TooltipIntervalSec float64
}

// QualityProfile defines one rendition.
type QualityProfile struct {
	Name         string // e.g. "360p"
	Width        int
	Height       int
	VideoBitrate string // e.g. "800k"
	AudioBitrate string // e.g. "96k"
}

// TranscodeOutput reports generated artifact paths (relative to output dir).
type TranscodeOutput struct {
	DurationSec  int
	HLSMaster    string
	DASHManifest string
	Thumbnail    string
	TooltipVTT   string
	TooltipPNG   string
}

// TranscodeJobOptions configures a transcode queue job.
type TranscodeJobOptions struct {
	Profiles []string
	Force    bool
}

// JobQueue enqueues background transcode jobs.
type JobQueue interface {
	EnqueueTranscode(ctx context.Context, videoID uuid.UUID, opts TranscodeJobOptions) error
}

// RenditionRepository persists per-profile playback outputs.
type RenditionRepository interface {
	Save(ctx context.Context, r *Rendition) error
	Update(ctx context.Context, r *Rendition) error
	FindByVideoID(ctx context.Context, videoID uuid.UUID) ([]*Rendition, error)
	FindLatestByProfile(ctx context.Context, videoID uuid.UUID, profile string) (*Rendition, error)
	FindByVideoProfileRevision(ctx context.Context, videoID uuid.UUID, profile, revision string) (*Rendition, error)
	DeleteByVideoID(ctx context.Context, videoID uuid.UUID) error
}

// TranscodeScheduler schedules transcode jobs via the outbox.
type TranscodeScheduler interface {
	ScheduleTranscode(ctx context.Context, videoID uuid.UUID, opts TranscodeJobOptions) error
}

// WebhookNotifier sends HTTP callbacks for lifecycle events.
type WebhookNotifier interface {
	NotifyRenditionReady(ctx context.Context, video *Video, rendition *Rendition, links RenditionLinks) error
}

// VideoUploadWriter atomically persists upload metadata and schedules transcoding.
type VideoUploadWriter interface {
	AcceptUpload(ctx context.Context, video *Video) error
}

// OutboxRelay publishes pending outbox events to the job queue.
type OutboxRelay interface {
	Run(ctx context.Context) error
}
