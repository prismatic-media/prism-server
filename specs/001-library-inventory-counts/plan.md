# Implementation Plan: Library Inventory Counts Fix & Expansion

**Branch**: `001-library-inventory-counts` | **Date**: 2026-07-26 | **Spec**: [spec.md](file:///home/benwelker/repos/prism-server/specs/001-library-inventory-counts/spec.md)

**Input**: Feature specification from `/specs/001-library-inventory-counts/spec.md`

## Summary

Fix the bug causing the Movies count on the Library settings page (`/admin/library`) to always show `0`, and introduce a new total TV episodes count item to the Total Inventory section.

The root cause of the movie count issue is that the Angular component `library-admin.component.ts` filters API responses from `/api/v1/movies?all=true` by `item.media_type === 'movie'`, but backend `Movie` response structs omit `media_type`. Removing this invalid filter fixes the movie count calculation. Total episode counts will be retrieved using the existing `GET /api/v1/episodes` endpoint and rendered in a new breakdown row under Total Inventory.

## Technical Context

**Language/Version**: TypeScript / Angular 17+ (Frontend), Go 1.22+ (Backend)

**Primary Dependencies**: Angular `HttpClient`, RxJS `forkJoin`, `go-chi/chi/v5`, SQLite

**Storage**: SQLite (`modernc.org/sqlite` in WAL mode)

**Testing**: Go `testing` package (`make test`), Angular frontend build validation (`make dev`)

**Target Platform**: Web Browsers (Chrome/Firefox/Safari) & Linux backend server

**Project Type**: Web Application (Angular SPA + Go API backend)

**Performance Goals**: <1s inventory load time on settings page

**Constraints**: Dark-themed glassmorphism UI consistency (Material Symbols, bento layout)

**Scale/Scope**: 2 frontend files (`library-admin.component.ts`, `library-admin.component.html`), 0 backend schema changes

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- [x] **Testability**: Independent unit & manual acceptance tests defined.
- [x] **API Compatibility**: Zero breaking changes to existing REST endpoints.
- [x] **UI Consistency**: Follows existing bento card layout and color tokens.

## Project Structure

### Documentation (this feature)

```text
specs/001-library-inventory-counts/
├── plan.md              # This file
├── research.md          # Technical analysis and root cause finding
├── data-model.md        # State definitions and API response structures
├── quickstart.md        # Manual verification guide
├── contracts/           # Endpoint specs
│   └── library-stats.md
├── checklists/
│   └── requirements.md
└── spec.md              # Feature specification
```

### Source Code (repository root)

```text
web/src/app/admin/library/
├── library-admin.component.ts     # Component logic (data fetching & stat calculations)
├── library-admin.component.html   # Bento card inventory template
└── library-admin.component.css    # Bento card inventory styles
```

**Structure Decision**: Web application layout. Edits are concentrated in the Angular frontend (`web/src/app/admin/library/`), consuming existing Go REST backend APIs (`/api/v1/movies`, `/api/v1/tv-shows`, `/api/v1/episodes`).

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| None | N/A | N/A |
