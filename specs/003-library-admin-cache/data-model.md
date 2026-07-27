# Phase 1 Data Model: Library Admin Data Caching Integration

## Entities & Component State Models

### 1. Library Statistics Model (`LibraryStats`)

The component state structure computed dynamically from cached media collections.

```typescript
export interface LibraryStats {
  moviesCount: number;             // Total cached movies (movies.length)
  showsCount: number;              // Total cached TV shows (tvShows.length)
  episodesCount: number;           // Total cached episodes (episodes.length)
  posterCoverage: number;          // Percentage of media items with a non-null poster_path
  resolvedTitles: number;          // Total media items with valid poster/metadata
  totalTitles: number;             // Sum of moviesCount and showsCount
  subtitleCoverage: number;        // Percentage of movies with transcode status 'done' (or fallback)
  missingLocalAssetsCount: number; // Movies with transcode status other than 'done'
}
```

### 2. Library Storage Folder Model (`Library`)

Administrative configuration object representing folder paths mounted on the server.

```typescript
export interface Library {
  id: string;                      // Unique identifier for storage mapping (UUID)
  path: string;                    // Absolute directory path on local filesystem
  media_type: 'movie' | 'tvshow' | 'music'; // Type of media in folder
  created_at: string;              // ISO-8601 timestamp
  updated_at: string;              // ISO-8601 timestamp
}
```

## Reactive Data Flow & State Transformations

### Stream Combination & Stat Computation

`LibraryAdminComponent` combines three reactive streams from `CacheService`:
- `cacheService.movies$`
- `cacheService.tvShows$`
- `cacheService.episodes$`

```
  movies$   ------------\
  tvShows$  ------------->  combineLatest  --->  computeLibraryStats()  --->  UI Update
  episodes$ ------------/
```

### Computation Logic:
- `moviesCount` = `movies ? movies.length : 0`
- `showsCount` = `tvShows ? tvShows.length : 0`
- `episodesCount` = `episodes ? episodes.length : 0`
- `totalTitles` = `moviesCount + showsCount`
- `resolvedTitles` = `movies.filter(m => m.poster_path).length + tvShows.filter(s => s.poster_path).length`
- `posterCoverage` = `totalTitles > 0 ? Math.round((resolvedTitles / totalTitles) * 1000) / 10 : 0`
- `missingLocalAssetsCount` = `movies.filter(m => m.transcode_status !== 'done').length`
- `subtitleCoverage` = `moviesCount > 0 ? Math.round(((moviesCount - missingLocalAssetsCount) / moviesCount) * 1000) / 10 : 74.5`
