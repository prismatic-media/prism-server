# Research & Technical Decisions: Library Inventory Counts

**Feature**: Library Inventory Counts Fix & Expansion
**Branch**: `001-library-inventory-counts`
**Date**: 2026-07-26

## 1. Movie Count Calculation Issue

### Problem Statement
On the Library settings page (`/admin/library`), the Movies count in the "Total Inventory" bento card always displays `0`, even when movie media items exist in the library database.

### Investigation & Root Cause
In `web/src/app/admin/library/library-admin.component.ts`, `fetchData()` fetches media items using `http.get<any[]>('/api/v1/movies?all=true')` and then attempts to filter the returned list:
```typescript
const movies = mediaItems ? mediaItems.filter((item) => item.media_type === 'movie') : [];
this.stats.moviesCount = movies.length;
```
However, the backend endpoint `GET /api/v1/movies` returns an array of `models.Movie` objects serialized via JSON. In `internal/models/models.go`, the `Movie` struct contains fields like `id`, `title`, `file_path`, `duration`, etc., but **does not** include a `media_type` field.

As a result, `item.media_type` evaluates to `undefined` in JavaScript/TypeScript for every item in `mediaItems`. The filter condition `item.media_type === 'movie'` evaluates to `false` for every element, reducing `movies` to `[]` and setting `moviesCount` to `0`.

### Decision
Update `library-admin.component.ts` to set `moviesCount = mediaItems.length` (or filter with fallback `!item.media_type || item.media_type === 'movie'`). Since `/api/v1/movies` strictly returns movie items, all returned items are valid movies.

### Alternatives Considered
- **Add `json:"media_type"` to `models.Movie` in Go backend**: Unnecessary API schema change since `/api/v1/movies` is an endpoint specific to movies. Updating the frontend consumer to accurately handle the response contract is cleaner and non-breaking.

---

## 2. Total Episode Count Data Retrieval

### Problem Statement
The user requested adding a new item to the inventory section of the Library settings page for the total number of episodes across all TV shows.

### Investigation & Available Endpoints
The Go backend already provides `GET /api/v1/episodes` handled by `ListAllEpisodes` in `internal/api/handler/tv.go`, which queries `sqlite.ListAllEpisodes(ctx, db)` to return all media items with `media_type = 'episode'`.

### Decision
Extend the `forkJoin` call in `library-admin.component.ts` `fetchData()` to fetch `/api/v1/episodes`. Calculate `episodesCount` directly from `episodes.length`.

### Alternatives Considered
- **Iterating TV Shows and fetching seasons/episodes per show**: Triggers $N+1$ HTTP requests. Rejected due to latency and unnecessary server load.
- **Adding a dedicated `/api/v1/libraries/stats` endpoint**: Could consolidate all inventory statistics into a single API request in the future, but using existing `GET /api/v1/episodes` achieves the requirement immediately with zero backend code changes required.

---

## 3. UI Layout & Breakdown Integration

### Decision
In `library-admin.component.html`, add a third breakdown row under the Total Inventory bento card:
```html
<div class="breakdown-row">
  <div class="breakdown-label">
    <span class="material-symbols-outlined text-amber">video_library</span>
    <span>Episodes</span>
  </div>
  <span class="breakdown-value">{{ stats.episodesCount }}</span>
</div>
```
Add CSS helper `.text-amber { color: #f59e0b; }` if needed, matching existing `.text-teal` (`#14b8a6`) and `.text-violet` (`#8b5cf6`) icon color helpers.

### Rationale
Maintains the existing bento card design language and dark-mode glassmorphism visual hierarchy.
