# Quickstart Validation: Fix Duplicate Data Loading

**Date**: 2026-07-26 | **Branch**: `002-fix-duplicate-data-loading`

## Prerequisites

- Go toolchain installed
- Node.js and npm installed
- FFmpeg installed (for full server functionality, not required for this validation)
- Media files in configured library directories (or mock data from server seed)

## Setup

```bash
cd /home/benwelker/repos/prism-server
make dev
```

Wait for the server to start. Log in at `http://localhost:PORT/login` with `admin` / `asdf`.

## Validation Scenarios

### V1: Single Initial Load (FR-001, SC-001)

**Steps**:
1. Open browser DevTools → Network tab
2. Clear the network log
3. Navigate to the Home page (`/`)
4. Filter network requests by `api/v1/movies` and `api/v1/tv-shows`

**Expected**:
- At most **one** request to `/api/v1/movies?sort=recent&limit=20` (Home-specific)
- At most **one** request to `/api/v1/tv-shows?sort=recent&limit=20` (Home-specific)
- **Zero** separate requests to `/api/v1/movies` (full list) or `/api/v1/tv-shows` (full list) from the Home page itself — counts are derived from cache subscriptions, not separate calls

### V2: No Duplicate on Navigation (FR-002, SC-002)

**Steps**:
1. From Home, navigate to Movies (`/movies`)
2. Observe network requests — one request to `/api/v1/movies` (cache load)
3. Navigate to Home
4. Navigate back to Movies

**Expected**:
- Step 2: Exactly one `/api/v1/movies` request
- Step 4: **Zero** additional `/api/v1/movies` requests — data served from cache

### V3: WebSocket Updates Without HTTP (FR-003, SC-003)

**Steps**:
1. Navigate to Movies page
2. Clear network log
3. Trigger a media scan or add a new file to the library (so the server emits a `media.created` event)
4. Wait up to 2 seconds for the WebSocket batch

**Expected**:
- The new movie appears in the list
- **Zero** HTTP requests to `/api/v1/movies` in the network log — update came via WebSocket

### V4: Home Page Event Handling (FR-003, FR-004)

**Steps**:
1. Navigate to the Home page
2. Clear network log
3. Trigger a `media.created` or `media.enriched` event (e.g., scan a new file)
4. Observe network tab

**Expected**:
- Movie/show counts update reactively (if the new item type matches)
- **Zero** requests to `/api/v1/movies?sort=recent&limit=20` or `/api/v1/tv-shows?sort=recent&limit=20` — no event-triggered refetch of dashboard data

### V5: In-Flight Deduplication (FR-005)

**Steps**:
1. Clear browser storage (force fresh state)
2. Navigate to Home (`/`)
3. Immediately click Movies (`/movies`) before Home finishes loading
4. Check network log for `/api/v1/movies` requests

**Expected**:
- At most **one** request to `/api/v1/movies` — both Home and Movies share the in-flight request

### V6: WebSocket Reconnection (FR-006, SC-004)

**Steps**:
1. Navigate to Movies page (cache loaded)
2. Open DevTools → Network → toggle "Offline" briefly (2–3 seconds), then re-enable
3. Wait for WebSocket reconnection (5-second reconnect timer)
4. Observe network requests after reconnection

**Expected**:
- Exactly **one** `/api/v1/movies` request after reconnection
- No duplicate requests during the reconnect cycle

### V7: Cache Load Retry on Failure (FR-008)

**Steps**:
1. Start the server
2. In DevTools → Network, block the `/api/v1/movies` URL (right-click → Block request URL)
3. Navigate to Movies page
4. Observe the retry behavior
5. Unblock the URL and verify retry behavior

**Expected**:
- First request fails
- System automatically retries once after a short delay
- If the retry also fails (still blocked), an error message is displayed
- If unblocked before retry, the retry succeeds and movies load

### V8: Logout Cache Clearance (FR-007)

**Steps**:
1. Navigate to Movies (cache loaded)
2. Log out
3. Log back in
4. Navigate to Movies

**Expected**:
- Step 4 triggers a fresh `/api/v1/movies` request (cache was cleared on logout)
- No stale data from the previous session
