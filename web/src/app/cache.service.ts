import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { BehaviorSubject, Observable, Subscription } from 'rxjs';
import { map } from 'rxjs/operators';
import { EventService } from './event.service';
import { AuthService } from './auth.service';
import { Movie } from './movies/movies.component';
import { TVShow } from './tv-shows/tv-shows.component';
import { Episode } from './media-details/media-details.component';
import { TranscodeJob } from './admin/transcoding/transcoding-admin.component';

@Injectable({
  providedIn: 'root',
})
export class CacheService {
  private http = inject(HttpClient);
  private eventService = inject(EventService);
  private authService = inject(AuthService);

  private moviesStore = new BehaviorSubject<Map<string, Movie> | null>(null);
  private tvShowsStore = new BehaviorSubject<Map<string, TVShow> | null>(null);
  private episodesStore = new BehaviorSubject<Map<string, Episode> | null>(null);
  private jobsStore = new BehaviorSubject<Map<string, TranscodeJob> | null>(null);

  public movies$: Observable<Movie[] | null> = this.moviesStore.asObservable().pipe(
    map((map) => (map ? Array.from(map.values()) : null)),
  );
  public tvShows$: Observable<TVShow[] | null> = this.tvShowsStore.asObservable().pipe(
    map((map) => (map ? Array.from(map.values()) : null)),
  );
  public episodes$: Observable<Episode[] | null> = this.episodesStore.asObservable().pipe(
    map((map) => (map ? Array.from(map.values()) : null)),
  );
  public jobs$: Observable<TranscodeJob[] | null> = this.jobsStore.asObservable().pipe(
    map((map) => (map ? Array.from(map.values()) : null)),
  );

  private eventSub?: Subscription;
  private authSub?: Subscription;
  private reconnectSub?: Subscription;
  private wasConnected = false;

  constructor() {
    this.eventSub = this.eventService.events$.subscribe((batch) => {
      this.handleEventBatch(batch);
    });

    this.authSub = this.authService.currentUser$.subscribe((user) => {
      if (!user) {
        this.clearAll();
      }
    });

    this.reconnectSub = this.eventService.connected$.subscribe((connected) => {
      if (connected && this.wasConnected) {
        this.reloadActiveCaches();
      }
      this.wasConnected = connected;
    });
  }

  public loadMovies(): void {
    if (this.moviesStore.getValue() !== null) return;
    this.reloadMovies();
  }

  public reloadMovies(): void {
    this.http.get<Movie[]>('/api/v1/movies').subscribe({
      next: (movies) => {
        const map = new Map<string, Movie>();
        (movies || []).forEach((m) => map.set(m.id, m));
        this.moviesStore.next(map);
      },
      error: (err) => console.error('Failed to fetch movies:', err),
    });
  }

  public loadTVShows(): void {
    if (this.tvShowsStore.getValue() !== null) return;
    this.reloadTVShows();
  }

  public reloadTVShows(): void {
    this.http.get<TVShow[]>('/api/v1/tv-shows').subscribe({
      next: (shows) => {
        const map = new Map<string, TVShow>();
        (shows || []).forEach((s) => map.set(s.id, s));
        this.tvShowsStore.next(map);
      },
      error: (err) => console.error('Failed to fetch TV shows:', err),
    });
  }

  public loadEpisodes(): void {
    if (this.episodesStore.getValue() !== null) return;
    this.reloadEpisodes();
  }

  public reloadEpisodes(): void {
    this.http.get<Episode[]>('/api/v1/episodes').subscribe({
      next: (episodes) => {
        const map = new Map<string, Episode>();
        (episodes || []).forEach((e) => map.set(e.id, e));
        this.episodesStore.next(map);
      },
      error: (err) => console.error('Failed to fetch episodes:', err),
    });
  }

  public loadJobs(): void {
    if (this.jobsStore.getValue() !== null) return;
    this.reloadJobs();
  }

  public reloadJobs(): void {
    this.http.get<TranscodeJob[]>('/api/v1/jobs').subscribe({
      next: (jobs) => {
        const map = new Map<string, TranscodeJob>();
        (jobs || []).forEach((j) => map.set(j.id, j));
        this.jobsStore.next(map);
      },
      error: (err) => console.error('Failed to fetch transcode jobs:', err),
    });
  }

  private reloadActiveCaches(): void {
    if (this.moviesStore.getValue() !== null) this.reloadMovies();
    if (this.tvShowsStore.getValue() !== null) this.reloadTVShows();
    if (this.episodesStore.getValue() !== null) this.reloadEpisodes();
    if (this.jobsStore.getValue() !== null) this.reloadJobs();
  }

  private clearAll(): void {
    this.moviesStore.next(null);
    this.tvShowsStore.next(null);
    this.episodesStore.next(null);
    this.jobsStore.next(null);
  }

  private handleEventBatch(batch: any[]): void {
    let moviesChanged = false;
    let tvShowsChanged = false;
    let episodesChanged = false;
    let jobsChanged = false;

    const moviesMap = this.moviesStore.getValue();
    const tvShowsMap = this.tvShowsStore.getValue();
    const episodesMap = this.episodesStore.getValue();
    const jobsMap = this.jobsStore.getValue();

    for (const evt of batch) {
      const payload = evt.payload;
      if (!payload) continue;

      switch (evt.type) {
        case 'media.created':
        case 'media.updated':
        case 'media.enriched': {
          const mediaItem = payload.media_item;
          if (!mediaItem) continue;

          if (mediaItem.media_type === 'movie' && moviesMap) {
            moviesMap.set(mediaItem.id, mediaItem as Movie);
            moviesChanged = true;
          } else if (mediaItem.media_type === 'episode' && episodesMap) {
            episodesMap.set(mediaItem.id, mediaItem as Episode);
            episodesChanged = true;
          }
          break;
        }

        case 'tvshow.created':
        case 'tvshow.updated': {
          const tvShow = payload.tv_show;
          if (tvShow && tvShowsMap) {
            tvShowsMap.set(tvShow.id, tvShow as TVShow);
            tvShowsChanged = true;
          }
          break;
        }

        case 'job.created':
        case 'job.updated': {
          const job = payload.job;
          if (job && jobsMap) {
            jobsMap.set(job.id, job as TranscodeJob);
            jobsChanged = true;
          }
          break;
        }

        case 'job.progress': {
          if (jobsMap) {
            const job = jobsMap.get(payload.job_id);
            if (job) {
              job.progress = payload.progress;
              if (payload.worker_id !== undefined) {
                job.worker_id = payload.worker_id;
              }
              if (payload.sub_jobs) {
                job.sub_jobs = payload.sub_jobs;
              }
              if (payload.done) {
                job.status = payload.error ? 'failed' : 'done';
                if (payload.error) {
                  job.error_msg = payload.error;
                }
                job.finished_at = new Date().toISOString();
              } else {
                job.status = 'processing';
              }
              jobsChanged = true;
            }
          }
          break;
        }
      }
    }

    if (moviesChanged && moviesMap) this.moviesStore.next(new Map(moviesMap));
    if (tvShowsChanged && tvShowsMap) this.tvShowsStore.next(new Map(tvShowsMap));
    if (episodesChanged && episodesMap) this.episodesStore.next(new Map(episodesMap));
    if (jobsChanged && jobsMap) this.jobsStore.next(new Map(jobsMap));
  }
}
