# Implementation Plan: Remove Legacy API Parameters & Unify Frontend Cache Routing

**Branch**: `004-remove-legacy-api-params` | **Date**: 2026-07-27 | **Spec**: [spec.md](file:///home/benwelker/repos/prism-server/specs/004-remove-legacy-api-params/spec.md)

**Input**: Feature specification from `/specs/004-remove-legacy-api-params/spec.md`

## Summary

Remove legacy API query parameters (`all=true` on `/movies`, `library_id` on `/tv-shows` and `/movies`) and obsolete data fields (`library_id`) from Angular frontend interfaces and mock schemas. Ensure all full-collection media loads (`movies`, `tvShows`, `episodes`) route through `CacheService` to enforce single-hydration caching and real-time WebSocket update synchronization across all components.

## Technical Context

**Language/Version**: TypeScript 5.x / Angular 19 (Frontend), Go 1.22+ (Backend)
**Primary Dependencies**: `@angular/core`, `@angular/common/http`, `rxjs` (BehaviorSubject, shareReplay, combineLatest)
**Storage**: Client-side reactive memory maps (`Map<string, Movie>`, `Map<string, TVShow>`, `Map<string, Episode>`) inside `CacheService`
**Testing**: `ng build` compilation check, Karma/Jasmine frontend tests, `make dev` manual network inspection
**Target Platform**: Modern desktop/mobile web browsers (Chrome, Firefox, Safari)
**Project Type**: Single Page Application (SPA) Web Application
**Performance Goals**: Zero redundant full-collection HTTP requests after initial cache population; real-time event UI latency < 500ms
**Constraints**: Single-library backend model standard; zero legacy parameters in outgoing requests

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **Pass**: All frontend modifications maintain backward compatibility with backend Go REST handlers while removing redundant query parameters. No breaking changes or unauthorized structural additions.

## Project Structure

### Documentation (this feature)

```text
specs/004-remove-legacy-api-params/
├── spec.md              # Feature specification
├── plan.md              # Implementation plan (this file)
├── research.md          # Phase 0 decisions
├── data-model.md        # Phase 1 cleaned interfaces
├── quickstart.md        # Verification and validation guide
├── contracts/
│   └── ui-cache-contract.md # UI Cache & API endpoint contract
└── checklists/
    └── requirements.md  # Quality checklist
```

### Source Code (repository root)

```text
web/src/app/
├── cache.service.ts                     # Centralized reactive cache & HTTP loader
├── home/home.component.ts               # Dashboard stats & recent media listings
├── tv-shows/tv-shows.component.ts       # TV show catalog view & TVShow/Episode interfaces
├── movies/movies.component.ts           # Movie catalog view & Movie interface
├── media-details/media-details.component.ts # Detail browser & model interfaces
└── admin/
    ├── library/library-admin.component.ts
    └── transcoding/transcoding-admin.component.ts
```

---

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| None | N/A | N/A |
