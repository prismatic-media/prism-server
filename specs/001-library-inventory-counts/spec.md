# Feature Specification: Library Inventory Counts Fix & Expansion

**Feature Branch**: `001-library-inventory-counts`

**Created**: 2026-07-26

**Status**: Draft

**Input**: User description: "In the prism-server repo, I want to improve the inventory section of the Library settings page. The movies item always says zero right now. I want to fix that, and I want to add a new item for the total number of episodes."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Accurate Movie Inventory Count (Priority: P1)

As a media server administrator viewing the Library settings page, I want the Movies inventory item to reflect the actual total number of indexed movies in my media library, so that I have accurate visibility into my server's content inventory.

**Why this priority**: Correcting inaccurate information currently displayed to administrators is the top priority for library status monitoring.

**Independent Test**: Can be tested by adding or verifying movies in the library, opening the Library settings page, and confirming that the Movies count matches the actual number of indexed movies rather than showing zero.

**Acceptance Scenarios**:

1. **Given** a library containing indexed movie files, **When** an administrator views the inventory section of the Library settings page, **Then** the Movies count shows the correct non-zero total number of movies.
2. **Given** an empty movie library, **When** an administrator views the inventory section of the Library settings page, **Then** the Movies count displays zero.
3. **Given** new movies are added or deleted from the library, **When** the administrator refreshes or views the inventory section, **Then** the Movies count updates to reflect the new total.

---

### User Story 2 - TV Episode Inventory Count Display (Priority: P2)

As a media server administrator, I want to see the total number of TV episodes across all TV shows in the inventory section of the Library settings page, so that I can see the detailed volume of serial content stored on the server alongside overall show counts.

**Why this priority**: Provides additional granularity for administrators to understand how many individual video episodes are indexed, complementing existing show-level metrics.

**Independent Test**: Can be tested by viewing the inventory section of the Library settings page and verifying that an Episodes item is present and displays the sum of all episodes across indexed TV shows.

**Acceptance Scenarios**:

1. **Given** a library containing TV shows with multiple seasons and episodes, **When** an administrator views the inventory section of the Library settings page, **Then** a distinct inventory item for Episodes displays the accurate total count of all indexed episodes.
2. **Given** TV shows exist with zero episodes indexed, **When** an administrator checks the inventory section, **Then** the Episodes count accurately counts only available/indexed episodes.

---

### Edge Cases

- What happens when a scan is actively running while viewing the inventory? The inventory counts should reflect the current database state at the time of query without failing or hanging.
- How does the system handle libraries with TV shows or movies in soft-deleted, missing, or unparsed states? The inventory count should only include valid, active media items indexed in the library.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST accurately count and report the total number of indexed movies in the Library settings inventory section.
- **FR-002**: System MUST display a new inventory metric for the total number of indexed TV episodes in the Library settings inventory section.
- **FR-003**: System MUST update the movie and episode inventory counts dynamically whenever library index state changes or the settings page is reloaded.
- **FR-004**: System MUST maintain clear visual consistency between all inventory stat metrics (e.g. Movies, TV Shows, Episodes) in the settings UI.

### Key Entities *(include if feature involves data)*

- **Library Inventory Summary**: Aggregated statistical metrics representing total media items indexed on the server, including Movies, TV Shows, and TV Episodes.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% accuracy in reported movie count when compared to the total number of playable/indexed movie entries in the library database.
- **SC-002**: 100% accuracy in reported total episode count across all TV series when compared to total indexed episode records.
- **SC-003**: Inventory section on the Library settings page displays updated movie and episode statistics within 1 second of loading the page.

## Assumptions

- Existing UI components for inventory items on the Library settings page can be extended to display the new Episode count metric with matching design styling.
- Existing database tables for movies and episodes contain sufficient indexing data to perform aggregate counting queries.
