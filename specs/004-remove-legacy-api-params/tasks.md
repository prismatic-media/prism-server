# Tasks: Remove Legacy API Parameters & Unify Frontend Cache Routing

**Input**: Design documents from `/specs/004-remove-legacy-api-params/`

**Prerequisites**: [plan.md](file:///home/benwelker/repos/prism-server/specs/004-remove-legacy-api-params/plan.md), [spec.md](file:///home/benwelker/repos/prism-server/specs/004-remove-legacy-api-params/spec.md), [research.md](file:///home/benwelker/repos/prism-server/specs/004-remove-legacy-api-params/research.md), [data-model.md](file:///home/benwelker/repos/prism-server/specs/004-remove-legacy-api-params/data-model.md), [ui-cache-contract.md](file:///home/benwelker/repos/prism-server/specs/004-remove-legacy-api-params/contracts/ui-cache-contract.md)

---

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Environment verification and preliminary checks

- [x] T001 Verify project structure and build pipeline in `web/`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core model interface cleanup that MUST be complete before user story testing

- [x] T002 Update `TVShow` and `Episode` interfaces in `web/src/app/tv-shows/tv-shows.component.ts` to remove legacy `library_id` field
- [x] T003 [P] Update `TVShow` and `Episode` interfaces in `web/src/app/media-details/media-details.component.ts` to remove legacy `library_id` field
- [x] T004 [P] Update mock data fallback objects in `web/src/app/home/home.component.ts` to remove legacy `library_id` fields

**Checkpoint**: Foundational interface cleanup complete - User story implementation can proceed.

---

## Phase 3: User Story 1 - Clean Frontend API Routing & Legacy Parameter Removal (Priority: P1) 🎯 MVP

**Goal**: Eliminate legacy API parameters (`all=true` on `/movies`, `library_id` on `/tv-shows`) from all frontend HTTP requests.

**Independent Test**: Navigate through all web views (Home, Movies, TV Shows, Media Details, Admin) and verify network request logs in browser developer tools to ensure zero requests contain `all=true` or `library_id` parameters.

### Implementation for User Story 1

- [x] T005 [US1] Inspect and verify HTTP request URLs in `web/src/app/cache.service.ts` to ensure full collection queries (`/api/v1/movies`, `/api/v1/tv-shows`, `/api/v1/episodes`) omit `all=true` and `library_id`
- [x] T006 [US1] Inspect and verify dashboard HTTP request URLs in `web/src/app/home/home.component.ts` to ensure zero legacy query parameters are transmitted
- [x] T007 [US1] Perform a workspace sweep across `web/src/app/` to ensure no lingering references or comments refer to `all=true` or `library_id`

**Checkpoint**: User Story 1 complete — All frontend HTTP endpoints clean and free of legacy parameters.

---

## Phase 4: User Story 2 - Unified Cache Service Routing for Media Collections (Priority: P2)

**Goal**: Ensure all full-collection media loads route through `CacheService` to guarantee single hydration and instant WebSocket update synchronization.

**Independent Test**: Trigger library scan or transcode completion in one window while observing Home dashboard and Admin pages update in real-time without page refresh.

### Implementation for User Story 2

- [x] T008 [US2] Update `ngOnInit()` in `web/src/app/home/home.component.ts` to explicitly invoke `CacheService.loadMovies()` and `CacheService.loadTVShows()` to hydrate inventory count stats
- [x] T009 [US2] Audit `web/src/app/movies/movies.component.ts` and `web/src/app/tv-shows/tv-shows.component.ts` to confirm full collection loads route through `CacheService`
- [x] T010 [US2] Audit `web/src/app/admin/library/library-admin.component.ts` and `web/src/app/admin/transcoding/transcoding-admin.component.ts` to confirm collection stats and lists consume `CacheService`

**Checkpoint**: User Story 2 complete — All full-collection media operations unified under `CacheService`.

---

## Phase 5: Polish & Cross-Cutting Concerns

**Purpose**: Validation and build verification

- [x] T011 Run `make dev` to verify clean compilation of the Angular web application into `web/dist/`
- [x] T012 Execute end-to-end verification scenarios from `specs/004-remove-legacy-api-params/quickstart.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Story 1 (Phase 3)**: Depends on Foundational phase completion
- **User Story 2 (Phase 4)**: Depends on Foundational phase completion; builds on clean API endpoints in US1
- **Polish (Phase 5)**: Depends on completion of User Stories 1 & 2

### Parallel Opportunities

- Foundational interface cleanup tasks T003 and T004 can run in parallel with T002 across different component files.
- User Story 1 tasks (T005, T006) and User Story 2 tasks (T008, T009, T010) target separate component files and can be executed efficiently.

---

## Implementation Strategy

### MVP Scope (User Story 1 Only)

1. Complete Phase 1 (Setup) and Phase 2 (Foundational interface cleanup)
2. Complete Phase 3 (User Story 1: Remove legacy parameters from HTTP requests)
3. Validate network log in browser to confirm zero legacy parameters sent

### Full Feature Increment

1. Complete MVP (User Story 1)
2. Complete Phase 4 (User Story 2: Ensure `HomeComponent` and all components hydrate via `CacheService`)
3. Complete Phase 5 (Polish & Verification: `make dev` build and quickstart test scenarios)
