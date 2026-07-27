# Interface Contract: Library Admin & CacheService Integration

## Overview

This contract defines the client-side component interface contract between `LibraryAdminComponent` and `CacheService`.

## Provided Interfaces by `CacheService`

### Data Request Triggers
- `loadMovies(): void` - Ensures movie catalog is requested if not already in memory or loading.
- `loadTVShows(): void` - Ensures TV show catalog is requested if not already in memory or loading.
- `loadEpisodes(): void` - Ensures episode catalog is requested if not already in memory or loading.

### Observables
- `movies$: Observable<Movie[] | null>` - Emits latest movie array or `null` when uninitialized.
- `tvShows$: Observable<TVShow[] | null>` - Emits latest TV show array or `null` when uninitialized.
- `episodes$: Observable<Episode[] | null>` - Emits latest episode array or `null` when uninitialized.

## HTTP Endpoints Used by `LibraryAdminComponent`

### 1. Dedicated Administrative Folder Mapping Endpoint (Preserved)

#### `GET /api/v1/libraries`
- **Purpose**: Fetch list of configured library storage folders.
- **Request**: No parameters.
- **Response**: `200 OK` with JSON array of `Library` objects.

#### `POST /api/v1/libraries`
- **Purpose**: Add a new storage folder mapping.
- **Body**: `{ path: string, media_type: 'movie' | 'tvshow' | 'music' }`
- **Response**: `201 Created` with created `Library` object.

#### `DELETE /api/v1/libraries/{id}`
- **Purpose**: Remove an existing storage folder mapping.
- **Response**: `200 OK` or `204 No Content`.

#### `POST /api/v1/libraries/{id}:scan`
- **Purpose**: Trigger a library scan for a specific folder path.
- **Response**: `200 OK`.

### 2. Deprecated HTTP Endpoints in `LibraryAdminComponent` (REMOVED)

The following direct endpoints MUST NOT be called directly by `LibraryAdminComponent`:
- `GET /api/v1/movies?all=true` -> Replaced by `CacheService.movies$`
- `GET /api/v1/episodes` -> Replaced by `CacheService.episodes$`
- `GET /api/v1/tv-shows?library_id=...` -> Replaced by `CacheService.tvShows$`
