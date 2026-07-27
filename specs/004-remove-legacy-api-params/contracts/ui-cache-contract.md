# Interface Contract: UI Cache & Clean Endpoint Standard

## 1. Clean API Endpoints (Frontend HTTP Calls)

All API calls issued by `CacheService` or UI components MUST use standard endpoints without legacy query parameters:

| Collection | Standard Endpoint | Legacy Parameter to Omit |
|---|---|---|
| Full Movie Collection | `GET /api/v1/movies` | Omit `all=true`, `library_id` |
| Full TV Show Collection | `GET /api/v1/tv-shows` | Omit `library_id` |
| Full Episode Collection | `GET /api/v1/episodes` | Omit `library_id` |
| Full Transcode Jobs | `GET /api/v1/jobs` | N/A |

---

## 2. Component Data Loading Contract

Every component displaying collection inventory or count statistics MUST conform to the following contract:

1. **Initialization**:
   - Call `cacheService.loadMovies()`, `cacheService.loadTVShows()`, or `cacheService.loadEpisodes()` in `ngOnInit()`.
2. **Subscription**:
   - Subscribe to `cacheService.movies$`, `cacheService.tvShows$`, or `cacheService.episodes$`.
3. **Re-render**:
   - Update component state when non-null values are emitted.
