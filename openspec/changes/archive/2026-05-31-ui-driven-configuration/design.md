## Context

The server currently requires a `config.yaml` file for all configuration. The `internal/config/config.go` package uses Viper to read this file and populate a `Config` struct, which is then passed into every subsystem at startup. Runtime behavior (TMDB enrichment, transcoder concurrency, Chromecast integration) is therefore fixed at startup and cannot be changed without editing a file and restarting.

The first-run experience is a terminal-interactive prompt that creates the initial admin account. This works only if you have terminal access at startup — it's not suitable for a headless or remote install.

This design replaces all of that with a database-backed settings system and a browser-based setup flow.

## Goals / Non-Goals

**Goals:**
- Eliminate `config.yaml` entirely
- Store all runtime settings in the SQLite `settings` table, loaded at startup
- Auto-generate and persist `jwt_secret` on first DB open — no user involvement
- Expose runtime settings (directories, ffmpeg, TMDB, cast, workers) in an admin UI
- Gate a fresh install behind a web setup wizard until `setup_complete = true`
- Keep startup configuration minimal: `--db` and `--port` only
- Remove Viper as a dependency

**Non-Goals:**
- Per-user settings
- Live/hot-reload of settings without restart
- Settings history or audit log
- Runtime validation of ffmpeg binary paths (done at startup when the transcoder pool initializes)

## Decisions

### D1: Key-Value Settings Table

**Decision**: Use a `settings(key TEXT PRIMARY KEY, value TEXT NOT NULL)` table rather than a single typed row.

**Rationale**: The settings surface will grow. A key-value table requires no schema migration to add new settings — just new default values in code. The tradeoff is that all values are strings and must be parsed/typed in Go, but the settings set is small and well-known, making this trivial. A typed singleton row would require a migration for every new setting.

**Alternative considered**: Single `app_settings` row with typed columns. Rejected because it creates migration overhead for every new setting added in future changes.

---

### D2: Auto-Generate `jwt_secret` on First DB Open

**Decision**: During the settings bootstrap (run once after migrations, before the HTTP server starts), check if `jwt_secret` exists in the settings table. If not, generate a cryptographically random 32-byte secret (hex-encoded), persist it, and use it for the lifetime of the installation.

**Rationale**: `jwt_secret` is a security credential, not a user preference. There's no good reason to expose it in the UI — auto-generation is strictly more secure. The secret lives in the DB file, which has the same trust boundary as the rest of the application data.

**Implication**: If the DB file is deleted or the `jwt_secret` row is removed, all existing sessions are invalidated. This is expected and acceptable.

---

### D3: Settings Loaded at Startup, Applied Next Restart

**Decision**: Settings are read from the DB once at startup and wired into components. Changes via the admin UI are persisted to the DB immediately but take effect only after the next server restart.

**Rationale**: The components that consume these settings (transcoder pool, metadata enricher, stream handler) are initialized once at startup and hold their values for the process lifetime. Hot-reloading them mid-run would require significant refactoring with no clear payoff — the server is not a high-availability managed service; a restart is low-cost. The UI communicates this to the admin via a toast/banner.

**Settings requiring restart**: `segments_dir`, `thumbs_dir`, `ffmpeg_path`, `ffprobe_path`, `transcode_workers`. `tmdb_api_key` and `cast_receiver_app_id` also require restart since they're wired at startup.

---

### D4: Setup Wizard Gated by `setup_complete` Setting

**Decision**: A `setup_complete` setting (value `"true"` or `"false"`) acts as the gate. On startup, if `setup_complete != "true"`, a middleware intercepts all requests and returns a redirect to `/setup`. The `/setup` API endpoint and Angular wizard route are always accessible, regardless of setup state.

The wizard requires only: create admin username + password. This is the minimum for the server to be usable. All other settings have sensible defaults and can be changed in the admin panel afterward.

**Rationale**: The minimal wizard reduces friction. A user who just wants to get started doesn't need to configure ffmpeg paths or a TMDB key on first run — those are optional enhancements.

**Alternative considered**: "Setup is complete when at least one admin user exists." Rejected because it's ambiguous — what if the admin is created via a direct DB insert? The explicit flag is unambiguous and easy to inspect.

---

### D5: Remove Viper

**Decision**: Remove the `github.com/spf13/viper` dependency. Replace it with direct `os.Args` / `flag` package parsing for `--db` and `--port`, and direct `os.Getenv` calls for `PRISM_DB` and `PRISM_PORT`.

**Rationale**: Viper was only useful when there were many config sources to merge (file + env + flags). With only two startup parameters, the standard library is sufficient and reduces the dependency footprint.

---

## Risks / Trade-offs

**[Risk] JWT secret lives inside the DB file** → The DB file must be protected (chmod 600 or equivalent). This is the same protection already needed for the user table (password hashes). Not a new exposure surface.

**[Risk] Settings API exposes sensitive values (TMDB key)** → The admin settings endpoint must be behind the `RequireAdmin` middleware. The TMDB key should be write-only in the UI (display as `••••••••` after save, like a password field). Consider whether the GET response should omit secret-like fields.

**[Risk] `setup_complete` redirect loop** → If the Angular router also redirects `/setup` routes, a redirect loop is possible. The middleware must always allow `GET /setup*` and `POST /api/v1/setup*` through.

**[Risk] Existing installs with `config.yaml`** → This is a breaking change. Users upgrading from a config.yaml-based install need a migration path. A migration tool or documented one-time import step is needed. The server could detect `config.yaml` presence at startup and offer to import it into the settings table, then delete it — but this adds scope. Alternatively, document the manual migration clearly in the release notes.

## Migration Plan

1. Deploy new binary against existing DB — migrations run automatically, adding the `settings` table.
2. If `config.yaml` exists: settings are NOT auto-imported in this version. Admin must manually re-enter values in the admin settings UI after first login.
3. `setup_complete` defaults to `"true"` in the migration for existing DBs (since users/libraries already exist). Only fresh DBs start with `setup_complete = "false"`.
4. `jwt_secret` is auto-generated on first startup if not present in settings.

**Rollback**: Not easily reversible — if the binary is rolled back to a version that requires `config.yaml`, the file must be restored. Document this in release notes.

## Open Questions

- Should the GET `/api/v1/admin/settings` response omit the `tmdb_api_key` value (returning empty string or a masked value) to avoid the key being visible in browser devtools? Worth discussing before implementing the handler.
- Should `setup_complete` in existing DBs default to `true` via migration SQL, or should the code detect "existing DB" (e.g., users table is non-empty) and set it at runtime? Migration SQL is cleaner.
