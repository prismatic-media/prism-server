import { Injectable, inject, NgZone } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { BehaviorSubject } from 'rxjs';

@Injectable({
  providedIn: 'root',
})
export class CastService {
  private http = inject(HttpClient);
  private zone = inject(NgZone);

  private castContext: any = null;
  private remotePlayer: any = null;
  private remotePlayerController: any = null;

  // Reactive state subjects
  public isConnected$ = new BehaviorSubject<boolean>(false);
  public isAvailable$ = new BehaviorSubject<boolean>(false);
  public currentMedia$ = new BehaviorSubject<any | null>(null);
  public isPlaying$ = new BehaviorSubject<boolean>(false);
  public currentTime$ = new BehaviorSubject<number>(0);
  public duration$ = new BehaviorSubject<number>(0);
  public volume$ = new BehaviorSubject<number>(100);
  public isMuted$ = new BehaviorSubject<boolean>(false);
  public deviceName$ = new BehaviorSubject<string>('');

  private currentPreviewItem: any = null;

  // Preserved state from last cast session (survives disconnect)
  private lastCastTime = 0;
  private lastCastDuration = 0;
  private lastCastMediaId: string | null = null;
  private lastCastMediaItem: any = null;
  private lastCastWasPlaying = false;

  // Watch history tracking
  private historyIntervalId: any = null;
  private lastSavedPosition = 0;
  private progressIntervalId: any = null;

  constructor() {
    this.initCastSDK();
  }

  private initCastSDK(): void {
    // 1. Fetch the Chromecast App ID from settings config
    this.http.get<{ app_id: string }>('/api/v1/cast/config').subscribe({
      next: (config) => {
        const appId = config.app_id;
        if (!appId) {
          console.warn('[Prism Cast] No Chromecast App ID configured.');
          return;
        }

        // 2. Setup the global callback for Google Cast SDK load
        (window as any)['__onGCastApiAvailable'] = (isAvailable: boolean) => {
          if (isAvailable) {
            this.initializeCastContext(appId);
          }
        };

        // If Cast SDK is already loaded (e.g. from cache or on hot reload), initialize immediately
        const cast = (window as any).cast;
        const chrome = (window as any).chrome;
        if (cast && cast.framework && chrome && chrome.cast) {
          this.initializeCastContext(appId);
          return;
        }

        // 3. Inject Cast Sender API script if not already present
        if (!document.getElementById('cast-sender-script')) {
          const script = document.createElement('script');
          script.id = 'cast-sender-script';
          script.src = 'https://www.gstatic.com/cv/js/sender/v1/cast_sender.js?loadCastFramework=1';
          document.head.appendChild(script);
        }
      },
      error: (err) => {
        console.error('[Prism Cast] Failed to fetch Cast config:', err);
      },
    });
  }

  private initializeCastContext(appId: string): void {
    try {
      const cast = (window as any).cast;
      const chrome = (window as any).chrome;

      if (!cast || !cast.framework) {
        console.error('[Prism Cast] Cast framework not available.');
        return;
      }

      const context = cast.framework.CastContext.getInstance();
      context.setOptions({
        receiverApplicationId: appId,
        autoJoinPolicy: chrome.cast.AutoJoinPolicy.ORIGIN_SCOPED,
        androidReceiverCompatible: false, // Must be false for custom Web Receivers (without native ATV app package)
      });

      this.castContext = context;

      // Set initial availability
      const initialCastState = context.getCastState();
      this.isAvailable$.next(initialCastState !== cast.framework.CastState.NO_DEVICES_AVAILABLE);

      // Listen to cast state changes
      context.addEventListener(
        cast.framework.CastContextEventType.CAST_STATE_CHANGED,
        (event: any) => {
          this.zone.run(() => {
            const state = event.castState;
            this.isAvailable$.next(state !== cast.framework.CastState.NO_DEVICES_AVAILABLE);
          });
        },
      );

      // Initialize remote player and controller
      this.remotePlayer = new cast.framework.RemotePlayer();
      this.remotePlayerController = new cast.framework.RemotePlayerController(this.remotePlayer);

      this.setupControllerListeners();
      console.log('[Prism Cast] CastContext successfully initialized with App ID:', appId);
    } catch (e) {
      console.error('[Prism Cast] Error initializing Cast context:', e);
    }
  }

  private setupControllerListeners(): void {
    const cast = (window as any).cast;
    const chrome = (window as any).chrome;
    const events = cast.framework.RemotePlayerEventType;

    // Listen to connection changes
    this.remotePlayerController.addEventListener(events.IS_CONNECTED_CHANGED, () => {
      this.zone.run(() => {
        const connected = this.remotePlayer.isConnected;
        console.log('[Prism Cast] Connection state changed:', connected);

        if (!connected) {
          // Snapshot the current playback position before clearing state
          // so the player component can resume local playback from here
          if (this.currentTime$.value > 0) {
            this.lastCastTime = this.currentTime$.value;
          }
          if (this.duration$.value > 0) {
            this.lastCastDuration = this.duration$.value;
          }
          const media = this.currentMedia$.value;
          if (media && media.id) {
            this.lastCastMediaId = media.id;
            this.lastCastMediaItem = media;
          }
          this.lastCastWasPlaying = this.isPlaying$.value;

          // Save final history position before disconnect
          this.saveHistory(true);

          this.currentMedia$.next(null);
          this.isPlaying$.next(false);
          this.deviceName$.next('');
          this.stopHistoryTimer();
          this.stopProgressTimer();

          // Emit connected change AFTER preserving state above
          this.isConnected$.next(false);
        } else {
          this.isConnected$.next(true);
          const session = this.castContext.getCurrentSession();
          if (session) {
            const device = session.getCastDevice();
            this.deviceName$.next(device ? device.friendlyName : 'Chromecast');

            // If connected and nothing is actively playing, push active preview
            if (this.currentPreviewItem && !this.currentMedia$.value) {
              this.showPreview(this.currentPreviewItem);
            }
          }
        }
      });
    });

    // Listen to play/pause state
    this.remotePlayerController.addEventListener(events.PLAYER_STATE_CHANGED, () => {
      this.zone.run(() => {
        const playerState = this.remotePlayer.playerState;
        const isPlaying =
          playerState === chrome.cast.media.PlayerState.PLAYING ||
          playerState === chrome.cast.media.PlayerState.BUFFERING;
        this.isPlaying$.next(isPlaying);
        if (this.remotePlayer.isConnected) {
          this.lastCastWasPlaying = isPlaying;
        }

        if (isPlaying) {
          this.startHistoryTimer();
          this.startProgressTimer();
        } else {
          this.stopHistoryTimer();
          this.stopProgressTimer();
        }
      });
    });

    // Listen to time updates
    this.remotePlayerController.addEventListener(events.CURRENT_TIME_CHANGED, () => {
      this.zone.run(() => {
        const time = this.remotePlayer.currentTime;
        this.currentTime$.next(time);
        if (this.remotePlayer.isConnected && time > 0) {
          this.lastCastTime = time;
        }
      });
    });

    // Listen to duration updates
    this.remotePlayerController.addEventListener(events.DURATION_CHANGED, () => {
      this.zone.run(() => {
        const duration = this.remotePlayer.duration;
        this.duration$.next(duration);
        if (this.remotePlayer.isConnected && duration > 0) {
          this.lastCastDuration = duration;
        }
      });
    });

    // Listen to volume changes
    this.remotePlayerController.addEventListener(events.VOLUME_LEVEL_CHANGED, () => {
      this.zone.run(() => {
        this.volume$.next(Math.round(this.remotePlayer.volumeLevel * 100));
      });
    });

    // Listen to mute changes
    this.remotePlayerController.addEventListener(events.IS_MUTED_CHANGED, () => {
      this.zone.run(() => {
        this.isMuted$.next(this.remotePlayer.isMuted);
      });
    });

    // Listen to media metadata changes (recovering state on connect/reload)
    this.remotePlayerController.addEventListener(events.MEDIA_INFO_CHANGED, () => {
      this.zone.run(() => {
        const mediaInfo = this.remotePlayer.mediaInfo;
        if (mediaInfo) {
          if (mediaInfo.customData && mediaInfo.customData.mediaItem) {
            const mediaItem = mediaInfo.customData.mediaItem;
            this.currentMedia$.next(mediaItem);
            this.lastCastMediaId = mediaItem.id;
            this.lastCastMediaItem = mediaItem;
          } else if (mediaInfo.metadata && mediaInfo.metadata.title) {
            const metadata = mediaInfo.metadata;
            this.currentMedia$.next({
              id: '',
              title: metadata.title,
              media_type: 'movie',
              duration: this.remotePlayer.duration || 0,
            });
          }
        } else {
          this.currentMedia$.next(null);
          if (this.currentPreviewItem) {
            this.showPreview(this.currentPreviewItem);
          }
        }
      });
    });

    // If already connected when initializing, query current state
    const connected = this.remotePlayer.isConnected;
    this.isConnected$.next(connected);
    if (connected) {
      const session = this.castContext.getCurrentSession();
      if (session) {
        const device = session.getCastDevice();
        this.deviceName$.next(device ? device.friendlyName : 'Chromecast');
      }
      const mediaInfo = this.remotePlayer.mediaInfo;
      if (mediaInfo && mediaInfo.customData && mediaInfo.customData.mediaItem) {
        const mediaItem = mediaInfo.customData.mediaItem;
        this.currentMedia$.next(mediaItem);
        this.lastCastMediaId = mediaItem.id;
        this.lastCastMediaItem = mediaItem;
      }
      const isPlaying =
        this.remotePlayer.playerState === chrome.cast.media.PlayerState.PLAYING ||
        this.remotePlayer.playerState === chrome.cast.media.PlayerState.BUFFERING;
      this.isPlaying$.next(isPlaying);
      this.currentTime$.next(this.remotePlayer.currentTime);
      this.duration$.next(this.remotePlayer.duration);
      this.volume$.next(Math.round(this.remotePlayer.volumeLevel * 100));
      this.isMuted$.next(this.remotePlayer.isMuted);

      if (this.remotePlayer.currentTime > 0) {
        this.lastCastTime = this.remotePlayer.currentTime;
      }
      if (this.remotePlayer.duration > 0) {
        this.lastCastDuration = this.remotePlayer.duration;
      }
      this.lastCastWasPlaying = isPlaying;

      if (isPlaying) {
        this.startHistoryTimer();
        this.startProgressTimer();
      }
    }
  }

  // --- Actions ---

  public requestSession(): Promise<void> {
    if (!this.castContext) {
      return Promise.reject('Cast SDK not initialized yet.');
    }
    return this.castContext.requestSession().then(
      () => {
        console.log('[Prism Cast] Session established');
      },
      (err: any) => {
        console.warn('[Prism Cast] Session request cancelled or failed:', err);
        throw err;
      },
    );
  }

  public startCasting(mediaItem: any, resumePosition = 0): void {
    if (!this.isConnected$.value) {
      this.requestSession().then(
        () => {
          this.loadMedia(mediaItem, resumePosition);
        },
        (err) => {
          console.error('[Prism Cast] Cannot start casting without a session:', err);
        },
      );
    } else {
      this.loadMedia(mediaItem, resumePosition);
    }
  }

  private loadMedia(mediaItem: any, resumePosition: number): void {
    const cast = (window as any).cast;
    const chrome = (window as any).chrome;

    this.http.post<{ token: string }>(`/api/v1/stream/${mediaItem.id}/cast-token`, {}).subscribe({
      next: (res) => {
        const token = res.token;
        const manifestUrl = `${window.location.origin}/api/v1/stream/${mediaItem.id}/manifest.mpd?cast_token=${token}`;

        const session = this.castContext.getCurrentSession();
        if (!session) {
          console.error('[Prism Cast] No active session found.');
          return;
        }

        const mediaInfo = new chrome.cast.media.MediaInfo(manifestUrl, 'application/dash+xml');

        // Build metadata based on media type
        let metadata;
        if (mediaItem.media_type === 'episode') {
          metadata = new chrome.cast.media.TvShowMediaMetadata();
          metadata.metadataType = chrome.cast.media.MetadataType.TV_SHOW;
          metadata.seriesTitle = mediaItem.tv_show_title || 'TV Show';
          metadata.episodeTitle = mediaItem.title;
          if (mediaItem.season_number) {
            metadata.season = mediaItem.season_number;
          }
          if (mediaItem.episode_number) {
            metadata.episode = mediaItem.episode_number;
          }
        } else {
          metadata = new chrome.cast.media.MovieMediaMetadata();
          metadata.metadataType = chrome.cast.media.MetadataType.MOVIE;
          metadata.title = mediaItem.title;
          if (mediaItem.year) {
            metadata.releaseDate = `${mediaItem.year}-01-01`;
          }
        }

        // Build poster image
        let posterUrl = '';
        if (mediaItem.poster_path) {
          if (mediaItem.media_type === 'movie' || mediaItem.media_type === 'episode') {
            posterUrl = `${window.location.origin}/api/v1/movies/${mediaItem.id}/poster`;
          } else {
            posterUrl = `${window.location.origin}/api/v1/tv-shows/${mediaItem.id}/poster`;
          }
        } else {
          posterUrl =
            'https://images.unsplash.com/photo-1594909122845-11baa439b7bf?q=80&w=400&auto=format&fit=crop';
        }
        metadata.images = [{ url: posterUrl }];
        mediaInfo.metadata = metadata;

        // Save exact mediaItem so we can recover it on reconnects
        mediaInfo.customData = { mediaItem };

        const loadRequest = new chrome.cast.media.LoadRequest(mediaInfo);
        loadRequest.currentTime = resumePosition;
        loadRequest.autoplay = true;

        // Set initial cast tracking state in case we disconnect quickly
        this.lastCastTime = resumePosition;
        this.lastCastDuration = mediaItem.duration || 0;
        this.lastCastMediaId = mediaItem.id;
        this.lastCastMediaItem = mediaItem;
        this.lastCastWasPlaying = true;

        session.loadMedia(loadRequest).then(
          () => {
            console.log('[Prism Cast] Media successfully loaded on Chromecast');
            this.currentMedia$.next(mediaItem);
            this.lastCastMediaId = mediaItem.id;
            this.lastCastMediaItem = mediaItem;
          },
          (err: any) => {
            console.error('[Prism Cast] Error loading media on receiver:', err);
          },
        );
      },
      error: (err) => {
        console.error('[Prism Cast] Error generating cast token:', err);
      },
    });
  }

  public play(): void {
    if (this.remotePlayerController && this.remotePlayer.isPaused) {
      this.remotePlayerController.playOrPause();
    }
  }

  public pause(): void {
    if (this.remotePlayerController && !this.remotePlayer.isPaused) {
      this.remotePlayerController.playOrPause();
    }
  }

  public seek(seconds: number): void {
    if (this.remotePlayerController) {
      this.remotePlayer.currentTime = seconds;
      this.remotePlayerController.seek();
    }
  }

  public setVolume(volumePercent: number): void {
    if (this.remotePlayerController) {
      this.remotePlayer.volumeLevel = volumePercent / 100;
      this.remotePlayerController.setVolumeLevel();
    }
  }

  public setMute(isMuted: boolean): void {
    if (this.remotePlayerController && this.remotePlayer.isMuted !== isMuted) {
      this.remotePlayerController.muteOrUnmute();
    }
  }

  public disconnect(): void {
    if (this.castContext) {
      this.castContext.endCurrentSession(true);
    }
  }

  /**
   * Returns the last known cast playback position after a disconnect.
   * This allows the player component to resume local playback at the
   * correct position instead of starting from the beginning.
   */
  public getLastCastPosition(
    mediaId: string,
  ): { time: number; duration: number; isPlaying: boolean } | null {
    if (this.lastCastMediaId === mediaId && this.lastCastTime > 0) {
      const result = {
        time: this.lastCastTime,
        duration: this.lastCastDuration,
        isPlaying: this.lastCastWasPlaying,
      };
      // Clear after reading to avoid stale reuse
      this.lastCastTime = 0;
      this.lastCastDuration = 0;
      this.lastCastMediaId = null;
      this.lastCastMediaItem = null;
      this.lastCastWasPlaying = false;
      return result;
    }
    return null;
  }

  // --- Watch History ---

  private startHistoryTimer(): void {
    this.stopHistoryTimer();
    this.historyIntervalId = setInterval(() => {
      this.saveHistory();
    }, 10000);
  }

  private stopHistoryTimer(): void {
    if (this.historyIntervalId) {
      clearInterval(this.historyIntervalId);
      this.historyIntervalId = null;
    }
  }

  // --- Progress Estimation ---

  private startProgressTimer(): void {
    this.stopProgressTimer();
    this.progressIntervalId = setInterval(() => {
      this.zone.run(() => {
        const session = this.castContext ? this.castContext.getCurrentSession() : null;
        if (session) {
          const mediaSession = session.getMediaSession();
          if (mediaSession && typeof mediaSession.getEstimatedTime === 'function') {
            const estimatedTime = mediaSession.getEstimatedTime();
            this.currentTime$.next(estimatedTime);
            if (this.remotePlayer && this.remotePlayer.isConnected && estimatedTime > 0) {
              this.lastCastTime = estimatedTime;
            }
          }
        }
      });
    }, 500);
  }

  private stopProgressTimer(): void {
    if (this.progressIntervalId) {
      clearInterval(this.progressIntervalId);
      this.progressIntervalId = null;
    }
  }

  private saveHistory(force = false): void {
    const mediaItem = this.currentMedia$.value || this.lastCastMediaItem;
    if (!mediaItem || !mediaItem.id) return;

    const current = this.currentTime$.value || this.lastCastTime;
    const duration = this.duration$.value || this.lastCastDuration || mediaItem.duration || 0;
    if (duration <= 0) return;

    const completed = duration - current <= 5;

    // Only update if position has moved significantly (at least 1 second) or force is true
    if (Math.abs(current - this.lastSavedPosition) >= 1 || force || completed) {
      this.lastSavedPosition = current;
      this.http
        .put(`/api/v1/history/${mediaItem.id}`, {
          position: current,
          completed: completed,
        })
        .subscribe({
          next: () => {
            if (completed) {
              this.stopHistoryTimer();
            }
          },
          error: (err) => {
            console.error('[Prism Cast] Failed to save cast watch history:', err);
          },
        });
    }
  }

  // --- Preview Actions ---

  public showPreview(mediaItem: any): void {
    this.currentPreviewItem = mediaItem;
    if (!mediaItem) return;
    if (!this.isConnected$.value || this.currentMedia$.value) return;

    const session = this.castContext ? this.castContext.getCurrentSession() : null;
    if (session) {
      let title = mediaItem.title || mediaItem.name || '';
      let subtitle = '';

      if (mediaItem.media_type === 'episode') {
        subtitle = mediaItem.tv_show_title
          ? `${mediaItem.tv_show_title} — S${mediaItem.season_number}E${mediaItem.episode_number}`
          : '';
      } else if (mediaItem.media_type === 'movie' && mediaItem.year) {
        subtitle = `${mediaItem.year}`;
      } else if (mediaItem.first_air_year) {
        subtitle = `${mediaItem.first_air_year}`;
      }

      let posterUrl = '';
      if (mediaItem.poster_path) {
        if (mediaItem.media_type === 'movie' || mediaItem.media_type === 'episode') {
          posterUrl = `${window.location.origin}/api/v1/movies/${mediaItem.id}/poster`;
        } else {
          posterUrl = `${window.location.origin}/api/v1/tv-shows/${mediaItem.id}/poster`;
        }
      } else {
        posterUrl =
          'https://images.unsplash.com/photo-1594909122845-11baa439b7bf?q=80&w=400&auto=format&fit=crop';
      }

      let backdropUrl = '';
      if (mediaItem.backdrop_path) {
        if (mediaItem.media_type === 'movie' || mediaItem.media_type === 'episode') {
          backdropUrl = `${window.location.origin}/api/v1/movies/${mediaItem.id}/backdrop`;
        } else {
          backdropUrl = `${window.location.origin}/api/v1/tv-shows/${mediaItem.id}/backdrop`;
        }
      } else {
        backdropUrl = posterUrl;
      }

      const message = {
        type: 'SHOW_PREVIEW',
        title,
        subtitle,
        posterUrl,
        backdropUrl,
      };

      session.sendMessage('urn:x-cast:com.prism.metadata', message).then(
        () => console.log('[Prism Cast] Sent preview metadata to receiver:', message),
        (err: any) => console.error('[Prism Cast] Failed to send preview metadata:', err),
      );
    }
  }

  public clearPreview(): void {
    this.currentPreviewItem = null;
    if (!this.isConnected$.value) return;

    const session = this.castContext ? this.castContext.getCurrentSession() : null;
    if (session) {
      const message = { type: 'CLEAR_PREVIEW' };
      session.sendMessage('urn:x-cast:com.prism.metadata', message).then(
        () => console.log('[Prism Cast] Sent clear preview to receiver'),
        (err: any) => console.error('[Prism Cast] Failed to send clear preview:', err),
      );
    }
  }
}
