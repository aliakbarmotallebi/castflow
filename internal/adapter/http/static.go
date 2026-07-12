package http

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// MediaServer serves transcoded assets from local storage (dev / single-node).
func MediaServer(localDir string) http.Handler {
	root := http.Dir(localDir)
	fileServer := http.FileServer(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		ext := strings.ToLower(filepath.Ext(r.URL.Path))
		switch ext {
		case ".m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		case ".ts":
			w.Header().Set("Content-Type", "video/mp2t")
		case ".mpd":
			w.Header().Set("Content-Type", "application/dash+xml")
		case ".m4s":
			w.Header().Set("Content-Type", "video/iso.segment")
		case ".vtt":
			w.Header().Set("Content-Type", "text/vtt")
		}
		if ext == ".m3u8" || ext == ".ts" || ext == ".mpd" {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=86400")
		}
		http.StripPrefix("/media", fileServer).ServeHTTP(w, r)
	})
}

// PlayerServer serves the embedded player static files.
func PlayerServer(playerDir string) http.Handler {
	if _, err := os.Stat(playerDir); err != nil {
		playerDir = "."
	}
	return http.StripPrefix("/player", http.FileServer(http.Dir(playerDir)))
}
