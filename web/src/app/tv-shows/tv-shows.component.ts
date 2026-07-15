import { Component, OnInit, OnDestroy, inject, ChangeDetectorRef } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { HttpClient } from '@angular/common/http';
import { Subscription } from 'rxjs';
import { RouterLink } from '@angular/router';
import { CacheService } from '../cache.service';
import { LibraryStateService } from '../library-state.service';

export interface TVShow {
  id: string;
  library_id: string;
  name: string;
  tmdb_id?: number;
  overview?: string;
  poster_path?: string;
  first_air_year?: number;
}

export interface TVSeason {
  id: string;
  tv_show_id: string;
  season_number: number;
  tmdb_id?: number;
  overview?: string;
  poster_path?: string;
}

export interface Episode {
  id: string;
  library_id: string;
  title: string;
  media_type: string;
  file_path: string;
  file_size: number;
  duration: number;
  width: number;
  height: number;
  video_codec: string;
  audio_codec: string;
  season_number: number;
  episode_number: number;
  transcode_status: string;
  mpd_path?: string;
  source_status: string;
  bundle_status: string;
}

import { AlphabetRailComponent } from '../shared/alphabet-rail/alphabet-rail.component';

@Component({
  selector: 'app-tv-shows',
  standalone: true,
  imports: [CommonModule, FormsModule, RouterLink, AlphabetRailComponent],
  templateUrl: './tv-shows.component.html',
  styleUrl: './tv-shows.component.css',
})
export class TVShowsComponent implements OnInit, OnDestroy {
  getSortLetter(show: TVShow): string {
    const name = show.name || '';
    let normalized = name.trimStart();
    if (/^the\s+/i.test(normalized)) {
      normalized = normalized.replace(/^the\s+/i, '');
    }
    const firstChar = normalized.charAt(0).toUpperCase();
    if (/[0-9]/.test(firstChar)) return '#';
    if (/[A-Z]/.test(firstChar)) return firstChar;
    return '#';
  }

  private http = inject(HttpClient);
  private cdr = inject(ChangeDetectorRef);
  private cacheService = inject(CacheService);
  private libraryStateService = inject(LibraryStateService);
  private tvShowsSub?: Subscription;

  allShows: TVShow[] = [];
  shows: TVShow[] = [];
  searchQuery = '';

  // View States
  loading = true;
  error = '';

  ngOnInit(): void {
    this.searchQuery = this.libraryStateService.tvShowsSearchQuery;

    this.cacheService.loadTVShows();
    this.tvShowsSub = this.cacheService.tvShows$.subscribe({
      next: (shows) => {
        if (shows !== null) {
          this.allShows = shows;
          this.filterShows();
          this.loading = false;
          this.cdr.detectChanges();
        }
      },
      error: (err) => {
        this.error = 'Could not load TV shows.';
        this.loading = false;
        this.cdr.detectChanges();
      }
    });
  }

  ngOnDestroy(): void {
    if (this.tvShowsSub) {
      this.tvShowsSub.unsubscribe();
    }
  }

  filterShows(): void {
    this.libraryStateService.tvShowsSearchQuery = this.searchQuery;
    if (!this.searchQuery.trim()) {
      this.shows = [...this.allShows];
      return;
    }

    const q = this.searchQuery.toLowerCase();
    this.shows = this.allShows.filter(
      (s) =>
        s.name.toLowerCase().includes(q) || (s.overview && s.overview.toLowerCase().includes(q)),
    );
  }

  getShowPosterUrl(show: TVShow): string {
    if (show.poster_path) {
      return `/api/v1/tv-shows/${show.id}/poster`;
    }
    return 'https://images.unsplash.com/photo-1594909122845-11baa439b7bf?q=80&w=400&auto=format&fit=crop';
  }

  trackByShowId(index: number, show: TVShow): string {
    return show.id;
  }
}
