# Research: Remove Legacy API Parameters & Unify Frontend Cache Routing

## Executive Summary

Research and architectural decisions for removing deprecated frontend API query parameters (`all=true`, `library_id`) and unifying all full-collection library loads through `CacheService`.

---

## Technical Decisions

### Decision 1: Removal of Legacy Query Parameters (`all=true`, `library_id`)

- **Decision**: Completely strip `all=true` query parameters from HTTP requests and eliminate `library_id` query parameters and data model fields from Angular component interfaces (`TVShow`, `Episode`) and mock data schemas.
- **Rationale**:
  - The server migrated to a unified single-library architecture. The `library_id` parameter has no effect on SQL queries or backend response objects.
  - The `/movies` endpoint returns full movie lists without requiring an `all=true` flag. Specifying `all=true` adds useless string payload and creates confusing API documentation/test expectations.
- **Alternatives Considered**:
  - *Keeping `library_id` as optional/deprecated in interfaces*: Rejected because preserving dead fields in frontend TypeScript interfaces introduces confusion and dead code.

---

### Decision 2: Centralized Cache Service Routing for Media Collections

- **Decision**: All Angular components that consume full collections of movies, TV shows, or episodes must use `CacheService` (`loadMovies()`, `loadTVShows()`, `loadEpisodes()`, and their corresponding `BehaviorSubject` observables `movies$`, `tvShows$`, `episodes$`).
- **Rationale**:
  - `CacheService` handles deduplicated HTTP fetching (`shareReplay(1)`), automatic retry logic, and real-time updates via WebSocket event subscription (`media.created`, `media.updated`, `tvshow.created`, etc.).
  - Calling `HttpClient` directly for full lists in individual components leads to duplicate requests, stale UI state when events fire, and lack of sync across views.
- **Affected Components**:
  - `HomeComponent`: Must invoke `cacheService.loadMovies()` and `cacheService.loadTVShows()` in `ngOnInit()` so library counts and statistics hydrate immediately from cache.
  - `MoviesComponent`: Already uses `CacheService.loadMovies()`.
  - `TVShowsComponent`: Already uses `CacheService.loadTVShows()`.
  - `MediaDetailsComponent`: Uses `CacheService` for movies, episodes, and TV shows.
  - `LibraryAdminComponent`: Uses `CacheService` for movies, shows, and episodes.
  - `TranscodingAdminComponent`: Uses `CacheService` for jobs, movies, and episodes.

---

### Decision 3: Separation of Targeted Queries vs. Full Collection Caches

- **Decision**: Specialized parameterized queries—such as text search (`GET /api/v1/movies?q=query`) or home dashboard recent items (`GET /api/v1/movies?sort=recent&limit=20`)—remain separate direct API requests and do not include legacy parameters.
- **Rationale**: Targeted search and recent items return filtered/limited subsets tailored for immediate UI presentation (e.g. top 20 recent items for a home carousel) and should not pollute or overwrite the full in-memory library cache.
- **Alternatives Considered**:
  - *Deriving recent items or search results client-side from `CacheService`*: Preserved server-side search and recent sorting to keep backend responsibility clear while keeping the full-library cache pure.
