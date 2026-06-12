# Prism Media Server — Feature Inventory

Prism is a self-hosted media server designed to deliver MPEG-DASH adaptive bitrate streaming. Below is a complete, detailed inventory of every system capability, technical mechanism, and user feature supported by the application.

---

## 1. Core Architecture & System Foundation

*   **Integrated Multi-tier Stack**: Built with a high-performance **Go backend** (using the lightweight `go-chi/chi` router) and a responsive, standalone **Angular frontend** featuring a modern, dark-themed glassmorphism design.
*   **Zero External Database Dependencies**: Uses pure Go SQLite (`modernc.org/sqlite`) in WAL (Write-Ahead Logging) mode, enabling concurrent reads and transaction safety without needing CGO or a separate PostgreSQL/MySQL instance.
*   **Embedded Migrations**: Embeds database migrations inside the Go binary using `pressly/goose/v3` and `embed.FS`. The database schema upgrades automatically on server startup.
*   **Real-time Event Broadcast Bus**: Features an in-memory broadcast event bus (`pkg/events`) that fans out system events (e.g., job progress, new media discovery, metadata updates) to WebSocket clients over `GET /api/v1/ws/events`.
*   **SPA Fallback Route Handler**: The Go server sub-routes and embeds the Angular compiled frontend directly, falling back to serving `index.html` for client-side routing on arbitrary browser paths.

---

## 2. User Authentication & Security

*   **Dual-Token JWT Architecture**:
    *   **Access Tokens**: Short-lived (15 minutes) state-carrying tokens validated at the middleware layer.
    *   **Refresh Tokens**: Long-lived (30 days) tokens used to request new access tokens via `POST /api/v1/auth/refresh`.
*   **Stateless with Revocation Support**: Instead of Redis, refresh token hashes (SHA-256) are stored in the SQLite `refresh_tokens` table. Logging out (`POST /api/v1/auth/logout`) revokes the token hash in the database.
*   **Opaque Token Query Parameters**: WebSockets and Chromecast connections accept access tokens via `?token=` or `?cast_token=` query parameters, as native browser APIs for WebSockets and HTML5 media elements cannot append custom authorization headers.
*   **RBAC (Role-Based Access Control)**: Restricts endpoints (e.g., settings edits, library modification, job triggers) to administrators via the `RequireAdmin` middleware.
*   **Automatic Token Cleanup**: Automatically deletes expired rows from `refresh_tokens` on server startup.

---

## 3. First-Run Setup Wizard (`/setup`)

*   **API/Browser Setup Guard**: Intercepts requests when configuration is incomplete. Browser requests redirect to `/setup`; API requests receive a `503 Service Unavailable` with setup redirect instructions.
*   **Unauthenticated Boostrapping**: Allows creation of the primary admin user and core directories without prior authentication. Once complete, setup is marked `true` in settings, locking down the endpoints.
*   **Initial Directory Verification**: Configures the initial metadata thumbnails directory (`thumbs_dir`) and the first segment transcode storage area (`segments_dir`).
*   **Optional TMDB & Cast Onboarding**: Allows entering a TMDB API Key and a Chromecast App ID during initial setup.
*   **Directory Browser Autocomplete**: Implements `GET /api/v1/fs/browse` to read local server paths (excluding hidden directories starting with `.`), supporting real-time autocomplete inputs on the frontend.

---

## 4. Library Management

*   **Multi-Format Video Scanner**: Supports discovery of `.mp4`, `.mkv`, `.avi`, `.mov`, `.wmv`, `.flv`, `.webm`, `.m4v`, `.ts`, `.m2ts`, `.mpeg`, and `.mpg` files.
*   **Real-time Directory Watching**: Uses `fsnotify` to track added, modified, renamed, or deleted media files in real-time, automatically updating the database.
*   **TV Show Hierarchy Generation**:
    *   Detects TV episodes via filename parsing (`[show_name] - s[SS]e[EE] - [episode_name]`).
    *   Automatically creates parent `tv_shows` and `tv_seasons` rows in the database, nesting the episode appropriately.
*   **Deduplication & File-Move Detection**:
    *   Generates a deterministic fingerprint of the first 64 KB of video files (sufficient to capture file container headers).
    *   If a file is moved or renamed, the scanner matches its fingerprint, updates the path in place, and preserves its watch history and TMDB metadata.
*   **Automatic Transcode Bundle Linking**: If a newly discovered file matches the fingerprint of an existing transcode bundle in a configured storage area, the scanner links them automatically. It sets `transcode_status = done`, updates `mpd_path`, and avoids re-transcoding.
*   **Manual Scan Trigger**: Admins can force an asynchronous library walk via `POST /api/v1/libraries/{id}/scan`.
*   **Database Pruning**: Prunes media records from the database when files are deleted from the disk (or marks source status as `missing` if transcode bundles remain).

---

## 5. Metadata Enrichment (TMDB Integration)

*   **TMDB API Integration**: Connects to The Movie Database API to fetch detailed information.
*   **Title and Year Regex Extraction**: Parses movie titles and release years from filenames (supporting `Title (Year)` and `Title.Year` formats).
*   **Best-Effort Asynchronous Enrichment**: Searches and fetches TMDB ID, release year, overview text, and poster path post-scan.
*   **Local Image Caching**: Downloads poster and TV episode still images to the server's local `thumbs_dir` directory.
*   **Poster Image Serving**: Exposes endpoints to serve cached posters for movies, shows, and seasons (`/api/v1/media/{id}/poster`, `/api/v1/tv/shows/{id}/poster`, etc.).
*   **Full Metadata Refresh**: Admins can trigger `POST /api/v1/admin/metadata/refresh` to clear cached metadata/posters and execute a complete re-fetch from TMDB in the background.

---

## 6. Transcoding Pipeline

*   **DB-Backed Worker Pool**: Manages an in-process transcode queue. If the server crashes or restarts, `pending` or `processing` jobs are recovered.
*   **Upscaling Prevention**: Probes source media dimensions using `ffprobe` and skips transcode profiles with resolutions higher than the source file.
*   **Single-Pass Multi-Rendition Output**: Invokes FFmpeg to encode multiple resolution renditions concurrently, outputting fMP4 segments:
    *   **360p** (400 kbps video + 64 kbps audio)
    *   **480p** (800 kbps video + 96 kbps audio)
    *   **720p** (2500 kbps video + 128 kbps audio)
    *   **1080p** (8000 kbps video + 192 kbps audio)
*   **Double-Track Subtitle Extraction**:
    *   **Internal Subtitles**: Detects and extracts subtitle streams embedded in the video container to WebVTT files.
    *   **Sidecar Subtitles**: Searches for external `.srt` or `.vtt` sidecars next to the source video and copies them to the transcode output directory.
*   **Master Manifest Generator (`pkg/dash`)**: Writes a master `manifest.mpd` linking all renditions, audio streams, and subtitle tracks.
*   **Live Transcode Telemetry**: Parses FFmpeg's stderr output in real-time, computing percentages and broadcasting them via WebSockets (`/api/v1/ws/jobs/{id}`) to update progress bars in the admin panel.
*   **Queue Control & Prioritization**:
    *   Allows manually enqueuing individual transcode jobs.
    *   Allows prioritizing jobs (`POST /api/v1/jobs/{id}/prioritize`) to process them next.
    *   Allows bulk-enqueuing all "untranscoded" or "failed" media items.
*   **Metadata Sidecars**: Writes an `artifact.json` metadata file to the output directory containing source paths, fingerprints, output directories, and rendition profiles for disaster recovery.

---

## 7. DASH Streaming & Segment Delivery

*   **Manifest & Segment Serving**: Delivers `.mpd` files with `application/dash+xml` and `.m4s` segments with `video/iso.segment` MIME headers.
*   **Range Request Support**: Native support for HTTP Range requests on segment files, facilitating browser seeks and playback optimizations.
*   **Path Traversal Shield**: Prevents directory traversal attacks by validating segment file requests and rejecting paths containing `..`.
*   **Aggressive Browser Caching**: Serves `.m4s` segment files with `Cache-Control: max-age=31536000, immutable` headers since segments never change, and init segments with short-lived cache headers.

---

## 8. Playback Experience (`dash.js` Player)

*   **Quality & Bitrate Selector**: Supports both automatic adaptive bitrate streaming (ABR) and manual resolution quality locks.
*   **Interactive Subtitle Track Selector**: Automatically detects and lists WebVTT subtitle tracks, parsing ISO-639 language codes into friendly English names (e.g., `fra -> French`).
*   **Playback Resume**: Fetches watch history for the active media item and automatically resumes playback from the last recorded position.
*   **Secure Transport Credentials**: intercepting requests from `dash.js` to attach the logged-in user's Bearer JWT to segment and manifest downloads.

---

## 9. Google Cast / Chromecast Integration

*   **Embedded Chromecast Receiver**: Serves an unauthenticated CAF (Cast Application Framework) custom receiver application at `/cast-receiver`.
*   **Dynamic App ID Routing**: The Angular frontend fetches the App ID dynamically from `GET /api/v1/cast/config` rather than hardcoding it.
*   **Media-Scoped Auth Tokens**: Issues short-lived, media-specific tokens (`/api/v1/stream/{id}/cast-token`) to authorize Chromecast playback.
*   **CAF Token Interception**: The custom receiver page intercepts segment downloads and appends the `cast_token` query parameter, ensuring secure playback on TV screens.
*   **Seamless Playback Takeover**: Pauses local video playback, initializes a remote Cast Session, and displays a remote player overlay with seek, rewind 10s, fast forward 30s, and play/pause controls.

---

## 10. Watch History & Resuming Playback

*   **Idempotent Progress Syncing**: The player reports playback progress to `PUT /api/v1/history/{mediaID}` every 10 seconds.
*   **Auto-Mark Completion**: Automatically flags a watch history entry as completed if the user stops watching within 5 seconds of the video's end.
*   **Continue Watching Row**: Displays a list of in-progress media items with visual progress bars.
*   **Now Playing Bar**: Displays the user's most recently updated in-progress media item, complete with metadata and quick-resume shortcuts.

---

## 11. Storage Management & Headroom Checks

*   **Multi-path Storage Allocation**: Supports spreading DASH transcode bundles across multiple designated paths.
*   **Disk Telemetry System**: Monitors storage area capacity and utilization metrics (total space, used space, free space) via system `statfs` calls.
*   **Reserve Headroom Safeguards**: Verifies write permissions and checks if available space is below `storage_min_free_bytes` to prevent disk overflow.
*   **Interactive Storage Toggles**: Admins can enable or disable individual storage paths in the UI.

---

## 12. Disaster Recovery & Artifact Indexing

*   **Artifact Scanner**: Walks storage paths to parse `artifact.json` files and catalog transcode outputs.
*   **Fingerprint Re-association**: Reconnects indexed transcode bundles to library files via fingerprint matching, restoring streaming capability if the database is reset.
*   **Health Diagnostics**: Classifies transcode bundle health statuses (`healthy`, `stale`, `missing`, etc.) and alerts admins to orphaned or broken folders.
*   **Bulk Sidecar Re-generation**: Re-writes missing `artifact.json` files for transcoded media items via `POST /api/v1/admin/artifacts/write-sidecars`.

---

## 13. Telemetry & Health Monitoring

*   **Real-time Dashboard**: Displays mock-animated metrics for CPU, RAM, Network bandwidth, and active stream counts to keep the UI interactive.
*   **Storage Warning Indicators**: Visual warnings for storage areas approaching limits (yellow above 75%, red above 90%).

---

## 14. System Configuration & Settings Keys

The following keys can be set in the database `settings` table or via the UI:

| Key | Default Value | Description |
|---|---|---|
| `thumbs_dir` | `""` | Directory on disk where poster art and stills are cached. |
| `ffmpeg_path` | `"ffmpeg"` | Path to the ffmpeg executable. |
| `ffprobe_path` | `"ffprobe"` | Path to the ffprobe executable. |
| `ffmpeg_hwaccel` | `"none"` | Hardware acceleration type (`none`, `nvenc`, `vaapi`, `qsv`, `videotoolbox`). |
| `transcode_workers` | `"2"` | Maximum number of concurrent transcoding worker threads. |
| `transcode_poll_interval`| `"15"` | Wait time (in seconds) before checking for new transcode queue items. |
| `storage_min_free_bytes` | `"21474836480"` | Headroom limit (default 20 GB) before a storage path is deemed full. |
| `auto_transcode_on_discovery` | `"false"` | Automatically queue transcoding jobs when a new media item is scanned. |
| `tmdb_api_key` | `""` | API Key used to authorize metadata lookups from TMDB. |
| `cast_receiver_app_id` | `""` | App ID for a custom Chromecast receiver. If blank, falls back to default. |
| `setup_complete` | `"false"` | Handled internally; signals whether setup wizard has completed. |
| `jwt_secret` | *(auto-generated)* | Handled internally; secure random key used to sign JWT tokens. |
