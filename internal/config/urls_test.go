package config

import (
	"os"
	"testing"
)

func TestResolvePublicURLsFromBase(t *testing.T) {
	t.Setenv("CASTFLOW_BASE_URL", "http://localhost:8080")
	t.Setenv("CASTFLOW_API_BASE_URL", "")
	t.Setenv("CASTFLOW_CDN_BASE_URL", "")
	t.Setenv("CASTFLOW_PLAYER_BASE_URL", "")

	api, cdn, player := resolvePublicURLs()
	if api != "http://localhost:8080" {
		t.Fatalf("api: got %q", api)
	}
	if cdn != "http://localhost:8080/media" {
		t.Fatalf("cdn: got %q", cdn)
	}
	if player != "http://localhost:8080/player" {
		t.Fatalf("player: got %q", player)
	}
}

func TestResolvePublicURLsOverrides(t *testing.T) {
	t.Setenv("CASTFLOW_BASE_URL", "http://localhost:8080")
	t.Setenv("CASTFLOW_CDN_BASE_URL", "https://cdn.example.com")
	t.Setenv("CASTFLOW_PLAYER_BASE_URL", "https://player.example.com")

	_, cdn, player := resolvePublicURLs()
	if cdn != "https://cdn.example.com" {
		t.Fatalf("cdn: got %q", cdn)
	}
	if player != "https://player.example.com" {
		t.Fatalf("player: got %q", player)
	}
}

func TestLoadUsesBaseURL(t *testing.T) {
	os.Setenv("CASTFLOW_BASE_URL", "http://test.local:9000")
	t.Cleanup(func() {
		os.Unsetenv("CASTFLOW_BASE_URL")
		os.Unsetenv("CASTFLOW_API_BASE_URL")
		os.Unsetenv("CASTFLOW_CDN_BASE_URL")
		os.Unsetenv("CASTFLOW_PLAYER_BASE_URL")
	})

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIBaseURL != "http://test.local:9000" {
		t.Fatalf("APIBaseURL: got %q", cfg.APIBaseURL)
	}
	if cfg.CDNBaseURL != "http://test.local:9000/media" {
		t.Fatalf("CDNBaseURL: got %q", cfg.CDNBaseURL)
	}
}
