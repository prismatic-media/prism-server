# Implementation Plan: Library Admin Data Caching Integration

**Branch**: `003-library-admin-cache` | **Date**: 2026-07-27 | **Spec**: [spec.md](file:///home/benwelker/repos/prism-server/specs/003-library-admin-cache/spec.md)

**Input**: Feature specification from `/specs/003-library-admin-cache/spec.md`

## Summary

Refactor the Library Admin component (`web/src/app/admin/library/library-admin.component.ts`) to consume media catalog observables (`movies$`, `tvShows$`, `episodes$`) from `CacheService` instead of making direct, un-cached HTTP GET requests for catalog collections. Remove local `EventService` subscriptions in the component that trigger full HTTP refetches on WebSocket events, leveraging `CacheService`'s built-in event handling to achieve instant rendering and zero duplicate network calls.

## Technical Context

**Language/Version**: TypeScript 5.x / Angular 19  
**Primary Dependencies**: `@angular/core`, `@angular/common`, `rxjs` (`combineLatest`, `map`, `Subscription`)  
**Storage**: Client-side reactive memory cache (`CacheService`), RxJS `BehaviorSubject` stores  
**Testing**: Angular component unit tests (`ng test`)  
**Target Platform**: Web browsers  
**Project Type**: Web application (Angular SPA in `web/` + Go backend API)  
**Performance Goals**: Render library inventory statistics in <50ms when cache is populated; 0 duplicate catalog HTTP requests  
**Constraints**: Preserve distinct administrative storage folder API calls (`/api/v1/libraries`) while eliminating catalog collection calls  
**Scale/Scope**: Single component refactor (`web/src/app/admin/library/library-admin.component.ts`)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **Testability**: Independent component testability maintained via Angular dependency injection (`CacheService`). PASS.
- **Reactive Data Flow**: Follows RxJS reactive stream pattern without side-effecting duplicate network calls. PASS.
- **Simplicity**: Simplifies `LibraryAdminComponent` code significantly by removing complex `forkJoin` / `switchMap` HTTP pipelines and duplicate event listeners. PASS.

## Project Structure

### Documentation (this feature)

```text
specs/003-library-admin-cache/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   └── ui-cache-contract.md
└── checklists/
    └── requirements.md
```

### Source Code (repository root)

```text
web/
└── src/
    └── app/
        ├── admin/
        │   └── library/
        │       ├── library-admin.component.css
        │       ├── library-admin.component.html
        │       └── library-admin.component.ts  # [MODIFY] Refactor to inject CacheService
        └── cache.service.ts                     # Reference for streams (movies$, tvShows$, episodes$)
```

**Structure Decision**: Web application component refactor within `web/src/app/admin/library/`.

## Complexity Tracking

*No violations detected. No complexity tracking entries required.*
