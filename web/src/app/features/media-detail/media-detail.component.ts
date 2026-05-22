import { Component, OnInit, DestroyRef, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { ApiService } from '../../core/services/api.service';
import { RealtimeService } from '../../core/services/realtime.service';
import type { MediaItem, WatchHistory } from '../../core/models';

@Component({
  selector: 'app-media-detail',
  standalone: true,
  imports: [CommonModule, RouterLink],
  template: `
    @if (loading()) {
      <p class="state-msg">Loading…</p>
    } @else if (!item()) {
      <p class="state-msg">Media not found.</p>
    } @else {
      <div class="detail">
        <div class="backdrop">
          @if (item()!.poster_path) {
            <img class="poster-bg" [src]="api.posterUrl(item()!.id)" aria-hidden="true" />
          }
          <div class="overlay"></div>
          <div class="hero">
            @if (item()!.poster_path) {
              <img class="poster" [src]="api.posterUrl(item()!.id)" [alt]="item()!.title" />
            }
            <div class="meta">
              <h1>{{ item()!.title }}</h1>
              <p class="sub">
                {{ item()!.year }}
                @if (item()!.year) { &bull; }
                {{ item()!.media_type }}
                &bull; {{ formatDuration(item()!.duration) }}
                &bull; {{ item()!.width }}×{{ item()!.height }}
              </p>
              @if (item()!.overview) {
                <p class="overview">{{ item()!.overview }}</p>
              }
              <div class="actions">
                @if (item()!.transcode_status === 'done') {
                  <a class="btn primary" [routerLink]="['/player', item()!.id]">
                    {{ history() ? '▶ Resume (' + formatPos(history()!.position) + ')' : '▶ Play' }}
                  </a>
                } @else if (item()!.transcode_status === 'processing') {
                  <div class="transcode-progress">
                    <span>Transcoding… {{ transcodeProgress() | number: '1.0-0' }}%</span>
                    <div class="progress-bar"><div class="progress-fill" [style.width.%]="transcodeProgress()"></div></div>
                  </div>
                } @else if (item()!.transcode_status === 'failed') {
                  <span class="btn danger">Transcode failed</span>
                  <button class="btn secondary" (click)="enqueue()">Retry</button>
                } @else {
                  <button class="btn secondary" (click)="enqueue()" [disabled]="enqueuing()">
                    {{ enqueuing() ? 'Queuing…' : 'Transcode' }}
                  </button>
                }
                <button class="btn secondary" (click)="goBack()">← Back</button>
              </div>
              @if (enqueueMsg()) {
                <p class="msg">{{ enqueueMsg() }}</p>
              }
            </div>
          </div>
        </div>

        <div class="tech-info">
          <h3>Technical info</h3>
          <dl>
            <dt>Video</dt><dd>{{ item()!.video_codec }}</dd>
            <dt>Audio</dt><dd>{{ item()!.audio_codec }}</dd>
            <dt>Size</dt><dd>{{ formatSize(item()!.file_size) }}</dd>
            <dt>Path</dt><dd>{{ item()!.file_path }}</dd>
            <dt>Transcode</dt><dd><span class="badge {{ item()!.transcode_status }}">{{ item()!.transcode_status }}</span></dd>
          </dl>
        </div>
      </div>
    }
  `,
  styleUrl: './media-detail.component.scss',
})
export class MediaDetailComponent implements OnInit {
  readonly api = inject(ApiService);
  private readonly realtime = inject(RealtimeService);
  private readonly destroyRef = inject(DestroyRef);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);

  item = signal<MediaItem | null>(null);
  history = signal<WatchHistory | null>(null);
  loading = signal(true);
  enqueuing = signal(false);
  enqueueMsg = signal('');
  transcodeProgress = signal(0);

  ngOnInit() {
    const id = this.route.snapshot.paramMap.get('id')!;
    this.api.getMedia(id).subscribe({
      next: (m) => {
        this.item.set(m);
        this.loading.set(false);
      },
      error: () => this.loading.set(false),
    });
    // Load history to show resume position.
    this.api.getHistory().subscribe({
      next: (items) => {
        const h = items.find((x) => x.media_item_id === id);
        this.history.set(h ?? null);
      },
    });

    // Live transcode progress.
    this.realtime.jobProgress$.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((evt) => {
      if (evt.media_item_id !== id) return;
      this.transcodeProgress.set(evt.progress);
    });

    // Live status changes (e.g. processing → done).
    this.realtime.mediaUpdated$.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((evt) => {
      if (evt.media_item_id !== id) return;
      this.item.update((m) => m ? { ...m, transcode_status: evt.transcode_status as MediaItem['transcode_status'] } : m);
      if (evt.transcode_status === 'done') {
        // Reload to get the MPD path.
        this.api.getMedia(id).subscribe((m) => this.item.set(m));
      }
    });
  }

  enqueue() {
    const id = this.item()!.id;
    this.enqueuing.set(true);
    this.api.enqueueTranscode(id).subscribe({
      next: () => {
        this.enqueuing.set(false);
        this.enqueueMsg.set('Transcode queued!');
        // Refresh item to show updated status.
        this.api.getMedia(id).subscribe((m) => this.item.set(m));
      },
      error: (err) => {
        this.enqueuing.set(false);
        this.enqueueMsg.set(err?.error?.error ?? 'Failed to enqueue');
      },
    });
  }

  goBack() { this.router.navigate(['/']); }

  formatDuration(s: number): string {
    const h = Math.floor(s / 3600);
    const m = Math.floor((s % 3600) / 60);
    return h ? `${h}h ${m}m` : `${m}m`;
  }

  formatPos(s: number): string {
    const h = Math.floor(s / 3600);
    const m = Math.floor((s % 3600) / 60);
    const sec = Math.floor(s % 60);
    return h
      ? `${h}:${String(m).padStart(2, '0')}:${String(sec).padStart(2, '0')}`
      : `${m}:${String(sec).padStart(2, '0')}`;
  }

  formatSize(bytes: number): string {
    if (bytes > 1e9) return `${(bytes / 1e9).toFixed(1)} GB`;
    return `${(bytes / 1e6).toFixed(0)} MB`;
  }
}
