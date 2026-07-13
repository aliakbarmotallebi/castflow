package config

import "log/slog"

func ExampleSlogLevel() {
	_ = SlogLevel("debug") == slog.LevelDebug
	_ = SlogLevel("info") == slog.LevelInfo
	_ = SlogLevel("unknown") == slog.LevelInfo
}
