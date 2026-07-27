# Data Model & State Definitions: Library Inventory Counts

**Feature**: Library Inventory Counts Fix & Expansion
**Branch**: `001-library-inventory-counts`
**Date**: 2026-07-26

## 1. Component State Models (Frontend)

### `LibraryStats` Interface
Location: `web/src/app/admin/library/library-admin.component.ts`

```typescript
export interface LibraryStats {
  moviesCount: number;              // Total number of indexed movies
  showsCount: number;               // Total number of indexed TV series
  episodesCount: number;            // Total number of indexed TV episodes [NEW]
  posterCoverage: number;           // Percentage of titles with poster artwork (0-100)
  resolvedTitles: number;           // Count of titles with resolved poster metadata
  totalTitles: number;              // Total count of top-level titles (moviesCount + showsCount)
  subtitleCoverage: number;         // Percentage coverage for subtitles / transcodes
  missingLocalAssetsCount: number;  // Count of items missing local transcode assets
}
```

### Entity Fields & Default Values

| Field | Type | Default Value | Description |
|-------|------|---------------|-------------|
| `moviesCount` | `number` | `0` | Count of objects returned from `GET /api/v1/movies?all=true` |
| `showsCount` | `number` | `0` | Aggregated count of objects returned from `GET /api/v1/tv-shows?library_id={id}` across all TV libraries |
| `episodesCount` | `number` | `0` | Count of objects returned from `GET /api/v1/episodes` |
| `totalTitles` | `number` | `0` | Sum of `moviesCount` + `showsCount` |

---

## 2. API Schema References (Backend Contracts)

### `Movie` Response Entity (`GET /api/v1/movies`)
```json
[
  {
    "id": "uuid-string",
    "library_id": "uuid-string",
    "title": "Movie Title",
    "file_path": "/path/to/movie.mkv",
    "duration": 7200.0,
    "transcode_status": "done"
  }
]
```
*Note*: Returned items do not contain a `media_type` key because the endpoint is implicitly movie-scoped.

### `Episode` Response Entity (`GET /api/v1/episodes`)
```json
[
  {
    "id": "uuid-string",
    "library_id": "uuid-string",
    "tv_show_id": "uuid-string",
    "tv_season_id": "uuid-string",
    "title": "Episode Title",
    "season_number": 1,
    "episode_number": 1,
    "media_type": "episode"
  }
]
```
