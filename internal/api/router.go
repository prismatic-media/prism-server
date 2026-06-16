package api

import (
	"database/sql"
	"io/fs"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/prismatic-media/prism-server/internal/api/handler"
	apimw "github.com/prismatic-media/prism-server/internal/api/middleware"
	"github.com/prismatic-media/prism-server/internal/config"
	"github.com/prismatic-media/prism-server/internal/metadata"
	"github.com/prismatic-media/prism-server/internal/scanner"
	"github.com/prismatic-media/prism-server/internal/transcoder"
	"github.com/prismatic-media/prism-server/pkg/events"
	"github.com/prismatic-media/prism-server/web"
)

// NewRouter wires up all routes and middleware.
func NewRouter(rs *config.RuntimeSettings, db *sql.DB, enricher *metadata.Enricher, scanManager *scanner.Manager, pool *transcoder.Pool, bus *events.Bus) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(apimw.Timeout(15 * time.Second))
	r.Use(corsMiddleware)

	authH := handler.NewAuthHandler(db, rs.JWTSecret)
	userH := handler.NewUsersHandler(db, rs.JWTSecret)
	libH := handler.NewLibraryHandler(db, scanManager)
	mediaH := handler.NewMediaHandler(db)
	jobsH := handler.NewJobsHandler(db, pool)
	streamH := handler.NewStreamHandler(db, pool.MPDCache(), rs.JWTSecret)
	historyH := handler.NewHistoryHandler(db)
	eventsH := handler.NewEventsHandler(bus)
	fsH := handler.NewFsHandler()
	tvH := handler.NewTVHandler(db)
	castH := handler.NewCastHandler(db)
	setupH := handler.NewSetupHandler(db)
	settingsH := handler.NewSettingsHandler(db)
	workerH := handler.NewWorkerHandler(db, pool, bus)
	workerAdminH := handler.NewWorkerAdminHandler(db)
	// Artifact admin handler for indexing and relinking.
	artifactIndexer := scanner.NewIndexer(db, bus)
	artifactH := handler.NewArtifactHandler(db, artifactIndexer)

	storageH := handler.NewStorageHandler(db, artifactIndexer)
	metadataH := handler.NewMetadataHandler(db, enricher)
	docsH := handler.NewDocsHandler()

	// Setup guard: redirects to /setup when setup is not yet complete.
	r.Use(apimw.SetupGuard(db))

	// Health check (unauthenticated)
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Chromecast custom receiver page — fetched directly by the Cast device.
	// Must be unauthenticated and publicly reachable.
	r.Get("/cast-receiver", castH.ServeReceiver)

	// API documentation (unauthenticated)
	r.Get("/docs", docsH.ServeDocsHTML)

	// API v1
	r.Route("/api/v1", func(r chi.Router) {
		// OpenAPI specification (unauthenticated)
		r.Get("/swagger.yaml", docsH.ServeSwaggerYAML)

		// Chromecast receiver (also served under /api so reverse proxies
		// that only forward /api/* to the Go backend will pass it through).
		r.Get("/cast-receiver", castH.ServeReceiver)

		// Auth endpoints — no JWT required
		r.Post("/auth/login", authH.Login)
		r.Post("/auth/refresh", authH.Refresh)
		r.Post("/auth/logout", authH.Logout)

		// Setup wizard — completes first-run configuration.
		r.Post("/setup", setupH.CompleteSetup)

		// Filesystem browse for path autocomplete (allows unauthenticated browsing only during setup).
		r.With(apimw.AuthenticateSetupOrAdmin(db, rs.JWTSecret)).Get("/fs/browse", fsH.BrowseDir)

		// User creation: open for first user, admin-only thereafter.
		// OptionalAuthenticate populates claims if a valid token is present.
		r.With(apimw.OptionalAuthenticate(rs.JWTSecret)).Post("/users", userH.CreateUser)

		// Poster and backdrop images are served unauthenticated so <img> tags work without
		// custom headers.
		r.Get("/media/{id}/poster", mediaH.ServePoster)
		r.Get("/media/{id}/backdrop", mediaH.ServeBackdrop)
		r.Get("/media/{id}/extra-posters/{index}", mediaH.ServeExtraPoster)
		r.Get("/tv/shows/{id}/poster", tvH.ServeShowPoster)
		r.Get("/tv/shows/{id}/backdrop", tvH.ServeShowBackdrop)
		r.Get("/tv/shows/{id}/extra-posters/{index}", tvH.ServeShowExtraPoster)
		r.Get("/tv/shows/{id}/seasons/{number}/poster", tvH.ServeSeasonPoster)

		// Streaming (Phase 5) — AuthenticateStream also accepts ?cast_token=
		// so Chromecast devices can fetch manifests and segments without
		// custom request headers.
		r.With(apimw.AuthenticateStream(rs.JWTSecret)).Get("/stream/{id}/manifest.mpd", streamH.ServeManifest)
		r.With(apimw.AuthenticateStream(rs.JWTSecret)).Get("/stream/{id}/segments/*", streamH.ServeSegment)

		// Remote transcoding worker endpoints
		r.Post("/worker/register", workerH.RegisterWorker)
		r.Route("/worker", func(r chi.Router) {
			r.Use(workerH.Authenticate)
			r.Post("/heartbeat", workerH.Heartbeat)
			r.Get("/media/{id}/download", workerH.DownloadSource)
			r.Post("/jobs/{id}/progress", workerH.UpdateProgress)
			r.Post("/jobs/{id}/bundle", workerH.UploadBundle)
		})

		// All routes below require a valid access JWT.
		r.Group(func(r chi.Router) {
			r.Use(apimw.Authenticate(rs.JWTSecret))

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
			r.Get("/search", mediaH.Search)
			r.Get("/media/{id}", mediaH.GetMedia)
			r.With(apimw.RequireAdmin).Delete("/media/{id}", mediaH.DeleteMedia)
			r.With(apimw.RequireAdmin).Post("/media/{id}/transcode", jobsH.EnqueueTranscode)

			// Cast token — issues a short-lived media-scoped token for Chromecast.
			r.Post("/stream/{id}/cast-token", streamH.IssueCastToken)

			// Watch history (Phase 5)
			r.Get("/history", historyH.GetHistory)
			r.Get("/history/now-playing", historyH.GetNowPlaying)
			r.Put("/history/{mediaID}", historyH.UpsertHistory)

			// Transcode jobs (Phase 4)
			r.With(apimw.RequireAdmin).Get("/jobs", jobsH.ListJobs)
			r.With(apimw.RequireAdmin).Get("/jobs/{id}", jobsH.GetJob)
			r.With(apimw.RequireAdmin).Post("/jobs/bulk-enqueue", jobsH.BulkEnqueueJobs)
			r.With(apimw.RequireAdmin).Post("/jobs/{id}/prioritize", jobsH.PrioritizeJob)

			// WebSocket for job progress (Phase 4) — no RequireAdmin; auth checked via JWT in query/header
			r.With(apimw.RequireAdmin).Get("/ws/jobs/{id}", jobsH.JobProgress)



			// TV show browsing
			r.Get("/tv/shows", tvH.ListShows)
			r.Get("/tv/shows/{id}", tvH.GetShow)
			r.Get("/tv/shows/{id}/seasons", tvH.ListSeasons)
			r.Get("/tv/shows/{id}/seasons/{number}/episodes", tvH.ListEpisodes)

			// WebSocket for global real-time events — any authenticated user.
			r.Get("/ws/events", eventsH.ServeEvents)

			// Cast config — returns the Cast App ID to the sender UI.
			r.Get("/cast/config", castH.GetConfig)

			// Admin settings
			r.With(apimw.RequireAdmin).Get("/admin/settings", settingsH.GetSettings)
			r.With(apimw.RequireAdmin).Put("/admin/settings", settingsH.UpdateSettings)
			r.With(apimw.RequireAdmin).Get("/admin/storage", storageH.ListStorage)
			r.With(apimw.RequireAdmin).Post("/admin/storage/areas", storageH.CreateStorageArea)
			r.With(apimw.RequireAdmin).Put("/admin/storage/areas/{id}", storageH.UpdateStorageArea)
			r.With(apimw.RequireAdmin).Delete("/admin/storage/areas/{id}", storageH.DeleteStorageArea)
			r.With(apimw.RequireAdmin).Put("/admin/storage/config", storageH.UpdateStorageConfig)

			// Admin workers
			r.With(apimw.RequireAdmin).Get("/admin/workers", workerAdminH.List)
			r.With(apimw.RequireAdmin).Post("/admin/workers", workerAdminH.Create)
			r.With(apimw.RequireAdmin).Put("/admin/workers/{id}", workerAdminH.Update)
			r.With(apimw.RequireAdmin).Delete("/admin/workers/{id}", workerAdminH.Delete)
			r.With(apimw.RequireAdmin).Get("/admin/workers/ephemeral-tokens", workerAdminH.ListEphemeralTokens)
			r.With(apimw.RequireAdmin).Post("/admin/workers/ephemeral-tokens", workerAdminH.CreateEphemeralToken)
			r.With(apimw.RequireAdmin).Delete("/admin/workers/ephemeral-tokens/{id}", workerAdminH.DeleteEphemeralToken)

			// Artifact indexing and relinking (admin only).
			r.With(apimw.RequireAdmin).Get("/admin/artifacts/status", artifactH.HandleStatus)
			r.With(apimw.RequireAdmin).Post("/admin/artifacts/index", artifactH.HandleIndex)
			r.With(apimw.RequireAdmin).Post("/admin/artifacts/relink", artifactH.HandleRelink)
			r.With(apimw.RequireAdmin).Post("/admin/artifacts/write-sidecars", artifactH.HandleWriteSidecars)

			// Metadata refresh (admin only).
			r.With(apimw.RequireAdmin).Post("/admin/metadata/refresh", metadataH.RefreshAllMetadata)
		})
	})

	// Serve Angular frontend (catch-all)
	// Angular 21 outputs to web/dist/browser/ via @angular/build:application.
	// Any path that doesn't match a real file is served as index.html so that
	// Angular's client-side router handles it (SPA fallback).
	r.Get("/*", spaHandler())

	return r
}

// spaHandler serves the embedded Angular application. If the requested path
// does not correspond to an existing file, index.html is served so Angular's
// client-side router can handle the route.
func spaHandler() http.HandlerFunc {
	// Sub into dist/browser so URLs map directly to file paths.
	subFS, err := fs.Sub(web.StaticFS, "dist/browser")
	if err != nil {
		panic("web: failed to sub into dist/browser: " + err.Error())
	}
	// http.FS normalises the leading "/" that fs.FS.Open does not accept,
	// so use it for both the existence check and the file server.
	httpFS := http.FS(subFS)
	fileServer := http.FileServer(httpFS)
	indexHTML, err := web.StaticFS.ReadFile("dist/browser/index.html")
	if err != nil {
		panic("web: index.html not found in embedded FS: " + err.Error())
	}
	return func(w http.ResponseWriter, r *http.Request) {
		// Check whether the file exists in the embedded FS.
		f, err := httpFS.Open(r.URL.Path)
		if err != nil {
			// File not found — serve index.html for client-side routing.
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(indexHTML)
			return
		}
		_ = f.Close()
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


