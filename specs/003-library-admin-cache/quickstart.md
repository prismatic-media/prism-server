# Quickstart & Manual Verification Guide: Library Admin Data Caching

## Overview

This guide outlines manual and automated verification procedures to confirm that the Library Admin page reuses the client-side data cache, eliminates duplicate network calls, and updates reactively without re-fetching full media collections via HTTP.

## Prerequisites

- Local development environment running Prism Server (`make dev` or `go run ./cmd/server`).
- Chrome or browser Developer Tools (Network and Console tabs open).

## Verification Scenarios

### Scenario 1: Initial App Load & Cache Reuse

1. Open a fresh browser window and log into the application at `http://localhost:8080`.
2. Open Developer Tools -> **Network** tab. Filter by `Fetch/XHR`.
3. Navigate to **Movies** catalog page. Observe HTTP requests to `/api/v1/movies`.
4. Click on **Settings** / **Library Admin** page (`/admin/library`).
5. **Expected Outcome**:
   - HTTP requests are sent **ONLY** for `/api/v1/libraries` (folder configurations).
   - Zero HTTP GET requests are issued for `/api/v1/movies`, `/api/v1/episodes`, or `/api/v1/tv-shows`.
   - Inventory statistics (Movies Count, TV Shows Count, Episodes Count, Poster Coverage) render instantaneously (<50ms).

### Scenario 2: Direct Admin Page Load (Cold Cache)

1. Hard refresh (`Ctrl+Shift+R` or `Cmd+Shift+R`) directly on the `/admin/library` URL.
2. Observe Network tab activity.
3. **Expected Outcome**:
   - `CacheService` initiates single GET requests for `/api/v1/movies`, `/api/v1/tv-shows`, and `/api/v1/episodes`.
   - `LibraryAdminComponent` fetches `/api/v1/libraries`.
   - Subsequent navigation away to Movies/TV Shows and back to Library Admin produces zero media GET requests.

### Scenario 3: Real-Time Event Updates without Network Calls

1. Keep Network tab open on the Library Admin page.
2. In another tab or terminal, trigger media enrichment or scan.
3. Observe incoming WebSocket messages in the Network -> WS tab (`/api/v1/events`).
4. **Expected Outcome**:
   - As `media.created`, `media.updated`, or `media.enriched` WebSocket events arrive, inventory counts update reactively on screen.
   - **Zero** secondary HTTP GET requests for `/api/v1/movies` or `/api/v1/episodes` are triggered.

## Automated Testing Command

```bash
# Build Angular frontend
make dev
```
