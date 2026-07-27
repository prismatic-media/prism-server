# Tasks: Library Admin Data Caching Integration

**Input**: Design documents from `/specs/003-library-admin-cache/`

**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project environment verification

- [x] T001 Verify active Angular build setup in `web/`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Verify core `CacheService` observables are available before component refactoring

- [x] T002 Verify `CacheService` exposes `movies$`, `tvShows$`, and `episodes$` observables in `web/src/app/cache.service.ts`

**Checkpoint**: Foundation ready - user story implementation can now begin

---

## Phase 3: User Story 1 - Reuse Shared Media Cache on Library Admin Page (Priority: P1) 🎯 MVP

**Goal**: Replace direct HTTP GET requests for media catalog collections on the Library Admin page with `CacheService` reactive streams, ensuring media data is loaded once and reused.

**Independent Test**: Navigate to the Movies page to populate the cache, then navigate to the Library Admin page. Verify in the Network tab that zero GET requests are issued for `/api/v1/movies`, `/api/v1/episodes`, or `/api/v1/tv-shows`.

### Implementation for User Story 1

- [x] T003 [P] [US1] Inject `CacheService` into `LibraryAdminComponent` in `web/src/app/admin/library/library-admin.component.ts`
- [x] T004 [US1] Refactor `fetchData()` into `fetchLibraries()` to request only folder configuration from `/api/v1/libraries` in `web/src/app/admin/library/library-admin.component.ts`
- [x] T005 [US1] Subscribe to `combineLatest([cacheService.movies$, cacheService.tvShows$, cacheService.episodes$])` in `web/src/app/admin/library/library-admin.component.ts`
- [x] T006 [US1] Update library inventory stats calculation logic (`moviesCount`, `showsCount`, `episodesCount`, `posterCoverage`, `resolvedTitles`, `subtitleCoverage`) to derive metrics from cached media arrays in `web/src/app/admin/library/library-admin.component.ts`

**Checkpoint**: At this point, User Story 1 is fully functional — the Library Admin page uses cached media data and avoids duplicate HTTP GET requests.

---

## Phase 4: User Story 2 - Event-Driven Inventory & Metric Updates (Priority: P1)

**Goal**: Eliminate event-triggered HTTP refetches on the Library Admin page by relying on `CacheService` real-time event updates.

**Independent Test**: Trigger a media scan or enrichment event while staying on the Library Admin page. Confirm displayed statistics update on screen without initiating any HTTP GET requests for media collections.

### Implementation for User Story 2

- [x] T007 [US2] Remove local `EventService` subscription in `LibraryAdminComponent` that calls `fetchData()` on media events in `web/src/app/admin/library/library-admin.component.ts`
- [x] T008 [US2] Update library scan, save mapping, and delete mapping action callbacks to invoke `fetchLibraries()` for folder configurations without re-fetching media catalogs in `web/src/app/admin/library/library-admin.component.ts`

**Checkpoint**: At this point, User Story 2 is functional — WebSocket event updates reflect on screen without triggering secondary HTTP GET requests.

---

## Phase 5: User Story 3 - Instantaneous Admin Navigation Experience (Priority: P2)

**Goal**: Ensure cold direct loads initialize the cache and cached loads render statistics instantly with no artificial loading delays.

**Independent Test**: Navigate between Movies/TV Shows views and the Library Admin page; verify stats render in <50ms with no loading spinner when cached.

### Implementation for User Story 3

- [x] T009 [US3] Call `loadMovies()`, `loadTVShows()`, and `loadEpisodes()` on `CacheService` during `ngOnInit()` in `web/src/app/admin/library/library-admin.component.ts`
- [x] T010 [US3] Update `loading` flag state management to clear `loading = false` immediately when cached media arrays emit in `web/src/app/admin/library/library-admin.component.ts`

**Checkpoint**: All user stories complete and functional.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Verification and final build validation

- [x] T011 [P] Run frontend build (`make dev` or `ng build`) to confirm clean TypeScript compilation with zero errors
- [x] T012 Execute manual verification scenarios from `specs/003-library-admin-cache/quickstart.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: Can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion
- **User Story 1 (Phase 3)**: Depends on Foundational phase completion (MVP)
- **User Story 2 (Phase 4)**: Depends on User Story 1 completion
- **User Story 3 (Phase 5)**: Depends on User Story 1 completion
- **Polish (Phase 6)**: Depends on all user story phases being complete

### Parallel Opportunities

- T003 can be prepared in parallel with T001/T002.
- T011 build check can run in parallel with manual verification.

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 & 2 (Setup & Foundation)
2. Complete Phase 3 (User Story 1: CacheService integration)
3. **STOP and VALIDATE**: Verify zero duplicate HTTP requests on navigation
4. Proceed to Phase 4 (User Story 2: Event-driven updates) and Phase 5 (User Story 3: Cold load handling)
