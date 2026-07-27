# Tasks: Fix Duplicate Data Loading

**Input**: Design documents from `specs/002-fix-duplicate-data-loading/`

**Prerequisites**: plan.md (loaded), spec.md (loaded), research.md (loaded), data-model.md (loaded), quickstart.md (loaded)

**Tests**: Not explicitly requested — test tasks omitted. Manual validation via quickstart.md.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: No setup needed — this is a refactor of existing code, not a new feature. No new files, dependencies, or project structure changes.

*Phase skipped — proceed directly to Foundational.*

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Add in-flight request deduplication and retry logic to the cache service. This is the core infrastructure that all user stories depend on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T001 Add in-flight tracking fields (`loadingMovies$`, `loadingTVShows$`, `loadingEpisodes$`, `loadingJobs$` of type `Observable | null`) to `web/src/app/cache.service.ts`
- [X] T002 Refactor `reloadMovies()` in `web/src/app/cache.service.ts` to use `shareReplay(1)` + `finalize()` pattern: store the HTTP Observable in `loadingMovies$`, populate the store on success, clear `loadingMovies$` on completion. Add `retry({ count: 1, delay: 2000 })` before the `shareReplay` for automatic single-retry on failure
- [X] T003 Refactor `loadMovies()` in `web/src/app/cache.service.ts` to check `loadingMovies$` in addition to the store null-check — if an in-flight request exists, return early (the subscriber will be notified when the shared Observable completes)
- [X] T004 [P] Apply the same in-flight + retry pattern from T002/T003 to `reloadTVShows()` / `loadTVShows()` in `web/src/app/cache.service.ts`
- [X] T005 [P] Apply the same in-flight + retry pattern from T002/T003 to `reloadEpisodes()` / `loadEpisodes()` in `web/src/app/cache.service.ts`
- [X] T006 [P] Apply the same in-flight + retry pattern from T002/T003 to `reloadJobs()` / `loadJobs()` in `web/src/app/cache.service.ts`
- [X] T007 Add `clearInFlight()` helper to `web/src/app/cache.service.ts` that nulls all four `loading*$` fields, and call it from `clearAll()` so that in-flight requests are cleaned up on logout

**Checkpoint**: Cache service now deduplicates in-flight requests and retries once on failure. All existing consumers continue to work — no behavioral changes yet.

---

## Phase 3: User Story 1 — Single Initial Data Load (Priority: P1) 🎯 MVP

**Goal**: Eliminate duplicate HTTP requests so each data type is fetched at most once across all components during initial load.

**Independent Test**: Open DevTools Network tab, navigate to Home → Movies → back to Home. Verify at most one `/api/v1/movies` request total (the cache load), plus one `/api/v1/movies?sort=recent&limit=20` (Home-specific). No duplicates.

### Implementation for User Story 1

- [X] T008 [US1] Remove `this.cacheService.loadMovies()` and `this.cacheService.loadTVShows()` calls from `ngOnInit()` in `web/src/app/home/home.component.ts` (lines 61–62). The Home page should NOT trigger cache loads — it uses its own `fetchDashboardData()` for recently-added data and derives counts from cache subscriptions
- [X] T009 [US1] Keep the `this.cacheService.movies$.subscribe()` and `this.cacheService.tvShows$.subscribe()` subscriptions in `web/src/app/home/home.component.ts` (lines 64–76) for reactive count updates, but ensure they don't trigger loads — they should only react when data is available (the cache will be populated by other pages or on-demand). Update the count logic to also populate from `fetchDashboardData()` results when the cache hasn't been loaded yet
- [X] T010 [US1] Remove the `this.eventSub = this.eventService.events$.subscribe(...)` block that calls `fetchDashboardData(true)` on media events in `web/src/app/home/home.component.ts` (lines 79–89). The cache already handles incremental WS updates — Home counts will update reactively via cache subscriptions

**Checkpoint**: Home page no longer triggers redundant cache loads or event-driven refetches. Navigate Home → Movies → Home and verify zero duplicate requests. Run quickstart scenarios V1, V2, V4.

---

## Phase 4: User Story 2 — WebSocket-Driven Real-Time Updates (Priority: P1)

**Goal**: After initial load, rely exclusively on WebSocket events for updates. No event-triggered HTTP refetches while the connection is healthy.

**Independent Test**: With Movies page open, trigger a `media.created` event (add a file). Verify the new movie appears with zero HTTP requests in the network tab.

### Implementation for User Story 2

- [X] T011 [US2] Verify that removing the event-triggered `fetchDashboardData(true)` in T010 is sufficient — the `CacheService.handleEventBatch()` in `web/src/app/cache.service.ts` already processes `media.created`, `media.updated`, `media.enriched`, `tvshow.created`, `tvshow.updated`, `job.created`, `job.updated`, and `job.progress` events incrementally. No additional changes expected — this task is a validation checkpoint
- [X] T012 [US2] Verify that the Home component's `movies$` and `tvShows$` subscriptions (retained in T009) receive reactive updates when the cache is mutated by `handleEventBatch()` — the `BehaviorSubject.next(new Map(...))` in `web/src/app/cache.service.ts` (lines 220–223) should trigger re-emission. No code changes expected — validation only

**Checkpoint**: WebSocket events drive all real-time updates. Run quickstart scenario V3 to verify no HTTP requests are made when events arrive.

---

## Phase 5: User Story 3 — Graceful WebSocket Reconnection (Priority: P2)

**Goal**: On reconnect, refresh active caches exactly once. No duplicate requests between reconnect reload and component-triggered loads.

**Independent Test**: Toggle network offline/online, observe exactly one refresh per active data type, no duplicates.

### Implementation for User Story 3

- [X] T013 [US3] Verify that `reloadActiveCaches()` in `web/src/app/cache.service.ts` (lines 126–131) now benefits from the in-flight deduplication added in Phase 2 — if a component calls `loadMovies()` while `reloadMovies()` is in-flight (from reconnect), the `loadingMovies$` guard prevents a second request. No additional code changes expected — validation checkpoint
- [X] T014 [US3] Review the `connected$` subscription in `web/src/app/cache.service.ts` (lines 54–59) and confirm the `wasConnected` flag correctly prevents initial-connection reload (only reconnections trigger `reloadActiveCaches()`). No changes expected — validation only

**Checkpoint**: WebSocket reconnection triggers at most one reload per active cache. Run quickstart scenario V6.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final cleanup and full validation

- [X] T015 Remove any unused imports resulting from the event subscription removal in `web/src/app/home/home.component.ts` (e.g., `EventService` import if no longer used, `Subscription` if the `eventSub` field is removed)
- [X] T016 Clean up the `ngOnDestroy()` method in `web/src/app/home/home.component.ts` to remove the `eventSub` unsubscribe block if the subscription was fully removed in T010
- [X] T017 Run `make dev` and perform full quickstart validation (scenarios V1–V8) per `specs/002-fix-duplicate-data-loading/quickstart.md`
- [X] T018 Verify `make build` completes without errors (Angular production build)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: Skipped — no setup needed
- **Foundational (Phase 2)**: No dependencies — start immediately
- **User Story 1 (Phase 3)**: Depends on Phase 2 completion (T001–T007)
- **User Story 2 (Phase 4)**: Depends on Phase 3 completion (specifically T010)
- **User Story 3 (Phase 5)**: Depends on Phase 2 completion only (independent from US1/US2)
- **Polish (Phase 6)**: Depends on Phases 3–5 completion

### User Story Dependencies

- **User Story 1 (P1)**: Depends on Foundational (Phase 2) — can start immediately after
- **User Story 2 (P1)**: Depends on US1 completion (T010 removes the event-triggered refetch)
- **User Story 3 (P2)**: Depends on Foundational (Phase 2) only — can run in parallel with US1/US2

### Within Each Phase

- Phase 2: T001 → T002 → T003 (sequential for movies pattern), then T004/T005/T006 [P] in parallel, then T007
- Phase 3: T008 → T009 → T010 (sequential — each builds on the previous removal)
- Phase 4: T011 → T012 (validation tasks, sequential)
- Phase 5: T013 → T014 (validation tasks, sequential)

### Parallel Opportunities

- T004, T005, T006 can all run in parallel (they apply the same pattern to different data types in the same file, but on non-overlapping code sections)
- Phase 5 (US3) can start as soon as Phase 2 completes, in parallel with Phase 3 (US1)

---

## Parallel Example: Phase 2 (Foundational)

```text
# Sequential first (establish the pattern):
T001 → T002 → T003 (movies in-flight pattern)

# Then parallel (apply pattern to remaining stores):
T004: Apply pattern to TV shows (same file, different functions)
T005: Apply pattern to episodes (same file, different functions)
T006: Apply pattern to jobs (same file, different functions)

# Then sequential:
T007: Add clearInFlight() helper
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 2: Foundational (T001–T007) — in-flight dedup + retry in cache service
2. Complete Phase 3: User Story 1 (T008–T010) — remove Home component redundant calls
3. **STOP and VALIDATE**: Run quickstart V1, V2, V4 — verify zero duplicate requests
4. This alone eliminates the primary reported issue

### Incremental Delivery

1. Phase 2 → Foundation ready (in-flight dedup)
2. Phase 3 → US1 complete → Validate (MVP — duplicate loading fixed)
3. Phase 4 → US2 complete → Validate (WS updates verified clean)
4. Phase 5 → US3 complete → Validate (reconnect path safe)
5. Phase 6 → Polish → Full validation pass

---

## Notes

- [P] tasks = different files or non-overlapping code in same file, no dependencies
- [Story] label maps task to specific user story for traceability
- US2 tasks (T011, T012) are primarily validation checkpoints — the actual code changes in US1 (T010) should already satisfy US2's requirements
- US3 tasks (T013, T014) are also validation checkpoints — the foundational in-flight dedup (Phase 2) should already satisfy US3's requirements
- The bulk of actual code changes happen in Phase 2 (cache.service.ts) and Phase 3 (home.component.ts)
