package application

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"castflow/internal/config"
	"castflow/internal/domain"
	"github.com/google/uuid"
)

// ProcessVideo runs the full transcode pipeline for one or more playback profiles.
type ProcessVideo struct {
	repo       domain.VideoRepository
	renditions domain.RenditionRepository
	storage    domain.ObjectStorage
	transcoder domain.Transcoder
	urlBuilder *domain.URLBuilder
	webhook    domain.WebhookNotifier
	playback   config.PlaybackConfig
	transcode  config.TranscodeConfig
	tempDir    string
}

func NewProcessVideo(
	repo domain.VideoRepository,
	renditions domain.RenditionRepository,
	storage domain.ObjectStorage,
	transcoder domain.Transcoder,
	urlBuilder *domain.URLBuilder,
	webhook domain.WebhookNotifier,
	playback config.PlaybackConfig,
	transcode config.TranscodeConfig,
	tempDir string,
) *ProcessVideo {
	return &ProcessVideo{
		repo:       repo,
		renditions: renditions,
		storage:    storage,
		transcoder: transcoder,
		urlBuilder: urlBuilder,
		webhook:    webhook,
		playback:   playback,
		transcode:  transcode,
		tempDir:    tempDir,
	}
}

func (uc *ProcessVideo) Execute(ctx context.Context, videoID uuid.UUID, opts domain.TranscodeJobOptions) error {
	video, err := uc.repo.FindByID(ctx, videoID)
	if err != nil {
		return err
	}
	if video.Status != domain.StatusUploaded && video.Status != domain.StatusProcessing && video.Status != domain.StatusReady && video.Status != domain.StatusError {
		return fmt.Errorf("video %s cannot be processed in status %s", videoID, video.Status)
	}

	profiles := uc.playback.ResolveProfiles(opts.Profiles)
	if len(profiles) == 0 {
		return domain.ErrInvalidInput
	}

	video.MarkProcessing()
	video.ErrorMessage = ""
	if err := uc.repo.Update(ctx, video); err != nil {
		return err
	}

	workDir := filepath.Join(uc.tempDir, videoID.String())
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("mkdir workdir: %w", err)
	}
	defer os.RemoveAll(workDir)

	keys := domain.StorageKeys(videoID.String())
	originPath := filepath.Join(workDir, "origin.mp4")
	if err := uc.downloadToFile(ctx, keys.Origin, originPath); err != nil {
		return uc.failVideo(ctx, video, err)
	}

	var (
		lastDuration int
		anySuccess   bool
		lastErr      error
		thumbDone    bool
	)

	for _, profile := range profiles {
		if err := uc.processProfile(ctx, video, originPath, workDir, profile, opts.Force, &thumbDone, &lastDuration); err != nil {
			lastErr = err
			slog.Error("profile transcode failed", "videoId", videoID, "profile", profile.Name, "err", err)
			continue
		}
		anySuccess = true
	}

	allRenditions, err := uc.renditions.FindByVideoID(ctx, videoID)
	if err != nil {
		return err
	}
	if err := uc.uploadConfig(ctx, video, allRenditions); err != nil {
		return uc.failVideo(ctx, video, err)
	}

	if !anySuccess {
		if lastErr != nil {
			return uc.failVideo(ctx, video, lastErr)
		}
		return uc.failVideo(ctx, video, fmt.Errorf("no profiles transcoded"))
	}

	video.PlaybackVariant = uc.primaryVariantPath(allRenditions)
	video.MarkReady(lastDuration)
	return uc.repo.Update(ctx, video)
}

func (uc *ProcessVideo) processProfile(
	ctx context.Context,
	video *domain.Video,
	originPath, workDir string,
	profile domain.PlaybackProfile,
	force bool,
	thumbDone *bool,
	lastDuration *int,
) error {
	revision := domain.BuildRevision(domain.RevisionInput{
		Profile:         profile.Name,
		Qualities:       profile.Qualities,
		HLSSegmentSec:   uc.transcode.HLSSegmentSeconds,
		ThumbnailAtSec:  uc.transcode.ThumbnailAtSec,
		TooltipInterval: uc.transcode.TooltipIntervalSec,
	})

	if !force {
		existing, err := uc.renditions.FindByVideoProfileRevision(ctx, video.ID, profile.Name, revision)
		if err == nil && existing.Status == domain.RenditionReady {
			slog.Info("skipping profile; revision ready", "videoId", video.ID, "profile", profile.Name, "revision", revision)
			if existing.DurationSec > 0 {
				*lastDuration = existing.DurationSec
			}
			return nil
		}
	}

	qualityNames := qualityNames(profile.Qualities)
	var rendition *domain.Rendition
	existing, findErr := uc.renditions.FindByVideoProfileRevision(ctx, video.ID, profile.Name, revision)
	if findErr == nil {
		rendition = existing
		rendition.Status = domain.RenditionProcessing
		rendition.ErrorMessage = ""
		rendition.Qualities = qualityNames
		if err := uc.renditions.Update(ctx, rendition); err != nil {
			return err
		}
	} else {
		rendition = &domain.Rendition{
			ID:        uuid.New(),
			VideoID:   video.ID,
			Profile:   profile.Name,
			Revision:  revision,
			Status:    domain.RenditionProcessing,
			Qualities: qualityNames,
		}
		if err := uc.renditions.Save(ctx, rendition); err != nil {
			return err
		}
	}

	variantPath := domain.BuildVariantPath(profile.Name, revision)
	output, err := uc.transcoder.Process(ctx, domain.TranscodeInput{
		VideoID:            video.ID.String(),
		InputPath:          originPath,
		OutputDir:          workDir,
		Variant:            variantPath,
		Qualities:          profile.Qualities,
		ThumbnailAtSec:     uc.transcode.ThumbnailAtSec,
		TooltipIntervalSec: uc.transcode.TooltipIntervalSec,
	})
	if err != nil {
		rendition.Status = domain.RenditionError
		rendition.ErrorMessage = err.Error()
		_ = uc.renditions.Update(ctx, rendition)
		return err
	}

	if err := uc.uploadProfileArtifacts(ctx, video.ID.String(), workDir, variantPath, output, !*thumbDone); err != nil {
		rendition.Status = domain.RenditionError
		rendition.ErrorMessage = err.Error()
		_ = uc.renditions.Update(ctx, rendition)
		return err
	}
	*thumbDone = true

	rendition.Status = domain.RenditionReady
	rendition.DurationSec = output.DurationSec
	rendition.ErrorMessage = ""
	if err := uc.renditions.Update(ctx, rendition); err != nil {
		return err
	}
	*lastDuration = output.DurationSec

	links := uc.urlBuilder.BuildRenditionLinks(video.ID.String(), video.Title, profile.Name, revision, qualityNames)
	if err := uc.webhook.NotifyRenditionReady(ctx, video, rendition, links); err != nil {
		slog.Warn("webhook failed", "videoId", video.ID, "profile", profile.Name, "err", err)
	}
	return nil
}

func (uc *ProcessVideo) uploadConfig(ctx context.Context, video *domain.Video, renditions []*domain.Rendition) error {
	cfg := uc.urlBuilder.BuildPlayerConfig(video, renditions, uc.playback.ProfileMap, uc.playback.PrimaryProfile)
	cfgBytes, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	keys := domain.StorageKeys(video.ID.String())
	return uc.storage.Upload(ctx, keys.Config, bytes.NewReader(cfgBytes), int64(len(cfgBytes)), "application/json")
}

func (uc *ProcessVideo) primaryVariantPath(renditions []*domain.Rendition) string {
	for _, r := range renditions {
		if r.Profile == uc.playback.PrimaryProfile && r.Status == domain.RenditionReady {
			return r.VariantPath()
		}
	}
	for _, r := range renditions {
		if r.Status == domain.RenditionReady {
			return r.VariantPath()
		}
	}
	return ""
}

func (uc *ProcessVideo) failVideo(ctx context.Context, video *domain.Video, err error) error {
	video.MarkError(err.Error())
	_ = uc.repo.Update(ctx, video)
	return err
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

func (uc *ProcessVideo) uploadProfileArtifacts(ctx context.Context, videoID, workDir, variantPath string, output *domain.TranscodeOutput, includeShared bool) error {
	keys := domain.StorageKeysWithVariant(videoID, variantPath)
	uploads := []struct {
		key  string
		path string
		ct   string
	}{
		{keys.HLSMaster, filepath.Join(workDir, output.HLSMaster), "application/vnd.apple.mpegurl"},
		{keys.DASHManifest, filepath.Join(workDir, output.DASHManifest), "application/dash+xml"},
	}
	if includeShared {
		uploads = append(uploads,
			struct{ key, path, ct string }{keys.Thumbnail, filepath.Join(workDir, output.Thumbnail), "image/jpeg"},
			struct{ key, path, ct string }{keys.TooltipVTT, filepath.Join(workDir, output.TooltipVTT), "text/vtt"},
			struct{ key, path, ct string }{keys.TooltipPNG, filepath.Join(workDir, output.TooltipPNG), "image/png"},
		)
	}

	for _, u := range uploads {
		if err := uc.uploadFile(ctx, u.key, u.path, u.ct); err != nil {
			return err
		}
	}

	hlsDir := filepath.Join(workDir, "hls", filepath.FromSlash(variantPath))
	if err := uc.uploadDir(ctx, filepath.Join(keys.Prefix, "hls", variantPath), hlsDir); err != nil {
		return err
	}
	dashDir := filepath.Join(workDir, "dash", filepath.FromSlash(variantPath))
	return uc.uploadDir(ctx, filepath.Join(keys.Prefix, "dash", variantPath), dashDir)
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

func qualityNames(qualities []domain.QualityProfile) []string {
	out := make([]string, len(qualities))
	for i, q := range qualities {
		out[i] = q.Name
	}
	return out
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

// RetranscodeVideo schedules a re-transcode for selected profiles.
type RetranscodeVideo struct {
	repo      domain.VideoRepository
	scheduler domain.TranscodeScheduler
}

func NewRetranscodeVideo(repo domain.VideoRepository, scheduler domain.TranscodeScheduler) *RetranscodeVideo {
	return &RetranscodeVideo{repo: repo, scheduler: scheduler}
}

type RetranscodeInput struct {
	Profiles []string
	Force    bool
}

func (uc *RetranscodeVideo) Execute(ctx context.Context, videoID uuid.UUID, input RetranscodeInput) error {
	video, err := uc.repo.FindByID(ctx, videoID)
	if err != nil {
		return err
	}
	if video.Status == domain.StatusUploading {
		return domain.ErrNotReady
	}
	if strings.TrimSpace(video.OriginKey) == "" {
		return domain.ErrInvalidInput
	}
	video.MarkProcessing()
	video.ErrorMessage = ""
	if err := uc.repo.Update(ctx, video); err != nil {
		return err
	}
	return uc.scheduler.ScheduleTranscode(ctx, videoID, domain.TranscodeJobOptions{
		Profiles: input.Profiles,
		Force:    input.Force,
	})
}

// GetVideoLinks returns playback URLs including all renditions.
type GetVideoLinks struct {
	repo       domain.VideoRepository
	renditions domain.RenditionRepository
	urlBuilder *domain.URLBuilder
	playback   config.PlaybackConfig
}

func NewGetVideoLinks(repo domain.VideoRepository, renditions domain.RenditionRepository, urlBuilder *domain.URLBuilder, playback config.PlaybackConfig) *GetVideoLinks {
	return &GetVideoLinks{repo: repo, renditions: renditions, urlBuilder: urlBuilder, playback: playback}
}

type LinksOutput struct {
	Video      *domain.Video
	Links      domain.PlaybackLinks
	Renditions []domain.RenditionLinks
	Primary    string
	Status     string
}

func (uc *GetVideoLinks) Execute(ctx context.Context, id uuid.UUID) (*LinksOutput, error) {
	video, err := uc.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	renditionRows, err := uc.renditions.FindByVideoID(ctx, id)
	if err != nil {
		return nil, err
	}

	var renditionLinks []domain.RenditionLinks
	var primaryLinks domain.PlaybackLinks
	primary := uc.playback.PrimaryProfile

	for _, r := range renditionRows {
		rl := uc.urlBuilder.BuildRenditionLinks(video.ID.String(), video.Title, r.Profile, r.Revision, r.Qualities)
		rl.Status = string(r.Status)
		rl.DurationSec = r.DurationSec
		renditionLinks = append(renditionLinks, rl)
		if r.Status == domain.RenditionReady && (r.Profile == primary || primaryLinks.HLS == "") {
			primaryLinks = uc.urlBuilder.BuildLinksForRendition(video, r)
		}
	}

	if primaryLinks.HLS == "" {
		primaryLinks = uc.urlBuilder.BuildLinksForVideo(video)
	}

	return &LinksOutput{
		Video:      video,
		Links:      primaryLinks,
		Renditions: renditionLinks,
		Primary:    primary,
		Status:     string(video.Status),
	}, nil
}
