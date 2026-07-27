# Tasks: Library Inventory Counts Fix & Expansion

**Input**: Design documents from `/specs/001-library-inventory-counts/`
**Prerequisites**: [plan.md](file:///home/benwelker/repos/prism-server/specs/001-library-inventory-counts/plan.md), [spec.md](file:///home/benwelker/repos/prism-server/specs/001-library-inventory-counts/spec.md), [research.md](file:///home/benwelker/repos/prism-server/specs/001-library-inventory-counts/research.md), [data-model.md](file:///home/benwelker/repos/prism-server/specs/001-library-inventory-counts/data-model.md), [contracts/library-stats.md](file:///home/benwelker/repos/prism-server/specs/001-library-inventory-counts/contracts/library-stats.md), [quickstart.md](file:///home/benwelker/repos/prism-server/specs/001-library-inventory-counts/quickstart.md)

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (`[US1]`, `[US2]`)
- Explicit file paths included in every task description

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Verify project environment and frontend/backend dependencies

- [x] T001 Verify project environment and build tooling readiness in `web/`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core data model updates required before implementing UI breakdowns

- [x] T002 Update `LibraryStats` interface in `web/src/app/admin/library/library-admin.component.ts` to include `episodesCount: number`

---

## Phase 3: User Story 1 - Accurate Movie Inventory Count (Priority: P1) 🎯 MVP

**Goal**: Ensure the Movies inventory item on the Library settings page reflects the actual number of indexed movies rather than displaying zero.

**Independent Test**: Load the Library settings page (`/admin/library`) with indexed movies and confirm the Movies count displays the accurate non-zero count matching indexed movies.

### Implementation for User Story 1

- [x] T003 [US1] Fix movie count calculation logic in `web/src/app/admin/library/library-admin.component.ts` by removing invalid `media_type` filtering on `/api/v1/movies` response

**Checkpoint**: User Story 1 is fully functional and testable independently (Movies count shows accurate numbers).

---

## Phase 4: User Story 2 - TV Episode Inventory Count Display (Priority: P2)

**Goal**: Display total TV episodes inventory count in the Library settings Total Inventory bento card.

**Independent Test**: Load the Library settings page and confirm an Episodes breakdown item renders with the `video_library` symbol and total episode count across all TV shows.

### Implementation for User Story 2

- [x] T004 [US2] Update `fetchData()` in `web/src/app/admin/library/library-admin.component.ts` to fetch `/api/v1/episodes` in `forkJoin` and populate `episodesCount`
- [x] T005 [P] [US2] Add Episodes breakdown row in `web/src/app/admin/library/library-admin.component.html` under Total Inventory bento card
- [x] T006 [P] [US2] Add `.text-amber` icon styling helper in `web/src/app/admin/library/library-admin.component.css`

**Checkpoint**: User Stories 1 and 2 are both functional independently.

---

## Phase 5: Polish & Validation

**Purpose**: Final build validation and visual verification

- [x] T007 Build application bundle and execute manual verification per `quickstart.md` using `make dev`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies.
- **Foundational (Phase 2)**: Depends on Phase 1 completion.
- **User Story 1 (Phase 3)**: Depends on Phase 2 completion.
- **User Story 2 (Phase 4)**: Depends on Phase 2 completion.
- **Polish & Validation (Phase 5)**: Depends on Phase 3 and Phase 4 completion.

### Parallel Opportunities

- Tasks T005 and T006 in Phase 4 can run in parallel as they target separate files (`.html` and `.css`).
