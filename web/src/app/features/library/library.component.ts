import { Component, OnInit, DestroyRef, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { ApiService } from '../../core/services/api.service';
import { RealtimeService } from '../../core/services/realtime.service';
import type { MediaItem, Library } from '../../core/models';

@Component({
  selector: 'app-library',
  standalone: true,
  imports: [CommonModule, RouterLink, FormsModule],
  template: `
    <div class="library">
      <header class="library-header">
        <h2>Browse</h2>
        <div class="filters">
          <select [(ngModel)]="selectedLibrary" (change)="loadMedia()">
            <option value="">All Libraries</option>
            @for (lib of libraries(); track lib.id) {
              <option [value]="lib.id">{{ lib.name }}</option>
            }
          </select>
          <input
            type="search"
            placeholder="Search…"
            [(ngModel)]="searchQuery"
          />
        </div>
      </header>

      @if (loading()) {
        <p class="state-msg">Loading…</p>
      } @else if (filtered().length === 0) {
        <p class="state-msg">No media found.</p>
      } @else {
        <div class="grid">
          @for (item of filtered(); track item.id) {
            <a class="card" [routerLink]="['/media', item.id]">
              <div class="poster">
                @if (item.poster_path) {
                  <img [src]="api.posterUrl(item.id)" [alt]="item.title" loading="lazy" />
                } @else {
                  <div class="no-poster">🎬</div>
                }
                <span class="badge" [class]="item.transcode_status">
                  {{ item.transcode_status }}
                </span>
              </div>
              <div class="info">
                <p class="title">{{ item.title }}</p>
                @if (item.year) {
                  <p class="year">{{ item.year }}</p>
                }
              </div>
            </a>
          }
        </div>
      }
    </div>
  `,
  styleUrl: './library.component.scss',
})
export class LibraryComponent implements OnInit {
  readonly api = inject(ApiService);
  private readonly realtime = inject(RealtimeService);
  private readonly destroyRef = inject(DestroyRef);

  libraries = signal<Library[]>([]);
  media = signal<MediaItem[]>([]);
  loading = signal(true);
  selectedLibrary = '';
  searchQuery = '';

  filtered = signal<MediaItem[]>([]);

  ngOnInit() {
    this.api.listLibraries().subscribe((libs) => this.libraries.set(libs));
    this.loadMedia();

    // Live badge updates when transcode status changes.
    this.realtime.mediaUpdated$.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((evt) => {
      this.media.update((list) =>
        list.map((m) =>
          m.id === evt.media_item_id ? { ...m, transcode_status: evt.transcode_status as MediaItem['transcode_status'] } : m,
        ),
      );
      this.applyFilter();
    });

    // Reload when a new file is discovered by the scanner.
    this.realtime.mediaCreated$.pipe(takeUntilDestroyed(this.destroyRef)).subscribe(() => {
      this.loadMedia();
    });
  }

  loadMedia() {
    this.loading.set(true);
    this.api.listMedia(this.selectedLibrary || undefined).subscribe({
      next: (items) => {
        this.media.set(items ?? []);
        this.applyFilter();
        this.loading.set(false);
      },
      error: () => this.loading.set(false),
    });
  }

  applyFilter() {
    const q = this.searchQuery.toLowerCase();
    this.filtered.set(
      q ? this.media().filter((m) => m.title.toLowerCase().includes(q)) : this.media(),
    );
  }
}
