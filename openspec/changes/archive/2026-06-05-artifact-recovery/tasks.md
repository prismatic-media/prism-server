## 1. Data Model and Metadata Foundations

- [x] 1.1 Add additive persistence for artifact records, artifact health state, and artifact-to-media links.
  - Migration 00006 (`migrations/00006_artifact_records.sql`) adds `artifact_records` and `artifact_media_links` tables with indexes and constraints.
  - Models in `internal/models/models.go` (ArtifactRecord, ArtifactMediaLink, ArtifactHealth, etc.).
  - Store layer in `internal/store/sqlite/artifacts.go` with 20+ functions (upsert, list, count, link, query).
  - **Verified**: Tests pass in `internal/store/sqlite/`.

- [x] 1.2 Define and implement deterministic source fingerprint generation for media items.
  - `pkg/fingerprint/fingerprint.go` with `GenerateDeterministic()` (SHA-256 of first 64KB), `SourcePath()`, `ResolvePath()`.
  - Tests in `pkg/fingerprint/fingerprint_test.go` cover generation, EOF handling for small files.
  - **Verified**: All tests pass, `go build ./...` succeeds.

- [x] 1.3 Write and validate artifact metadata sidecar output on successful transcode completion.
  - `internal/artifact/metadata.go` with `SidecarMetadata` struct, `WriteSidecar()`, `ReadSidecar()`, `ValidateBundle()`.
  - `internal/artifact/metadata_test.go` for sidecar roundtrip and bundle validation.
  - Transcoder modified in `internal/transcoder/pool.go` — `writeArtifactSidecar()` called after MPD generation succeeds.
  - **Verified**: All artifact and transcoder tests pass.

- [x] 1.4 Add migration-safe read/write paths that preserve existing mpd_path behavior during rollout.
  - `ArtifactSchemaReady()` in `internal/store/sqlite/artifacts.go` checks table existence before operations.
  - `writeArtifactSidecar()` in transcoder is purely file-based (no DB dependency).
  - `IndexAll()` in artifact_indexer checks schema readiness before scanning.
  - **Verified**: No regressions in store/sqlite or transcoder tests.

## 2. Artifact Indexing Workflow

- [x] 2.1 Implement storage-area artifact scanner for enabled segment storage paths.
  - `internal/scanner/artifact_indexer.go` with `Indexer` type, `IndexStorageArea()`, `IndexAll()` methods.
  - Walks enabled segment storage areas looking for `.artifact.json` sidecars.
  - **Verified**: Integration tests in `artifact_integration_test.go` pass.

- [x] 2.2 Implement bundle shape validation and artifact registration logic.
  - `ValidateBundle()` in `internal/artifact/metadata.go` checks Version, MediaItemID, SourcePath, SourceFingerprint.
  - `BundleValidation.IsBundleHealthy()` for health assessment.
  - Used by artifact indexer during registration.
  - **Verified**: `artifact/metadata_test.go` tests for validation.

- [x] 2.3 Make indexing idempotent with last-seen updates and stale/missing artifact state transitions.
  - `UpsertArtifactRecord()` with idempotent upsert logic (ON CONFLICT updates `last_seen_at`).
  - Indexer marks stale artifacts (>7 days since last_seen as `health=stale`).
  - Indexer marks missing artifacts (sidecar deleted) as `health=missing`.
  - Indexer re-marks healthy when missing artifacts reappear.
  - **Verified**: `TestIndexer_IdempotentIndexing` and `TestIndexer_DatabaseLossRecovery` integration tests pass.

- [x] 2.4 Add admin-invokable indexing operation and response summaries.
  - `HandleIndex()` in `internal/api/handler/artifacts.go` — POST `/api/v1/admin/artifacts/index`.
  - Returns `IndexResponse` with per-area `IndexSummaryResponse` (registered, updated, removed, errors).
  - Requires admin authentication via `RequireAdmin` middleware.
  - **Verified**: `TestArtifactIndex_AdminAuthorized` passes (200), `TestArtifactIndex_Unauthorized` passes (403).

## 3. Artifact Relinking Workflow

- [x] 3.1 Implement deterministic exact fingerprint relinking between indexed artifacts and media items.
  - `RelinkExact()` in `internal/scanner/artifact_relink.go` compares artifact `source_fingerprint` against media item file paths.
  - Generates fingerprints from media file content and matches against artifact fingerprints.
  - Creates `ArtifactMediaLink` with `matched_via=fingerprint` for exact matches.
  - **Verified**: `TestRelink_RelinkExact` integration test passes.

- [x] 3.2 Implement unmatched and ambiguous classification with explicit persisted status.
  - Artifacts with no matching media item → `status=unmatched`.
  - Artifacts matching multiple media items → `status=ambiguous` (first match linked).
  - Artifacts without fingerprints → `status=invalid`.
  - Persisted in `artifact_media_links.status` column.
  - **Verified**: `TestRelink_RelinkUnmatched` and `TestRelink_RelinkInvalid` integration tests pass.

- [x] 3.3 Add admin-invokable relink operation with auditable linked/unmatched/ambiguous/invalid counts.
  - `HandleRelink()` in `internal/api/handler/artifacts.go` — POST `/api/v1/admin/artifacts/relink`.
  - Returns `RelinkResponse` with linked, unmatched, ambiguous, invalid, skipped counts.
  - Requires admin authentication via `RequireAdmin` middleware.
  - **Verified**: `TestArtifactRelink_AdminAuthorized` passes (200), `TestArtifactRelink_Unauthorized` passes (403).

- [x] 3.4 Update manifest resolution paths to use repaired artifact links while retaining safe fallback behavior.
  - `ServeManifest()` in `internal/api/handler/stream.go` updated to check `artifact_media_links` when `mpd_path` is empty.
  - Resolution chain: DB `mpd_path` → in-process cache → linked artifact's `OutputDir+MPDPath`.
  - Safe fallback: returns 404 if no manifest found through any path.
  - **Verified**: Stream handler compiles and passes existing tests; no regressions.

## 4. Testing and Operational Readiness

- [x] 4.1 Add unit tests for fingerprint generation, metadata parsing, and indexing validation edge cases.
  - `pkg/fingerprint/fingerprint_test.go` — fingerprint generation, EOF handling for small files.
  - `internal/artifact/metadata_test.go` — sidecar write/read roundtrip, bundle validation.
  - `internal/scanner/artifact_indexer_test.go` — sidecar write/read, bundle validation.
  - **Verified**: All unit tests pass (`go test ./...`).

- [x] 4.2 Add integration tests for database-loss recovery and storage-area import with preexisting transcodes.
  - `internal/scanner/artifact_integration_test.go` with 6 tests:
    - `TestIndexer_IndexStorageArea` — basic indexing
    - `TestIndexer_IdempotentIndexing` — re-indexing updates existing records
    - `TestIndexer_DatabaseLossRecovery` — index artifacts, delete DB entries, verify restoration
    - `TestRelink_RelinkExact` — fingerprint-based relinking
    - `TestRelink_RelinkUnmatched` — artifacts with no matching media item
    - `TestRelink_RelinkInvalid` — artifacts without fingerprints
  - **Verified**: All integration tests pass (`go test ./internal/scanner/`).

- [x] 4.3 Add authorization tests for indexing and relinking admin operations.
  - `internal/api/handler/artifacts_test.go` with 9 tests:
    - `TestArtifactIndex_Unauthenticated` — 401 for unauthenticated
    - `TestArtifactIndex_Unauthorized` — 403 for non-admin
    - `TestArtifactIndex_AdminAuthorized` — 200 for admin
    - `TestArtifactStatus_Unauthenticated` — 401 for unauthenticated
    - `TestArtifactStatus_Unauthorized` — 403 for non-admin
    - `TestArtifactStatus_AdminAuthorized` — 200 for admin
    - `TestArtifactRelink_Unauthenticated` — 401 for unauthenticated
    - `TestArtifactRelink_Unauthorized` — 403 for non-admin
    - `TestArtifactRelink_AdminAuthorized` — 200 for admin
  - Router includes `Authenticate` + `RequireAdmin` middleware chain.
  - **Verified**: All 9 tests pass (`go test ./internal/api/handler/ -run "TestArtifact"`).

- [x] 4.4 Document recovery runbooks for index and relink flows, including rollback and ambiguity handling.
  - `openspec/changes/artifact-recovery/runbook.md` with comprehensive documentation:
    - Prerequisites and schema status checking
    - Indexing workflow with curl examples and result interpretation
    - Relinking workflow with curl examples and result interpretation
    - Database loss recovery step-by-step procedure
    - Storage path change recovery procedure
    - Ambiguity handling guide with SQL queries
    - Troubleshooting section (409, no matches, manifest failures)
    - Rollback procedures with SQL commands
  - **Verified**: File exists and is comprehensive (8.1KB).
