import { Component, inject, OnInit, OnDestroy, HostListener, ElementRef, ChangeDetectorRef } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterOutlet, RouterLink, RouterLinkActive } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { HttpClient } from '@angular/common/http';
import { Subject, Subscription, of } from 'rxjs';
import { debounceTime, distinctUntilChanged, switchMap, catchError } from 'rxjs/operators';
import { AuthService } from '../auth.service';

@Component({
  selector: 'app-layout',
  standalone: true,
  imports: [CommonModule, RouterOutlet, RouterLink, RouterLinkActive, FormsModule],
  templateUrl: './layout.component.html',
  styleUrl: './layout.component.css'
})
export class LayoutComponent implements OnInit, OnDestroy {
  public authService = inject(AuthService);
  private http = inject(HttpClient);
  private elementRef = inject(ElementRef);
  private cdr = inject(ChangeDetectorRef);

  userDropdownOpen = false;
  
  // Search state
  searchQuery = '';
  searchResults: any[] = [];
  showResultsDropdown = false;
  loadingSearch = false;
  private searchSubject = new Subject<string>();
  private searchSub?: Subscription;

  ngOnInit(): void {
    this.searchSub = this.searchSubject.pipe(
      debounceTime(300),
      distinctUntilChanged(),
      switchMap(query => {
        if (!query.trim()) {
          this.loadingSearch = false;
          this.cdr.detectChanges();
          return of([]);
        }
        this.loadingSearch = true;
        this.cdr.detectChanges();
        return this.http.get<any[]>(`/api/v1/search?q=${encodeURIComponent(query)}`).pipe(
          catchError(() => {
            this.loadingSearch = false;
            this.cdr.detectChanges();
            return of([]);
          })
        );
      })
    ).subscribe({
      next: (results) => {
        this.searchResults = results;
        this.loadingSearch = false;
        this.showResultsDropdown = true;
        this.cdr.detectChanges();
      },
      error: () => {
        this.searchResults = [];
        this.loadingSearch = false;
        this.cdr.detectChanges();
      }
    });
  }

  ngOnDestroy(): void {
    if (this.searchSub) {
      this.searchSub.unsubscribe();
    }
  }

  @HostListener('document:click', ['$event'])
  onDocumentClick(event: MouseEvent): void {
    const target = event.target as HTMLElement;
    let changed = false;

    if (!target.closest('.search-container')) {
      if (this.showResultsDropdown) {
        this.showResultsDropdown = false;
        changed = true;
      }
    }
    if (!target.closest('.user-profile')) {
      if (this.userDropdownOpen) {
        this.userDropdownOpen = false;
        changed = true;
      }
    }

    if (changed) {
      this.cdr.detectChanges();
    }
  }

  onSearchInput(): void {
    if (!this.searchQuery.trim()) {
      this.clearSearch();
      return;
    }
    this.loadingSearch = true;
    this.showResultsDropdown = true;
    this.cdr.detectChanges();
    this.searchSubject.next(this.searchQuery);
  }

  onSearchFocus(): void {
    if (this.searchQuery.trim()) {
      this.showResultsDropdown = true;
      this.cdr.detectChanges();
      if (this.searchResults.length === 0 && !this.loadingSearch) {
        this.searchSubject.next(this.searchQuery);
      }
    }
  }

  clearSearch(): void {
    this.searchQuery = '';
    this.searchResults = [];
    this.showResultsDropdown = false;
    this.loadingSearch = false;
    this.cdr.detectChanges();
  }

  selectResult(): void {
    this.showResultsDropdown = false;
    this.searchQuery = '';
    this.searchResults = [];
    this.cdr.detectChanges();
  }

  getRoute(result: any): string[] {
    if (result.media_type === 'movie') {
      return ['/movies', result.id];
    } else if (result.media_type === 'tvshow') {
      return ['/tv-shows', result.id];
    }
    return ['/'];
  }

  getPosterUrl(result: any): string {
    if (result.poster_path) {
      if (result.media_type === 'movie') {
        return `/api/v1/media/${result.id}/poster`;
      } else if (result.media_type === 'tvshow') {
        return `/api/v1/tv/shows/${result.id}/poster`;
      }
    }
    return 'https://images.unsplash.com/photo-1594909122845-11baa439b7bf?q=80&w=400&auto=format&fit=crop';
  }

  toggleUserDropdown(): void {
    this.userDropdownOpen = !this.userDropdownOpen;
    this.cdr.detectChanges();
  }

  logout(): void {
    this.authService.logout();
  }
}

