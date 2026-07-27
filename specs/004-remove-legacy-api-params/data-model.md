# Data Model: Cleaned Frontend Interface Contracts

This document defines the cleaned TypeScript interfaces for media entities in the Angular application after removing legacy `library_id` fields and legacy API parameter expectations.

---

## 1. TVShow Interface

```typescript
export interface TVShow {
  id: string;
  name: string;
  tmdb_id?: number;
  overview?: string;
  poster_path?: string;
  first_air_year?: number;
  director?: string;
  cast?: { name: string; character: string; profile_path: string }[];
  backdrop_path?: string;
  extra_posters?: string[];
}
```

*Changes*: Removed legacy `library_id: string;` property.

---

## 2. Episode Interface

```typescript
export interface Episode {
  id: string;
  title: string;
  media_type: string;
  file_path: string;
  file_size: number;
  duration: number;
  width: number;
  height: number;
  video_codec: string;
  audio_codec: string;
  tv_show_id: string;
  tv_season_id: string;
  season_number: number;
  episode_number: number;
  tv_show_title?: string;
  transcode_status: string;
  mpd_path?: string;
  source_status: string;
  bundle_status: string;
}
```

*Changes*: Removed legacy `library_id: string;` property.

---

## 3. Movie Interface

```typescript
export interface Movie {
  id: string;
  title: string;
  media_type: string;
  file_path: string;
  file_size: number;
  duration: number;
  width: number;
  height: number;
  video_codec: string;
  audio_codec: string;
  tmdb_id?: number;
  year?: number;
  overview?: string;
  poster_path?: string;
  transcode_status: string;
  mpd_path?: string;
  source_status: string;
  bundle_status: string;
  director?: string;
  cast?: { name: string; character: string; profile_path: string }[];
  backdrop_path?: string;
  extra_posters?: string[];
}
```

---

## 4. CacheService State Schema

```typescript
export interface CacheState {
  moviesStore: BehaviorSubject<Map<string, Movie> | null>;
  tvShowsStore: BehaviorSubject<Map<string, TVShow> | null>;
  episodesStore: BehaviorSubject<Map<string, Episode> | null>;
  jobsStore: BehaviorSubject<Map<string, TranscodeJob> | null>;
}
```
