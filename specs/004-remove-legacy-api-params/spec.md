# Feature Specification: Remove Legacy API Parameters & Unify Frontend Cache Routing

**Feature Branch**: `004-remove-legacy-api-params`

**Created**: 2026-07-27

**Status**: Draft

**Input**: User description: "There are some remaining references to legacy api behavior on the frontend, specificly the `all=true` parameter on the `/movies` endpoint and the `library_id` parameter on the `/tv-shows` endpoint. There may be others, too. Identify any additional legacy behaviors or parameters that are still in use in the UI that have no meaningful effect on the response, and remove references to them. Ensure that all attempts to load all movies, tv shows, episodes, etc. are routed through the cache service to ensure proper cache and real-time update handling."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Clean Frontend API Routing & Legacy Parameter Removal (Priority: P1)

As a media server user, when interacting with movies, TV shows, and media items across the web interface, the frontend sends clean API requests without legacy query parameters (such as `all=true` or obsolete `library_id` filters) and operates smoothly without data discrepancies or redundant network requests.

**Why this priority**: Eliminating legacy API parameters and unused metadata properties cleans up technical debt, prevents frontend-backend contract mismatches, and ensures predictable API behavior across all pages.

**Independent Test**: Can be fully tested by navigating through all frontend views (Home, Movies, TV Shows, Media Details, Library Admin, Transcoding Admin) and verifying network request logs in browser developer tools to ensure no requests include obsolete query parameters or redundant full-list fetches.

**Acceptance Scenarios**:

1. **Given** a user navigating to the Movies catalog or TV Shows library, **When** the page initializes, **Then** all API data requests sent to load catalog collections omit deprecated query parameters like `all=true` or `library_id`.
2. **Given** frontend data models for TV Shows, Episodes, and Movies, **When** data is rendered in UI cards, detail views, and admin dashboards, **Then** legacy un-used fields (such as `library_id`) are removed from component definitions and mock data schemas.

---

### User Story 2 - Unified Cache Service Routing for Media Collections (Priority: P2)

As a media server user, when changes occur to movies, TV shows, or episodes (such as new media indexing or transcode completion), all views displaying full media lists update instantly via the centralized real-time cache service without requiring manual browser refreshes.

**Why this priority**: Routing all full-collection loads through a unified cache service guarantees consistent state across views, eliminates duplicate HTTP calls, and enables instant WebSocket-driven UI updates.

**Independent Test**: Can be tested independently by opening the Home dashboard and Admin pages in different browser tabs, triggering library updates or transcode events, and verifying that all views reflect updated item counts and list data via the cache service.

**Acceptance Scenarios**:

1. **Given** a user accessing the Home dashboard or Library Admin view, **When** full collection totals or item lists are required, **Then** component initialization explicitly invokes `CacheService` load methods (`loadMovies()`, `loadTVShows()`, `loadEpisodes()`) to retrieve cached state and listen for real-time WebSocket updates.
2. **Given** an ongoing library background scan or transcoding job completion, **When** real-time events fire, **Then** all cached reactive streams automatically update subscriber components across the application without secondary manual API fetches.

---

### Edge Cases

- What happens if a component initializes before WebSocket connection is established? The cache service falls back to standard HTTP load retries and automatically reloads active caches upon WebSocket reconnection.
- How does the system handle rapid navigation between views? Subscriptions to shared `CacheService` observables prevent redundant in-flight HTTP calls and share existing replay buffers across components.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST remove legacy `all=true` query parameters from all frontend HTTP requests for movies.
- **FR-002**: System MUST remove legacy `library_id` query parameters and data model properties from frontend TV show, episode, and media item interfaces and mock data.
- **FR-003**: System MUST route all full-collection requests for movies, TV shows, and episodes through `CacheService` across all frontend components (including Home, Movies, TV Shows, Media Details, and Admin pages).
- **FR-004**: System MUST ensure `HomeComponent` explicitly calls `CacheService.loadMovies()` and `CacheService.loadTVShows()` during initialization so library statistics and items are immediately hydrated and synced.
- **FR-005**: System MUST maintain specialized API requests (such as text search via `?q=` or recent items via `?sort=recent&limit=20`) without adding legacy query parameters.

### Key Entities *(include if feature involves data)*

- **Movie Collection State**: Centralized reactive map of movie items stored in `CacheService`, populated by single API load and updated via real-time events.
- **TV Show Collection State**: Centralized reactive map of TV show items stored in `CacheService`, populated by single API load and updated via real-time events.
- **Episode Collection State**: Centralized reactive map of episode items stored in `CacheService`, populated by single API load and updated via real-time events.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of full-collection data loads for movies, TV shows, and episodes in the web UI route exclusively through `CacheService`.
- **SC-002**: 0 network requests initiated by the frontend contain legacy `all=true` or `library_id` query parameters.
- **SC-003**: Navigating between Home, Movies, TV Shows, and Admin views results in 0 duplicate HTTP requests for media collections once cache is hydrated.
- **SC-004**: Real-time library events update all subscribed UI components within 500 milliseconds of event receipt.

## Assumptions

- Single-library architecture is standard across the server, making `library_id` parameters obsolete.
- Search queries (`?q=`) and paginated/recent queries (`?sort=recent&limit=N`) remain valid targeted endpoints and are distinct from full-collection library loads.
- `CacheService` handles retry logic and WebSocket connection lifecycle management internally.
