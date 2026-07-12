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

// Transcoder implements domain.Transcoder using FFmpeg CLI.
type Transcoder struct {
	ffmpegPath  string
	ffprobePath string
	segmentSec  int
}

func NewTranscoder(ffmpegPath, ffprobePath string, segmentSec int) *Transcoder {
	return &Transcoder{ffmpegPath: ffmpegPath, ffprobePath: ffprobePath, segmentSec: segmentSec}
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
	return exec.CommandContext(ctx, t.ffmpegPath,
		"-y", "-ss", fmt.Sprintf("%.2f", atSec),
		"-i", input, "-vframes", "1", "-q:v", "2", dest,
	).Run()
}

func (t *Transcoder) generateTooltip(ctx context.Context, input, outDir string, interval float64) error {
	framesDir := filepath.Join(outDir, "tooltip_frames")
	if err := os.MkdirAll(framesDir, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(framesDir)

	fps := 1.0 / interval
	if err := exec.CommandContext(ctx, t.ffmpegPath,
		"-y", "-i", input,
		"-vf", fmt.Sprintf("fps=%.4f,scale=160:90", fps),
		filepath.Join(framesDir, "frame_%04d.jpg"),
	).Run(); err != nil {
		return fmt.Errorf("tooltip frames: %w", err)
	}

	entries, err := os.ReadDir(framesDir)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("no tooltip frames generated")
	}

	spritePath := filepath.Join(outDir, "tooltip.png")
	// tile: up to 10 columns
	cols := 10
	if len(entries) < cols {
		cols = len(entries)
	}
	tileFilter := fmt.Sprintf("tile=%dx", cols)
	inputs := make([]string, 0, len(entries)*2)
	for i, e := range entries {
		inputs = append(inputs, "-i", filepath.Join(framesDir, e.Name()))
		_ = i
	}
	args := append([]string{"-y"}, inputs...)
	args = append(args, "-filter_complex", fmt.Sprintf("%s%s", buildConcatFilter(len(entries)), tileFilter),
		"-frames:v", "1", spritePath)
	if err := exec.CommandContext(ctx, t.ffmpegPath, args...).Run(); err != nil {
		return fmt.Errorf("tooltip sprite: %w", err)
	}

	return writeTooltipVTT(outDir, spritePath, len(entries), interval)
}

func buildConcatFilter(n int) string {
	var parts []string
	for i := 0; i < n; i++ {
		parts = append(parts, fmt.Sprintf("[%d:v]", i))
	}
	return strings.Join(parts, "") + fmt.Sprintf("concat=n=%d:v=1:a=0[v];[v]", n)
}

func writeTooltipVTT(outDir, spriteURL string, frameCount int, interval float64) error {
	var buf bytes.Buffer
	buf.WriteString("WEBVTT\n\n")
	cols := 10
	w, h := 160, 90
	for i := 0; i < frameCount; i++ {
		start := float64(i) * interval
		end := start + interval
		col := i % cols
		row := i / cols
		x := col * w
		y := row * h
		buf.WriteString(fmt.Sprintf("%s --> %s\n", formatVTTTime(start), formatVTTTime(end)))
		buf.WriteString(fmt.Sprintf("tooltip.png#xywh=%d,%d,%d,%d\n\n", x, y, w, h))
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
		if err := exec.CommandContext(ctx, t.ffmpegPath, args...).Run(); err != nil {
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
	return exec.CommandContext(ctx, t.ffmpegPath, args...).Run()
}
