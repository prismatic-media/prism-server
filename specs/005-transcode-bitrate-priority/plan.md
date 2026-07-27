# Implementation Plan: Transcode Bitrate Priority

**Branch**: `005-transcode-bitrate-priority` | **Date**: 2026-07-27 | **Spec**: [spec.md](file:///home/benwelker/repos/prism-server/specs/005-transcode-bitrate-priority/spec.md)

**Input**: Feature specification from `/specs/005-transcode-bitrate-priority/spec.md`

## Summary

Prioritize transcode sub-job execution by target video bitrate in ascending order (`COALESCE(video_bitrate_k, 0) ASC`). When a media item is enqueued for transcoding, low bitrate renditions (e.g. 360p) will finish first, enabling faster initial video playback availability while higher resolution renditions (e.g. 1080p) continue processing in the background.

## Technical Context

**Language/Version**: Go 1.22+  
**Primary Dependencies**: `modernc.org/sqlite` (pure Go SQLite in WAL mode)  
**Storage**: SQLite (`transcode_sub_jobs`, `transcode_jobs`, `media_items` tables)  
**Testing**: `go test ./...` (`internal/store/sqlite/jobs_test.go`, `internal/api/handler/worker_handler_test.go`)  
**Target Platform**: Linux / macOS / Windows server backend  
**Project Type**: Go REST & WebSocket API media server  
**Performance Goals**: Instant sub-job claim query execution (<5ms per claim)  
**Constraints**: Zero database schema migration required (`video_bitrate_k` already exists on `transcode_sub_jobs`); pure SQL `ORDER BY` clause update in `ClaimNextSubJob` and sub-job list queries.  
**Scale/Scope**: Server worker pool + remote worker API endpoints  

## Constitution Check

*GATE: Passed prior to Phase 0 research & Phase 1 design.*

- **Library-First & Clean Boundaries**: No boundary breaks; changes localized to `internal/store/sqlite`.
- **Test-First**: Unit test cases will verify sub-job claim ordering by bitrate prior to verifying implementation behavior.
- **Data Integrity**: Preserves atomic SQLite transaction semantics for claiming sub-jobs.

## Project Structure

### Documentation (this feature)

```text
specs/005-transcode-bitrate-priority/
├── plan.md              # Implementation Plan
├── research.md          # Phase 0 Research
├── data-model.md        # Phase 1 Data Model
├── quickstart.md        # Phase 1 Quickstart & Validation Guide
└── contracts/           # Phase 1 Interface Contracts
    └── subjob_claiming.md
```

### Source Code (repository root)

```text
internal/
├── store/
│   └── sqlite/
│       ├── jobs.go      # ClaimNextSubJob & ListTranscodeSubJobsByJob ORDER BY updates
│       └── jobs_test.go  # Unit test suite for bitrate priority claim order
```

**Structure Decision**: Single Go project layout under `internal/store/sqlite/`.

## Complexity Tracking

*No constitution violations; no tracking required.*
