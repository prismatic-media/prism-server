# Research: Fix Duplicate Data Loading

**Date**: 2026-07-26 | **Branch**: `002-fix-duplicate-data-loading`

## R1: Root Causes of Duplicate HTTP Requests

### Decision: Four distinct root causes identified in the current codebase

### Findings

#### 1. Home Component Double-Dipping (Primary)

[home.component.ts](file:///home/benwelker/repos/prism-server/web/src/app/home/home.component.ts) calls both `cacheService.loadMovies()` / `cacheService.loadTVShows()` for counts (lines 61–62) **and** `fetchDashboardData()` which makes independent `http.get('/api/v1/movies?sort=recent&limit=20')` and `http.get('/api/v1/tv-shows?sort=recent&limit=20')` calls (lines 110–111). This creates 2 parallel requests for each data type on Home page load.

**Resolution**: Remove the redundant `cacheService.loadMovies()` / `cacheService.loadTVShows()` calls from Home and derive counts from `fetchDashboardData()` results instead, OR remove the direct HTTP calls for counts and rely solely on the cache. Per the spec clarification (Q1: Option B), Home keeps its own targeted calls for recently-added and continue-watching but removes the overlapping cache load calls for counts. The solution is to derive counts from the cache subscription and eliminate the duplicate load trigger.

#### 2. Home Component Event-Triggered Refetch (Primary)

[home.component.ts](file:///home/benwelker/repos/prism-server/web/src/app/home/home.component.ts) subscribes to `eventService.events$` and calls `fetchDashboardData(true)` on every `media.created/updated/enriched` event (lines 79–89). This triggers full HTTP round-trips for data that the cache already handles via its own WebSocket event handler in [cache.service.ts](file:///home/benwelker/repos/prism-server/web/src/app/cache.service.ts) (lines 140–223).

**Resolution**: Remove the event-driven `fetchDashboardData()` call. The cache's `handleEventBatch()` already applies incremental updates. Home page counts will update reactively through the `movies$` and `tvShows$` subscriptions.

#### 3. No In-Flight Request Deduplication (Secondary)

[cache.service.ts](file:///home/benwelker/repos/prism-server/web/src/app/cache.service.ts) `loadMovies()` guards against re-loading when the store is non-null (line 63), but `reloadMovies()` (called on WebSocket reconnect via `reloadActiveCaches()`) has no guard against concurrent duplicate requests. If reconnect fires while a component also calls `loadMovies()`, two HTTP requests may be issued.

**Resolution**: Add an in-flight tracking mechanism (e.g., store the Observable from the HTTP call and share it with `shareReplay`) so that concurrent callers receive the same response.

#### 4. Cache Load Race on Initial Navigation (Minor)

When the user first navigates to `/` (Home), `loadMovies()` and `loadTVShows()` are called. If they then navigate to `/movies`, the Movies component calls `loadMovies()` again. The null-check guard in `loadMovies()` protects against this **only if** the first request has already completed and populated the store. If the first request is still in-flight, the store is still null, and a second request fires.

**Resolution**: Same as R1.3 — track in-flight requests so `loadMovies()` returns the pending Observable instead of starting a new one.

### Alternatives Considered
- **Server-side deduplication**: Rejected — the server correctly responds to each request; deduplication belongs on the client
- **HTTP caching headers**: Rejected — doesn't solve the in-flight race and adds server-side complexity for a client-side problem
- **RxJS `shareReplay` at the HTTP level**: Considered and selected as part of the solution (see R1.3/R1.4)

---

## R2: Best Practice for RxJS In-Flight Deduplication in Angular Services

### Decision: Use a per-store `Observable | null` pattern with `shareReplay(1)` and `finalize()`

### Rationale

The established Angular pattern for deduplicating concurrent HTTP requests is to store the Observable from the first call and share it with subsequent callers until it completes:

```
private loadingMovies$: Observable<Movie[]> | null = null;

loadMovies(): void {
  if (this.moviesStore.getValue() !== null) return; // already loaded
  if (this.loadingMovies$) return; // already in-flight — subscribers will get notified

  this.loadingMovies$ = this.http.get<Movie[]>('/api/v1/movies').pipe(
    shareReplay(1),
    finalize(() => this.loadingMovies$ = null)
  );

  this.loadingMovies$.subscribe({
    next: (movies) => { /* populate store */ },
    error: (err) => { /* handle error, retry once */ }
  });
}
```

This pattern is already used in the codebase for token refresh (`AuthService.refreshObservable`).

### Alternatives Considered
- **`Subject`-based signaling**: More complex, no advantage for this use case
- **NgRx/signal store**: Over-engineered for the scope of this change; the existing BehaviorSubject pattern is sufficient

---

## R3: Retry Strategy for Failed Cache Loads

### Decision: Single retry with a 2-second delay using RxJS `retry({ count: 1, delay: 2000 })`

### Rationale

Per spec clarification Q2, the system should retry once automatically then surface an error. RxJS 7.8's `retry()` operator with config object provides this cleanly. The 2-second delay is short enough for a local network server without being imperceptible.

### Alternatives Considered
- **Exponential backoff**: Overkill for a self-hosted LAN server with typically one user
- **No retry**: Too fragile for transient errors during startup
