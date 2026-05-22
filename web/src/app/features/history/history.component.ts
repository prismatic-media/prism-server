import { Component, OnInit, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink } from '@angular/router';
import { ApiService } from '../../core/services/api.service';
import type { WatchHistory, MediaItem } from '../../core/models';

interface HistoryRow {
  history: WatchHistory;
  media?: MediaItem;
}

@Component({
  selector: 'app-history',
  standalone: true,
  imports: [CommonModule, RouterLink],
  template: `
    <div class="history">
      <h2>Continue Watching</h2>
      @if (loading()) {
        <p class="state-msg">Loading…</p>
      } @else if (rows().length === 0) {
        <p class="state-msg">Nothing in progress. Start watching something!</p>
      } @else {
        <div class="list">
          @for (row of rows(); track row.history.id) {
            <div class="row">
              <a [routerLink]="['/media', row.history.media_item_id]" class="poster-link">
                @if (row.media?.poster_path) {
                  <img [src]="api.posterUrl(row.history.media_item_id)" [alt]="row.media?.title" />
                } @else {
                  <div class="no-poster">🎬</div>
                }
              </a>
              <div class="info">
                <a [routerLink]="['/media', row.history.media_item_id]" class="title">
                  {{ row.media?.title ?? row.history.media_item_id }}
                </a>
                <div class="progress-bar">
                  <div class="fill" [style.width.%]="pct(row)"></div>
                </div>
                <p class="pos">{{ formatPos(row.history.position) }} / {{ formatPos(row.media?.duration ?? 0) }}</p>
              </div>
              <a class="play-btn" [routerLink]="['/player', row.history.media_item_id]">▶</a>
            </div>
          }
        </div>
      }
    </div>
  `,
  styleUrl: './history.component.scss',
})
export class HistoryComponent implements OnInit {
  readonly api = inject(ApiService);

  rows = signal<HistoryRow[]>([]);
  loading = signal(true);

  ngOnInit() {
    this.api.getHistory().subscribe({
      next: (items) => {
        const histItems = items ?? [];
        // Load media details for each history entry.
        let remaining = histItems.length;
        if (remaining === 0) {
          this.rows.set([]);
          this.loading.set(false);
          return;
        }
        const rowMap = new Map<string, HistoryRow>(
          histItems.map((h) => [h.media_item_id, { history: h }]),
        );
        histItems.forEach((h) => {
          this.api.getMedia(h.media_item_id).subscribe({
            next: (m) => {
              const row = rowMap.get(h.media_item_id)!;
              row.media = m;
            },
            complete: () => {
              remaining--;
              if (remaining === 0) {
                this.rows.set(Array.from(rowMap.values()));
                this.loading.set(false);
              }
            },
          });
        });
      },
      error: () => this.loading.set(false),
    });
  }

  pct(row: HistoryRow): number {
    if (!row.media?.duration) return 0;
    return Math.min(100, (row.history.position / row.media.duration) * 100);
  }

  formatPos(s: number): string {
    const h = Math.floor(s / 3600);
    const m = Math.floor((s % 3600) / 60);
    const sec = Math.floor(s % 60);
    return h
      ? `${h}:${String(m).padStart(2, '0')}:${String(sec).padStart(2, '0')}`
      : `${m}:${String(sec).padStart(2, '0')}`;
  }
}
