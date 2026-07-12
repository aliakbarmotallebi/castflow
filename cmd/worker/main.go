package main

import (
	"log/slog"
	"os"

	"castflow/internal/app"
	"castflow/internal/config"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	worker, err := app.NewWorker(cfg)
	if err != nil {
		slog.Error("bootstrap", "err", err)
		os.Exit(1)
	}

	if err := worker.Run(); err != nil {
		slog.Error("shutdown", "err", err)
		os.Exit(1)
	}
}
