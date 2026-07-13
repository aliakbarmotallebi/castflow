package config

import (
	"log/slog"
	"strings"
)

// SlogLevel maps CASTFLOW_LOG_LEVEL to slog.Level.
func SlogLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
