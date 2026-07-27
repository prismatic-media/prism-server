# Tasks: Transcode Bitrate Priority

**Input**: Design documents from `/specs/005-transcode-bitrate-priority/`

**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, contracts/

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (`US1`, `US2`)
- Exact file paths included in all task descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Review existing sub-job queue logic

- [x] T001 Review existing sub-job claim and list queries in `internal/store/sqlite/jobs.go`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure prerequisites

*No foundational blocking tasks required; database schema already includes `video_bitrate_k`.*

---

## Phase 3: User Story 1 - Ascending Bitrate Sub-Job Execution (Priority: P1) 🎯 MVP

**Goal**: When a media item is enqueued for transcoding, process its sub-jobs in ascending order of target video bitrate (e.g. 360p @ 800k before 1080p @ 4500k).

**Independent Test**: Enqueue a single media item with 360p, 720p, and 1080p renditions. Verify `sqlite.ClaimNextSubJob` claims sub-jobs in order of lowest to highest `video_bitrate_k`.

### Implementation for User Story 1

- [x] T002 [P] [US1] Add unit test `TestClaimNextSubJob_AscendingBitrateOrder` in `internal/store/sqlite/jobs_test.go` to assert sub-jobs are claimed in ascending bitrate order
- [x] T003 [US1] Update `ClaimNextSubJob` SQL query `ORDER BY` clause in `internal/store/sqlite/jobs.go` to include `COALESCE(video_bitrate_k, 0) ASC` after sub-job type
- [x] T004 [P] [US1] Update `ListTranscodeSubJobsByJob` query in `internal/store/sqlite/jobs.go` to order returned sub-jobs by `COALESCE(video_bitrate_k, 0) ASC`
- [x] T005 [US1] Run unit tests in `internal/store/sqlite/jobs_test.go` to verify sub-job claim ordering passes

**Checkpoint**: At this point, User Story 1 is fully functional and testable independently.

---

## Phase 4: User Story 2 - Multiple Enqueued Media Items Priority Handling (Priority: P2)

**Goal**: Ensure sub-job queue ordering across multiple enqueued media items maintains FIFO item arrival order while processing each item's sub-jobs in ascending bitrate order.

**Independent Test**: Enqueue two media items in succession. Verify sub-jobs of the first item finish before the second item's sub-jobs begin, each in ascending bitrate order.

### Implementation for User Story 2

- [x] T006 [P] [US2] Add unit test `TestClaimNextSubJob_MultiItemFIFOWithBitrateOrder` in `internal/store/sqlite/jobs_test.go`
- [x] T007 [P] [US2] Update worker claim handler test `TestWorkerClaimSubJob_BitrateOrder` in `internal/api/handler/worker_handler_test.go`
- [x] T008 [US2] Run worker handler tests in `internal/api/handler/worker_handler_test.go`

**Checkpoint**: User Story 1 AND User Story 2 are fully functional and independently verified.

---

## Phase 5: Polish & Cross-Cutting Concerns

**Purpose**: Verification and validation

- [x] T009 [P] Run full Go backend test suite (`make test`)
- [x] T010 Validate end-to-end scenario per `specs/005-transcode-bitrate-priority/quickstart.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: Can start immediately
- **User Story 1 (Phase 3)**: Depends on Setup (Phase 1)
- **User Story 2 (Phase 4)**: Depends on User Story 1 (Phase 3)
- **Polish (Phase 5)**: Depends on User Story 1 and 2 completion

### Parallel Opportunities

- T002 (`jobs_test.go`) and T004 (`jobs.go` listing) can be written in parallel.
- T006 (`jobs_test.go`) and T007 (`worker_handler_test.go`) can be written in parallel.

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 (Setup)
2. Complete Phase 3 (User Story 1)
3. **Validate**: Run `go test ./internal/store/sqlite -run TestClaimNextSubJob` to confirm 360p is claimed first.
