package domain

import (
	"time"

	"github.com/google/uuid"
)

// VideoStatus represents the lifecycle stage of a video asset.
type VideoStatus string

const (
	StatusUploading  VideoStatus = "uploading"
	StatusUploaded   VideoStatus = "uploaded"
	StatusProcessing VideoStatus = "processing"
	StatusReady      VideoStatus = "ready"
	StatusError      VideoStatus = "error"
)

// Video is the core aggregate for a VOD asset.
type Video struct {
	ID           uuid.UUID
	Title        string
	Description  string
	Status       VideoStatus
	DurationSec  int
	FileSize     int64
	ContentType  string
	OriginKey    string
	ErrorMessage string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// NewVideo creates a video in uploading state.
func NewVideo(title, description, contentType string, fileSize int64) *Video {
	now := time.Now().UTC()
	return &Video{
		ID:          uuid.New(),
		Title:       title,
		Description: description,
		Status:      StatusUploading,
		FileSize:    fileSize,
		ContentType: contentType,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func (v *Video) MarkUploaded(originKey string) {
	v.OriginKey = originKey
	v.Status = StatusUploaded
	v.UpdatedAt = time.Now().UTC()
}

func (v *Video) MarkProcessing() {
	v.Status = StatusProcessing
	v.ErrorMessage = ""
	v.UpdatedAt = time.Now().UTC()
}

func (v *Video) MarkReady(durationSec int) {
	v.Status = StatusReady
	v.DurationSec = durationSec
	v.ErrorMessage = ""
	v.UpdatedAt = time.Now().UTC()
}

func (v *Video) MarkError(msg string) {
	v.Status = StatusError
	v.ErrorMessage = msg
	v.UpdatedAt = time.Now().UTC()
}
