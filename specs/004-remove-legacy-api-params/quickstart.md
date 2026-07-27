# Quickstart & Verification Guide: Remove Legacy API Parameters & Unify Frontend Cache Routing

## Prerequisites

- Go 1.22+
- Node.js & Angular CLI (`ng`)
- Chrome DevTools or network inspector tool

---

## Verification Scenarios

### Scenario 1: Verify Endpoint Cleanup in Network Inspector

1. Build and launch the development environment:
   ```bash
   make dev
   ```
2. Open the browser to `http://localhost:8080/login` and sign in with admin credentials (`admin` / `asdf`).
3. Open Chrome DevTools -> **Network** tab.
4. Navigate through the application:
   - Click **Home**
   - Click **Movies**
   - Click **TV Shows**
   - Click **Settings / Library Admin**
5. Inspect the Network Requests log:
   - Confirm `GET /api/v1/movies` has **NO** `all=true` or `library_id` parameters.
   - Confirm `GET /api/v1/tv-shows` has **NO** `library_id` parameters.
   - Confirm `GET /api/v1/episodes` has **NO** `library_id` parameters.

---

### Scenario 2: Verify Single Hydration & Cache Routing

1. Clear Network log while on the **Movies** page.
2. Click to navigate to **TV Shows**, then back to **Movies**, then **Home**.
3. Verify that zero additional `GET /api/v1/movies` or `GET /api/v1/tv-shows` full-list requests are triggered once cache is populated.

---

### Scenario 3: Verify Real-Time Cache Sync

1. Open two browser windows side-by-side:
   - Window A: **Home** dashboard (showing Movie & Show totals).
   - Window B: **Library Admin** page.
2. Trigger a library scan from Window B.
3. Verify that Window A updates total counts in real time via `CacheService` WebSocket updates without a manual page refresh.
