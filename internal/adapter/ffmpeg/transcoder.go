package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"castflow/internal/domain"
)

const (
	defaultTooltipMaxFrames = 60
	defaultTooltipCols      = 10
	tooltipFrameW           = 160
	tooltipFrameH           = 90
)

// Transcoder implements domain.Transcoder using FFmpeg CLI.
type Transcoder struct {
	ffmpegPath       string
	ffprobePath      string
	segmentSec       int
	tooltipMaxFrames int
	tooltipCols      int
}

func NewTranscoder(ffmpegPath, ffprobePath string, segmentSec, tooltipMaxFrames, tooltipCols int) *Transcoder {
	if tooltipMaxFrames <= 0 {
		tooltipMaxFrames = defaultTooltipMaxFrames
	}
	if tooltipCols <= 0 {
		tooltipCols = defaultTooltipCols
	}
	return &Transcoder{
		ffmpegPath:       ffmpegPath,
		ffprobePath:      ffprobePath,
		segmentSec:       segmentSec,
		tooltipMaxFrames: tooltipMaxFrames,
		tooltipCols:      tooltipCols,
	}
}

func (t *Transcoder) ProbeDuration(ctx context.Context, inputPath string) (int, error) {
	out, err := exec.CommandContext(ctx, t.ffprobePath,
		"-v", "error", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", inputPath,
	).Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe: %w", err)
	}
	sec, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0, err
	}
	return int(sec + 0.5), nil
}

func (t *Transcoder) Process(ctx context.Context, input domain.TranscodeInput) (*domain.TranscodeOutput, error) {
	if len(input.Qualities) == 0 {
		return nil, fmt.Errorf("no quality profiles configured")
	}

	hlsDir := filepath.Join(input.OutputDir, "hls")
	dashDir := filepath.Join(input.OutputDir, "dash")
	if err := os.MkdirAll(hlsDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dashDir, 0o755); err != nil {
		return nil, err
	}

	if err := t.generateThumbnail(ctx, input.InputPath, input.OutputDir, input.ThumbnailAtSec); err != nil {
		return nil, err
	}
	if err := t.generateTooltip(ctx, input.InputPath, input.OutputDir, input.TooltipIntervalSec); err != nil {
		return nil, err
	}

	if err := t.transcodeHLS(ctx, input, hlsDir); err != nil {
		return nil, err
	}
	if err := t.transcodeDASH(ctx, input, dashDir); err != nil {
		return nil, err
	}

	duration, err := t.ProbeDuration(ctx, input.InputPath)
	if err != nil {
		return nil, err
	}

	return &domain.TranscodeOutput{
		DurationSec:  duration,
		HLSMaster:    "hls/master.m3u8",
		DASHManifest: "dash/manifest.mpd",
		Thumbnail:    "thumbnail.jpg",
		TooltipVTT:   "tooltip.vtt",
		TooltipPNG:   "tooltip.png",
	}, nil
}

func (t *Transcoder) generateThumbnail(ctx context.Context, input, outDir string, atSec float64) error {
	dest := filepath.Join(outDir, "thumbnail.jpg")
	return runFFmpeg(ctx, t.ffmpegPath,
		"-y", "-ss", fmt.Sprintf("%.2f", atSec),
		"-i", input, "-vframes", "1", "-q:v", "2", dest,
	)
}

func (t *Transcoder) generateTooltip(ctx context.Context, input, outDir string, interval float64) error {
	durationSec, err := t.ProbeDuration(ctx, input)
	if err != nil {
		return fmt.Errorf("tooltip duration: %w", err)
	}

	effectiveInterval := tooltipInterval(float64(durationSec), interval, t.tooltipMaxFrames)

	framesDir := filepath.Join(outDir, "tooltip_frames")
	if err := os.MkdirAll(framesDir, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(framesDir)

	framePattern := filepath.Join(framesDir, "frame_%04d.jpg")
	fps := 1.0 / effectiveInterval
	if err := runFFmpeg(ctx, t.ffmpegPath,
		"-y", "-i", input,
		"-vf", fmt.Sprintf("fps=%.4f,scale=%d:%d", fps, tooltipFrameW, tooltipFrameH),
		framePattern,
	); err != nil {
		return fmt.Errorf("tooltip frames: %w", err)
	}

	frameCount, err := countFrameFiles(framesDir)
	if err != nil {
		return err
	}
	if frameCount == 0 {
		return fmt.Errorf("no tooltip frames generated")
	}
	if frameCount > t.tooltipMaxFrames {
		frameCount = t.tooltipMaxFrames
	}

	cols, rows := tooltipGrid(frameCount, t.tooltipCols)
	spritePath := filepath.Join(outDir, "tooltip.png")
	if err := runFFmpeg(ctx, t.ffmpegPath,
		"-y",
		"-framerate", "1",
		"-start_number", "1",
		"-i", framePattern,
		"-frames:v", strconv.Itoa(frameCount),
		"-vf", fmt.Sprintf("tile=%dx%d", cols, rows),
		spritePath,
	); err != nil {
		return fmt.Errorf("tooltip sprite: %w", err)
	}

	return writeTooltipVTT(outDir, frameCount, effectiveInterval, cols)
}

// tooltipInterval widens the sampling interval for long videos so frame count stays bounded.
func tooltipInterval(durationSec, requestedInterval float64, maxFrames int) float64 {
	if requestedInterval <= 0 {
		requestedInterval = 5
	}
	if maxFrames <= 0 {
		maxFrames = defaultTooltipMaxFrames
	}
	if durationSec <= 0 {
		return requestedInterval
	}
	if durationSec/requestedInterval <= float64(maxFrames) {
		return requestedInterval
	}
	return durationSec / float64(maxFrames)
}

func tooltipGrid(frameCount, maxCols int) (cols, rows int) {
	if maxCols <= 0 {
		maxCols = defaultTooltipCols
	}
	cols = maxCols
	if frameCount < cols {
		cols = frameCount
	}
	rows = (frameCount + cols - 1) / cols
	return cols, rows
}

func countFrameFiles(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), "frame_") && strings.HasSuffix(e.Name(), ".jpg") {
			count++
		}
	}
	return count, nil
}

func runFFmpeg(ctx context.Context, ffmpegPath string, args ...string) error {
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}

func writeTooltipVTT(outDir string, frameCount int, interval float64, cols int) error {
	var buf bytes.Buffer
	buf.WriteString("WEBVTT\n\n")
	for i := 0; i < frameCount; i++ {
		start := float64(i) * interval
		end := start + interval
		col := i % cols
		row := i / cols
		x := col * tooltipFrameW
		y := row * tooltipFrameH
		buf.WriteString(fmt.Sprintf("%s --> %s\n", formatVTTTime(start), formatVTTTime(end)))
		buf.WriteString(fmt.Sprintf("tooltip.png#xywh=%d,%d,%d,%d\n\n", x, y, tooltipFrameW, tooltipFrameH))
	}
	return os.WriteFile(filepath.Join(outDir, "tooltip.vtt"), buf.Bytes(), 0o644)
}

func formatVTTTime(sec float64) string {
	h := int(sec) / 3600
	m := (int(sec) % 3600) / 60
	s := int(sec) % 60
	ms := int((sec - float64(int(sec))) * 1000)
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}

func (t *Transcoder) transcodeHLS(ctx context.Context, input domain.TranscodeInput, hlsDir string) error {
	for _, q := range input.Qualities {
		qDir := filepath.Join(hlsDir, q.Name)
		if err := os.MkdirAll(qDir, 0o755); err != nil {
			return err
		}
		segPattern := filepath.Join(qDir, "seg_%05d.ts")
		playlist := filepath.Join(qDir, "playlist.m3u8")
		scale := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2",
			q.Width, q.Height, q.Width, q.Height)
		args := []string{
			"-y", "-i", input.InputPath,
			"-vf", scale,
			"-c:v", "libx264", "-b:v", q.VideoBitrate,
			"-c:a", "aac", "-b:a", q.AudioBitrate,
			"-f", "hls",
			"-hls_time", strconv.Itoa(t.segmentSec),
			"-hls_playlist_type", "vod",
			"-hls_segment_filename", segPattern,
			playlist,
		}
		if err := runFFmpeg(ctx, t.ffmpegPath, args...); err != nil {
			return fmt.Errorf("hls %s: %w", q.Name, err)
		}
	}
	return writeMasterPlaylist(hlsDir, input.Qualities)
}

func writeMasterPlaylist(hlsDir string, qualities []domain.QualityProfile) error {
	var buf bytes.Buffer
	buf.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n")
	for _, q := range qualities {
		bw := bitrateToInt(q.VideoBitrate) + bitrateToInt(q.AudioBitrate)
		buf.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%dx%d\n", bw, q.Width, q.Height))
		buf.WriteString(q.Name + "/playlist.m3u8\n")
	}
	return os.WriteFile(filepath.Join(hlsDir, "master.m3u8"), buf.Bytes(), 0o644)
}

func bitrateToInt(s string) int {
	s = strings.TrimSuffix(strings.ToLower(s), "k")
	n, _ := strconv.Atoi(s)
	return n * 1000
}

func (t *Transcoder) transcodeDASH(ctx context.Context, input domain.TranscodeInput, dashDir string) error {
	args := []string{"-y", "-i", input.InputPath}
	var maps []string
	for range input.Qualities {
		maps = append(maps, "-map", "0:v", "-map", "0:a")
	}
	args = append(args, maps...)
	for i, q := range input.Qualities {
		scale := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2",
			q.Width, q.Height, q.Width, q.Height)
		args = append(args,
			fmt.Sprintf("-filter:v:%d", i), scale,
			fmt.Sprintf("-c:v:%d", i), "libx264",
			fmt.Sprintf("-b:v:%d", i), q.VideoBitrate,
			fmt.Sprintf("-c:a:%d", i), "aac",
			fmt.Sprintf("-b:a:%d", i), q.AudioBitrate,
		)
	}
	manifest := filepath.Join(dashDir, "manifest.mpd")
	args = append(args,
		"-use_timeline", "1",
		"-use_template", "1",
		"-init_seg_name", "init-$RepresentationID$.m4s",
		"-media_seg_name", "chunk-$RepresentationID$-$Number%05d$.m4s",
		"-seg_duration", strconv.Itoa(t.segmentSec),
		"-f", "dash", manifest,
	)
	return runFFmpeg(ctx, t.ffmpegPath, args...)
}
