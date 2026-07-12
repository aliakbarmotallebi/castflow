package app

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"castflow/internal/adapter/queue"
	"castflow/internal/config"
)

// Worker runs only the transcode job consumer (no HTTP).
type Worker struct {
	bootstrap *Bootstrap
}

// NewWorker builds a worker-only process.
func NewWorker(cfg *config.Config) (*Worker, error) {
	bootstrap, err := NewBootstrap(cfg)
	if err != nil {
		return nil, err
	}
	return &Worker{bootstrap: bootstrap}, nil
}

// Run blocks until SIGINT/SIGTERM.
func (w *Worker) Run() error {
	defer w.bootstrap.QueueRunner.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		slog.Info("outbox relay started", "interval", w.bootstrap.Cfg.Outbox.PollInterval)
		if err := w.bootstrap.OutboxRelay.Run(ctx); err != nil && ctx.Err() == nil {
			slog.Error("outbox relay stopped", "err", err)
		}
	}()

	go func() {
		slog.Info("worker started", "concurrency", w.bootstrap.Cfg.Worker.Concurrency, "queue", queue.Name)
		if err := w.bootstrap.QueueRunner.Run(ctx); err != nil && ctx.Err() == nil {
			slog.Error("worker stopped", "err", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down worker")
	cancel()
	w.bootstrap.QueueRunner.Stop()
	return nil
}
