## Why

Server configuration currently requires a `config.yaml` file on disk, which creates friction for new users and makes the server harder to operate — especially when running as a binary on a bare system. Moving configuration into the database and exposing it through an admin UI makes the server self-contained and approachable from the browser alone.

## What Changes

- **BREAKING**: `config.yaml` is eliminated. The server no longer reads or requires this file.
- The server now accepts only two startup parameters: `--db` (or `PRISM_DB`) for the database path, and `--port` (or `PRISM_PORT`) for the HTTP port. Both have sensible defaults.
- A `settings` key-value table is added to the database. All runtime configuration (directories, ffmpeg paths, API keys, worker counts) is stored there with defaults, and loaded at startup.
- `jwt_secret` is auto-generated on first DB open and persisted to the settings table. It is never user-facing.
- A web setup wizard replaces the terminal-interactive first-run flow. The server detects a fresh install via a `setup_complete` setting and redirects all traffic to `/setup` until initial configuration (admin account creation) is complete.
- An admin settings page in the Angular UI exposes all runtime settings. Changes that require a server restart display a toast/banner informing the admin.
- `media_dir` config field is removed (was unused).

## Capabilities

### New Capabilities

- `server-settings`: Persistent key-value settings store — schema, store layer, and admin API endpoints (`GET /api/v1/admin/settings`, `PUT /api/v1/admin/settings`). Covers all runtime settings including directories, ffmpeg paths, TMDB key, cast app ID, and transcode workers.
- `setup-wizard`: First-run web setup wizard gated by the `setup_complete` setting. Covers server-side gating middleware, the setup API endpoint, and the Angular wizard UI.

### Modified Capabilities

<!-- No existing specs — no modifications. -->

## Impact

- `internal/config/config.go`: Rewritten. `Config` struct is trimmed to startup-only fields (`DBPath`, `Port`). Runtime settings are loaded from the DB after open.
- `internal/store/sqlite/`: New `settings.go` store with `Get`, `Set`, `GetAll` functions and bootstrap logic (defaults + jwt_secret generation).
- `migrations/`: New migration adding the `settings` table.
- `cmd/server/main.go`: Simplified — removes Viper, removes terminal first-run flow, adds settings bootstrap on DB open.
- `internal/api/router.go` and handlers: Components that currently receive config fields (`JWTSecret`, `ThumbsDir`, `SegmentsDir`, etc.) will receive them from the DB-loaded settings at startup.
- `internal/api/handler/`: New `settings.go` handler for admin settings API. New `setup.go` handler for setup wizard API.
- `internal/api/middleware/`: New `setup_guard.go` middleware to redirect to `/setup` when `setup_complete` is false.
- `web/src/app/`: New setup wizard feature. New admin settings feature.
- `config.yaml`: Deleted.
- Dependencies: Viper (`github.com/spf13/viper`) can be removed from `go.mod`.
