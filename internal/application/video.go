package application

import (
	"context"
	"fmt"
	"io"

	"castflow/internal/domain"
	"github.com/google/uuid"
)

// UploadVideo handles multipart video upload and schedules processing transactionally.
type UploadVideo struct {
	repo       domain.VideoRepository
	storage    domain.ObjectStorage
	uploader   domain.VideoUploadWriter
	urlBuilder *domain.URLBuilder
}

func NewUploadVideo(repo domain.VideoRepository, storage domain.ObjectStorage, uploader domain.VideoUploadWriter, urlBuilder *domain.URLBuilder) *UploadVideo {
	return &UploadVideo{repo: repo, storage: storage, uploader: uploader, urlBuilder: urlBuilder}
}

type UploadInput struct {
	Title       string
	Description string
	ContentType string
	FileSize    int64
	Body        io.Reader
}

type UploadOutput struct {
	Video   *domain.Video
	Message string
}

func (uc *UploadVideo) Execute(ctx context.Context, input UploadInput) (*UploadOutput, error) {
	if input.Title == "" || input.Body == nil || input.FileSize <= 0 {
		return nil, domain.ErrInvalidInput
	}
	if input.ContentType == "" {
		input.ContentType = "video/mp4"
	}

	video := domain.NewVideo(input.Title, input.Description, input.ContentType, input.FileSize)
	keys := domain.StorageKeys(video.ID.String())

	if err := uc.storage.Upload(ctx, keys.Origin, input.Body, input.FileSize, input.ContentType); err != nil {
		return nil, fmt.Errorf("upload origin: %w", err)
	}

	video.MarkUploaded(keys.Origin)
	video.MarkProcessing()
	if err := uc.uploader.AcceptUpload(ctx, video); err != nil {
		return nil, fmt.Errorf("accept upload: %w", err)
	}

	return &UploadOutput{
		Video:   video,
		Message: "upload accepted; transcoding started",
	}, nil
}

// ListVideos returns paginated videos.
type ListVideos struct {
	repo domain.VideoRepository
}

func NewListVideos(repo domain.VideoRepository) *ListVideos {
	return &ListVideos{repo: repo}
}

func (uc *ListVideos) Execute(ctx context.Context, limit, offset int) ([]*domain.Video, int, error) {
	if limit <= 0 {
		limit = 20
	}
	return uc.repo.List(ctx, limit, offset)
}

// GetVideo returns a single video by ID.
type GetVideo struct {
	repo domain.VideoRepository
}

func NewGetVideo(repo domain.VideoRepository) *GetVideo {
	return &GetVideo{repo: repo}
}

func (uc *GetVideo) Execute(ctx context.Context, id uuid.UUID) (*domain.Video, error) {
	return uc.repo.FindByID(ctx, id)
}

// DeleteVideo removes a video and its storage artifacts.
type DeleteVideo struct {
	repo       domain.VideoRepository
	renditions domain.RenditionRepository
	storage    domain.ObjectStorage
}

func NewDeleteVideo(repo domain.VideoRepository, renditions domain.RenditionRepository, storage domain.ObjectStorage) *DeleteVideo {
	return &DeleteVideo{repo: repo, renditions: renditions, storage: storage}
}

func (uc *DeleteVideo) Execute(ctx context.Context, id uuid.UUID) error {
	video, err := uc.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	keys := domain.StorageKeys(id.String())
	if err := uc.storage.DeletePrefix(ctx, keys.Prefix); err != nil {
		return fmt.Errorf("delete storage: %w", err)
	}
	_ = uc.renditions.DeleteByVideoID(ctx, id)
	return uc.repo.Delete(ctx, video.ID)
}
