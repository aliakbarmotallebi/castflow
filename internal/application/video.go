package application

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

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

// ProcessVideo runs the full transcode pipeline for one video.
type ProcessVideo struct {
	repo       domain.VideoRepository
	storage    domain.ObjectStorage
	transcoder domain.Transcoder
	urlBuilder *domain.URLBuilder
	tempDir    string
}

func NewProcessVideo(repo domain.VideoRepository, storage domain.ObjectStorage, transcoder domain.Transcoder, urlBuilder *domain.URLBuilder, tempDir string) *ProcessVideo {
	return &ProcessVideo{repo: repo, storage: storage, transcoder: transcoder, urlBuilder: urlBuilder, tempDir: tempDir}
}

func (uc *ProcessVideo) Execute(ctx context.Context, videoID uuid.UUID, qualities []domain.QualityProfile, thumbAt, tooltipInterval float64) error {
	video, err := uc.repo.FindByID(ctx, videoID)
	if err != nil {
		return err
	}
	if video.Status != domain.StatusUploaded && video.Status != domain.StatusProcessing {
		return fmt.Errorf("video %s cannot be processed in status %s", videoID, video.Status)
	}

	workDir := filepath.Join(uc.tempDir, videoID.String())
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("mkdir workdir: %w", err)
	}
	defer os.RemoveAll(workDir)

	keys := domain.StorageKeys(videoID.String())
	originPath := filepath.Join(workDir, "origin.mp4")
	if err := uc.downloadToFile(ctx, keys.Origin, originPath); err != nil {
		video.MarkError(err.Error())
		_ = uc.repo.Update(ctx, video)
		return err
	}

	output, err := uc.transcoder.Process(ctx, domain.TranscodeInput{
		VideoID:            videoID.String(),
		InputPath:          originPath,
		OutputDir:          workDir,
		Qualities:          qualities,
		ThumbnailAtSec:     thumbAt,
		TooltipIntervalSec: tooltipInterval,
	})
	if err != nil {
		video.MarkError(err.Error())
		_ = uc.repo.Update(ctx, video)
		return err
	}

	if err := uc.uploadArtifacts(ctx, videoID.String(), workDir, output); err != nil {
		video.MarkError(err.Error())
		_ = uc.repo.Update(ctx, video)
		return err
	}

	cfg := uc.urlBuilder.BuildPlayerConfig(video)
	cfgBytes, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := uc.storage.Upload(ctx, keys.Config, bytes.NewReader(cfgBytes), int64(len(cfgBytes)), "application/json"); err != nil {
		return fmt.Errorf("upload config: %w", err)
	}

	video.MarkReady(output.DurationSec)
	return uc.repo.Update(ctx, video)
}

func (uc *ProcessVideo) downloadToFile(ctx context.Context, key, dest string) error {
	rc, err := uc.storage.Download(ctx, key)
	if err != nil {
		return fmt.Errorf("download %s: %w", key, err)
	}
	defer rc.Close()

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, rc); err != nil {
		return err
	}
	return nil
}

func (uc *ProcessVideo) uploadArtifacts(ctx context.Context, videoID, workDir string, output *domain.TranscodeOutput) error {
	keys := domain.StorageKeys(videoID)
	uploads := []struct {
		key  string
		path string
		ct   string
	}{
		{keys.Thumbnail, filepath.Join(workDir, output.Thumbnail), "image/jpeg"},
		{keys.TooltipVTT, filepath.Join(workDir, output.TooltipVTT), "text/vtt"},
		{keys.TooltipPNG, filepath.Join(workDir, output.TooltipPNG), "image/png"},
		{keys.HLSMaster, filepath.Join(workDir, output.HLSMaster), "application/vnd.apple.mpegurl"},
		{keys.DASHManifest, filepath.Join(workDir, output.DASHManifest), "application/dash+xml"},
	}

	for _, u := range uploads {
		if err := uc.uploadFile(ctx, u.key, u.path, u.ct); err != nil {
			return err
		}
	}

	hlsDir := filepath.Join(workDir, "hls")
	if err := uc.uploadDir(ctx, filepath.Join(keys.Prefix, "hls"), hlsDir); err != nil {
		return err
	}
	dashDir := filepath.Join(workDir, "dash")
	return uc.uploadDir(ctx, filepath.Join(keys.Prefix, "dash"), dashDir)
}

func (uc *ProcessVideo) uploadFile(ctx context.Context, key, path, contentType string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	return uc.storage.Upload(ctx, key, f, st.Size(), contentType)
}

func (uc *ProcessVideo) uploadDir(ctx context.Context, prefix, dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		key := prefix + "/" + filepath.ToSlash(rel)
		ct := contentTypeForExt(filepath.Ext(path))
		return uc.uploadFile(ctx, key, path, ct)
	})
}

func contentTypeForExt(ext string) string {
	switch ext {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".ts":
		return "video/mp2t"
	case ".m4s":
		return "video/iso.segment"
	case ".mpd":
		return "application/dash+xml"
	default:
		return "application/octet-stream"
	}
}

// GetVideoLinks returns all playback URLs for a video.
type GetVideoLinks struct {
	repo       domain.VideoRepository
	urlBuilder *domain.URLBuilder
}

func NewGetVideoLinks(repo domain.VideoRepository, urlBuilder *domain.URLBuilder) *GetVideoLinks {
	return &GetVideoLinks{repo: repo, urlBuilder: urlBuilder}
}

type LinksOutput struct {
	Video  *domain.Video
	Links  domain.PlaybackLinks
	Status string
}

func (uc *GetVideoLinks) Execute(ctx context.Context, id uuid.UUID) (*LinksOutput, error) {
	video, err := uc.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	links := uc.urlBuilder.BuildLinks(video.ID.String(), video.Title)
	return &LinksOutput{Video: video, Links: links, Status: string(video.Status)}, nil
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
	repo    domain.VideoRepository
	storage domain.ObjectStorage
}

func NewDeleteVideo(repo domain.VideoRepository, storage domain.ObjectStorage) *DeleteVideo {
	return &DeleteVideo{repo: repo, storage: storage}
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
	return uc.repo.Delete(ctx, video.ID)
}
