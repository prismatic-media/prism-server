# Feature Specification: Library Admin Data Caching Integration

**Feature Branch**: `003-library-admin-cache`

**Created**: 2026-07-27

**Status**: Draft

**Input**: User description: "The library admin page seems to be using a different data path than the rest of the app. I want this page to use the cache for its data where possible so that the data is only loaded once."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Reuse Shared Media Cache on Library Admin Page (Priority: P1)

As a media server administrator navigating to the Library Admin page, I want the page to reuse the existing application media cache rather than issuing duplicate HTTP network requests to fetch movie and TV show lists, so that the admin page loads instantaneously without redundant network traffic.

**Why this priority**: Currently, the Library Admin page bypasses the application's central data cache and issues multiple independent HTTP requests for movies, episodes, and TV shows. Reusing the cache eliminates unnecessary network overhead, avoids duplicate data transfers, and ensures visual consistency across the application.

**Independent Test**: Can be verified by opening the browser network tab, populating the media library by navigating to Movies or TV Shows, and then switching to the Library Admin page. Zero new HTTP requests should be observed for media collections (movies, TV shows, episodes).

**Acceptance Scenarios**:

1. **Given** media catalog data (movies, TV shows, episodes) is already present in the application cache, **When** the administrator opens the Library Admin page, **Then** all inventory stats and metadata coverage metrics are computed using the cached data without making HTTP requests for media lists.
2. **Given** an administrator opens the Library Admin page on initial application load (empty cache), **When** the page loads, **Then** media data is loaded once into the shared cache, and subsequent navigation to/from the Library Admin page produces zero network requests for media lists.
3. **Given** administrative configuration data (such as storage folder mappings), **When** the Library Admin page loads, **Then** only the dedicated library configuration endpoint is called, while media counts and coverage metrics are derived from the shared cache.

---

### User Story 2 - Event-Driven Inventory & Metric Updates (Priority: P1)

As an administrator monitoring the Library Admin page, I want library inventory counts and metadata health metrics to update automatically when media changes occur, without triggering full HTTP refetches of all library media items.

**Why this priority**: When media items are created, updated, or enriched, the Library Admin page currently executes full HTTP refetches of all media collections. Updating stats reactively via cache and event notifications eliminates server load spikes during background scanning and enrichment.

**Independent Test**: Can be verified by triggering a media creation or enrichment event while staying on the Library Admin page. The displayed inventory stats and coverage percentages should update immediately in response to the event without sending HTTP GET requests for media catalogs.

**Acceptance Scenarios**:

1. **Given** the Library Admin page is open and a media event (`media.created`, `media.updated`, or `media.enriched`) is received, **When** the event payload updates the shared cache, **Then** the displayed inventory counts and health metrics update dynamically without executing full HTTP refetches of media lists.
2. **Given** multiple media events arrive during an active library scan, **When** real-time updates arrive, **Then** the stats update reactively from the shared cache without queuing redundant network requests.

---

### User Story 3 - Instantaneous Admin Navigation Experience (Priority: P2)

As an administrator switching between administrative settings and catalog views, I want page transitions to be instant with no loading spinners or layout re-renders for already-cached content.

**Why this priority**: Improves user experience and perceived server performance by eliminating artificial loading delays on administrative dashboards.

**Independent Test**: Can be verified by rapidly navigating between the Movies view, TV Shows view, and Library Admin page, confirming that cached statistics render instantly without loading indicators or visual stutter.

**Acceptance Scenarios**:

1. **Given** media catalog data is already cached, **When** navigating from any catalog page to the Library Admin page, **Then** the page renders immediately with cached values and no loading spinner is shown for catalog stats.

---

### Edge Cases

- What happens if the shared media cache is empty or incomplete when the Library Admin page is accessed directly via URL? The page should trigger a single fetch to populate the shared media cache, ensuring all metrics are calculated accurately once data arrives.
- What happens when a new library folder mapping is added or removed? Adding or removing a library path should issue the appropriate configuration update request to the server and refresh the library folder list without forcing a full redundant re-download of all media items if catalog state has not changed.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST source media inventory statistics (movie counts, TV show counts, episode counts) on the Library Admin page from the application's central media data cache whenever available.
- **FR-002**: System MUST NOT issue independent, un-cached network requests for full movie or TV show lists from the Library Admin page when that data exists in the client cache.
- **FR-003**: System MUST update Library Admin inventory counts and metadata health metrics reactively when catalog state changes, relying on shared cache updates and real-time events rather than executing full HTTP refetches.
- **FR-004**: System MUST isolate administrative library storage path configuration (fetching and managing directory paths) to dedicated configuration requests without coupling them to catalog media list re-downloads.
- **FR-005**: System MUST compute metadata health metrics (such as poster coverage and title resolution totals) using cached media objects rather than fetching raw un-cached media collections.

### Key Entities *(include if feature involves data)*

- **Library Inventory Summary**: Aggregated statistical metrics (movie count, TV show count, episode count, poster coverage percentage, resolved titles count) calculated from cached catalog items.
- **Library Storage Mapping**: Administrative folder path configurations specifying directory locations and media types (movies, TV shows, music) managed on the server.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Zero duplicate network requests for media catalog collections (movies, TV shows, episodes) are initiated when opening the Library Admin page if data is already cached.
- **SC-002**: Library Admin page initial stat display renders in under 50 milliseconds when navigating from an already-populated catalog view.
- **SC-003**: 100% of real-time library event updates on the Library Admin page take effect without triggering secondary HTTP GET requests for media catalogs.
- **SC-004**: Overall network payload size transferred when opening the Library Admin page during normal navigation is reduced by over 80%.

## Assumptions

- The central media cache maintained by the application correctly tracks movies, TV shows, and episodes.
- Real-time websocket events properly notify the application cache of media creation, updates, and enrichment.
- Administrative storage folder configuration (`/api/v1/libraries`) remains a lightweight administrative endpoint separate from media catalog data.
