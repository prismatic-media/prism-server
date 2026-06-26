import { Component, OnInit, OnDestroy, inject, ChangeDetectorRef } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { HttpClient } from '@angular/common/http';
import { RouterModule } from '@angular/router';
import { Subject, Subscription, of, timer } from 'rxjs';
import { debounce, switchMap, tap } from 'rxjs/operators';

@Component({
  selector: 'app-settings-admin',
  standalone: true,
  imports: [CommonModule, FormsModule, RouterModule],
  templateUrl: './settings-admin.component.html',
  styleUrls: ['./settings-admin.component.css'],
})
export class SettingsAdminComponent implements OnInit, OnDestroy {
  private http = inject(HttpClient);
  private cdr = inject(ChangeDetectorRef);

  // Form Model State
  thumbsDir = '';
  tmdbApiKey = '';
  castReceiverAppId = '';
  whisperBinaryPath = '';
  whisperModelPath = '';

  // Copy of settings for comparison
  private originalSettings: Record<string, string> = {};

  // Auto-save State
  saveStatus: 'idle' | 'saving' | 'saved' | 'error' = 'idle';
  private saveSubject = new Subject<{ immediate: boolean }>();
  private saveSubscription?: Subscription;
  private clearStatusTimeout: any = null;

  // UI States
  loading = true;
  error = '';

  showTmdbKey = false;
  showCastId = false;

  // Path Autocomplete/Browser
  thumbsSuggestions: string[] = [];
  activeInput: 'thumbs' | null = null;

  ngOnInit(): void {
    this.fetchSettings();
    this.initAutoSave();
  }

  ngOnDestroy(): void {
    this.saveSubscription?.unsubscribe();
    if (this.clearStatusTimeout) {
      clearTimeout(this.clearStatusTimeout);
    }
  }

  private initAutoSave(): void {
    this.saveSubscription = this.saveSubject
      .pipe(
        debounce((event) => (event.immediate ? of(null) : timer(600))),
        switchMap(() => {
          this.saveStatus = 'saving';
          this.error = '';
          this.cdr.detectChanges();

          const payload: Record<string, string> = {
            thumbs_dir: this.thumbsDir.trim(),
            tmdb_api_key: this.tmdbApiKey.trim(),
            cast_receiver_app_id: this.castReceiverAppId.trim(),
            whisper_binary_path: this.whisperBinaryPath.trim(),
            whisper_model_path: this.whisperModelPath.trim(),
          };

          return this.http.put('/api/v1/admin/settings', payload).pipe(
            tap({
              next: () => {
                this.originalSettings = {
                  ...this.originalSettings,
                  ...payload,
                };
                this.saveStatus = 'saved';
                this.error = '';
                this.cdr.detectChanges();

                // Clear saved status after 3 seconds, returning to idle
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
                this.error = err.error?.error || 'Failed to save settings.';
                this.cdr.detectChanges();
              },
            }),
          );
        }),
      )
      .subscribe();
  }

  fetchSettings(): void {
    this.loading = true;
    this.error = '';
    this.saveStatus = 'idle';

    this.http.get<Record<string, string>>('/api/v1/admin/settings').subscribe({
      next: (settings) => {
        this.originalSettings = { ...settings };
        this.loadFormValues(settings);
        this.loading = false;
        this.cdr.detectChanges();
      },
      error: (err) => {
        this.error = 'Failed to load system settings.';
        this.loading = false;
        this.cdr.detectChanges();
      },
    });
  }

  private loadFormValues(settings: Record<string, string>): void {
    this.thumbsDir = settings['thumbs_dir'] || '';
    this.tmdbApiKey = settings['tmdb_api_key'] || '';
    this.castReceiverAppId = settings['cast_receiver_app_id'] || '';
    this.whisperBinaryPath = settings['whisper_binary_path'] || '';
    this.whisperModelPath = settings['whisper_model_path'] || '';
  }

  onSettingChange(immediate: boolean): void {
    this.saveSubject.next({ immediate });
  }

  getSaveStatusLabel(): string {
    switch (this.saveStatus) {
      case 'saving':
        return 'Saving changes...';
      case 'saved':
        return 'All changes saved';
      case 'error':
        return 'Error saving changes';
      default:
        return 'Changes save automatically';
    }
  }

  onPathInput(value: string): void {
    this.activeInput = 'thumbs';
    if (!value) {
      this.thumbsSuggestions = [];
      this.cdr.detectChanges();
      return;
    }

    this.http
      .get<{ dirs: string[] }>(`/api/v1/fs/browse?path=${encodeURIComponent(value)}`)
      .subscribe({
        next: (res) => {
          this.thumbsSuggestions = res.dirs || [];
          this.cdr.detectChanges();
        },
        error: () => {
          this.thumbsSuggestions = [];
          this.cdr.detectChanges();
        },
      });
  }

  selectSuggestion(path: string): void {
    this.thumbsDir = path;
    this.thumbsSuggestions = [];
    this.activeInput = null;
    this.onSettingChange(true);
    this.cdr.detectChanges();
  }

  closeSuggestions(): void {
    setTimeout(() => {
      this.activeInput = null;
      this.cdr.detectChanges();
    }, 200);
  }
}
