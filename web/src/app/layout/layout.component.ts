import {
  Component,
  inject,
  OnInit,
  OnDestroy,
  HostListener,
  ElementRef,
  ChangeDetectorRef,
  ViewChild,
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { Router, RouterOutlet, RouterLink, RouterLinkActive, NavigationEnd } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { HttpClient } from '@angular/common/http';
import { Subject, Subscription, of } from 'rxjs';
import { debounceTime, distinctUntilChanged, switchMap, catchError } from 'rxjs/operators';
import { AuthService } from '../auth.service';
import { CastService } from '../cast.service';
import { LibraryStateService } from '../library-state.service';

@Component({
  selector: 'app-layout',
  standalone: true,
  imports: [CommonModule, RouterOutlet, RouterLink, RouterLinkActive, FormsModule],
  templateUrl: './layout.component.html',
  styleUrl: './layout.component.css',
})
export class LayoutComponent implements OnInit, OnDestroy {
  public authService = inject(AuthService);
  public castService = inject(CastService);
  private http = inject(HttpClient);
  private elementRef = inject(ElementRef);
  private cdr = inject(ChangeDetectorRef);
  private router = inject(Router);
  public libraryStateService = inject(LibraryStateService);

  @ViewChild('contentContainer', { static: false }) contentContainerRef?: ElementRef<HTMLElement>;

  private routerSub?: Subscription;

  userDropdownOpen = false;
  castDropdownOpen = false;

  // Search state
  searchQuery = '';
  searchResults: any[] = [];
  showResultsDropdown = false;
  loadingSearch = false;
  private searchSubject = new Subject<string>();
  private searchSub?: Subscription;

  ngOnInit(): void {
    this.routerSub = this.router.events.subscribe((event) => {
      if (event instanceof NavigationEnd) {
        this.restoreScrollForUrl(event.urlAfterRedirects);
      }
    });

    this.searchSub = this.searchSubject
      .pipe(
        debounceTime(300),
        distinctUntilChanged(),
        switchMap((query) => {
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
            }),
          );
        }),
      )
      .subscribe({
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
        },
      });
  }

  ngOnDestroy(): void {
    if (this.searchSub) {
      this.searchSub.unsubscribe();
    }
    if (this.routerSub) {
      this.routerSub.unsubscribe();
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
    if (!target.closest('.cast-profile')) {
      if (this.castDropdownOpen) {
        this.castDropdownOpen = false;
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
    this.castDropdownOpen = false;
    this.cdr.detectChanges();
  }

  toggleCastHeader(): void {
    if (this.castService.isConnected$.value) {
      this.castDropdownOpen = !this.castDropdownOpen;
      this.userDropdownOpen = false;
    } else {
      this.castService.requestSession().catch((err) => {
        console.warn('[Prism] Failed to request Cast session:', err);
      });
    }
    this.cdr.detectChanges();
  }

  logout(): void {
    this.authService.logout();
  }

  // --- Chromecast Control Wrappers ---

  playCast(): void {
    this.castService.play();
  }

  pauseCast(): void {
    this.castService.pause();
  }

  toggleCastPlay(): void {
    if (this.castService.isPlaying$.value) {
      this.castService.pause();
    } else {
      this.castService.play();
    }
  }

  replayCast10s(): void {
    const current = this.castService.currentTime$.value;
    this.castService.seek(Math.max(0, current - 10));
  }

  forwardCast30s(): void {
    const current = this.castService.currentTime$.value;
    const duration = this.castService.duration$.value || 0;
    this.castService.seek(Math.min(duration, current + 30));
  }

  onCastVolumeChange(event: Event): void {
    const target = event.target as HTMLInputElement;
    const val = parseInt(target.value, 10);
    this.castService.setVolume(val);
  }

  toggleCastMute(): void {
    this.castService.setMute(!this.castService.isMuted$.value);
  }

  disconnectCast(): void {
    this.castService.disconnect();
  }

  onCastTimelineChange(event: Event): void {
    const target = event.target as HTMLInputElement;
    const val = parseFloat(target.value);
    this.castService.seek(val);
  }

  formatCastTime(seconds: number): string {
    if (isNaN(seconds) || seconds === Infinity || seconds < 0) return '00:00';
    const hrs = Math.floor(seconds / 3600);
    const mins = Math.floor((seconds % 3600) / 60);
    const secs = Math.floor(seconds % 60);
    const pad = (n: number) => n.toString().padStart(2, '0');
    if (hrs > 0) {
      return `${pad(hrs)}:${pad(mins)}:${pad(secs)}`;
    }
    return `${pad(mins)}:${pad(secs)}`;
  }

  getCastPosterUrl(mediaItem: any): string {
    if (mediaItem && mediaItem.poster_path) {
      if (mediaItem.media_type === 'movie' || mediaItem.media_type === 'episode') {
        return `/api/v1/media/${mediaItem.id}/poster`;
      } else {
        return `/api/v1/tv/shows/${mediaItem.id}/poster`;
      }
    }
    return 'https://images.unsplash.com/photo-1594909122845-11baa439b7bf?q=80&w=400&auto=format&fit=crop';
  }

  getCastProgressPercent(current: number | null, duration: number | null): number {
    const curVal = current || 0;
    const durVal = duration || 0;
    if (durVal <= 0) return 0;
    return (curVal / durVal) * 100;
  }

  goToActiveCastPlayer(): void {
    const media = this.castService.currentMedia$.value;
    if (media && media.id) {
      this.router.navigate(['/watch', media.id]);
    }
  }

  onScroll(event: Event): void {
    const element = event.target as HTMLElement;
    const currentUrl = this.router.url;
    if (currentUrl === '/movies') {
      this.libraryStateService.moviesScrollPosition = element.scrollTop;
    } else if (currentUrl === '/tv-shows') {
      this.libraryStateService.tvShowsScrollPosition = element.scrollTop;
    }
  }

  private restoreScrollForUrl(url: string): void {
    if (url === '/movies') {
      setTimeout(() => {
        const container = this.contentContainerRef?.nativeElement;
        if (container) {
          container.scrollTop = this.libraryStateService.moviesScrollPosition;
        }
      }, 50);
    } else if (url === '/tv-shows') {
      setTimeout(() => {
        const container = this.contentContainerRef?.nativeElement;
        if (container) {
          container.scrollTop = this.libraryStateService.tvShowsScrollPosition;
        }
      }, 50);
    }
  }
}
