# Phase 0: Research & Decision Log - Library Admin Data Caching Integration

## Problem Analysis

The `LibraryAdminComponent` (`web/src/app/admin/library/library-admin.component.ts`) currently operates independently of the application's central client-side cache (`CacheService`). 

Specifically:
1. It executes direct HTTP GET requests via `HttpClient` to `/api/v1/movies?all=true`, `/api/v1/episodes`, and `/api/v1/tv-shows` inside `fetchData()`.
2. It subscribes to `EventService` and invokes `fetchData()` whenever real-time events (`media.created`, `media.updated`, `media.enriched`) fire, causing full HTTP refetches of all media catalog collections.
3. Operations like scanning a library or saving/deleting library folder mappings invoke `fetchData()`, re-requesting full media collections from the server.

This results in:
- Duplicate network requests when navigating to the Library Admin page from an already-populated section of the app.
- Massive network chatter and server load spikes during background scans as real-time events trigger full catalog HTTP refetches.
- Visual flickering and unnecessary loading states on administrative dashboards.

## Research Findings & Decisions

### 1. Data Path Alignment with CacheService

- **Decision**: Refactor `LibraryAdminComponent` to consume media streams directly from `CacheService` (`movies$`, `tvShows$`, `episodes$`).
- **Rationale**: `CacheService` already handles single-in-flight request deduplication, memory caching, and real-time updates via WebSocket event subscription.
- **Alternatives Considered**: 
  - *Keep separate direct HTTP calls with local component caching*: Rejected because it duplicates cache logic and does not share memory with the rest of the application.
  - *Expose a specialized admin stats endpoint on backend*: Rejected because the media data is already cached on the client side, and calculating inventory stats from cached media array lengths in memory is instantaneous (0ms backend latency).

### 2. Event Handling Strategy

- **Decision**: Remove local `EventService` subscriptions from `LibraryAdminComponent` that invoke HTTP `fetchData()`.
- **Rationale**: `CacheService` already subscribes to `EventService` and mutates its internal `BehaviorSubject` stores (`moviesStore`, `tvShowsStore`, `episodesStore`) incrementally when real-time events arrive. Subscribing to `CacheService` observables in `LibraryAdminComponent` provides automatic, zero-HTTP real-time updates.
- **Alternatives Considered**:
  - *Keep EventService subscription and call `cacheService.reloadMovies()`*: Rejected because `CacheService` already applies incremental mutations from event payloads without making HTTP calls.

### 3. Separation of Configuration Data vs Catalog Data

- **Decision**: Keep fetching `/api/v1/libraries` (folder path mappings) as a lightweight, targeted administrative HTTP request inside `fetchLibraries()`.
- **Rationale**: Storage folder path configurations are specific to the Library Admin view and change infrequently (only when an admin adds or removes folder mappings). Separating `fetchLibraries()` from catalog stat calculation ensures folder management operations don't force catalog refetches.

## Implementation Architecture Summary

```
+--------------------------+       +------------------------------------+
|  LibraryAdminComponent   |       |           CacheService             |
+--------------------------+       +------------------------------------+
| - fetchLibraries() [HTTP]|       | - movies$: Observable<Movie[]>     |
| - stats computed from    |<------| - tvShows$: Observable<TVShow[]>   |
|   CacheService streams   |       | - episodes$: Observable<Episode[]> |
+--------------------------+       +------------------------------------+
                                                     ^
                                                     | (WebSocket updates)
                                           +-------------------+
                                           |   EventService    |
                                           +-------------------+
```
