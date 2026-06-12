## 1. Database Migration

- [x] 1.1 Write migration SQL to create `settings(key TEXT PRIMARY KEY, value TEXT NOT NULL)` table
- [x] 1.2 In the migration for existing DBs, insert `setup_complete = 'true'` as a default row (so existing installs aren't sent to the wizard)

## 2. Settings Store Layer

- [x] 2.1 Create `internal/store/sqlite/settings.go` with `Get(key)`, `Set(key, value)`, `GetAll()`, and `Delete(key)` functions
- [x] 2.2 Write bootstrap function that inserts default values for all known keys if they don't exist (segments_dir, thumbs_dir, ffmpeg_path, ffprobe_path, transcode_workers, tmdb_api_key, cast_receiver_app_id, setup_complete)
- [x] 2.3 In bootstrap, auto-generate and persist `jwt_secret` if not present (crypto/rand, 32 bytes, hex-encoded)
- [x] 2.4 Write tests for settings store (Get, Set, GetAll, bootstrap idempotency, jwt_secret generation)

## 3. Config Refactor

- [x] 3.1 Rewrite `internal/config/config.go` — strip `Config` struct to startup-only fields: `DBPath` and `Port`
- [x] 3.2 Replace Viper-based `Load()` with simple `flag` + `os.Getenv` parsing for `--db`/`PRISM_DB` and `--port`/`PRISM_PORT`
- [x] 3.3 Remove `github.com/spf13/viper` from `go.mod` and `go.sum`
- [x] 3.4 Define a `RuntimeSettings` struct (or load-from-DB function) that reads the settings table into typed fields used by subsystems

## 4. main.go Refactor

- [x] 4.1 Update `cmd/server/main.go` to use new config.Load() (flags only)
- [x] 4.2 After DB open and migrations, call settings bootstrap
- [x] 4.3 Load runtime settings from DB and wire them into: metadata enricher, scanner manager, transcoder pool, router
- [x] 4.4 Remove the terminal-interactive first-run admin creation flow (`maybeFirstRun`)
- [x] 4.5 Delete `config.yaml`

## 5. Setup Wizard — Backend

- [x] 5.1 Create `internal/api/handler/setup.go` with `POST /api/v1/setup` handler: validates input, creates admin user, sets `setup_complete = "true"`
- [x] 5.2 Ensure `POST /api/v1/setup` returns 409 if `setup_complete` is already `"true"`
- [x] 5.3 Create `internal/api/middleware/setup_guard.go`: checks `setup_complete`; if false, allows `/setup*` and `/api/v1/setup*` through, redirects everything else
- [x] 5.4 Register setup guard middleware and setup handler in `internal/api/router.go`
- [x] 5.5 Write tests for setup handler (success, already-complete, invalid input)

## 6. Admin Settings — Backend

- [x] 6.1 Create `internal/api/handler/settings.go` with `GET /api/v1/admin/settings` (returns all configurable settings; excludes `jwt_secret` and `setup_complete`)
- [x] 6.2 Implement `PUT /api/v1/admin/settings` — validates keys against allowed list, persists changes
- [x] 6.3 Register settings endpoints in router under `RequireAdmin` middleware
- [x] 6.4 Write tests for settings handler (get, update, unknown key rejection, auth enforcement)

## 7. Setup Wizard — Frontend

- [x] 7.1 Create Angular `setup` feature module with a wizard component (`web/src/app/features/setup/`)
- [x] 7.2 Add `/setup` route to Angular router; add setup guard that redirects to `/setup` when server is in setup mode (detect via API or redirect response)
- [x] 7.3 Build wizard form: username + password fields, submit button
- [x] 7.4 On success, redirect to `/login`
- [x] 7.5 Wire setup API call in `api.service.ts`

## 8. Admin Settings — Frontend

- [x] 8.1 Create Angular `admin-settings` feature component (`web/src/app/features/admin/settings/`)
- [x] 8.2 Build settings form with fields: Segments Dir, Thumbs Dir, FFmpeg Path, FFprobe Path, Transcode Workers, TMDB API Key (write-only display), Cast Receiver App ID
- [x] 8.3 On save, call PUT `/api/v1/admin/settings` and show a toast/banner: "Settings saved. Restart the server to apply changes."
- [x] 8.4 Add admin settings link/route in the admin section of the UI
- [x] 8.5 Wire settings API calls (GET and PUT) in `api.service.ts`
