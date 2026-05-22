import { Component, OnInit, DestroyRef, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { ApiService } from '../../core/services/api.service';
import { RealtimeService } from '../../core/services/realtime.service';
import type { Library, TranscodeJob } from '../../core/models';

@Component({
  selector: 'app-admin',
  standalone: true,
  imports: [CommonModule, FormsModule],
  template: `
    <div class="admin">
      <h2>Admin Panel</h2>

      <!-- Libraries -->
      <section>
        <h3>Libraries</h3>
        <div class="lib-list">
          @for (lib of libraries(); track lib.id) {
            <div class="lib-row">
              <div class="lib-info">
                <strong>{{ lib.name }}</strong>
                <span class="path">{{ lib.path }}</span>
                <span class="type">{{ lib.media_type }}</span>
              </div>
              <div class="lib-actions">
                <button (click)="scan(lib.id)" [disabled]="scanning() === lib.id">
                  {{ scanning() === lib.id ? 'Scanning…' : 'Scan' }}
                </button>
                <button class="danger" (click)="deleteLib(lib.id)">Delete</button>
              </div>
            </div>
          }
        </div>

        <form class="add-lib" (ngSubmit)="addLibrary()">
          <h4>Add Library</h4>
          <div class="form-row">
            <input placeholder="Name" [(ngModel)]="newLib.name" name="name" required />
            <input placeholder="Path (e.g. /mnt/movies)" [(ngModel)]="newLib.path" name="path" required />
            <select [(ngModel)]="newLib.media_type" name="media_type">
              <option value="movie">Movies</option>
              <option value="tvshow">TV Shows</option>
              <option value="music">Music</option>
            </select>
            <button type="submit" [disabled]="addingLib()">
              {{ addingLib() ? 'Adding…' : 'Add' }}
            </button>
          </div>
          @if (libMsg()) {
            <p class="msg">{{ libMsg() }}</p>
          }
        </form>
      </section>

      <!-- Transcode Jobs -->
      <section>
        <div class="job-table-header">
          <h3>Transcode Jobs</h3>
          <button class="refresh-btn" (click)="loadJobs()">Refresh</button>
        </div>
        <table class="job-table">
          <thead>
            <tr>
              <th>Job ID</th>
              <th>Media</th>
              <th>Status</th>
              <th>Progress</th>
              <th>Started</th>
              <th>Finished</th>
            </tr>
          </thead>
          <tbody>
            @for (job of jobs(); track job.id) {
              <tr>
                <td class="mono">{{ job.id.slice(0, 8) }}…</td>
                <td class="mono">{{ job.media_item_id.slice(0, 8) }}…</td>
                <td><span class="badge {{ job.status }}">{{ job.status }}</span></td>
                <td>{{ job.progress | number: '1.0-0' }}%
                  @if (job.status === 'processing') {
                    <div class="progress-bar"><div class="progress-fill" [style.width.%]="job.progress"></div></div>
                  }
                </td>
                <td>{{ job.started_at ? (job.started_at | date: 'short') : '—' }}</td>
                <td>{{ job.finished_at ? (job.finished_at | date: 'short') : '—' }}</td>
              </tr>
            } @empty {
              <tr><td colspan="6" class="empty">No jobs found.</td></tr>
            }
          </tbody>
        </table>
      </section>
    </div>
  `,
  styleUrl: './admin.component.scss',
})
export class AdminComponent implements OnInit {
  private readonly api = inject(ApiService);
  private readonly realtime = inject(RealtimeService);
  private readonly destroyRef = inject(DestroyRef);

  libraries = signal<Library[]>([]);
  jobs = signal<TranscodeJob[]>([]);
  scanning = signal('');
  addingLib = signal(false);
  libMsg = signal('');
  newLib = { name: '', path: '', media_type: 'movie' };

  ngOnInit() {
    this.api.listLibraries().subscribe((libs) => this.libraries.set(libs ?? []));
    this.loadJobs();

    // Live updates for in-progress jobs.
    this.realtime.jobProgress$.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((evt) => {
      this.jobs.update((list) =>
        list.map((j) =>
          j.id === evt.job_id
            ? { ...j, progress: evt.progress, status: evt.done ? (evt.error ? 'failed' : 'done') : j.status }
            : j,
        ),
      );
    });
  }

  loadJobs() {
    this.api.listJobs().subscribe((jobs) => this.jobs.set(jobs ?? []));
  }

  scan(id: string) {
    this.scanning.set(id);
    this.api.scanLibrary(id).subscribe({
      next: () => this.scanning.set(''),
      error: () => this.scanning.set(''),
    });
  }

  deleteLib(id: string) {
    if (!confirm('Delete this library and all its media items?')) return;
    this.api.deleteLibrary(id).subscribe(() => {
      this.libraries.update((libs) => libs.filter((l) => l.id !== id));
    });
  }

  addLibrary() {
    if (!this.newLib.name || !this.newLib.path) return;
    this.addingLib.set(true);
    this.libMsg.set('');
    this.api.createLibrary(this.newLib).subscribe({
      next: (lib) => {
        this.libraries.update((libs) => [...libs, lib]);
        this.newLib = { name: '', path: '', media_type: 'movie' };
        this.addingLib.set(false);
        this.libMsg.set('Library added!');
      },
      error: (err) => {
        this.addingLib.set(false);
        this.libMsg.set(err?.error?.error ?? 'Failed to add library');
      },
    });
  }
}
