# Prism Media Server

A self-hosted media server with MPEG-DASH adaptive bitrate streaming, built with Go and Angular.

## Architecture

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

## MPEG-DASH Pipeline

Source files are transcoded by FFmpeg into multiple quality renditions stored as fMP4 segments:

| Rendition | Resolution | Video Bitrate | Audio Bitrate |
| --------- | ---------- | ------------- | ------------- |
| 360p      | 640×360    | 400 kbps      | 64 kbps       |
| 480p      | 854×480    | 800 kbps      | 96 kbps       |
| 720p      | 1280×720   | 2500 kbps     | 128 kbps      |
| 1080p     | 1920×1080  | 8000 kbps     | 192 kbps      |

The Angular player uses [dash.js](https://github.com/Dash-Industry-Forum/dash.js) to select the appropriate rendition based on available bandwidth.

## Project Structure

```
prism/
├── cmd/server/          # Application entry point
├── internal/
│   ├── api/             # HTTP router + handlers
│   ├── auth/            # JWT authentication
│   ├── config/          # Configuration (env + yaml)
│   ├── jobs/            # Transcode job queue + worker pool
│   ├── metadata/        # TMDB metadata enrichment
│   ├── models/          # Shared domain types
│   ├── scanner/         # Library directory watcher (fsnotify)
│   ├── streaming/       # DASH MPD serving + segment delivery
│   ├── store/postgres/  # Database layer (pgx)
│   └── transcoder/      # FFmpeg DASH transcode orchestration
├── pkg/
│   ├── ffmpeg/          # FFprobe + FFmpeg wrappers
│   └── dash/            # MPD manifest generation
├── migrations/          # SQL migrations (golang-migrate)
├── web/                 # Angular frontend (dash.js player)
├── scripts/             # Dev helper scripts
├── docker-compose.yml
└── Dockerfile
```

## Development Setup

### Prerequisites

- Go 1.24+
- Docker + Docker Compose
- Node.js 20+ / npm (for the frontend)
- FFmpeg (installed inside Docker, or locally with `brew install ffmpeg`)

### Quick Start

```bash
# 1. Copy and configure environment
cp .env.example .env
# Edit .env — set PRISM_JWT_SECRET and MEDIA_DIR

# 2. Start backing services (postgres + redis)
docker compose up postgres redis -d

# 3. Run the server (auto-migrates on startup)
PRISM_JWT_SECRET=dev_secret go run ./cmd/server

# 4. Bootstrap and run the Angular frontend
./scripts/init-angular.sh   # first time only
cd web && ng serve           # proxies API to :8080
```

### Docker (full stack)

```bash
docker compose up --build
```

Server available at http://localhost:8080

### Running Tests

```bash
go test ./...
```

## Configuration

All config values can be set via environment variables (`PRISM_` prefix) or a `config.yaml` file in the working directory.

| Variable                  | Default                  | Description               |
| ------------------------- | ------------------------ | ------------------------- |
| `PRISM_PORT`              | `8080`                   | HTTP listen port          |
| `PRISM_DATABASE_URL`      | `postgres://...`         | PostgreSQL DSN            |
| `PRISM_REDIS_URL`         | `redis://localhost:6379` | Redis URL                 |
| `PRISM_JWT_SECRET`        | _(required)_             | JWT signing secret        |
| `PRISM_MEDIA_DIR`         | `/media`                 | Root media directory      |
| `PRISM_SEGMENTS_DIR`      | `/data/segments`         | DASH segment output       |
| `PRISM_THUMBS_DIR`        | `/data/thumbs`           | Thumbnail cache           |
| `PRISM_FFMPEG_PATH`       | `ffmpeg`                 | FFmpeg binary path        |
| `PRISM_FFPROBE_PATH`      | `ffprobe`                | FFprobe binary path       |
| `PRISM_TRANSCODE_WORKERS` | `2`                      | Concurrent transcode jobs |
| `PRISM_TMDB_API_KEY`      | _(optional)_             | TMDB metadata enrichment  |

## Roadmap

- [x] Project scaffold + DB schema
- [x] Configuration + Docker Compose
- [ ] JWT authentication (login / refresh)
- [ ] Library scanner (fsnotify + FFprobe)
- [ ] TMDB metadata enrichment
- [ ] Transcode worker pool + job queue
- [ ] DASH MPD generation + segment serving
- [ ] Angular media browser UI
- [ ] dash.js player component
- [ ] Watch history + resume position
- [ ] Hardware transcoding (NVENC / QSV / VideoToolbox)
- [ ] Direct play (skip transcode for compatible formats)
- [ ] Multi-user profiles
- [ ] Subtitle support (WebVTT / DASH)
