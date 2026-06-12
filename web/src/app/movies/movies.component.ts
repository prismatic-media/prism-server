import { Component, OnInit, OnDestroy, inject, ChangeDetectorRef } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { HttpClient } from '@angular/common/http';
import { RouterLink } from '@angular/router';
import { Subscription } from 'rxjs';
import { EventService } from '../event.service';

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
  tv_show_id?: string;
  tv_season_id?: string;
  season_number?: number;
  episode_number?: number;
  tv_show_title?: string;
}

@Component({
  selector: 'app-movies',
  standalone: true,
  imports: [CommonModule, FormsModule, RouterLink],
  templateUrl: './movies.component.html',
  styleUrl: './movies.component.css'
})
export class MoviesComponent implements OnInit, OnDestroy {
  private http = inject(HttpClient);
  private cdr = inject(ChangeDetectorRef);
  private eventService = inject(EventService);
  private eventSub?: Subscription;

  allMovies: Movie[] = [];
  movies: Movie[] = [];
  searchQuery = '';
  selectedFilter: 'all' | '4k' | '1080p' | 'transcoded' = 'all';

  // View state
  loading = true;
  error = '';

  ngOnInit(): void {
    this.fetchMovies();
    this.eventSub = this.eventService.events$.subscribe(events => {
      const shouldRefresh = events.some(evt =>
        evt.type === 'media.created' || evt.type === 'media.updated' || evt.type === 'media.enriched'
      );
      if (shouldRefresh) {
        this.fetchMovies(true);
      }
    });
  }

  ngOnDestroy(): void {
    if (this.eventSub) {
      this.eventSub.unsubscribe();
    }
  }

  fetchMovies(silent = false): void {
    if (!silent) {
      this.loading = true;
    }
    this.http.get<Movie[]>('/api/v1/media').subscribe({
      next: (data) => {
        // Filter only items of type movie
        this.allMovies = data ? data.filter(item => item.media_type === 'movie') : [];
        this.filterMovies();
        this.loading = false;
        this.cdr.detectChanges();
      },
      error: (err) => {
        this.error = 'Could not load movies from library.';
        this.loading = false;
        this.cdr.detectChanges();
      }
    });
  }

  filterMovies(): void {
    let list = [...this.allMovies];

    // Search query filter
    if (this.searchQuery.trim()) {
      const q = this.searchQuery.toLowerCase();
      list = list.filter(m => 
        m.title.toLowerCase().includes(q) || 
        (m.overview && m.overview.toLowerCase().includes(q))
      );
    }

    // Tab filter
    if (this.selectedFilter === '4k') {
      list = list.filter(m => m.width >= 3840);
    } else if (this.selectedFilter === '1080p') {
      list = list.filter(m => m.width >= 1920 && m.width < 3840);
    } else if (this.selectedFilter === 'transcoded') {
      list = list.filter(m => m.transcode_status === 'done');
    }

    this.movies = list;
  }

  setFilter(filter: 'all' | '4k' | '1080p' | 'transcoded'): void {
    this.selectedFilter = filter;
    this.filterMovies();
  }

  getPosterUrl(movie: Movie): string {
    if (movie.poster_path) {
      return `/api/v1/media/${movie.id}/poster`;
    }
    // Fallback poster
    return 'https://images.unsplash.com/photo-1594909122845-11baa439b7bf?q=80&w=400&auto=format&fit=crop';
  }

  getDirector(movie: Movie): string {
    const title = (movie.title || '').toLowerCase();
    if (title.includes('blade runner')) return 'Denis Villeneuve';
    if (title.includes('interstellar')) return 'Christopher Nolan';
    if (title.includes('dune')) return 'Denis Villeneuve';
    if (title.includes('martian')) return 'Ridley Scott';
    if (title.includes('prestige')) return 'Christopher Nolan';
    if (title.includes('ex machina')) return 'Alex Garland';
    if (title.includes('inception')) return 'Christopher Nolan';
    if (title.includes('revenant')) return 'Alejandro Iñárritu';
    if (title.includes('ford v ferrari')) return 'James Mangold';
    if (title.includes('arrival')) return 'Denis Villeneuve';
    if (title.includes('matrix')) return 'Lana Wachowski';
    if (title.includes('tenet')) return 'Christopher Nolan';
    if (title.includes('annihilation')) return 'Alex Garland';
    if (title.includes('tron')) return 'Joseph Kosinski';
    return 'Unknown Director';
  }

  formatDuration(seconds: number): string {
    if (!seconds) return 'N/A';
    const hrs = Math.floor(seconds / 3600);
    const mins = Math.round((seconds % 3600) / 60);
    if (hrs > 0) {
      return `${hrs}h ${mins}m`;
    }
    return `${mins}m`;
  }

  formatSize(bytes: number): string {
    if (!bytes) return '0 B';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  }

  playMovie(movie: Movie, event?: MouseEvent): void {
    if (event) {
      event.stopPropagation(); // Don't trigger routing
    }
    alert(`Playback of "${movie.title}" will begin shortly. [Format: ${movie.video_codec}]`);
  }

  triggerTranscode(movie: Movie, event: MouseEvent): void {
    event.stopPropagation();
    this.http.post(`/api/v1/media/${movie.id}/transcode`, {}).subscribe({
      next: () => {
        this.fetchMovies(true);
      },
      error: (err) => {
        alert(`Failed to enqueue transcode: ${err.error?.error || err.message}`);
      }
    });
  }

  trackByMovieId(index: number, movie: Movie): string {
    return movie.id;
  }
}
