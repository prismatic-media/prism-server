# Feature Specification: Fix Duplicate Data Loading

**Feature Branch**: `002-fix-duplicate-data-loading`

**Created**: 2026-07-26

**Status**: Draft

**Input**: User description: "The web UI is making multiple requests to load movies, tv shows, etc. There doesn't seem to be any issue with the websocket, so it should only load the data one time, then rely on the websocket events to keep everything updated."

## Clarifications

### Session 2026-07-26

- Q: Should the Home page stop making its own HTTP calls for "recently added" and "continue watching" data, or keep them as separate targeted requests? → A: Keep separate targeted calls for Home-specific data (recently added, continue watching) but remove overlapping calls that duplicate the cache (counts, event-triggered refetches)
- Q: When an HTTP request to initially populate the cache fails, what should happen? → A: Retry once automatically after a short delay, then show an error with a manual retry option if the second attempt also fails
- Q: Is the layout search bar's direct HTTP calls (bypassing the cache) in scope for this fix? → A: Out of scope — search queries are distinct parameterized requests and are not causing the duplicate loading problem

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Single Initial Data Load (Priority: P1)

When a user navigates to any page in the media library (Home, Movies, TV Shows), the system should fetch each data type from the server exactly once. Subsequent navigation between pages should reuse the previously loaded data from the client-side cache, with no additional network requests.

**Why this priority**: Duplicate requests waste bandwidth, increase server load, and cause visible UI flickering/re-rendering that degrades the user experience. This is the core issue being reported.

**Independent Test**: Can be verified by opening the browser's network tab, navigating to a page, and confirming only one HTTP request is made per data type. Navigating away and returning should produce zero additional requests.

**Acceptance Scenarios**:

1. **Given** a user navigates to the Movies page for the first time, **When** the page loads, **Then** exactly one request is made to fetch the movie list
2. **Given** a user has already visited the Movies page and the cache is populated, **When** the user navigates to the Home page and then back to Movies, **Then** no new HTTP requests are made for movie data
3. **Given** a user navigates to the Home page, **When** the page loads, **Then** the movie/show counts are derived from the shared cache (no separate HTTP request), while the Home-specific data (recently added, continue watching) uses its own targeted API calls that do not overlap with cached data

---

### User Story 2 - WebSocket-Driven Real-Time Updates (Priority: P1)

After the initial data load, the system should rely exclusively on WebSocket events to keep displayed data up to date. No polling or event-triggered full data refetches should occur while the WebSocket connection is healthy.

**Why this priority**: The WebSocket connection is already working correctly. The problem is that event handlers are triggering full HTTP refetches instead of applying incremental updates from the event payloads, causing redundant network traffic.

**Independent Test**: Can be verified by watching the network tab while a media item is added or updated on the server — the client should receive the update via WebSocket and apply it to the local cache without making any additional HTTP requests.

**Acceptance Scenarios**:

1. **Given** the Movies page is open and the WebSocket is connected, **When** a `media.created` event is received for a new movie, **Then** the movie appears in the list without any HTTP request being made
2. **Given** the Home page is open, **When** a `media.updated` or `media.enriched` event is received, **Then** the dashboard updates its display using the event payload data without triggering a full data refetch
3. **Given** any library page is open and the WebSocket is connected, **When** multiple events arrive in quick succession, **Then** only the local cache is updated — no HTTP requests are made

---

### User Story 3 - Graceful WebSocket Reconnection (Priority: P2)

When the WebSocket connection drops and reconnects, the system should perform a single, controlled refresh of any actively loaded data caches to catch up on missed events — but should not duplicate requests that are already in flight.

**Why this priority**: Reconnection recovery is an important edge case, but the primary issue is the happy-path duplicate loading. This story ensures the reconnect path doesn't introduce its own redundancy.

**Independent Test**: Can be verified by temporarily disrupting the WebSocket connection (e.g., toggling network), observing a single refresh cycle on reconnect, and confirming no duplicate requests occur.

**Acceptance Scenarios**:

1. **Given** the WebSocket connection is lost and then restored, **When** the reconnection occurs, **Then** each active cache is refreshed at most once
2. **Given** a component calls the cache load method while a reconnection reload is already in progress for that data type, **When** both compete, **Then** only one HTTP request is actually made (the in-flight request is shared or the duplicate is suppressed)

---

### Edge Cases

- What happens when a user rapidly navigates between pages — does each navigation trigger redundant requests, or does the "already loaded" guard in the cache prevent duplicates?
- If the initial cache load fails, the system retries once automatically after a short delay; if the retry also fails, an error is displayed with a manual retry option
- What happens when the user logs out and logs back in — is the cache properly invalidated and reloaded fresh?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST fetch each data type (movies, TV shows, episodes, jobs) at most once during initial page load across all components
- **FR-002**: System MUST use cached data for subsequent navigations to pages that display the same data type, without making new HTTP requests
- **FR-003**: System MUST apply WebSocket event payloads as incremental updates to the local cache rather than triggering full data refetches from the server
- **FR-004**: The Home page dashboard MUST derive its movie and show counts from the shared cache rather than making independent HTTP requests for the same data. Home-specific data (recently added items, continue watching history) MAY use separate targeted API calls as long as they do not duplicate data already available in the shared cache
- **FR-005**: System MUST protect against concurrent duplicate requests for the same data type — if a load is already in progress, additional calls MUST wait for or share the existing request rather than starting a new one
- **FR-006**: System MUST reload active caches exactly once on WebSocket reconnection to recover from any missed events during the disconnection period
- **FR-007**: System MUST fully clear all cached data on user logout so that a subsequent login starts with a clean state
- **FR-008**: If an initial cache load request fails, the system MUST retry once automatically after a short delay; if the retry also fails, an error MUST be displayed to the user with a manual retry option

### Key Entities

- **Client-Side Cache**: An in-memory store that holds the most recent full dataset for each data type (movies, TV shows, episodes, jobs), populated once and updated incrementally via WebSocket events
- **WebSocket Event**: A real-time server-pushed message containing a type (e.g., `media.created`, `job.progress`) and a payload with the updated entity data

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Each data type (movies, TV shows, episodes, jobs) results in at most one HTTP request per user session until a WebSocket reconnection occurs
- **SC-002**: Navigating between the Home, Movies, and TV Shows pages after initial load produces zero additional HTTP data-fetch requests (verifiable via browser dev tools network tab)
- **SC-003**: When a media item is created or updated on the server, the UI reflects the change within the existing WebSocket event delivery window (currently batched at 1 second) with zero additional HTTP requests
- **SC-004**: After a WebSocket reconnection, at most one HTTP request per active data type is made to refresh stale data

## Assumptions

- The WebSocket connection and event delivery mechanism are functioning correctly and do not need modification
- The existing cache service architecture (BehaviorSubject-based stores with Map backing) is sound and should be preserved — the issue is how and when callers trigger loads, not the cache infrastructure itself
- The Home page's "recently added" and "continue watching" sections keep their own targeted API calls (sorted/limited queries, user-specific history) since they serve a different purpose than the full library cache — the fix focuses on removing the overlapping calls (counts, event-triggered full refetches) not these distinct queries
- The server-side API endpoints do not need modification — this is purely a client-side data loading optimization
- The global search bar's HTTP calls are out of scope — search queries are parameterized, debounced requests that serve a different purpose than full library loading and do not contribute to the duplicate request problem
