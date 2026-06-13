import { Injectable } from '@angular/core';
import { Movie } from './movies/movies.component';
import { TVShow } from './tv-shows/tv-shows.component';

@Injectable({
  providedIn: 'root',
})
export class LibraryStateService {
  // Movies state
  moviesCache: Movie[] | null = null;
  moviesFilter: 'all' | '4k' | '1080p' | 'transcoded' = 'all';
  moviesSearchQuery = '';
  moviesScrollPosition = 0;

  // TV Shows state
  tvShowsCache: TVShow[] | null = null;
  tvShowsSearchQuery = '';
  tvShowsScrollPosition = 0;

  clearCache(): void {
    this.moviesCache = null;
    this.tvShowsCache = null;
    this.moviesFilter = 'all';
    this.moviesSearchQuery = '';
    this.moviesScrollPosition = 0;
    this.tvShowsSearchQuery = '';
    this.tvShowsScrollPosition = 0;
  }
}
