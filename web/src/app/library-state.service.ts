import { Injectable } from '@angular/core';

@Injectable({
  providedIn: 'root',
})
export class LibraryStateService {
  // Movies state
  moviesFilter: 'all' | '4k' | '1080p' | 'transcoded' = 'all';
  moviesSearchQuery = '';
  moviesScrollPosition = 0;

  // TV Shows state
  tvShowsSearchQuery = '';
  tvShowsScrollPosition = 0;

  clearCache(): void {
    this.moviesFilter = 'all';
    this.moviesSearchQuery = '';
    this.moviesScrollPosition = 0;
    this.tvShowsSearchQuery = '';
    this.tvShowsScrollPosition = 0;
  }
}
