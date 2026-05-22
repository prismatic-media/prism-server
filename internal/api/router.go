package api

import (
	"database/sql"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/ringmaster217/galactic-media-server/internal/api/handler"
	apimw "github.com/ringmaster217/galactic-media-server/internal/api/middleware"
	"github.com/ringmaster217/galactic-media-server/internal/config"
	"github.com/ringmaster217/galactic-media-server/internal/scanner"
	"github.com/ringmaster217/galactic-media-server/internal/transcoder"
	"github.com/ringmaster217/galactic-media-server/pkg/events"
)

// NewRouter wires up all routes and middleware.
func NewRouter(cfg *config.Config, db *sql.DB, scanManager *scanner.Manager, pool *transcoder.Pool, bus *events.Bus) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(corsMiddleware)

	authH := handler.NewAuthHandler(db, cfg.JWTSecret)
	userH := handler.NewUsersHandler(db, cfg.JWTSecret)
	libH := handler.NewLibraryHandler(db, scanManager)
	mediaH := handler.NewMediaHandler(db, cfg.ThumbsDir)
	jobsH := handler.NewJobsHandler(db, pool)
	streamH := handler.NewStreamHandler(db, cfg.SegmentsDir, pool.MPDCache())
	historyH := handler.NewHistoryHandler(db)
	eventsH := handler.NewEventsHandler(bus)

	// Health check (unauthenticated)
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// API v1
	r.Route("/api/v1", func(r chi.Router) {
		// Auth endpoints — no JWT required
		r.Post("/auth/login", authH.Login)
		r.Post("/auth/refresh", authH.Refresh)
		r.Post("/auth/logout", authH.Logout)

		// User creation: open for first user, admin-only thereafter.
		// OptionalAuthenticate populates claims if a valid token is present.
		r.With(apimw.OptionalAuthenticate(cfg.JWTSecret)).Post("/users", userH.CreateUser)

		// Poster images are served unauthenticated so <img> tags work without
		// custom headers.
		r.Get("/media/{id}/poster", mediaH.ServePoster)

		// All routes below require a valid access JWT.
		r.Group(func(r chi.Router) {
			r.Use(apimw.Authenticate(cfg.JWTSecret))

			// Current user profile
			r.Get("/me", userH.GetMe)
			r.Put("/me", userH.UpdateMe)

			// Libraries (Phase 2)
			r.Get("/libraries", libH.ListLibraries)
			r.With(apimw.RequireAdmin).Post("/libraries", libH.CreateLibrary)
			r.Get("/libraries/{id}", libH.GetLibrary)
			r.With(apimw.RequireAdmin).Delete("/libraries/{id}", libH.DeleteLibrary)
			r.With(apimw.RequireAdmin).Post("/libraries/{id}/scan", libH.ScanLibrary)

			// Media items (Phase 2)
			r.Get("/media", mediaH.ListMedia)
			r.Get("/media/{id}", mediaH.GetMedia)
			r.With(apimw.RequireAdmin).Delete("/media/{id}", mediaH.DeleteMedia)
			r.With(apimw.RequireAdmin).Post("/media/{id}/transcode", jobsH.EnqueueTranscode)

			// Streaming (Phase 5)
			r.Get("/stream/{id}/manifest.mpd", streamH.ServeManifest)
			r.Get("/stream/{id}/segments/*", streamH.ServeSegment)

			// Watch history (Phase 5)
			r.Get("/history", historyH.GetHistory)
			r.Put("/history/{mediaID}", historyH.UpsertHistory)

			// Transcode jobs (Phase 4)
			r.With(apimw.RequireAdmin).Get("/jobs", jobsH.ListJobs)
			r.With(apimw.RequireAdmin).Get("/jobs/{id}", jobsH.GetJob)

			// WebSocket for job progress (Phase 4) — no RequireAdmin; auth checked via JWT in query/header
			r.With(apimw.RequireAdmin).Get("/ws/jobs/{id}", jobsH.JobProgress)

			// WebSocket for global real-time events — any authenticated user.
			r.Get("/ws/events", eventsH.ServeEvents)
		})
	})

	// Serve Angular frontend (catch-all)
	// Angular 21 outputs to web/dist/browser/ via @angular/build:application.
	// Any path that doesn't match a real file is served as index.html so that
	// Angular's client-side router handles it (SPA fallback).
	r.Get("/*", spaHandler("web/dist/browser"))

	return r
}

// spaHandler serves static files from root. If the requested path does not
// correspond to an existing file, it serves index.html so Angular's router
// can handle the route on the client side.
func spaHandler(root string) http.HandlerFunc {
	fs := http.Dir(root)
	fileServer := http.FileServer(fs)
	return func(w http.ResponseWriter, r *http.Request) {
		// Check whether the file exists in the dist directory.
		f, err := fs.Open(r.URL.Path)
		if err != nil {
			// File not found — serve index.html for client-side routing.
			http.ServeFile(w, r, root+"/index.html")
			return
		}
		f.Close()
		fileServer.ServeHTTP(w, r)
	}
}

// corsMiddleware adds permissive CORS headers for local dev.
// TODO: tighten allowed origins in production via config.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func placeholder(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		w.Write([]byte(`{"error":"not implemented","route":"` + name + `"}`))
	}
}
