package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestURLBuilder_BuildLinks(t *testing.T) {
	b := NewURLBuilder("https://cdn.example.com", "https://player.example.com", []string{"360p", "720p"})
	links := b.BuildLinks("abc-123", "Test Video")

	if links.HLS != "https://cdn.example.com/v/abc-123/hls/master.m3u8" {
		t.Errorf("unexpected HLS: %s", links.HLS)
	}
	if links.Config != "https://cdn.example.com/v/abc-123/config.json" {
		t.Errorf("unexpected config: %s", links.Config)
	}
	if links.Thumbnail != "https://cdn.example.com/v/abc-123/thumbnail.jpg" {
		t.Errorf("unexpected thumbnail: %s", links.Thumbnail)
	}
	if links.OriginMP4 != "https://cdn.example.com/v/abc-123/origin.mp4" {
		t.Errorf("unexpected origin: %s", links.OriginMP4)
	}
	if links.IFrame == "" {
		t.Error("iframe should not be empty")
	}
}

func TestURLBuilder_BuildPlayerConfig_Qualities(t *testing.T) {
	b := NewURLBuilder("https://cdn.example.com", "https://player.example.com", []string{"720p", "1080p"})
	video := &Video{Title: "Test", ID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")}
	cfg := b.BuildPlayerConfig(video)
	if len(cfg.Qualities) != 2 || cfg.Qualities[0] != "720p" {
		t.Errorf("qualities: %v", cfg.Qualities)
	}
}

func TestStorageKeys(t *testing.T) {
	keys := StorageKeys("vid-1")
	if keys.Origin != "v/vid-1/origin.mp4" {
		t.Errorf("origin key: %s", keys.Origin)
	}
	if keys.HLSMaster != "v/vid-1/hls/master.m3u8" {
		t.Errorf("hls master: %s", keys.HLSMaster)
	}
}
