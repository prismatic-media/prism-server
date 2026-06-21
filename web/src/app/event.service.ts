import { Injectable, inject, OnDestroy } from '@angular/core';
import { AuthService } from './auth.service';
import { Subject, Subscription, Observable, bufferTime, filter } from 'rxjs';

@Injectable({
  providedIn: 'root',
})
export class EventService implements OnDestroy {
  private authService = inject(AuthService);
  private ws: WebSocket | null = null;
  private authSub: Subscription;
  private isDestroyed = false;
  private reconnectTimeout: any;
  private isRefreshingToken = false;

  private eventSubject = new Subject<any>();
  public events$: Observable<any[]> = this.eventSubject.asObservable().pipe(
    bufferTime(1000),
    filter((batch) => batch.length > 0),
  );

  constructor() {
    this.authSub = this.authService.currentUser$.subscribe((user) => {
      if (user) {
        this.connect();
      } else {
        this.disconnect();
      }
    });
  }

  ngOnDestroy(): void {
    this.isDestroyed = true;
    if (this.authSub) {
      this.authSub.unsubscribe();
    }
    this.disconnect();
  }

  private connect(): void {
    if (this.ws || this.isDestroyed) return;

    const token = this.authService.getToken();
    if (!token) return;

    if (this.authService.isTokenExpired(token)) {
      if (this.isRefreshingToken) {
        console.error('Token refresh loop detected for WebSocket connection. Logging out.');
        this.authService.logout();
        return;
      }
      this.isRefreshingToken = true;
      this.authService.refreshToken().subscribe({
        next: () => {
          this.isRefreshingToken = false;
          this.connect();
        },
        error: (err) => {
          this.isRefreshingToken = false;
          console.error('Failed to refresh token for WebSocket connection:', err);
          this.authService.logout();
        }
      });
      return;
    }

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = window.location.host;
    const wsUrl = `${protocol}//${host}/api/v1/ws/events?token=${token}`;

    this.ws = new WebSocket(wsUrl);

    this.ws.onmessage = (event) => {
      try {
        const evt = JSON.parse(event.data);
        this.eventSubject.next(evt);
      } catch (e) {
        console.error('Error parsing WebSocket event:', e);
      }
    };

    this.ws.onclose = () => {
      this.ws = null;
      if (!this.isDestroyed && this.authService.isLoggedIn()) {
        clearTimeout(this.reconnectTimeout);
        this.reconnectTimeout = setTimeout(() => this.connect(), 5000);
      }
    };

    this.ws.onerror = (err) => {
      console.error('WebSocket connection error:', err);
    };
  }

  private disconnect(): void {
    clearTimeout(this.reconnectTimeout);
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }
}
