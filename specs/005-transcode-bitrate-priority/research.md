# Phase 0 Research: Transcode Bitrate Priority

## Overview

This document captures technical research and design decisions for prioritizing transcode sub-jobs by target bitrate in ascending order.

## Research Decisions

### Decision 1: Sub-Job Ordering in SQLite Claim Query

**Decision**: Modify the `ORDER BY` clause in `ClaimNextSubJob` (`internal/store/sqlite/jobs.go`) to order sub-jobs by target bitrate (`COALESCE(video_bitrate_k, 0) ASC`) after sub-job type and parent item priority/creation order.

**Rationale**:
- `ClaimNextSubJob` is the single source of truth for sub-job dispatching for both local worker pool goroutines and remote transcode workers.
- Ordering by `COALESCE(video_bitrate_k, 0) ASC` ensures that lower bitrate renditions (e.g., 360p / 800k) are claimed and executed before higher bitrate renditions (e.g., 1080p / 4500k) for the same media item.
- Using `COALESCE(..., 0)` places non-video sub-jobs (such as subtitle extraction or audio processing where `video_bitrate_k` is NULL) alongside or ahead of video sub-jobs, ensuring essential metadata/audio is ready first.
- Preserving `pinning_category ASC, priority DESC, created_at ASC` before bitrate ordering guarantees that item queue arrival order (FIFO) and parent job priority are strictly maintained across multiple media items.

**Alternatives Considered**:
- *Global bitrate sorting across all enqueued items*: Rejected per user decision (Option A). Items must preserve arrival order (FIFO).
- *Sorting at job creation time only*: Sorting in `ClaimNextSubJob` query is superior because it dynamically handles worker claiming, job re-queuing, and remote worker recovery without relying on insertion order assumptions.

---

### Decision 2: Sub-Job Listing & Presentation Order

**Decision**: Update `ListTranscodeSubJobsByJob` and associated query helpers to order sub-jobs by `COALESCE(video_bitrate_k, 0) ASC` as secondary sort after `type DESC`.

**Rationale**:
- Guarantees consistent ordering in API responses (`GET /api/v1/jobs/{id}`) and WebSocket progress broadcasts (`ProgressEvent`).
- Frontend UI and admin dashboards will consistently display sub-jobs ordered from lowest bitrate (fastest) to highest bitrate (slowest).

**Alternatives Considered**:
- *Leaving presentation order unsorted*: Rejected because UI progress bars and step lists would reflect arbitrary UUID sorting rather than actual execution order.

---

### Decision 3: Sub-Job Type Ordering Hierarchy

**Decision**: Maintain type precedence as non-video fast-path tasks first (Subtitles, Whisper), followed by Video renditions sorted by `video_bitrate_k ASC`.

**Rationale**:
- Subtitle extraction and Whisper transcription are non-video sidecar jobs that take minimal time and are required for full playback features.
- Sorting video sub-jobs ascending by `video_bitrate_k` ensures 360p finishes first, followed by 480p, 720p, 1080p, and 4K.
