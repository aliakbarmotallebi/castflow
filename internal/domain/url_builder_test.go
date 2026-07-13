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
}

func TestURLBuilder_ProfileRevisionURLs(t *testing.T) {
	b := NewURLBuilder("https://cdn.example.com", "https://player.example.com", nil)
	hls := b.hlsURL("abc-123", "default", "a3f2b1c4")
	if hls != "https://cdn.example.com/v/abc-123/hls/default/a3f2b1c4/master.m3u8" {
		t.Fatalf("hls: %s", hls)
	}
	dash := b.dashURL("abc-123", "mobile", "deadbeef")
	if dash != "https://cdn.example.com/v/abc-123/dash/mobile/deadbeef/manifest.mpd" {
		t.Fatalf("dash: %s", dash)
	}
}

func TestURLBuilder_BuildPlayerConfig_Qualities(t *testing.T) {
	b := NewURLBuilder("https://cdn.example.com", "https://player.example.com", []string{"720p", "1080p"})
	video := &Video{Title: "Test", ID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")}
	renditions := []*Rendition{{
		Profile: "default", Revision: "abc12345", Status: RenditionReady,
		Qualities: []string{"720p", "1080p"},
	}}
	cfg := b.BuildPlayerConfig(video, renditions, map[string]PlaybackProfile{
		"default": {Name: "default", PlayerQualities: []string{"720p", "1080p"}},
	}, "default")
	if len(cfg.Qualities) != 2 || cfg.Qualities[0] != "720p" {
		t.Errorf("qualities: %v", cfg.Qualities)
	}
	if len(cfg.Renditions) != 1 || cfg.Renditions[0].Profile != "default" {
		t.Errorf("renditions: %+v", cfg.Renditions)
	}
}

func TestStorageKeysWithVariant(t *testing.T) {
	keys := StorageKeysWithVariant("vid-1", "default/a3f2b1c4")
	if keys.HLSMaster != "v/vid-1/hls/default/a3f2b1c4/master.m3u8" {
		t.Errorf("hls master: %s", keys.HLSMaster)
	}
}
