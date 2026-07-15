import {
  Component,
  Input,
  OnChanges,
  OnInit,
  OnDestroy,
  SimpleChanges,
  ElementRef,
  ViewChild,
  ChangeDetectorRef
} from '@angular/core';
import { CommonModule } from '@angular/common';

@Component({
  selector: 'app-alphabet-rail',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './alphabet-rail.component.html',
  styleUrl: './alphabet-rail.component.css'
})
export class AlphabetRailComponent implements OnInit, OnChanges, OnDestroy {
  @Input() items: any[] = [];
  @Input() titleKey: string = 'title';

  @ViewChild('railContainer', { static: false }) railContainerRef!: ElementRef;

  availableLetters: string[] = [];
  activeLetter: string = '';
  scrubbing: boolean = false;

  private scrollContainer: ElementRef | null = null;
  private resizeObserver: ResizeObserver | null = null;

  constructor(private cdr: ChangeDetectorRef) {}

  ngOnInit(): void {
    this.setupScrollListener();
  }

  ngOnChanges(changes: SimpleChanges): void {
    if (changes['items'] || changes['titleKey']) {
      this.computeAvailableLetters();
      // Wait for DOM to update, then sync active letter
      setTimeout(() => {
        this.syncActiveLetterFromScroll();
      }, 50);
    }
  }

  ngOnDestroy(): void {
    this.removeScrollListener();
    if (this.resizeObserver) {
      this.resizeObserver.disconnect();
    }
  }

  computeAvailableLetters(): void {
    const lettersSet = new Set<string>();
    for (const item of this.items) {
      const title = item[this.titleKey] || '';
      const key = this.getSortKey(title);
      lettersSet.add(key);
    }
    
    // Sort letters alphabetically: A-Z, then '#' at the end (or start, let's put '#' at the top per spec)
    const sorted = Array.from(lettersSet).sort((a, b) => {
      if (a === '#') return -1;
      if (b === '#') return 1;
      return a.localeCompare(b);
    });
    
    this.availableLetters = sorted;
    if (this.availableLetters.length > 0 && !this.availableLetters.includes(this.activeLetter)) {
      this.activeLetter = this.availableLetters[0];
    }
    this.cdr.detectChanges();
  }

  getSortKey(title: string): string {
    if (!title) return '#';
    let normalized = title.trimStart();
    if (/^the\s+/i.test(normalized)) {
      normalized = normalized.replace(/^the\s+/i, '');
    }
    const firstChar = normalized.charAt(0).toUpperCase();
    if (/[0-9]/.test(firstChar)) return '#';
    if (/[A-Z]/.test(firstChar)) return firstChar;
    return '#';
  }

  scrollToLetter(letter: string): void {
    const target = document.querySelector(`[data-sort-letter="${letter}"]`);
    if (target) {
      target.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
  }

  // --- Scroll Synchronization ---
  private setupScrollListener(): void {
    // Wait for the parent components to render and find the content container
    setTimeout(() => {
      const containerEl = document.querySelector('.content-container');
      if (containerEl) {
        containerEl.addEventListener('scroll', this.onScroll, { passive: true });
        
        // Also listen to window resize to ensure sync works on resize
        this.resizeObserver = new ResizeObserver(() => {
          this.syncActiveLetterFromScroll();
        });
        this.resizeObserver.observe(containerEl);
      }
    }, 100);
  }

  private removeScrollListener(): void {
    const containerEl = document.querySelector('.content-container');
    if (containerEl) {
      containerEl.removeEventListener('scroll', this.onScroll);
    }
  }

  private onScroll = (): void => {
    if (this.scrubbing) return;
    this.syncActiveLetterFromScroll();
  };

  syncActiveLetterFromScroll(): void {
    const container = document.querySelector('.content-container');
    if (!container || this.availableLetters.length === 0) return;
    
    const containerRect = container.getBoundingClientRect();
    const cards = Array.from(document.querySelectorAll('[data-sort-letter]'));
    if (cards.length === 0) return;

    let currentLetter = this.availableLetters[0];

    for (const card of cards) {
      const cardRect = card.getBoundingClientRect();
      // When card top is above the upper portion of the scroll area (with header offset)
      if (cardRect.top - containerRect.top <= 140) {
        const letter = card.getAttribute('data-sort-letter');
        if (letter) {
          currentLetter = letter;
        }
      } else {
        break;
      }
    }

    if (this.activeLetter !== currentLetter) {
      this.activeLetter = currentLetter;
      this.cdr.detectChanges();
    }
  }

  // --- Touch Gesture scrubbing support ---
  onTouchStart(event: TouchEvent): void {
    this.scrubbing = true;
    this.handleTouch(event);
  }

  onTouchMove(event: TouchEvent): void {
    this.handleTouch(event);
  }

  onTouchEnd(): void {
    this.scrubbing = false;
    this.cdr.detectChanges();
  }

  private handleTouch(event: TouchEvent): void {
    event.preventDefault(); // Prevent scrolling the main grid while scrubbing
    if (event.touches.length === 0 || !this.railContainerRef) return;

    const touch = event.touches[0];
    const railEl = this.railContainerRef.nativeElement;
    const rect = railEl.getBoundingClientRect();
    
    const relativeY = touch.clientY - rect.top;
    const percent = relativeY / rect.height;
    const index = Math.floor(percent * this.availableLetters.length);
    const safeIndex = Math.max(0, Math.min(index, this.availableLetters.length - 1));
    const letter = this.availableLetters[safeIndex];

    if (letter && letter !== this.activeLetter) {
      this.activeLetter = letter;
      this.scrollToLetter(letter);
      this.cdr.detectChanges();
    }
  }
}
