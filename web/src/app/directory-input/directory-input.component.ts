import { Component, Input, Output, EventEmitter, inject, ChangeDetectorRef } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { HttpClient } from '@angular/common/http';

@Component({
  selector: 'app-directory-input',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './directory-input.component.html',
  styleUrls: ['./directory-input.component.css'],
})
export class DirectoryInputComponent {
  private http = inject(HttpClient);
  private cdr = inject(ChangeDetectorRef);

  @Input() value = '';
  @Output() valueChange = new EventEmitter<string>();

  @Input() placeholder = '';
  @Input() inputClass = 'input-glass';
  @Input() id = '';
  @Input() theme: 'glass' | 'borderless' = 'glass';

  suggestions: string[] = [];
  showSuggestions = false;

  onInput(val: string): void {
    this.value = val;
    this.valueChange.emit(this.value);

    if (!val) {
      this.suggestions = [];
      this.showSuggestions = false;
      this.cdr.detectChanges();
      return;
    }

    this.http
      .get<{ dirs: string[] }>(`/api/v1/fs:browse?path=${encodeURIComponent(val)}`)
      .subscribe({
        next: (res) => {
          this.suggestions = res.dirs || [];
          this.showSuggestions = this.suggestions.length > 0;
          this.cdr.detectChanges();
        },
        error: () => {
          this.suggestions = [];
          this.showSuggestions = false;
          this.cdr.detectChanges();
        },
      });
  }

  selectSuggestion(path: string): void {
    this.value = path;
    this.valueChange.emit(this.value);
    this.suggestions = [];
    this.showSuggestions = false;
    this.cdr.detectChanges();
  }

  onBlur(): void {
    // Delay to allow click event to register on suggestions before closing
    setTimeout(() => {
      this.showSuggestions = false;
      this.cdr.detectChanges();
    }, 200);
  }

  onFocus(): void {
    if (this.value) {
      this.onInput(this.value);
    }
  }
}
