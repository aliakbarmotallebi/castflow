package domain

import "testing"

func TestURLBuilder_BuildLinks(t *testing.T) {
	b := NewURLBuilder("https://cdn.example.com", "https://player.example.com")
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

func TestStorageKeys(t *testing.T) {
	keys := StorageKeys("vid-1")
	if keys.Origin != "v/vid-1/origin.mp4" {
		t.Errorf("origin key: %s", keys.Origin)
	}
	if keys.HLSMaster != "v/vid-1/hls/master.m3u8" {
		t.Errorf("hls master: %s", keys.HLSMaster)
	}
}
