import { Component, inject, ViewChild, ElementRef } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { HttpClient } from '@angular/common/http';
import { Router } from '@angular/router';

@Component({
  selector: 'app-setup',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './setup.component.html',
  styleUrls: ['./setup.component.css']
})
export class SetupComponent {
  private http = inject(HttpClient);
  private router = inject(Router);

  private lastFocusedElement: HTMLInputElement | null = null;

  @ViewChild('firstInput') set firstInput(element: ElementRef<HTMLInputElement> | undefined) {
    const inputEl = element?.nativeElement || null;
    if (inputEl && this.lastFocusedElement !== inputEl) {
      this.lastFocusedElement = inputEl;
      setTimeout(() => {
        inputEl.focus();
      }, 0);
    }
  }

  currentStep = 1;
  totalSteps = 4;

  // Step 1: Admin Account
  username = '';
  password = '';
  confirmPassword = '';
  step1Error = '';

  // Step 2: Storage Provisioning (No defaults as requested)
  thumbsDir = '';
  segmentsDir = '';
  thumbsSuggestions: string[] = [];
  segmentsSuggestions: string[] = [];
  step2Error = '';

  // Step 3: Onboarding Services (No defaults as requested)
  tmdbApiKey = '';
  castReceiverAppId = '';
  telemetry = false;
  step3Error = '';

  // Step 4: Finalizing
  finalizingError = '';

  // Autocomplete UI state
  activeInput: 'thumbs' | 'segments' | null = null;

  onPathInput(field: 'thumbs' | 'segments', value: string) {
    this.activeInput = field;
    if (!value) {
      if (field === 'thumbs') this.thumbsSuggestions = [];
      else this.segmentsSuggestions = [];
      return;
    }

    this.http.get<{ dirs: string[] }>(`/api/v1/fs/browse?path=${encodeURIComponent(value)}`)
      .subscribe({
        next: (res) => {
          if (field === 'thumbs') {
            this.thumbsSuggestions = res.dirs || [];
          } else {
            this.segmentsSuggestions = res.dirs || [];
          }
        },
        error: () => {
          if (field === 'thumbs') this.thumbsSuggestions = [];
          else this.segmentsSuggestions = [];
        }
      });
  }

  selectSuggestion(field: 'thumbs' | 'segments', path: string) {
    if (field === 'thumbs') {
      this.thumbsDir = path;
      this.thumbsSuggestions = [];
    } else {
      this.segmentsDir = path;
      this.segmentsSuggestions = [];
    }
    this.activeInput = null;
  }

  closeSuggestions() {
    // Delay to allow click event to register on suggestions before closing
    setTimeout(() => {
      this.activeInput = null;
    }, 200);
  }

  next() {
    if (this.currentStep === 1) {
      if (!this.username) {
        this.step1Error = 'Username is required.';
        return;
      }
      if (!this.password || this.password.length < 4) {
        this.step1Error = 'Password must be at least 4 characters.';
        return;
      }
      if (this.password !== this.confirmPassword) {
        this.step1Error = 'Passwords do not match.';
        return;
      }
      this.step1Error = '';
    }

    if (this.currentStep === 2) {
      if (!this.thumbsDir || !this.segmentsDir) {
        this.step2Error = 'Both directories are required.';
        return;
      }
      this.step2Error = '';
    }

    if (this.currentStep === 3) {
      this.step3Error = '';
      this.submitSetup();
      return;
    }

    this.currentStep++;
  }

  back() {
    if (this.currentStep > 1) {
      this.currentStep--;
    }
  }

  submitSetup() {
    this.currentStep = 4;
    const payload = {
      username: this.username,
      password: this.password,
      thumbs_dir: this.thumbsDir,
      segments_dir: this.segmentsDir,
      tmdb_api_key: this.tmdbApiKey,
      cast_receiver_app_id: this.castReceiverAppId
    };

    this.http.post('/api/v1/setup', payload).subscribe({
      next: () => {
        setTimeout(() => {
          this.router.navigate(['/login']);
        }, 3000);
      },
      error: (err) => {
        this.finalizingError = err.error?.error || 'Setup failed. Please try again.';
        // Allow going back to correct input
        this.currentStep = 3;
        this.step3Error = this.finalizingError;
      }
    });
  }
}
