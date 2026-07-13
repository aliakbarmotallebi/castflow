package domain

import (
	"time"

	"github.com/google/uuid"
)

// RenditionStatus tracks transcode progress for one profile output.
type RenditionStatus string

const (
	RenditionProcessing RenditionStatus = "processing"
	RenditionReady      RenditionStatus = "ready"
	RenditionError      RenditionStatus = "error"
)

// Rendition is one profile/revision playback output for a video.
type Rendition struct {
	ID           uuid.UUID
	VideoID      uuid.UUID
	Profile      string
	Revision     string
	Status       RenditionStatus
	DurationSec  int
	Qualities    []string
	ErrorMessage string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// VariantPath returns the storage/URL path segment profile/revision.
func (r *Rendition) VariantPath() string {
	return BuildVariantPath(r.Profile, r.Revision)
}

// RenditionLinks holds public URLs for one rendition.
type RenditionLinks struct {
	Profile    string   `json:"profile"`
	Revision   string   `json:"revision"`
	Status     string   `json:"status"`
	HLS        string   `json:"hlsUrl"`
	DASH       string   `json:"dashUrl"`
	Qualities  []string `json:"qualities,omitempty"`
	DurationSec int     `json:"durationSec,omitempty"`
}

// RenditionSource is one rendition entry inside config.json.
type RenditionSource struct {
	Profile   string         `json:"profile"`
	Revision  string         `json:"revision"`
	Qualities []string       `json:"qualities,omitempty"`
	Source    []PlayerSource `json:"source"`
	Poster    string         `json:"poster,omitempty"`
	Thumbnail string         `json:"thumbnail,omitempty"`
}
