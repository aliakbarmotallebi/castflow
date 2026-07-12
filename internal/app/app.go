package app

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	httpadapter "castflow/internal/adapter/http"
	"castflow/internal/adapter/queue"
	"castflow/internal/application"
	"castflow/internal/config"
)

// App wires dependencies and runs HTTP + optional background worker.
type App struct {
	cfg       *config.Config
	server    *http.Server
	bootstrap *Bootstrap
}

// New builds the application dependency graph.
func New(cfg *config.Config) (*App, error) {
	bootstrap, err := NewBootstrap(cfg)
	if err != nil {
		return nil, err
	}

	uploadUC := application.NewUploadVideo(bootstrap.Repo, bootstrap.Store, bootstrap.UploadWriter, bootstrap.URLBuilder)
	h := httpadapter.NewHandler(httpadapter.Deps{
		Upload:   uploadUC,
		GetVideo: application.NewGetVideo(bootstrap.Repo),
		List:     application.NewListVideos(bootstrap.Repo),
		Links:    application.NewGetVideoLinks(bootstrap.Repo, bootstrap.URLBuilder),
		Delete:   application.NewDeleteVideo(bootstrap.Repo, bootstrap.Store),
		APIKey:   cfg.APIKey,
	})

	root := http.NewServeMux()
	root.Handle("/media/", httpadapter.MediaServer(cfg.Storage.LocalDir))
	root.Handle("/player/", httpadapter.PlayerServer("web/player"))
	root.Handle("/", h.Router())

	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      root,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return &App{
		cfg:       cfg,
		server:    srv,
		bootstrap: bootstrap,
	}, nil
}

// Run starts HTTP server and transcode worker until SIGINT/SIGTERM.
func (a *App) Run() error {
	defer a.bootstrap.QueueRunner.Close()

	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	go func() {
		slog.Info("outbox relay started", "interval", a.cfg.Outbox.PollInterval)
		if err := a.bootstrap.OutboxRelay.Run(workerCtx); err != nil && workerCtx.Err() == nil {
			slog.Error("outbox relay stopped", "err", err)
		}
	}()

	if a.cfg.Worker.EnableEmbedded {
		go func() {
			slog.Info("worker started", "concurrency", a.cfg.Worker.Concurrency, "queue", queue.Name)
			if err := a.bootstrap.QueueRunner.Run(workerCtx); err != nil && workerCtx.Err() == nil {
				slog.Error("worker stopped", "err", err)
			}
		}()
	} else {
		slog.Info("embedded worker disabled")
	}

	go func() {
		slog.Info("castflow listening", "addr", a.cfg.HTTPAddr)
		slog.Info("cdn base", "url", a.cfg.CDNBaseURL)
		slog.Info("player base", "url", a.cfg.PlayerBaseURL)
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")
	if a.cfg.Worker.EnableEmbedded {
		workerCancel()
		a.bootstrap.QueueRunner.Stop()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return a.server.Shutdown(ctx)
}
