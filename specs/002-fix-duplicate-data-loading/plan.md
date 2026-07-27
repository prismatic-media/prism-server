# Implementation Plan: Fix Duplicate Data Loading

**Branch**: `002-fix-duplicate-data-loading` | **Date**: 2026-07-26 | **Spec**: [spec.md](file:///home/benwelker/repos/prism-server/specs/002-fix-duplicate-data-loading/spec.md)

**Input**: Feature specification from `specs/002-fix-duplicate-data-loading/spec.md`

## Summary

The Prism web UI is making multiple redundant HTTP requests when loading media data (movies, TV shows, episodes). The root causes are: (1) the Home component calls both cache loads and its own direct HTTP fetches for overlapping data, (2) WebSocket events trigger full data refetches instead of relying on the cache's incremental update mechanism, (3) no in-flight request deduplication exists in the cache service, creating races during navigation and reconnection. The fix restructures the cache service to deduplicate in-flight requests and modifies the Home component to eliminate redundant calls while preserving its distinct dashboard-specific queries.

## Technical Context

**Language/Version**: TypeScript 5.9, Angular 21.2, RxJS 7.8

**Primary Dependencies**: Angular HttpClient, RxJS BehaviorSubject/Observable, Angular Router

**Storage**: N/A (client-side in-memory cache only)

**Testing**: Manual validation via browser DevTools network tab (see [quickstart.md](file:///home/benwelker/repos/prism-server/specs/002-fix-duplicate-data-loading/quickstart.md))

**Target Platform**: Web browser (Angular SPA served by Go backend)

**Project Type**: Web application (frontend SPA)

**Performance Goals**: Zero duplicate HTTP requests per data type per user session

**Constraints**: No server-side API changes; preserve existing cache architecture (BehaviorSubject + Map stores)

**Scale/Scope**: 5 files modified, 0 new files, purely client-side changes

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Constitution is a blank template — no project-specific gates defined. Gate passes by default.

**Post-Phase 1 re-check**: Still passes. No new architectural patterns introduced; the in-flight tracking pattern already exists in `AuthService.refreshObservable`.

## Project Structure

### Documentation (this feature)

```text
specs/002-fix-duplicate-data-loading/
├── spec.md              # Feature specification
├── plan.md              # This file
├── research.md          # Phase 0: Root cause analysis and pattern research
├── data-model.md        # Phase 1: State model changes
├── quickstart.md        # Phase 1: Manual validation scenarios
└── tasks.md             # Phase 2 output (created by /speckit-tasks)
```

### Source Code (affected files)

```text
web/src/app/
├── cache.service.ts           # Primary: Add in-flight deduplication + retry logic
├── home/
│   └── home.component.ts      # Primary: Remove redundant cache loads + event-triggered refetches
├── movies/
│   └── movies.component.ts    # Minor: No changes expected (already correct pattern)
├── tv-shows/
│   └── tv-shows.component.ts  # Minor: No changes expected (already correct pattern)
└── media-details/
    └── media-details.component.ts  # Minor: No changes expected (already correct pattern)
```

**Structure Decision**: No structural changes. All modifications are within existing files in the `web/src/app/` directory. The cache service gets the bulk of the changes (in-flight tracking, retry logic), while the Home component gets behavioral corrections (removing duplicate calls and event-triggered refetches).

## Complexity Tracking

No constitution violations to justify. The changes are minimal and follow established patterns already present in the codebase (`AuthService.refreshObservable` pattern for in-flight deduplication).
