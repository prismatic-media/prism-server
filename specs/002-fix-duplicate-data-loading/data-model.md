# Data Model: Fix Duplicate Data Loading

**Date**: 2026-07-26 | **Branch**: `002-fix-duplicate-data-loading`

## Overview

This change does not introduce new entities or modify the server-side data model. All changes are client-side, affecting the state management layer of the Angular SPA. The "data model" here describes the internal state shape of the `CacheService`.

## Entity: CacheService State (Modified)

The existing `CacheService` maintains four independent data stores. This change adds in-flight tracking fields alongside each store.

### Current Fields

| Field | Type | Purpose |
|-------|------|---------|
| `moviesStore` | `BehaviorSubject<Map<string, Movie> \| null>` | Holds all movies; null = not loaded |
| `tvShowsStore` | `BehaviorSubject<Map<string, TVShow> \| null>` | Holds all TV shows; null = not loaded |
| `episodesStore` | `BehaviorSubject<Map<string, Episode> \| null>` | Holds all episodes; null = not loaded |
| `jobsStore` | `BehaviorSubject<Map<string, TranscodeJob> \| null>` | Holds all transcode jobs; null = not loaded |

### New Fields (Added)

| Field | Type | Purpose |
|-------|------|---------|
| `loadingMovies$` | `Observable<Movie[]> \| null` | Tracks in-flight movies request; null = idle |
| `loadingTVShows$` | `Observable<TVShow[]> \| null` | Tracks in-flight TV shows request; null = idle |
| `loadingEpisodes$` | `Observable<Episode[]> \| null` | Tracks in-flight episodes request; null = idle |
| `loadingJobs$` | `Observable<TranscodeJob[]> \| null` | Tracks in-flight jobs request; null = idle |

### State Transitions

```
[null store, null loading$]  →  loadX()  →  [null store, loading$ set]
[null store, loading$ set]   →  HTTP success  →  [populated store, null loading$]
[null store, loading$ set]   →  HTTP fail + retry fail  →  [null store, null loading$] + error emitted
[populated store, null loading$]  →  loadX()  →  no-op (guard: store != null)
[populated store, null loading$]  →  reloadX()  →  [populated store, loading$ set]
[populated store, loading$ set]   →  HTTP success  →  [updated store, null loading$]
[any store, any loading$]  →  logout  →  [null store, null loading$]
```

## Entity: HomeComponent State (Modified)

### Removed Behaviors

| Current Behavior | Replacement |
|-----------------|-------------|
| Calls `cacheService.loadMovies()` and `cacheService.loadTVShows()` for counts | Derive counts from the Home-specific `fetchDashboardData()` response which already returns movie/show lists |
| Subscribes to `eventService.events$` to call `fetchDashboardData(true)` | Removed — cache handles incremental updates via its own event subscription; counts update reactively via cache subscriptions |

### Retained Behaviors

| Behavior | Reason |
|----------|--------|
| `fetchDashboardData()` for recently-added movies/shows and continue-watching | Per spec clarification Q1: these are distinct targeted queries |
| Direct subscription to `cacheService.movies$` / `cacheService.tvShows$` for counts | Reactive count updates from cache — no separate HTTP call needed |
