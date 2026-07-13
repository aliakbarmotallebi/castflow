package main

import (
	"log/slog"
	"os"

	"castflow/internal/app"
	"castflow/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: config.SlogLevel(cfg.LogLevel),
	})))

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
