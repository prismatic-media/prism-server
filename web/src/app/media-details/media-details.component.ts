import { Component, OnInit, OnDestroy, inject, ChangeDetectorRef } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { HttpClient } from '@angular/common/http';
import { forkJoin, map, of, switchMap, Subscription } from 'rxjs';
import { AuthService } from '../auth.service';
import { EventService } from '../event.service';
import { CastService } from '../cast.service';

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
  transcode_progress?: number;
  sub_jobs?: any[];
  mpd_path?: string;
  source_status: string;
  bundle_status: string;
  tv_show_id?: string;
  tv_season_id?: string;
  season_number?: number;
  episode_number?: number;
  tv_show_title?: string;
  director?: string;
  cast?: { name: string; character: string; profile_path: string }[];
  backdrop_path?: string;
  extra_posters?: string[];
}

export interface TVShow {
  id: string;
  library_id: string;
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
  transcode_progress?: number;
  sub_jobs?: any[];
  mpd_path?: string;
  source_status: string;
  bundle_status: string;
  poster_path?: string;
}

export interface WatchHistory {
  id: string;
  user_id: string;
  media_item_id: string;
  position: number;
  completed: boolean;
  updated_at: string;
}

export interface RenditionSize {
  resolution: string;
  size: number;
}

export interface TranscodeSizesInfo {
  renditions: RenditionSize[];
  total_size: number;
}

@Component({
  selector: 'app-media-details',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './media-details.component.html',
  styleUrl: './media-details.component.css',
})
export class MediaDetailsComponent implements OnInit, OnDestroy {
  private route = inject(ActivatedRoute);
  private router = inject(Router);
  private http = inject(HttpClient);
  private cdr = inject(ChangeDetectorRef);
  public authService = inject(AuthService);
  private eventService = inject(EventService);
  public castService = inject(CastService);

  protected readonly Math = Math;

  private eventSub?: Subscription;
  private castSub?: Subscription;

  mediaType: 'movie' | 'tvshow' = 'movie';
  id = '';

  // Movie Data
  movie: Movie | null = null;

  // Transcode Sizes
  transcodeSizes: TranscodeSizesInfo | null = null;
  transcodeSizesLoading = false;
  transcodeSizesError = '';

  // TV Show Data
  tvShow: TVShow | null = null;
  seasons: TVSeason[] = [];
  selectedSeason: TVSeason | null = null;
  episodes: Episode[] = [];

  // View States
  loading = true;
  error = '';
  seasonsLoading = false;
  episodesLoading = false;
  watchHistoryList: WatchHistory[] = [];

  // Subtitles modal state
  showSubtitlesModal = false;
  subtitleMediaId = '';
  subtitleMediaTitle = '';
  uploadedSubtitles: any[] = [];
  selectedLanguage = 'eng';
  customLanguage = '';
  subtitleLabel = 'English';
  selectedFile: File | null = null;
  subtitlesLoading = false;
  subtitlesError = '';

  ngOnInit(): void {
    // Determine media type from URL path
    const urlSegment = this.route.snapshot.url[0]?.path || '';
    if (urlSegment === 'tv-shows') {
      this.mediaType = 'tvshow';
    } else {
      this.mediaType = 'movie';
    }

    this.route.paramMap.subscribe((params) => {
      this.id = params.get('id') || '';
      if (this.id) {
        this.loadDetails();
      }
    });

    this.eventSub = this.eventService.events$.subscribe((events) => {
      let changed = false;
      let shouldReloadDetails = false;
      let shouldReloadSubtitles = false;

      for (const evt of events) {
        if (evt.type === 'job.progress') {
          if (this.handleJobProgressEvent(evt.payload)) {
            changed = true;
          }
        } else if (evt.type === 'media.updated' || evt.type === 'media.created') {
          if (this.handleMediaUpdatedEvent(evt.payload)) {
            changed = true;
          }
        } else if (evt.type === 'media.enriched') {
          const payload = evt.payload;
          if (this.mediaType === 'movie' && this.movie && this.movie.id === payload.media_item_id) {
            shouldReloadDetails = true;
          } else if (this.mediaType === 'tvshow') {
            if (
              this.tvShow &&
              (this.tvShow.id === payload.media_item_id ||
                this.episodes.some((e) => e.id === payload.media_item_id))
            ) {
              shouldReloadDetails = true;
            }
          }
        } else if (evt.type === 'subtitle.aligned') {
          const payload = evt.payload;
          if (this.showSubtitlesModal && payload.media_item_id === this.subtitleMediaId) {
            shouldReloadSubtitles = true;
          }
        }
      }

      if (shouldReloadDetails) {
        this.loadDetails(true);
      } else if (shouldReloadSubtitles) {
        this.loadUploadedSubtitles();
      } else if (changed) {
        this.cdr.detectChanges();
      }
    });

    this.castSub = this.castService.isConnected$.subscribe((connected) => {
      if (connected) {
        if (this.mediaType === 'movie' && this.movie) {
          this.castService.showPreview(this.movie);
        } else if (this.mediaType === 'tvshow' && this.tvShow) {
          this.castService.showPreview(this.tvShow);
        }
      }
    });
  }

  ngOnDestroy(): void {
    if (this.eventSub) {
      this.eventSub.unsubscribe();
    }
    if (this.castSub) {
      this.castSub.unsubscribe();
    }
    this.castService.clearPreview();
  }

  loadDetails(silent = false): void {
    if (!silent) {
      this.loading = true;
      this.error = '';
      this.movie = null;
      this.tvShow = null;
      this.seasons = [];
      this.selectedSeason = null;
      this.episodes = [];
      this.transcodeSizes = null;
    }

    this.http.get<WatchHistory[]>('/api/v1/history').subscribe({
      next: (historyList) => {
        this.watchHistoryList = historyList || [];
        this.cdr.detectChanges();
      },
    });

    if (this.mediaType === 'movie') {
      this.http.get<Movie>(`/api/v1/movies/${this.id}`).subscribe({
        next: (data) => {
          this.movie = data;
          this.loading = false;
          this.cdr.detectChanges();
          this.castService.showPreview(data);
          this.loadTranscodeSizes(this.id);
        },
        error: (err) => {
          if (!silent) {
            this.error = 'Could not load movie details.';
            this.movie = null;
          }
          this.loading = false;
          this.cdr.detectChanges();
        },
      });
    } else {
      this.http.get<TVShow>(`/api/v1/tv-shows/${this.id}`).subscribe({
        next: (data) => {
          this.tvShow = data;
          this.loadSeasons(this.id, silent);
          this.castService.showPreview(data);
        },
        error: (err) => {
          if (!silent) {
            this.error = 'Could not load TV show details.';
            this.tvShow = null;
          }
          this.loading = false;
          this.cdr.detectChanges();
        },
      });
    }
  }

  loadSeasons(showId: string, silent = false): void {
    if (!silent) {
      this.seasonsLoading = true;
    }
    this.http.get<TVSeason[]>(`/api/v1/tv-shows/${showId}/seasons`).subscribe({
      next: (seasonsList) => {
        const prevSelectedSeasonNumber = this.selectedSeason
          ? this.selectedSeason.season_number
          : null;
        this.seasons = seasonsList
          ? seasonsList.sort((a, b) => a.season_number - b.season_number)
          : [];
        this.seasonsLoading = false;
        this.loading = false;

        if (this.seasons.length > 0) {
          let toSelect = this.seasons[0];
          if (prevSelectedSeasonNumber !== null) {
            const found = this.seasons.find((s) => s.season_number === prevSelectedSeasonNumber);
            if (found) {
              toSelect = found;
            }
          }
          this.selectSeason(toSelect, silent);
        }
        this.cdr.detectChanges();
      },
      error: () => {
        this.seasonsLoading = false;
        this.loading = false;
        this.cdr.detectChanges();
      },
    });
  }

  selectSeason(season: TVSeason, silent = false): void {
    this.selectedSeason = season;
    if (!silent) {
      this.episodes = [];
    }
    if (this.tvShow) {
      this.loadEpisodes(this.tvShow.id, season.season_number, silent);
    }
  }

  loadEpisodes(showId: string, seasonNumber: number, silent = false): void {
    if (!silent) {
      this.episodesLoading = true;
    }
    this.http
      .get<Episode[]>(`/api/v1/tv-shows/${showId}/seasons/${seasonNumber}/episodes`)
      .subscribe({
        next: (episodesList) => {
          this.episodes = episodesList
            ? episodesList.sort((a, b) => a.episode_number - b.episode_number)
            : [];
          this.episodesLoading = false;
          this.cdr.detectChanges();
        },
        error: () => {
          this.episodesLoading = false;
          this.cdr.detectChanges();
        },
      });
  }

  loadTranscodeSizes(mediaId: string): void {
    this.transcodeSizesLoading = true;
    this.transcodeSizesError = '';
    this.http.get<TranscodeSizesInfo>(`/api/v1/movies/${mediaId}/transcode-sizes`).subscribe({
      next: (info) => {
        this.transcodeSizes = info;
        this.transcodeSizesLoading = false;
        this.cdr.detectChanges();
      },
      error: (err) => {
        this.transcodeSizesError = 'Failed to load transcode sizes';
        this.transcodeSizesLoading = false;
        this.cdr.detectChanges();
      }
    });
  }

  getPosterUrl(): string {
    if (this.mediaType === 'movie' && this.movie) {
      if (this.movie.poster_path) {
        return `/api/v1/movies/${this.movie.id}/poster`;
      }
    } else if (this.mediaType === 'tvshow' && this.tvShow) {
      if (this.tvShow.poster_path) {
        return `/api/v1/tv-shows/${this.tvShow.id}/poster`;
      }
    }
    return 'https://images.unsplash.com/photo-1594909122845-11baa439b7bf?q=80&w=400&auto=format&fit=crop';
  }

  getSeasonPosterUrl(season: TVSeason): string {
    if (season && season.poster_path && this.tvShow) {
      return `/api/v1/tv-shows/${this.tvShow.id}/seasons/${season.season_number}/poster`;
    }
    return 'https://images.unsplash.com/photo-1594909122845-11baa439b7bf?q=80&w=400&auto=format&fit=crop';
  }

  getEpisodeStillUrl(ep: Episode): string {
    if (ep && ep.poster_path) {
      return `/api/v1/movies/${ep.id}/poster`;
    }
    return 'https://images.unsplash.com/photo-1574267431629-2e570984a62f?q=80&w=400&auto=format&fit=crop';
  }

  goBack(): void {
    if (this.mediaType === 'movie') {
      this.router.navigate(['/movies']);
    } else {
      this.router.navigate(['/tv-shows']);
    }
  }

  playMovie(movie: Movie): void {
    this.router.navigate(['/watch', movie.id]);
  }

  playMovieFromBeginning(movie: Movie): void {
    this.router.navigate(['/watch', movie.id], { queryParams: { startOver: true } });
  }

  playEpisode(episode: Episode, event: MouseEvent): void {
    event.stopPropagation();
    this.router.navigate(['/watch', episode.id]);
  }

  playEpisodeFromBeginning(episode: Episode, event: MouseEvent): void {
    event.stopPropagation();
    this.router.navigate(['/watch', episode.id], { queryParams: { startOver: true } });
  }

  previewEpisode(ep: Episode): void {
    const previewItem = {
      ...ep,
      tv_show_title: this.tvShow?.name,
      backdrop_path: this.tvShow?.backdrop_path,
      media_type: 'episode',
    };
    this.castService.showPreview(previewItem);
  }

  restoreShowPreview(): void {
    if (this.tvShow) {
      this.castService.showPreview(this.tvShow);
    }
  }

  triggerTranscode(item: Movie | Episode, event?: MouseEvent): void {
    if (event) {
      event.stopPropagation();
    }
    const isDone = item.transcode_status === 'done';
    this.http.post('/api/v1/jobs', { media_item_id: item.id, force: isDone }).subscribe({
      next: () => {
        item.transcode_status = 'pending';
        this.cdr.detectChanges();
        // If it's the main movie, reload after a bit or update state
        if (this.mediaType === 'movie' && this.movie && this.movie.id === item.id) {
          this.movie.transcode_status = 'pending';
        }
      },
      error: (err) => {
        alert(`Failed to enqueue transcode: ${err.error?.error || err.message}`);
      },
    });
  }

  formatDuration(seconds: number | undefined): string {
    if (!seconds) return 'N/A';
    const hrs = Math.floor(seconds / 3600);
    const mins = Math.round((seconds % 3600) / 60);
    if (hrs > 0) {
      return `${hrs}h ${mins}m`;
    }
    return `${mins}m`;
  }

  formatSize(bytes: number | undefined): string {
    if (!bytes) return '0 B';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  }

  getBackdropUrl(): string {
    if (this.mediaType === 'movie' && this.movie) {
      if (this.movie.backdrop_path) {
        return `/api/v1/movies/${this.movie.id}/backdrop`;
      }
    } else if (this.mediaType === 'tvshow' && this.tvShow) {
      if (this.tvShow.backdrop_path) {
        return `/api/v1/tv-shows/${this.tvShow.id}/backdrop`;
      }
    }
    return 'https://images.unsplash.com/photo-1574267431629-2e570984a62f?q=80&w=1600&auto=format&fit=crop';
  }

  getExtraPosterUrls(): string[] {
    const urls: string[] = [];
    if (this.mediaType === 'movie' && this.movie && this.movie.extra_posters) {
      for (let i = 0; i < this.movie.extra_posters.length; i++) {
        urls.push(`/api/v1/movies/${this.movie.id}/extra-posters/${i}`);
      }
    } else if (this.mediaType === 'tvshow' && this.tvShow && this.tvShow.extra_posters) {
      for (let i = 0; i < this.tvShow.extra_posters.length; i++) {
        urls.push(`/api/v1/tv-shows/${this.tvShow.id}/extra-posters/${i}`);
      }
    }
    return urls;
  }

  handleJobProgressEvent(payload: any): boolean {
    let changed = false;
    if (this.mediaType === 'movie' && this.movie && this.movie.id === payload.media_item_id) {
      this.movie.transcode_progress = payload.progress;
      this.movie.sub_jobs = payload.sub_jobs;
      if (payload.done) {
        this.movie.transcode_status = payload.error ? 'failed' : 'done';
      } else {
        this.movie.transcode_status = 'processing';
      }
      changed = true;
    } else if (this.mediaType === 'tvshow' && this.episodes) {
      const ep = this.episodes.find((e) => e.id === payload.media_item_id);
      if (ep) {
        ep.transcode_progress = payload.progress;
        ep.sub_jobs = payload.sub_jobs;
        if (payload.done) {
          ep.transcode_status = payload.error ? 'failed' : 'done';
        } else {
          ep.transcode_status = 'processing';
        }
        changed = true;
      }
    }
    return changed;
  }

  handleMediaUpdatedEvent(payload: any): boolean {
    let changed = false;
    if (this.mediaType === 'movie' && this.movie && this.movie.id === payload.media_item_id) {
      this.movie.transcode_status = payload.transcode_status;
      if (payload.transcode_status === 'done') {
        this.loadTranscodeSizes(this.movie.id);
      }
      changed = true;
    } else if (this.mediaType === 'tvshow' && this.episodes) {
      const ep = this.episodes.find((e) => e.id === payload.media_item_id);
      if (ep) {
        ep.transcode_status = payload.transcode_status;
        changed = true;
      }
    }
    return changed;
  }

  getMovieHistory(): WatchHistory | undefined {
    if (!this.movie) return undefined;
    return this.watchHistoryList.find((h) => h.media_item_id === this.movie!.id);
  }

  getEpisodeHistory(ep: Episode): WatchHistory | undefined {
    return this.watchHistoryList.find((h) => h.media_item_id === ep.id);
  }

  calculateProgressPercent(hist: WatchHistory, duration: number): number {
    if (!duration) return 0;
    return Math.min(100, Math.max(0, (hist.position / duration) * 100));
  }

  formatRemainingTime(hist: WatchHistory, duration: number): string {
    if (!duration) return '0m';
    const remaining = Math.max(0, duration - hist.position);
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

  trackByEpisodeId(index: number, ep: Episode): string {
    return ep.id;
  }

  trackBySeasonId(index: number, season: TVSeason): string {
    return season.id;
  }

  trackByCastName(index: number, member: { name: string; character: string }): string {
    return member.name + '-' + member.character;
  }

  trackByUrl(index: number, url: string): string {
    return url;
  }

  openSubtitlesModal(mediaId: string, title: string, event?: Event): void {
    if (event) {
      event.stopPropagation();
    }
    this.subtitleMediaId = mediaId;
    this.subtitleMediaTitle = title;
    this.showSubtitlesModal = true;
    this.uploadedSubtitles = [];
    this.subtitlesError = '';
    this.selectedLanguage = 'eng';
    this.customLanguage = '';
    this.subtitleLabel = 'English';
    this.selectedFile = null;
    this.loadUploadedSubtitles();
  }

  closeSubtitlesModal(): void {
    this.showSubtitlesModal = false;
  }

  loadUploadedSubtitles(): void {
    this.subtitlesLoading = true;
    this.http.get<any[]>(`/api/v1/media/${this.subtitleMediaId}/subtitles`).subscribe({
      next: (subs) => {
        this.uploadedSubtitles = subs;
        this.subtitlesLoading = false;
        this.cdr.detectChanges();
      },
      error: (err) => {
        console.error('Failed to load subtitles:', err);
        this.subtitlesError = 'Failed to load subtitles list.';
        this.subtitlesLoading = false;
        this.cdr.detectChanges();
      }
    });
  }

  onLanguageChange(): void {
    if (this.selectedLanguage === 'eng') this.subtitleLabel = 'English';
    else if (this.selectedLanguage === 'spa') this.subtitleLabel = 'Spanish';
    else if (this.selectedLanguage === 'fra') this.subtitleLabel = 'French';
    else if (this.selectedLanguage === 'deu') this.subtitleLabel = 'German';
    else if (this.selectedLanguage === 'ita') this.subtitleLabel = 'Italian';
    else if (this.selectedLanguage === 'jpn') this.subtitleLabel = 'Japanese';
    else if (this.selectedLanguage === 'zho') this.subtitleLabel = 'Chinese';
    else if (this.selectedLanguage === 'rus') this.subtitleLabel = 'Russian';
    else if (this.selectedLanguage === 'other') this.subtitleLabel = '';
  }

  onFileSelected(event: any): void {
    const file = event.target.files?.[0];
    if (file) {
      const ext = file.name.split('.').pop()?.toLowerCase();
      if (ext !== 'srt') {
        this.subtitlesError = 'Only .srt files are supported.';
        this.selectedFile = null;
        return;
      }
      this.selectedFile = file;
      this.subtitlesError = '';
    }
  }

  uploadSubtitle(): void {
    if (!this.selectedFile) {
      this.subtitlesError = 'Please select an SRT file to upload.';
      return;
    }

    const lang = this.selectedLanguage === 'other' ? this.customLanguage.trim().toLowerCase() : this.selectedLanguage;
    if (!lang || lang.length !== 3) {
      this.subtitlesError = 'Please enter a valid 3-letter language code (e.g. eng, spa).';
      return;
    }

    const label = this.subtitleLabel.trim();
    if (!label) {
      this.subtitlesError = 'Please provide a track label.';
      return;
    }

    this.subtitlesLoading = true;
    const formData = new FormData();
    formData.append('file', this.selectedFile);
    formData.append('language', lang);
    formData.append('label', label);

    this.http.post(`/api/v1/media/${this.subtitleMediaId}/subtitles`, formData).subscribe({
      next: () => {
        this.selectedFile = null;
        const fileInput = document.getElementById('subtitle-file-input') as HTMLInputElement;
        if (fileInput) fileInput.value = '';
        this.loadUploadedSubtitles();
      },
      error: (err) => {
        console.error('Failed to upload subtitle:', err);
        this.subtitlesError = err.error?.message || 'Failed to upload subtitle file.';
        this.subtitlesLoading = false;
        this.cdr.detectChanges();
      }
    });
  }

  deleteSubtitle(id: string): void {
    if (!confirm('Are you sure you want to delete this subtitle track?')) {
      return;
    }

    this.http.delete(`/api/v1/media/${this.subtitleMediaId}/subtitles/${id}`).subscribe({
      next: () => {
        this.loadUploadedSubtitles();
      },
      error: (err) => {
        console.error('Failed to delete subtitle:', err);
        this.subtitlesError = 'Failed to delete subtitle.';
        this.subtitlesLoading = false;
        this.cdr.detectChanges();
      }
    });
  }

  triggerAutoSync(id: string): void {
    this.subtitlesLoading = true;
    this.subtitlesError = '';
    this.http.post(`/api/v1/media/${this.subtitleMediaId}/subtitles/${id}:sync`, {}).subscribe({
      next: () => {
        this.loadUploadedSubtitles();
      },
      error: (err) => {
        console.error('Failed to trigger auto-sync:', err);
        this.subtitlesError = err.error?.message || 'Failed to start auto-sync.';
        this.subtitlesLoading = false;
        this.cdr.detectChanges();
      }
    });
  }

  applyManualShift(id: string, offsetStr: string): void {
    const offset = parseFloat(offsetStr);
    if (isNaN(offset)) {
      this.subtitlesError = 'Please enter a valid number of seconds.';
      return;
    }

    this.subtitlesLoading = true;
    this.subtitlesError = '';
    this.http.post(`/api/v1/media/${this.subtitleMediaId}/subtitles/${id}:sync`, { offset }).subscribe({
      next: () => {
        this.loadUploadedSubtitles();
      },
      error: (err) => {
        console.error('Failed to apply manual shift:', err);
        this.subtitlesError = err.error?.message || 'Failed to apply time shift.';
        this.subtitlesLoading = false;
        this.cdr.detectChanges();
      }
    });
  }
}
