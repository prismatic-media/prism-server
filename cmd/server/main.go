package main

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/ringmaster217/galactic-media-server/internal/api"
	"github.com/ringmaster217/galactic-media-server/internal/auth"
	"github.com/ringmaster217/galactic-media-server/internal/config"
	"github.com/ringmaster217/galactic-media-server/internal/metadata"
	"github.com/ringmaster217/galactic-media-server/internal/models"
	"github.com/ringmaster217/galactic-media-server/internal/scanner"
	"github.com/ringmaster217/galactic-media-server/internal/store/sqlite"
	"github.com/ringmaster217/galactic-media-server/internal/transcoder"
	"github.com/ringmaster217/galactic-media-server/pkg/dash"
	"github.com/ringmaster217/galactic-media-server/pkg/events"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	db, err := sqlite.Open(cfg.DBPath)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := sqlite.Migrate(db); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	if err := sqlite.DeleteExpiredTokens(context.Background(), db); err != nil {
		slog.Warn("failed to clean up expired refresh tokens", "error", err)
	}

	if err := maybeFirstRun(db); err != nil {
		slog.Error("first-run setup failed", "error", err)
		os.Exit(1)
	}

	enricher := metadata.NewEnricher(db, cfg.TMDBApiKey, cfg.ThumbsDir)
	bus := events.NewBus()
	scanManager := scanner.NewManager(db, cfg.FFprobePath, enricher, bus)
	if err := scanManager.StartAll(context.Background()); err != nil {
		slog.Warn("failed to start library scanners", "error", err)
	}

	workers := cfg.TranscodeWorkers
	if workers <= 0 {
		workers = 2
	}
	mpdCache := &dash.Cache{}
	pool := transcoder.NewPool(db, cfg.FFmpegPath, cfg.FFprobePath, cfg.SegmentsDir, workers, mpdCache, bus)
	if err := pool.Start(context.Background()); err != nil {
		slog.Warn("failed to start transcode pool", "error", err)
	}

	router := api.NewRouter(cfg, db, scanManager, pool, bus)

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

// maybeFirstRun interactively creates the initial admin account when no users
// exist in the database. It is a no-op on every subsequent startup.
func maybeFirstRun(db *sql.DB) error {
	count, err := sqlite.CountUsers(context.Background(), db)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("  Galactic Media — First-Run Setup")
	fmt.Println("  No users found. Create an admin account.")
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)

	fmt.Print("Username: ")
	scanner.Scan()
	username := strings.TrimSpace(scanner.Text())
	if username == "" {
		return fmt.Errorf("username cannot be empty")
	}

	fmt.Print("Email: ")
	scanner.Scan()
	email := strings.TrimSpace(scanner.Text())
	if email == "" {
		return fmt.Errorf("email cannot be empty")
	}

	var password string
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Print("Password: ")
		raw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return fmt.Errorf("reading password: %w", err)
		}
		password = string(raw)
	} else {
		fmt.Print("Password: ")
		scanner.Scan()
		password = scanner.Text()
	}
	if password == "" {
		return fmt.Errorf("password cannot be empty")
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	u := &models.User{
		Username:     username,
		Email:        email,
		PasswordHash: hash,
		IsAdmin:      true,
	}
	if err := sqlite.CreateUser(context.Background(), db, u); err != nil {
		return fmt.Errorf("creating admin user: %w", err)
	}

	fmt.Printf("\nAdmin account '%s' created. Starting server...\n\n", username)
	return nil
}
