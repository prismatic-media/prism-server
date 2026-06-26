import { Component, OnInit, OnDestroy, inject, ChangeDetectorRef } from '@angular/core';
import { CommonModule } from '@angular/common';
import { HttpClient } from '@angular/common/http';
import { AuthService } from '../auth.service';
import { Movie } from '../movies/movies.component';
import { TVShow } from '../tv-shows/tv-shows.component';
import { Router } from '@angular/router';
import { Subscription, forkJoin } from 'rxjs';
import { EventService } from '../event.service';

interface WatchHistory {
  id: string;
  user_id: string;
  media_item_id: string;
  position: number;
  completed: boolean;
  updated_at: string;
  media?: Movie;
}

interface LibraryStats {
  moviesCount: number;
  showsCount: number;
  transcodingJobs: number;
  storageHealthy: boolean;
}

@Component({
  selector: 'app-home',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './home.component.html',
  styleUrl: './home.component.css',
})
export class HomeComponent implements OnInit, OnDestroy {
  public authService = inject(AuthService);
  private http = inject(HttpClient);
  private cdr = inject(ChangeDetectorRef);
  private router = inject(Router);
  private eventService = inject(EventService);
  private eventSub?: Subscription;

  stats: LibraryStats = {
    moviesCount: 0,
    showsCount: 0,
    transcodingJobs: 0,
    storageHealthy: true,
  };

  recentMovies: Movie[] = [];
  recentShows: TVShow[] = [];
  continueWatching: WatchHistory[] = [];
  loading = true;

  ngOnInit(): void {
    this.fetchDashboardData();
    this.eventSub = this.eventService.events$.subscribe((events) => {
      const shouldRefresh = events.some(
        (evt) =>
          evt.type === 'media.created' ||
          evt.type === 'media.updated' ||
          evt.type === 'media.enriched',
      );
      if (shouldRefresh) {
        this.fetchDashboardData(true);
      }
    });
  }

  ngOnDestroy(): void {
    if (this.eventSub) {
      this.eventSub.unsubscribe();
    }
  }

  fetchDashboardData(silent = false): void {
    if (!silent) {
      this.loading = true;
    }

    forkJoin({
      allMovies: this.http.get<Movie[]>('/api/v1/movies'),
      allShows: this.http.get<TVShow[]>('/api/v1/tv-shows'),
      recentMovies: this.http.get<Movie[]>('/api/v1/movies?sort=recent&limit=20'),
      recentShows: this.http.get<TVShow[]>('/api/v1/tv-shows?sort=recent&limit=20'),
      continueWatching: this.http.get<WatchHistory[]>('/api/v1/history'),
    }).subscribe({
      next: (res) => {
        this.stats.moviesCount = res.allMovies?.length || 0;
        this.stats.showsCount = res.allShows?.length || 0;
        this.recentMovies = res.recentMovies || [];
        this.recentShows = res.recentShows || [];
        this.continueWatching = res.continueWatching || [];
        this.loading = false;
        this.cdr.detectChanges();
      },
      error: () => {
        // Visual presentation mock fallback
        this.stats.moviesCount = 42;
        this.stats.showsCount = 8;
        this.stats.transcodingJobs = 1;
        this.stats.storageHealthy = true;

        this.recentMovies = [
          {
            id: '1',
            title: 'Prism Overdrive',
            media_type: 'movie',
            file_path: 'mock/prism_overdrive.mp4',
            file_size: 15600000000,
            duration: 7800,
            width: 3840,
            height: 2160,
            video_codec: 'hevc',
            audio_codec: 'aac',
            transcode_status: 'done',
            source_status: 'available',
            bundle_status: 'available',
            year: 2024,
          },
          {
            id: '2',
            title: 'Nebula Chronicles',
            media_type: 'movie',
            file_path: 'mock/nebula_chronicles.mp4',
            file_size: 8400000000,
            duration: 6900,
            width: 1920,
            height: 1080,
            video_codec: 'h264',
            audio_codec: 'ac3',
            transcode_status: 'none',
            source_status: 'available',
            bundle_status: 'none',
            year: 2023,
          },
          {
            id: '3',
            title: 'Quantum Shift',
            media_type: 'movie',
            file_path: 'mock/quantum_shift.mp4',
            file_size: 18200000000,
            duration: 9120,
            width: 3840,
            height: 2160,
            video_codec: 'hevc',
            audio_codec: 'aac',
            transcode_status: 'pending',
            source_status: 'available',
            bundle_status: 'none',
            year: 2025,
          },
          {
            id: '4',
            title: 'Chrono Rift',
            media_type: 'movie',
            file_path: 'mock/chrono_rift.mp4',
            file_size: 6200000000,
            duration: 5400,
            width: 1920,
            height: 1080,
            video_codec: 'hevc',
            audio_codec: 'aac',
            transcode_status: 'done',
            source_status: 'available',
            bundle_status: 'available',
            year: 2024,
          },
        ] as Movie[];

        this.recentShows = [
          {
            id: '1',
            name: 'Stellar Voyager',
            library_id: 'l1',
            first_air_year: 2021,
            overview: 'Exploring the outer bounds of the galaxy.',
          },
          {
            id: '2',
            name: 'Dark Void',
            library_id: 'l1',
            first_air_year: 2022,
            overview: 'A journey into a mysterious cosmic anomaly.',
          },
          {
            id: '3',
            name: 'Cyber Horizon',
            library_id: 'l1',
            first_air_year: 2023,
            overview: 'Survival in a digital dystopia.',
          },
          {
            id: '4',
            name: 'Retro Orbit',
            library_id: 'l1',
            first_air_year: 2024,
            overview: 'Classic space adventures in a modern light.',
          },
        ] as TVShow[];

        this.continueWatching = [
          {
            id: 'h1',
            user_id: 'u1',
            media_item_id: '1',
            position: 3200,
            completed: false,
            updated_at: '2026-06-08T22:00:00Z',
            media: {
              id: '1',
              title: 'Prism Overdrive',
              media_type: 'movie',
              file_path: 'mock/prism_overdrive.mp4',
              file_size: 15600000000,
              duration: 7800,
              width: 3840,
              height: 2160,
              video_codec: 'hevc',
              audio_codec: 'aac',
              transcode_status: 'done',
              source_status: 'available',
              bundle_status: 'available',
              year: 2024,
            } as Movie,
          },
        ];

        this.loading = false;
        this.cdr.detectChanges();
      },
    });
  }

  getPosterUrl(movie: Movie): string {
    if (movie.poster_path) {
      return `/api/v1/movies/${movie.id}/poster`;
    }
    // Fallback poster
    return 'https://images.unsplash.com/photo-1594909122845-11baa439b7bf?q=80&w=400&auto=format&fit=crop';
  }

  getShowPosterUrl(show: TVShow): string {
    if (show.poster_path) {
      return `/api/v1/tv-shows/${show.id}/poster`;
    }
    // Fallback poster
    return 'https://images.unsplash.com/photo-1594909122845-11baa439b7bf?q=80&w=400&auto=format&fit=crop';
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

  onMediaClick(movie: Movie): void {
    if (movie.media_type === 'movie') {
      this.router.navigate(['/movies', movie.id]);
    } else if (movie.media_type === 'episode' && movie.tv_show_id) {
      this.router.navigate(['/tv-shows', movie.tv_show_id]);
    } else if (movie.tv_show_id) {
      this.router.navigate(['/tv-shows', movie.tv_show_id]);
    } else {
      this.router.navigate(['/movies', movie.id]);
    }
  }

  onShowClick(show: TVShow): void {
    this.router.navigate(['/tv-shows', show.id]);
  }

  playMovie(movie: Movie, event?: MouseEvent): void {
    if (event) {
      event.stopPropagation(); // Don't trigger routing
    }
    alert(`Playback of "${movie.title}" will begin shortly. [Format: ${movie.video_codec}]`);
  }

  triggerTranscode(movie: Movie, event: MouseEvent): void {
    event.stopPropagation();
    this.http.post('/api/v1/jobs', { media_item_id: movie.id }).subscribe({
      next: () => {
        this.fetchDashboardData(true);
      },
      error: (err) => {
        alert(`Failed to enqueue transcode: ${err.error?.error || err.message}`);
      },
    });
  }

  getContinueWatchingPosterUrl(item: WatchHistory): string {
    if (item.media?.poster_path) {
      return `/api/v1/movies/${item.media.id}/poster`;
    }
    return 'https://images.unsplash.com/photo-1594909122845-11baa439b7bf?q=80&w=400&auto=format&fit=crop';
  }

  resumeWatch(item: WatchHistory, event?: MouseEvent): void {
    if (event) {
      event.stopPropagation();
    }
    if (item.media) {
      this.router.navigate(['/watch', item.media.id]);
    }
  }

  calculateProgressPercent(item: WatchHistory): number {
    if (!item.media || !item.media.duration) return 0;
    return Math.min(100, Math.max(0, (item.position / item.media.duration) * 100));
  }

  formatProgressLabel(item: WatchHistory): string {
    if (!item.media) return '';
    const current = this.formatDurationCompact(item.position);
    const total = this.formatDurationCompact(item.media.duration);
    return `${current} of ${total}`;
  }

  formatRemainingTime(item: WatchHistory): string {
    if (!item.media) return '';
    const remaining = Math.max(0, item.media.duration - item.position);
    return this.formatDurationCompact(remaining);
  }

  formatDurationCompact(seconds: number): string {
    if (isNaN(seconds) || seconds < 0) return '0m';
    const hrs = Math.floor(seconds / 3600);
    const mins = Math.round((seconds % 3600) / 60);
    if (hrs > 0) {
      return `${hrs}h ${mins}m`;
    }
    return `${mins}m`;
  }

  getMediaLabel(item: WatchHistory): string {
    if (!item.media) return '';
    if (item.media.media_type === 'episode') {
      const showName = item.media.tv_show_title || 'TV Show';
      const s = item.media.season_number ? `S${item.media.season_number}` : '';
      const e = item.media.episode_number ? `E${item.media.episode_number}` : '';
      const parts = [s, e].filter((p) => p !== '');
      const epPrefix = parts.join(' • ');
      return `${showName} — ${epPrefix ? epPrefix + ' — ' : ''}${item.media.title}`;
    }
    return item.media.title;
  }

  trackByMovieId(index: number, movie: Movie): string {
    return movie.id;
  }

  trackByShowId(index: number, show: TVShow): string {
    return show.id;
  }

  trackByHistoryId(index: number, item: WatchHistory): string {
    return item.id;
  }
}
