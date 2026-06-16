import { Component, OnInit, OnDestroy, inject, ChangeDetectorRef, NgZone } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { HttpClient } from '@angular/common/http';
import { RouterModule } from '@angular/router';
import { Subject, Subscription, of, timer } from 'rxjs';
import { debounce, switchMap, tap } from 'rxjs/operators';
import { AuthService } from '../../auth.service';
import { EventService } from '../../event.service';

export interface TranscodeJob {
  id: string;
  media_item_id: string;
  status: 'pending' | 'processing' | 'done' | 'failed';
  progress: number;
  priority: number;
  error_msg?: string;
  started_at?: string;
  finished_at?: string;
  created_at: string;

  // Custom mapped properties
  title?: string;
  file_path?: string;
  resolution?: string;
  codec?: string;
  duration?: number;
  formattedETA?: string;
}

export interface MediaItem {
  id: string;
  title: string;
  media_type: string;
  file_path: string;
  width: number;
  height: number;
  video_codec: string;
  duration: number;
  poster_path?: string;
  season_number?: number;
  episode_number?: number;
}

export interface TranscodeWorker {
  id: string;
  name: string;
  api_key?: string;
  threads: number;
  hwaccel: string;
  status: 'offline' | 'idle' | 'transcoding';
  last_heartbeat?: string;
  is_ephemeral?: boolean;
  created_at: string;
  updated_at: string;
}

export interface EphemeralWorkerToken {
  id: string;
  token: string;
  name: string;
  created_at: string;
}

@Component({
  selector: 'app-transcoding-admin',
  standalone: true,
  imports: [CommonModule, FormsModule, RouterModule],
  templateUrl: './transcoding-admin.component.html',
  styleUrl: './transcoding-admin.component.css',
})
export class TranscodingAdminComponent implements OnInit, OnDestroy {
  private http = inject(HttpClient);
  private cdr = inject(ChangeDetectorRef);
  private zone = inject(NgZone);
  protected authService = inject(AuthService);
  protected readonly Math = Math;

  loading = true;
  error = '';

  // Tabs: 'active' | 'completed' | 'settings'
  activeTab: 'active' | 'completed' | 'settings' = 'active';

  // Lists of jobs
  allJobs: TranscodeJob[] = [];
  activeJobs: TranscodeJob[] = [];
  completedJobs: TranscodeJob[] = [];
  workers: TranscodeWorker[] = [];

  // Transcode settings state
  ffmpegHwaccel = 'none';
  transcodeWorkers = 2;
  autoTranscodeOnDiscovery = false;

  // Local Pool Management Flipped Logic
  enableLocalTranscoder = true;
  previousTranscodeWorkers = 2;

  // Auto-save State
  saveStatus: 'idle' | 'saving' | 'saved' | 'error' = 'idle';
  private saveSubject = new Subject<{ immediate: boolean }>();
  private saveSubscription?: Subscription;
  private clearStatusTimeout: any = null;

  // Add Worker State
  newWorkerName = '';
  showApiKeyModal = false;
  showRegisterModal = false;
  newWorkerRegistered = false;
  generatedApiKey = '';

  // Ephemeral Token State
  ephemeralTokens: EphemeralWorkerToken[] = [];
  newEphemeralTokenName = '';
  generatedEphemeralToken = '';
  showEphemeralTokenModal = false;
  newEphemeralTokenCreated = false;

  // Mapped MediaItems cache
  mediaMap = new Map<string, MediaItem>();

  private eventService = inject(EventService);
  private eventSub?: Subscription;
  private etaIntervalId: any;

  ngOnInit(): void {
    this.fetchData();
    this.fetchSettings();
    this.initAutoSave();

    this.eventSub = this.eventService.events$.subscribe((events) => {
      let changed = false;
      let shouldFetchData = false;
      let shouldFetchJobs = false;

      for (const evt of events) {
        if (evt.type === 'job.progress') {
          const res = this.handleJobProgressEvent(evt.payload);
          if (res.changed) {
            changed = true;
          }
          if (res.shouldFetchJobs) {
            shouldFetchJobs = true;
          }
        } else if (
          evt.type === 'media.updated' ||
          evt.type === 'media.created' ||
          evt.type === 'media.enriched'
        ) {
          shouldFetchData = true;
        }
      }

      if (shouldFetchData) {
        this.fetchData();
      } else if (shouldFetchJobs) {
        this.fetchJobs();
      } else if (changed) {
        this.cdr.detectChanges();
      }
    });

    // Start interval to update ETAs dynamically every second
    this.etaIntervalId = setInterval(() => {
      this.updateActiveJobsETAs();
    }, 1000);
  }

  ngOnDestroy(): void {
    if (this.eventSub) {
      this.eventSub.unsubscribe();
    }
    if (this.etaIntervalId) {
      clearInterval(this.etaIntervalId);
    }
    if (this.saveSubscription) {
      this.saveSubscription.unsubscribe();
    }
    if (this.clearStatusTimeout) {
      clearTimeout(this.clearStatusTimeout);
    }
  }

  fetchData(): void {
    this.loading = true;
    this.error = '';

    // Fetch both media items and jobs to map them
    this.http.get<MediaItem[]>('/api/v1/media?all=true').subscribe({
      next: (mediaItems) => {
        this.mediaMap.clear();
        if (mediaItems) {
          mediaItems.forEach((item) => {
            this.mediaMap.set(item.id, item);
          });
        }

        // Fetch workers
        this.fetchWorkers();
        // Now fetch jobs
        this.fetchJobs();
      },
      error: (err) => {
        this.error = 'Failed to load media metadata.';
        this.loading = false;
        this.cdr.detectChanges();
      },
    });
  }

  fetchJobs(): void {
    this.http.get<TranscodeJob[]>('/api/v1/jobs').subscribe({
      next: (jobs) => {
        this.allJobs = jobs || [];
        this.mapJobsAndSplit();
        this.loading = false;
        this.cdr.detectChanges();
      },
      error: (err) => {
        this.error = 'Failed to load transcode jobs.';
        this.loading = false;
        this.cdr.detectChanges();
      },
    });
  }

  mapJobsAndSplit(): void {
    const active: TranscodeJob[] = [];
    const completed: TranscodeJob[] = [];

    this.allJobs.forEach((job) => {
      const media = this.mediaMap.get(job.media_item_id);
      if (media) {
        if (media.media_type === 'episode') {
          const showName = this.getShowNameFromPath(media.file_path);
          const season =
            media.season_number !== undefined ? `S${this.padZero(media.season_number)}` : '';
          const episode =
            media.episode_number !== undefined ? `E${this.padZero(media.episode_number)}` : '';
          const epCode = season && episode ? `${season}${episode}` : '';
          job.title = showName ? `${showName} - ${epCode} - ${media.title}` : media.title;
        } else {
          job.title = media.title;
        }
        job.file_path = media.file_path;
        job.resolution = this.getResolutionLabel(media.width, media.height);
        job.codec = media.video_codec ? media.video_codec.toUpperCase() : 'UNKNOWN';
        job.duration = media.duration;
      } else {
        job.title = 'Unknown File';
        job.resolution = 'Unknown';
        job.codec = 'UNKNOWN';
      }

      if (job.status === 'pending' || job.status === 'processing') {
        this.calculateETA(job);
        active.push(job);
      } else {
        completed.push(job);
      }
    });

    // Sort active jobs: processing first, then by priority (desc) and creation time (asc)
    this.activeJobs = active.sort((a, b) => {
      if (a.status === 'processing' && b.status !== 'processing') return -1;
      if (b.status === 'processing' && a.status !== 'processing') return 1;
      if (a.priority !== b.priority) {
        return b.priority - a.priority; // higher priority first
      }
      return new Date(a.created_at).getTime() - new Date(b.created_at).getTime(); // oldest created first
    });

    // Sort completed jobs: latest finished or created first
    this.completedJobs = completed.sort((a, b) => {
      const timeA = a.finished_at
        ? new Date(a.finished_at).getTime()
        : new Date(a.created_at).getTime();
      const timeB = b.finished_at
        ? new Date(b.finished_at).getTime()
        : new Date(b.created_at).getTime();
      return timeB - timeA;
    });
  }

  getShowNameFromPath(filePath?: string): string {
    if (!filePath) return 'Unknown Show';
    const parts = filePath.split('/');
    const fileName = parts[parts.length - 1];
    const cleanName = fileName.replace(/\.[^/.]+$/, ''); // strip extension
    const match = cleanName.match(/^(.+?)\s+-\s+s\d+e\d+\s+-\s+(.+)$/i);
    if (match && match[1]) {
      return match[1].trim();
    }
    // Fallback: check parts. If grandparent/parent is Season XX, then great-grandparent is Show Name.
    if (parts.length > 2) {
      const parent = parts[parts.length - 2];
      if (/^season\s+\d+/i.test(parent) && parts.length > 3) {
        return parts[parts.length - 3];
      }
      return parent; // fallback to parent directory
    }
    return 'Unknown Show';
  }

  padZero(num?: number): string {
    if (num === undefined || num === null) return '';
    return num.toString().padStart(2, '0');
  }

  getResolutionLabel(width: number, height: number): string {
    if (!width || !height) return 'SDR';
    if (width >= 3840 || height >= 2160) return '4K';
    if (width >= 1920 || height >= 1080) return '1080P';
    if (width >= 1280 || height >= 720) return '720P';
    if (width >= 854 || height >= 480) return '480P';
    return '360P';
  }

  getPosterUrl(job: TranscodeJob): string {
    return `/api/v1/media/${job.media_item_id}/poster`;
  }

  handleJobProgressEvent(payload: any): { changed: boolean; shouldFetchJobs: boolean } {
    let jobChanged = false;
    let shouldFetchJobs = false;

    // Find job in active jobs
    const activeIndex = this.activeJobs.findIndex((j) => j.id === payload.job_id);
    if (activeIndex !== -1) {
      const job = this.activeJobs[activeIndex];
      job.progress = payload.progress;
      if (payload.done) {
        job.status = payload.error ? 'failed' : 'done';
        if (payload.error) {
          job.error_msg = payload.error;
        }
        job.finished_at = new Date().toISOString();
        // Remove from active, add to completed
        this.activeJobs.splice(activeIndex, 1);
        this.completedJobs.unshift(job);
      } else {
        job.status = 'processing';
        this.calculateETA(job);
      }
      jobChanged = true;
    } else {
      // It might be a brand new job that was just enqueued
      const exists = this.allJobs.some((j) => j.id === payload.job_id);
      if (!exists) {
        shouldFetchJobs = true;
      }
    }

    return { changed: jobChanged, shouldFetchJobs };
  }

  // Calculate ETA for a processing job
  calculateETA(job: TranscodeJob): void {
    if (
      job.status !== 'processing' ||
      !job.started_at ||
      job.progress <= 0 ||
      job.progress >= 100
    ) {
      job.formattedETA = undefined;
      return;
    }

    const startedTime = new Date(job.started_at).getTime();
    const now = new Date().getTime();
    const elapsedSeconds = (now - startedTime) / 1000;

    if (elapsedSeconds <= 0) {
      job.formattedETA = '--:--';
      return;
    }

    const percentPerSecond = job.progress / elapsedSeconds;
    const remainingPercent = 100 - job.progress;
    const remainingSeconds = remainingPercent / percentPerSecond;

    if (isFinite(remainingSeconds) && remainingSeconds > 0) {
      job.formattedETA = this.formatSeconds(Math.round(remainingSeconds));
    } else {
      job.formattedETA = '--:--';
    }
  }

  updateActiveJobsETAs(): void {
    let updated = false;
    this.activeJobs.forEach((job) => {
      if (job.status === 'processing') {
        this.calculateETA(job);
        updated = true;
      }
    });
    if (updated) {
      this.cdr.detectChanges();
    }
  }

  // Format seconds to mm:ss or hh:mm:ss
  formatSeconds(totalSeconds: number): string {
    const hours = Math.floor(totalSeconds / 3600);
    const minutes = Math.floor((totalSeconds % 3600) / 60);
    const seconds = totalSeconds % 60;

    const pad = (num: number) => num.toString().padStart(2, '0');

    if (hours > 0) {
      return `${hours}:${pad(minutes)}:${pad(seconds)}`;
    }
    return `${pad(minutes)}:${pad(seconds)}`;
  }

  fetchSettings(): void {
    this.http.get<Record<string, string>>('/api/v1/admin/settings').subscribe({
      next: (settings) => {
        this.ffmpegHwaccel = settings['ffmpeg_hwaccel'] || 'none';
        const workers = parseInt(settings['transcode_workers'] || '2', 10);
        this.transcodeWorkers = workers;
        this.enableLocalTranscoder = workers > 0;
        if (workers > 0) {
          this.previousTranscodeWorkers = workers;
        }
        this.autoTranscodeOnDiscovery = settings['auto_transcode_on_discovery'] === 'true';
        this.cdr.detectChanges();
      },
      error: (err) => {
        console.error('Failed to load settings', err);
      },
    });
  }

  toggleLocalTranscoder(): void {
    this.enableLocalTranscoder = !this.enableLocalTranscoder;
    if (this.enableLocalTranscoder) {
      this.transcodeWorkers = this.previousTranscodeWorkers || 2;
    } else {
      if (this.transcodeWorkers > 0) {
        this.previousTranscodeWorkers = this.transcodeWorkers;
      }
      this.transcodeWorkers = 0;
    }
    this.onSettingChange(true);
  }

  onThreadCountChange(count: number): void {
    this.transcodeWorkers = count;
    if (count > 0) {
      this.previousTranscodeWorkers = count;
    }
    this.onSettingChange(true);
  }

  openRegisterModal(): void {
    this.zone.run(() => {
      this.newWorkerName = '';
      this.generatedApiKey = '';
      this.newWorkerRegistered = false;
      this.showRegisterModal = true;
      setTimeout(() => {
        this.cdr.detectChanges();
      }, 0);
    });
  }

  closeRegisterModal(): void {
    this.zone.run(() => {
      this.showRegisterModal = false;
      setTimeout(() => {
        this.cdr.detectChanges();
      }, 0);
    });
  }

  copyConfigYaml(): void {
    navigator.clipboard.writeText(this.getWorkerConfigYaml()).then(() => {
      // Config copied successfully
    });
  }

  copyStartCommand(): void {
    navigator.clipboard.writeText('./prism-worker -config worker_config.yaml').then(() => {
      // Command copied successfully
    });
  }

  getOrigin(): string {
    return window.location.origin;
  }

  private initAutoSave(): void {
    this.saveSubscription = this.saveSubject
      .pipe(
        debounce((event) => (event.immediate ? of(null) : timer(600))),
        switchMap(() => {
          this.saveStatus = 'saving';
          this.cdr.detectChanges();

          const payload: Record<string, string> = {
            ffmpeg_hwaccel: this.ffmpegHwaccel,
            transcode_workers: String(this.transcodeWorkers),
            auto_transcode_on_discovery: String(this.autoTranscodeOnDiscovery),
          };

          return this.http.put('/api/v1/admin/settings', payload).pipe(
            tap({
              next: () => {
                this.saveStatus = 'saved';
                this.cdr.detectChanges();

                if (this.clearStatusTimeout) {
                  clearTimeout(this.clearStatusTimeout);
                }
                this.clearStatusTimeout = setTimeout(() => {
                  this.saveStatus = 'idle';
                  this.cdr.detectChanges();
                }, 3000);
              },
              error: (err) => {
                this.saveStatus = 'error';
                this.cdr.detectChanges();
              },
            }),
          );
        }),
      )
      .subscribe();
  }

  onSettingChange(immediate: boolean): void {
    this.saveSubject.next({ immediate });
  }

  getSaveStatusLabel(): string {
    switch (this.saveStatus) {
      case 'saving':
        return 'Saving...';
      case 'saved':
        return 'Saved';
      case 'error':
        return 'Error';
      default:
        return '';
    }
  }

  // Workers CRUD operations
  fetchWorkers(): void {
    this.http.get<TranscodeWorker[]>('/api/v1/admin/workers').subscribe({
      next: (workers) => {
        this.zone.run(() => {
          this.workers = workers || [];
          setTimeout(() => {
            this.cdr.detectChanges();
          }, 0);
        });
        this.fetchEphemeralTokens();
      },
      error: (err) => {
        this.zone.run(() => {
          alert('Failed to load transcode workers.');
        });
      },
    });
  }

  createWorker(): void {
    if (!this.newWorkerName.trim()) return;

    // Blur active elements to avoid focus/keyboard lockups preventing UI updates
    if (document.activeElement instanceof HTMLElement) {
      document.activeElement.blur();
    }

    this.http
      .post<TranscodeWorker>('/api/v1/admin/workers', { name: this.newWorkerName.trim() })
      .subscribe({
        next: (worker) => {
          this.zone.run(() => {
            this.generatedApiKey = worker.api_key || '';
            this.newWorkerRegistered = true;
            this.fetchWorkers();

            // Force a full change detection cycle in the next VM turn to ensure template renders step 2
            setTimeout(() => {
              this.cdr.detectChanges();
            }, 0);
          });
        },
        error: (err) => {
          this.zone.run(() => {
            alert(`Failed to add worker: ${err.error?.error || err.message}`);
            setTimeout(() => {
              this.cdr.detectChanges();
            }, 0);
          });
        },
      });
  }

  getWorkerConfigYaml(): string {
    return `server_url: ${window.location.origin}
api_key: ${this.generatedApiKey}
scratch_dir: /tmp/prism-scratch`;
  }

  updateWorker(worker: TranscodeWorker): void {
    this.http
      .put<TranscodeWorker>(`/api/v1/admin/workers/${worker.id}`, {
        threads: worker.threads,
        hwaccel: worker.hwaccel,
      })
      .subscribe({
        next: () => {
          this.fetchWorkers();
        },
        error: (err) => {
          alert(`Failed to update worker settings: ${err.error?.error || err.message}`);
        },
      });
  }

  deleteWorker(id: string): void {
    if (
      !confirm(
        'Are you sure you want to delete this transcode worker? Any active jobs assigned to it will be orphaned.',
      )
    )
      return;
    this.http.delete(`/api/v1/admin/workers/${id}`).subscribe({
      next: () => {
        this.fetchWorkers();
      },
      error: (err) => {
        alert(`Failed to delete worker: ${err.error?.error || err.message}`);
      },
    });
  }

  // Actions
  bulkEnqueue(filter: 'untranscoded' | 'failed'): void {
    this.http.post<any>('/api/v1/jobs/bulk-enqueue', { filter }).subscribe({
      next: (res) => {
        const count = res.enqueued || 0;
        alert(`Successfully enqueued ${count} job(s) for transcode.`);
        this.fetchJobs();
      },
      error: (err) => {
        alert(`Failed to bulk enqueue jobs: ${err.error?.error || err.message}`);
      },
    });
  }

  prioritizeJob(jobId: string, event: MouseEvent): void {
    event.stopPropagation();
    this.http.post<any>(`/api/v1/jobs/${jobId}/prioritize`, {}).subscribe({
      next: () => {
        this.fetchJobs();
      },
      error: (err) => {
        alert(`Failed to prioritize job: ${err.error?.error || err.message}`);
      },
    });
  }

  retranscodeMedia(mediaId: string, event: MouseEvent): void {
    event.stopPropagation();
    this.http.post<any>(`/api/v1/media/${mediaId}/transcode`, {}).subscribe({
      next: () => {
        this.fetchJobs();
      },
      error: (err) => {
        alert(`Failed to enqueue transcode: ${err.error?.error || err.message}`);
      },
    });
  }

  // Formatter helpers
  formatBytes(bytes: number): string {
    if (!bytes) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
  }

  formatDuration(seconds: number): string {
    if (!seconds) return '00:00';
    const hrs = Math.floor(seconds / 3600);
    const mins = Math.floor((seconds % 3600) / 60);
    const secs = Math.round(seconds % 60);

    const pad = (num: number) => num.toString().padStart(2, '0');

    if (hrs > 0) {
      return `${hrs}h ${pad(mins)}m`;
    }
    return `${pad(mins)}:${pad(secs)}`;
  }

  formatDateTime(isoString?: string): string {
    if (!isoString) return '';
    const date = new Date(isoString);
    return date.toLocaleString();
  }

  fetchEphemeralTokens(): void {
    this.http.get<EphemeralWorkerToken[]>('/api/v1/admin/workers/ephemeral-tokens').subscribe({
      next: (tokens) => {
        this.zone.run(() => {
          this.ephemeralTokens = tokens || [];
          setTimeout(() => {
            this.cdr.detectChanges();
          }, 0);
        });
      },
      error: (err) => {
        console.error('Failed to load ephemeral worker tokens.', err);
      }
    });
  }

  createEphemeralToken(): void {
    if (!this.newEphemeralTokenName.trim()) return;

    if (document.activeElement instanceof HTMLElement) {
      document.activeElement.blur();
    }

    this.http
      .post<EphemeralWorkerToken>('/api/v1/admin/workers/ephemeral-tokens', { name: this.newEphemeralTokenName.trim() })
      .subscribe({
        next: (token) => {
          this.zone.run(() => {
            this.generatedEphemeralToken = token.token;
            this.newEphemeralTokenCreated = true;
            this.fetchEphemeralTokens();
            setTimeout(() => {
              this.cdr.detectChanges();
            }, 0);
          });
        },
        error: (err) => {
          this.zone.run(() => {
            alert(`Failed to create ephemeral token: ${err.error?.error || err.message}`);
            setTimeout(() => {
              this.cdr.detectChanges();
            }, 0);
          });
        }
      });
  }

  deleteEphemeralToken(id: string): void {
    if (!confirm('Are you sure you want to revoke this registration token? Existing ephemeral workers registered with this token will continue working, but no new workers can register with it.')) {
      return;
    }
    this.http.delete(`/api/v1/admin/workers/ephemeral-tokens/${id}`).subscribe({
      next: () => {
        this.fetchEphemeralTokens();
      },
      error: (err) => {
        alert(`Failed to delete token: ${err.error?.error || err.message}`);
      }
    });
  }

  openEphemeralTokenModal(): void {
    this.zone.run(() => {
      this.newEphemeralTokenName = '';
      this.generatedEphemeralToken = '';
      this.newEphemeralTokenCreated = false;
      this.showEphemeralTokenModal = true;
      setTimeout(() => {
        this.cdr.detectChanges();
      }, 0);
    });
  }

  closeEphemeralTokenModal(): void {
    this.zone.run(() => {
      this.showEphemeralTokenModal = false;
      setTimeout(() => {
        this.cdr.detectChanges();
      }, 0);
    });
  }

  copyEphemeralToken(): void {
    navigator.clipboard.writeText(this.generatedEphemeralToken).then(() => {
      // Copy success
    });
  }

  copyEphemeralStartCommand(): void {
    const command = `./prism-worker --ephemeral --token ${this.generatedEphemeralToken} --server ${window.location.origin}`;
    navigator.clipboard.writeText(command).then(() => {
      // Copy success
    });
  }
}
