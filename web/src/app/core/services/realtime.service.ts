import { Injectable, inject, OnDestroy } from '@angular/core';
import { Subject, Observable } from 'rxjs';
import { filter, map } from 'rxjs/operators';
import { AuthService } from './auth.service';

// ── Event payload types (mirror pkg/events in Go) ─────────────────────────────

export interface JobProgressPayload {
  job_id: string;
  media_item_id: string;
  progress: number;
  done: boolean;
  error?: string;
}

export interface MediaUpdatedPayload {
  media_item_id: string;
  library_id: string;
  transcode_status: string;
}

export interface MediaCreatedPayload {
  media_item_id: string;
  library_id: string;
  title: string;
}

export type EventType = 'job.progress' | 'media.updated' | 'media.created';

export interface RealtimeEvent {
  type: EventType;
  payload: JobProgressPayload | MediaUpdatedPayload | MediaCreatedPayload;
}

// ── Service ────────────────────────────────────────────────────────────────────

@Injectable({ providedIn: 'root' })
export class RealtimeService implements OnDestroy {
  private readonly auth = inject(AuthService);

  private ws: WebSocket | null = null;
  private readonly events$ = new Subject<RealtimeEvent>();
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private destroyed = false;

  /** All real-time events as a shared observable. */
  readonly all$: Observable<RealtimeEvent> = this.events$.asObservable();

  /** Filtered stream of job-progress events. */
  readonly jobProgress$: Observable<JobProgressPayload> = this.all$.pipe(
    filter((e) => e.type === 'job.progress'),
    map((e) => e.payload as JobProgressPayload),
  );

  /** Filtered stream of media-updated events. */
  readonly mediaUpdated$: Observable<MediaUpdatedPayload> = this.all$.pipe(
    filter((e) => e.type === 'media.updated'),
    map((e) => e.payload as MediaUpdatedPayload),
  );

  /** Filtered stream of media-created events. */
  readonly mediaCreated$: Observable<MediaCreatedPayload> = this.all$.pipe(
    filter((e) => e.type === 'media.created'),
    map((e) => e.payload as MediaCreatedPayload),
  );

  /** Connect to the server. Call once when the user is authenticated. */
  connect() {
    if (this.ws && (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING)) {
      return;
    }
    this.openSocket();
  }

  /** Disconnect and stop reconnecting. */
  disconnect() {
    this.destroyed = true;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }

  ngOnDestroy() {
    this.disconnect();
    this.events$.complete();
  }

  private openSocket() {
    const token = this.auth.accessToken();
    if (!token) return;

    // Build the WebSocket URL from the current origin so it works in both dev
    // (proxy rewrites /api/v1 → localhost:8080) and production.
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    const url = `${proto}://${location.host}/api/v1/ws/events?token=${encodeURIComponent(token)}`;

    this.ws = new WebSocket(url);

    this.ws.onmessage = (msg) => {
      try {
        const evt = JSON.parse(msg.data) as RealtimeEvent;
        if (evt.type) {
          this.events$.next(evt);
        }
      } catch {
        // ignore malformed frames
      }
    };

    this.ws.onclose = () => {
      if (this.destroyed) return;
      // Reconnect with a fixed 3 s delay.
      this.reconnectTimer = setTimeout(() => {
        if (!this.destroyed) this.openSocket();
      }, 3000);
    };

    this.ws.onerror = () => {
      // onclose fires right after onerror, so reconnect happens there.
    };
  }
}
