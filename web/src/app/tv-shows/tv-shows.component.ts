import { Component, OnInit, OnDestroy, inject, ChangeDetectorRef } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { HttpClient } from '@angular/common/http';
import { forkJoin, map, of, switchMap, Subscription } from 'rxjs';
import { RouterLink } from '@angular/router';
import { EventService } from '../event.service';

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

@Component({
  selector: 'app-tv-shows',
  standalone: true,
  imports: [CommonModule, FormsModule, RouterLink],
  templateUrl: './tv-shows.component.html',
  styleUrl: './tv-shows.component.css'
})
export class TVShowsComponent implements OnInit, OnDestroy {
  private http = inject(HttpClient);
  private cdr = inject(ChangeDetectorRef);
  private eventService = inject(EventService);
  private eventSub?: Subscription;

  allShows: TVShow[] = [];
  shows: TVShow[] = [];
  searchQuery = '';

  // View States
  loading = true;
  error = '';

  ngOnInit(): void {
    this.fetchTVShows();
    this.eventSub = this.eventService.events$.subscribe(events => {
      const shouldRefresh = events.some(evt =>
        evt.type === 'media.created' || evt.type === 'media.updated' || evt.type === 'media.enriched'
      );
      if (shouldRefresh) {
        this.fetchTVShows(true);
      }
    });
  }

  ngOnDestroy(): void {
    if (this.eventSub) {
      this.eventSub.unsubscribe();
    }
  }

  fetchTVShows(silent = false): void {
    if (!silent) {
      this.loading = true;
    }
    this.error = '';

    // First fetch libraries, find tvshow libraries, and then fetch shows
    this.http.get<any[]>('/api/v1/libraries').pipe(
      map(libs => libs ? libs.filter(l => l.media_type === 'tvshow') : []),
      switchMap(tvLibs => {
        if (tvLibs.length === 0) {
          return of([]);
        }
        // Query /api/v1/tv/shows?library_id=xxx for each tv library
        const requests = tvLibs.map(lib => 
          this.http.get<TVShow[]>(`/api/v1/tv/shows?library_id=${lib.id}`)
        );
        return forkJoin(requests).pipe(
          map(results => results.reduce((acc, val) => acc.concat(val), []))
        );
      })
    ).subscribe({
      next: (data) => {
        this.allShows = data || [];
        this.filterShows();
        this.loading = false;
        this.cdr.detectChanges();
      },
      error: (err) => {
        this.error = 'Could not fetch TV shows library.';
        this.loading = false;
        this.cdr.detectChanges();
      }
    });
  }

  filterShows(): void {
    if (!this.searchQuery.trim()) {
      this.shows = [...this.allShows];
      return;
    }

    const q = this.searchQuery.toLowerCase();
    this.shows = this.allShows.filter(s => 
      s.name.toLowerCase().includes(q) || 
      (s.overview && s.overview.toLowerCase().includes(q))
    );
  }

  getShowPosterUrl(show: TVShow): string {
    if (show.poster_path) {
      return `/api/v1/tv/shows/${show.id}/poster`;
    }
    return 'https://images.unsplash.com/photo-1594909122845-11baa439b7bf?q=80&w=400&auto=format&fit=crop';
  }

  trackByShowId(index: number, show: TVShow): string {
    return show.id;
  }
}
