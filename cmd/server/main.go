package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prismatic-media/prism-server/internal/api"
	"github.com/prismatic-media/prism-server/internal/config"
	"github.com/prismatic-media/prism-server/internal/metadata"
	"github.com/prismatic-media/prism-server/internal/scanner"
	"github.com/prismatic-media/prism-server/internal/store/sqlite"
	"github.com/prismatic-media/prism-server/internal/transcoder"
	"github.com/prismatic-media/prism-server/pkg/dash"
	"github.com/prismatic-media/prism-server/pkg/events"
)

// @title Prism Media Server API
// @version 1.0
// @description REST & WebSocket API specification for Prism, a self-hosted media server.
// @host localhost:8080
// @BasePath /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer <your-token>" to authenticate.

// @securityDefinitions.apikey WorkerAuth
// @in header
// @name X-Worker-API-Key
// @description The secret API Key assigned to the transcode worker.

// Main entry point for the Prism media server.
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg := config.Load()

	db, err := sqlite.Open(cfg.DBPath)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	if err := sqlite.Migrate(db); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	if err := sqlite.BootstrapSettings(context.Background(), db); err != nil {
		slog.Error("failed to bootstrap settings", "error", err)
		os.Exit(1)
	}

	if err := sqlite.DeleteExpiredTokens(context.Background(), db); err != nil {
		slog.Warn("failed to clean up expired refresh tokens", "error", err)
	}

	rawSettings, err := sqlite.GetAllSettings(context.Background(), db, false)
	if err != nil {
		slog.Error("failed to load settings", "error", err)
		os.Exit(1)
	}
	rs := config.RuntimeSettingsFromMap(rawSettings)

	if rs.JWTSecret == "" {
		slog.Error("jwt_secret is empty after bootstrap — cannot start")
		os.Exit(1)
	}

	enricher := metadata.NewEnricher(db)
	bus := events.NewBus()
	scanManager := scanner.NewManager(db, enricher, bus)
	if err := scanManager.StartAll(context.Background()); err != nil {
		slog.Warn("failed to start library scanners", "error", err)
	}

	workers := rs.TranscodeWorkers
	if val, ok := rawSettings["transcode_workers"]; ok && val == "0" {
		workers = 0
	} else if workers <= 0 {
		workers = 2
	}
	mpdCache := &dash.Cache{}
	pool := transcoder.NewPool(db, workers, mpdCache, bus)
	if err := pool.Start(context.Background()); err != nil {
		slog.Warn("failed to start transcode pool", "error", err)
	}

	router := api.NewRouter(&rs, db, enricher, scanManager, pool, bus)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("forced shutdown", "error", err)
	}
}
