# Agent Developer Guide — Prism

Welcome! This guide is designed to help AI agents and human developers quickly get up to speed with the architecture, code structure, development workflow, and conventions of the Prism Media Server project.

## 1. High-Level Architecture

Prism is a self-hosted media server with MPEG-DASH adaptive bitrate streaming. It features:

- **Frontend**: A standalone Angular SPA (located in [web](file:///home/benwelker/repos/galactic-media-server/web/)) featuring a dark-themed glassmorphism interface and a player built on `dash.js`.
- **Backend**: A Go-based REST & WebSocket API server (located in [cmd/server](file:///home/benwelker/repos/galactic-media-server/cmd/server/)) using the `go-chi/chi/v5` router.
- **Database**: Pure Go SQLite (`modernc.org/sqlite`) running in WAL mode with auto-migrations.
- **Transcoding**: A DB-backed worker pool running FFmpeg to transcode source files into multiple quality renditions (360p, 480p, 720p, 1080p) and generate MPEG-DASH manifests.

---

## 2. Project Structure

Here is a breakdown of the repository directories and their responsibilities:

### Backend Architecture (`internal/` & `pkg/`)

- **[cmd/server/main.go](file:///home/benwelker/repos/galactic-media-server/cmd/server/main.go)**: Entry point for the Go server. Bootstraps the SQLite connection, runs migrations, initializes internal managers/workers, and starts the HTTP/WebSocket server.
- **[internal/api](file:///home/benwelker/repos/galactic-media-server/internal/api/)**: HTTP routing and handlers.
  - [router.go](file:///home/benwelker/repos/galactic-media-server/internal/api/router.go): Defines all route patterns and applies middlewares.
  - [handler/](file:///home/benwelker/repos/galactic-media-server/internal/api/handler/): API endpoint handlers (e.g. settings, setup, media list, play history).
  - [middleware/](file:///home/benwelker/repos/galactic-media-server/internal/api/middleware/): Custom middlewares (auth guards, setup wizard gate).
- **[internal/auth](file:///home/benwelker/repos/galactic-media-server/internal/auth/)**: Authentication layer. Manages password hashing using bcrypt, JWT token signing/verification (access and refresh tokens), and Cast authorization tokens.
- **[internal/config](file:///home/benwelker/repos/galactic-media-server/internal/config/)**: Configuration loader supporting both environment variables (with `PRISM_` prefix) and database-backed dynamic runtime settings.
- **[internal/metadata](file:///home/benwelker/repos/galactic-media-server/internal/metadata/)**: Handles TMDB API integration, parsing filenames to extract show/movie titles and years, and caching posters locally.
- **[internal/models](file:///home/benwelker/repos/galactic-media-server/internal/models/models.go)**: Central definitions of database entities and REST/JSON API contracts.
- **[internal/scanner](file:///home/benwelker/repos/galactic-media-server/internal/scanner/)**: File scanner that indexes media directories, watches for changes using `fsnotify`, handles TV show hierarchy grouping, performs media deduplication/fingerprinting, and does artifact recovery.
- **[internal/store/sqlite](file:///home/benwelker/repos/galactic-media-server/internal/store/sqlite/)**: SQLite data store logic.
  - [db.go](file:///home/benwelker/repos/galactic-media-server/internal/store/sqlite/db.go): Configures SQLite pragmas (WAL mode, foreign keys, synchronous write configuration) and handles Goose migrations.
  - [users.go](file:///home/benwelker/repos/galactic-media-server/internal/store/sqlite/users.go): Database queries for user management.
  - [settings.go](file:///home/benwelker/repos/galactic-media-server/internal/store/sqlite/settings.go): DB-backed key-value settings.
- **[internal/transcoder](file:///home/benwelker/repos/galactic-media-server/internal/transcoder/pool.go)**: Transcoding job worker pool. Executes FFmpeg processes in the background, parses progress output, writes output DASH files/manifests, and generates recovery `artifact.json` sidecar files.
- **[pkg/dash](file:///home/benwelker/repos/galactic-media-server/pkg/dash/dash.go)**: MPD master manifest generation helper.
- **[pkg/events](file:///home/benwelker/repos/galactic-media-server/pkg/events/bus.go)**: Central pub-sub message bus for fanning out system events (e.g. transcode progress, library updates) to WebSocket endpoints.
- **[pkg/ffmpeg](file:///home/benwelker/repos/galactic-media-server/pkg/ffmpeg/ffmpeg.go)**: Wrappers around standard FFmpeg and FFprobe binaries.
- **[pkg/fingerprint](file:///home/benwelker/repos/galactic-media-server/pkg/fingerprint/fingerprint.go)**: Hash generator for identifying/deduplicating media files by header contents.
- **[migrations/](file:///home/benwelker/repos/galactic-media-server/migrations/)**: SQL migration files run sequentially by Goose.

### Frontend Architecture (`web/`)

The Angular application resides in the [web/](file:///home/benwelker/repos/galactic-media-server/web/) subdirectory:

- **[web/src/app/app.routes.ts](file:///home/benwelker/repos/galactic-media-server/web/src/app/app.routes.ts)**: Application routing table (home, movies, tv-shows, login, setup).
- **[web/src/app/auth.service.ts](file:///home/benwelker/repos/galactic-media-server/web/src/app/auth.service.ts)**: Handles authentication states, login/logout, token storage, and refresh cycles.
- **[web/src/app/auth.interceptor.ts](file:///home/benwelker/repos/galactic-media-server/web/src/app/auth.interceptor.ts)**: HTTP interceptor attaching JWT headers to all outward requests.
- **[web/src/app/home/](file:///home/benwelker/repos/galactic-media-server/web/src/app/home/)**: Landing page showing recently watched media, resume bars, and server system stats.
- **[web/src/app/movies/](file:///home/benwelker/repos/galactic-media-server/web/src/app/movies/)**: Movie catalog and details browser page.
- **[web/src/app/tv-shows/](file:///home/benwelker/repos/galactic-media-server/web/src/app/tv-shows/)**: TV Show episode browser and season navigator.

---

## 3. Development Workflow & Commands

A [Makefile](file:///home/benwelker/repos/galactic-media-server/Makefile) is provided to automate build and development tasks.

### Running the Server Locally

To start the server in development mode:

```bash
make dev
```

This target performs the following actions:

1. Runs `ng build --configuration development` to compile the Angular frontend into the `web/dist/` directory.
2. Starts the Go API backend server via `go run ./cmd/server`. The Go server automatically hosts the static files in `web/dist/` and runs any pending SQLite database migrations.

After making code changes, remember to run `make dev` to rebuild and restart the server.

### Default Credentials

Prism is seeded with default administrator credentials:

- **Username**: `admin`
- **Password**: `asdf`

You can use these credentials to log in on the login page (`/login`) once the server is running.

### Other Useful Commands

- **Build everything (Production)**:
  ```bash
  make build
  ```
- **Run Go backend tests**:
  ```bash
  make test
  ```
- **Run backend linter**:
  ```bash
  make lint
  ```
- **Clear build artifacts and reset database/transcodes**:
  ```bash
  make clean
  make reset
  ```

---

## 4. API Documentation & Swagger Annotations

Prism dynamically generates its OpenAPI/Swagger specification from annotations directly in the Go handler source code.

### Tooling

We use [swaggo/swag](https://github.com/swaggo/swag) to parse code comments and generate spec files.

* **Generate Spec**:
  ```bash
  make swagger
  ```
  This command executes:
  ```bash
  go run github.com/swaggo/swag/cmd/swag init -g cmd/server/main.go -o internal/api/handler/docs
  ```
  *Note*: Spec files are generated inside `internal/api/handler/docs/` (`swagger.yaml`, `swagger.json`, and `docs.go`). These files are embedded at compile-time and served via the `/docs` UI page and `/api/v1/swagger.yaml` endpoints.

### Annotation Rules & Requirements

When creating new endpoints or modifying existing ones, you **must** document them using the following rules:

1. **Global Configuration**: Global details (title, version, base path) and security schemas are configured in `cmd/server/main.go`.
2. **Security Definitions**:
   * Use `@Security BearerAuth` for admin/user endpoints requiring a JWT access token in the `Authorization` header.
   * Use `@Security WorkerAuth` for transcode worker endpoints requiring the `X-Worker-API-Key` header.
3. **Handler Comment Format**: Every handler function must be preceded by a comment block containing:
   ```go
   // @Summary [Short Title]
   // @Description [Detailed description of the endpoint behavior]
   // @Tags [Group name, e.g. User Profile, Media Streaming, Worker Interface]
   // @Security [BearerAuth or WorkerAuth (optional)]
   // @Accept json (optional)
   // @Produce json
   // @Param [name] [paramType] [dataType] [required] "[description]" [attributes]
   // @Success [code] {[schemaType]} [dataType] "[description]"
   // @Failure [code] {[schemaType]} [dataType] "[description]"
   // @Router /[path] [method]
   ```
4. **Model Schemas**:
   * Always reference precise model definitions from `internal/models/models.go` (e.g. `{object} models.User`, `{array} models.TranscodeJob`) so fields and types are accurately documented.
   * Avoid defining request/response structures inline inside handler functions, as the generator cannot parse function-scoped structs. Instead, define them at the package level.
5. **Path & Query Parameters**: Ensure path variables match Chi router patterns exactly (e.g. `/tv/shows/{id}` maps to `@Param id path string true "Show ID" format(uuid)`).
