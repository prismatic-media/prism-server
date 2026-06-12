# Prism Media Server — Project Plan

## Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                    Angular Frontend                      │
│  Media Browser │ dash.js Player │ Admin Panel           │
└───────────────────────┬─────────────────────────────────┘
                        │ HTTP / WebSocket
┌───────────────────────▼─────────────────────────────────┐
│                    Go API Server (chi)                   │
│  REST API  │  JWT Auth Middleware  │  WebSocket          │
└─────┬──────────┬──────────┬──────────────┬──────────────┘
      │          │          │              │
┌─────▼──┐  ┌───▼────┐  ┌──▼───────┐  ┌──▼──────────┐
│Library │  │Metadata│  │Transcoder│  │  DASH        │
│Scanner │  │Service │  │  Queue   │  │  Streamer    │
│fsnotify│  │  TMDB  │  │  FFmpeg  │  │ MPD+Segments │
└────────┘  └───┬────┘  └──┬───────┘  └──┬──────────┘
                │           │              │
         ┌──────▼───────────▼──────────────▼──────┐
         │           PostgreSQL  +  Redis           │
         └────────────────────────────────────────┘
```

## Technology Decisions

| Concern | Choice | Rationale |
|---|---|---|
| HTTP framework | `chi` | Lightweight, stdlib-compatible, composable middleware |
| Database | `modernc.org/sqlite` (pure Go, no CGO) | Embedded, zero external deps, WAL mode for concurrent reads |
| Migrations | `pressly/goose/v3` + `embed.FS` | Migration SQL compiled into the binary; no migration files on disk |
| Auth | JWT + refresh tokens | Stateless; refresh token revocation stored in SQLite `refresh_tokens` table |
| Transcoder | FFmpeg (exec wrapper) | Industry standard, native DASH support via `libavformat` |
| DASH segments | fMP4 (fragmented MP4) | Broad browser compatibility, seekable without full download |
| Job queue | In-process with SQLite persistence | No external dependencies |
| Frontend player | `dash.js` | Reference MPEG-DASH implementation, actively maintained |
| MPD cache | In-memory `sync.Map` | MPDs are small XML; Redis not needed |
| Config | Viper | Supports env vars, YAML file, and defaults |

> **No external services required.** The server binary + FFmpeg is the entire stack.
> SQLite data and DASH segments are stored on the local filesystem.

## MPEG-DASH Pipeline

```
Source File
    │
    ▼
FFprobe ──► Extract codec / resolution / duration metadata
    │
    ▼
Transcode Job ──► Multiple renditions via FFmpeg
    │               360p  @  400 kbps video +  64 kbps audio
    │               480p  @  800 kbps video +  96 kbps audio
    │               720p  @ 2500 kbps video + 128 kbps audio
    │               1080p @ 8000 kbps video + 192 kbps audio
    │
    ▼
Segment Output ──► fMP4 chunks (4-second segments)
    │               /data/segments/{mediaID}/{rendition}/init.mp4
    │               /data/segments/{mediaID}/{rendition}/seg_00001.m4s
    │               ...
    ▼
MPD Manifest ──► Generated and cached
                  /api/v1/stream/{mediaID}/manifest.mpd
```

The Angular player uses `dash.js` to fetch the MPD and automatically select the best rendition based on available bandwidth.

---

## Phased Build Order

### Phase 1 — Authentication  ✅ Complete
**Files:** `internal/auth/`, `internal/api/handler/auth.go`

- [x] `POST /api/v1/auth/login` — bcrypt password check, issue access + refresh JWT
- [x] `POST /api/v1/auth/refresh` — validate refresh token hash against `refresh_tokens` table, issue new access token
- [x] `POST /api/v1/auth/logout` — mark refresh token as `revoked = 1` in SQLite
- [x] Auth middleware in `internal/api/middleware/auth.go` — validate `Authorization: Bearer <token>`
- [x] First-run: if no users exist, allow unauthenticated `POST /users` to create admin
- [x] `GET /api/v1/me`, `PUT /api/v1/me`
- [x] Unit + integration test suite (53 tests passing)

**Key decisions:**
- Access token TTL: 15 minutes
- Refresh token TTL: 30 days; stored as SHA-256 hash in the `refresh_tokens` SQLite table
- Periodic cleanup: delete expired rows from `refresh_tokens` on server startup
- **No Redis** — token revocation is handled entirely by the `refresh_tokens` table

---

### Phase 2 — Library Management  ✅ Complete
**Files:** `internal/scanner/`, `internal/api/handler/library.go`, `internal/api/handler/media.go`

- [x] `POST /api/v1/libraries` — register a directory path + media type
- [x] `GET /api/v1/libraries`, `GET /api/v1/libraries/{id}`, `DELETE /api/v1/libraries/{id}`
- [x] `POST /api/v1/libraries/{id}/scan` — trigger a manual scan (async, returns 202)
- [x] Scanner: walk the library path with `filepath.WalkDir`, call FFprobe on each video file
- [x] Auto-scan on startup for each registered library
- [x] Background watcher with `fsnotify` to detect new/deleted files
- [x] Store discovered items in `media_items`, prune stale rows after scan
- [x] `GET /api/v1/media`, `GET /api/v1/media/{id}`, `DELETE /api/v1/media/{id}`
- [x] Unit + integration test suite (68 tests passing total)

---

### Phase 3 — Metadata Enrichment  ✅ Complete
**Files:** `internal/metadata/`

- [x] TMDB client (`GET /search/movie`, `/search/tv`) keyed by `PRISM_TMDB_API_KEY`
- [x] Match scanned file titles (parse year + title from filename via regex)
- [x] Fetch and store: overview, poster URL, year, TMDB ID
- [x] Download and cache poster images to `PRISM_THUMBS_DIR`
- [x] Serve thumbnails via `GET /api/v1/media/{id}/poster`
- [x] Run enrichment as a post-scan step (non-blocking, best-effort)
- [x] Unit + integration test suite (74 tests passing total)

---

### Phase 4 — Transcode Engine  ✅ Complete
**Files:** `internal/transcoder/`, `internal/jobs/`, `pkg/ffmpeg/`, `pkg/dash/`

- [x] Job worker pool: configurable concurrency via `PRISM_TRANSCODE_WORKERS`
- [x] Pick up `pending` jobs from DB on startup; process one per worker goroutine
- [x] `POST /api/v1/media/{id}/transcode` — manually re-enqueue a transcode job
- [x] `GET /api/v1/jobs`, `GET /api/v1/jobs/{id}`
- [x] FFmpeg DASH transcode: produce all renditions in one invocation (see `pkg/ffmpeg/ffmpeg.go`)
- [x] Subtitle extraction: extract embedded subtitles to WebVTT for DASH text tracks
- [x] Real-time progress via `GET /api/v1/ws/jobs/{id}` WebSocket (parse FFmpeg stderr)
- [x] On completion, write `mpd_path` to `media_items` and set `transcode_status = done`
- [x] On failure, set `transcode_status = failed`, store error message in `transcode_jobs`

**MPD generation** (`pkg/dash/`):
- [x] Generate a master MPD XML referencing all renditions and text tracks
- [x] Cache generated MPDs in a `sync.Map` (keyed by media item ID); invalidate on re-transcode
- [x] Unit + integration test suite (108 tests passing total)

---

### Phase 5 — Streaming  ✅ Complete
**Files:** `internal/api/handler/stream.go`, `internal/api/handler/history.go`, `internal/store/sqlite/history.go`

- [x] `GET /api/v1/stream/{id}/manifest.mpd` — serve MPD (from DB path or in-process cache)
- [x] `GET /api/v1/stream/{id}/segments/*` — serve fMP4 segment files with correct headers
  - Set `Content-Type: video/iso.segment` for `.m4s`, `video/mp4` for init segments
  - Support `Range` requests for seek support via `http.ServeFile`
  - Set `Cache-Control: max-age=31536000, immutable` on segment files (they never change)
  - Path traversal guard (reject `..` in wildcard segment path)
- [x] Resume position: `PUT /api/v1/history/{mediaID}` saves position every 10s from the player
- [x] `GET /api/v1/history` returns in-progress (not completed) items for the "Continue Watching" row
- [x] `UpsertWatchHistory` uses `ON CONFLICT DO UPDATE` for idempotent position saves
- [x] Unit + integration test suite (135 tests passing total)

---

### Phase 6 — Angular Frontend ✅ Complete
**Files:** `web/` (Angular 21 standalone components, SCSS, dash.js)

- [x] `ApiService` — typed HTTP client wrapping all REST endpoints
- [x] `AuthService` + login page + auth guard (admin guard via JWT payload `adm` claim)
- [x] `LibraryBrowserComponent` — grid of media items with poster art, library filter, search
- [x] `MediaDetailComponent` — metadata, transcode status, "Play" / "Resume" button
- [x] `PlayerComponent` — dash.js integration
  - Initialize `MediaPlayer` with the MPD URL
  - Quality selector (manual or auto ABR via `updateSettings`)
  - Subtitle track selector
  - Report position back to `PUT /history/{mediaID}` every 10 seconds
- [x] `AdminComponent` — library management, job queue monitor
- [x] `HistoryComponent` — Continue Watching list with progress bars
- [x] `ng build --configuration production` output to `web/dist/browser/`, served by Go catch-all
  - Go router updated to serve from `web/dist/browser/`

---

### Phase 7 — Polish & Advanced Features
- [ ] Hardware transcoding: NVENC (`h264_nvenc`), QSV (`h264_qsv`), VideoToolbox (`h264_videotoolbox`)
- [ ] Direct play: detect when the client can play the source codec natively, skip transcode
- [ ] Direct stream: remux to fMP4 on-the-fly without re-encoding (lower CPU, instant start)
- [ ] Chapter markers in MPD from embedded FFmpeg chapter metadata
- [ ] Multi-user profiles with isolated watch history
- [ ] Collections / playlists
- [ ] Mobile-responsive UI / PWA

---

## API Surface (v1)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/v1/auth/login` | No | Issue JWT |
| POST | `/api/v1/auth/refresh` | No | Refresh access token |
| POST | `/api/v1/auth/logout` | Yes | Revoke refresh token |
| POST | `/api/v1/users` | Admin | Create user |
| GET | `/api/v1/me` | Yes | Current user |
| PUT | `/api/v1/me` | Yes | Update current user |
| GET | `/api/v1/libraries` | Yes | List libraries |
| POST | `/api/v1/libraries` | Admin | Add library |
| GET | `/api/v1/libraries/{id}` | Yes | Get library |
| DELETE | `/api/v1/libraries/{id}` | Admin | Remove library |
| POST | `/api/v1/libraries/{id}/scan` | Admin | Trigger scan |
| GET | `/api/v1/media` | Yes | List media items |
| GET | `/api/v1/media/{id}` | Yes | Get media item |
| DELETE | `/api/v1/media/{id}` | Admin | Delete media item |
| GET | `/api/v1/media/{id}/poster` | Yes | Serve poster image |
| POST | `/api/v1/media/{id}/transcode` | Admin | Re-queue transcode |
| GET | `/api/v1/stream/{id}/manifest.mpd` | Yes | DASH MPD manifest |
| GET | `/api/v1/stream/{id}/segments/*` | Yes | DASH segment files |
| GET | `/api/v1/jobs` | Admin | List transcode jobs |
| GET | `/api/v1/jobs/{id}` | Admin | Get job detail |
| GET | `/api/v1/history` | Yes | Watch history |
| PUT | `/api/v1/history/{mediaID}` | Yes | Update watch position |
| GET | `/api/v1/ws/jobs/{id}` | Admin | WebSocket job progress |

---

## Directory Structure

```
prism/
├── cmd/server/              # main.go — entry point
├── internal/
│   ├── api/
│   │   ├── router.go        # chi router, all routes registered here
│   │   ├── handler/         # one file per resource (auth, library, media, stream, jobs)
│   │   └── middleware/      # auth.go, cors.go
│   ├── auth/                # JWT issue/validate, bcrypt helpers
│   ├── config/              # viper config struct
│   ├── jobs/                # worker pool, job dispatcher
│   ├── metadata/            # TMDB client, title parser
│   ├── models/              # shared domain types
│   ├── scanner/             # library walker + fsnotify watcher
│   ├── streaming/           # MPD serving, segment handler
│   ├── store/
│   │   └── postgres/        # pgxpool connection, query functions
│   └── transcoder/          # FFmpeg job orchestration
├── pkg/
│   ├── ffmpeg/              # ffprobe wrapper, transcode builder
│   └── dash/                # MPD XML generation
├── migrations/              # 000001_initial_schema.{up,down}.sql
├── web/                     # Angular app (ng new, then dash.js)
├── scripts/
│   └── init-angular.sh      # bootstrap Angular + install dash.js
├── Dockerfile
├── docker-compose.yml
├── .env.example
└── README.md
```

---

## Development Workflow

```bash
# Run the server — no external services needed
PRISM_JWT_SECRET=dev go run ./cmd/server
# SQLite DB is created automatically at prism.db
# Migrations run automatically on startup

# Run with live reload (install air: go install github.com/air-verse/air@latest)
PRISM_JWT_SECRET=dev air ./cmd/server

# Run Angular dev server (proxies /api to :8080)
cd web && ng serve --proxy-config proxy.conf.json

# Run tests
go test ./...

# Build a fully self-contained production binary
# (embeds all migrations; only runtime dep is ffmpeg)
go build -o prism ./cmd/server

# Docker (single container, no compose deps)
docker compose up --build
```
