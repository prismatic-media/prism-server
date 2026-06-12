import {
  Component,
  OnInit,
  OnDestroy,
  AfterViewInit,
  ElementRef,
  ViewChild,
  inject,
  ChangeDetectorRef,
} from '@angular/core';
import { CommonModule, Location } from '@angular/common';
import { ActivatedRoute, Router } from '@angular/router';
import { HttpClient } from '@angular/common/http';
import * as dashjs from 'dashjs';
import { AuthService } from '../auth.service';

interface MediaItem {
  id: string;
  title: string;
  media_type: string;
  file_path: string;
  file_size: number;
  duration: number;
  width: number;
  height: number;
  video_codec: string;
  audio_codec: string;
  transcode_status: string;
  mpd_path?: string;
  backdrop_path?: string;
  season_number?: number;
  episode_number?: number;
}

interface WatchHistory {
  media_item_id: string;
  position: number;
  completed: boolean;
}

@Component({
  selector: 'app-player',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './player.component.html',
  styleUrl: './player.component.css',
})
export class PlayerComponent implements OnInit, OnDestroy, AfterViewInit {
  private route = inject(ActivatedRoute);
  private router = inject(Router);
  private http = inject(HttpClient);
  private cdr = inject(ChangeDetectorRef);
  private authService = inject(AuthService);
  private location = inject(Location);

  @ViewChild('videoPlayer') videoElement!: ElementRef<HTMLVideoElement>;

  // Media info
  mediaId = '';
  mediaItem: MediaItem | null = null;
  loading = true;
  error = '';

  // dash.js MediaPlayer instance
  private player: dashjs.MediaPlayerClass | null = null;

  // Controls UI states
  isPlaying = false;
  currentTimeStr = '00:00';
  totalTimeStr = '00:00';
  progressPercent = 0;
  volume = 70; // 0-100
  isMuted = false;
  isFullscreen = false;
  aspectRatioMode: 'fit' | 'fill' | 'stretch' = 'fit';

  // Subtitle/Audio dynamic tracks
  subtitleTracks: any[] = [];
  activeSubtitleTrack: any = null;
  showSubtitlePanel = false;
  subtitleSize: 'small' | 'medium' | 'large' | 'xlarge' = 'medium';

  audioTracks: any[] = [];
  activeAudioTrack: any = null;
  showAudioPanel = false;

  // Telemetry info
  showTelemetry = false;
  telemetryBitrate = '0 Mbps';
  telemetryBuffer = '0s';
  telemetryCodec = 'Unknown';
  telemetryDropped = 0;

  // History tracking
  private historyIntervalId: any = null;
  private lastSavedPosition = 0;
  private resumePosition: number | null = null;
  private isStreamInitialized = false;

  // UI Visibility timers (for auto-hiding controls)
  controlsVisible = true;
  private controlsTimer: any = null;

  // Drag-to-scrub state
  isDragging = false;
  private timelineRect: DOMRect | null = null;

  private fullscreenListener = () => {
    this.isFullscreen = !!document.fullscreenElement;
    this.cdr.detectChanges();
  };

  ngOnInit(): void {
    this.mediaId = this.route.snapshot.paramMap.get('id') || '';
    if (!this.mediaId) {
      this.error = 'No media ID provided.';
      this.loading = false;
      return;
    }

    const savedSize = localStorage.getItem('prism_subtitle_size');
    if (savedSize && ['small', 'medium', 'large', 'xlarge'].includes(savedSize)) {
      this.subtitleSize = savedSize as any;
    }

    this.loadMedia();
    this.resetControlsTimer();
  }

  ngAfterViewInit(): void {
    // Media initialization will occur inside loadMedia once HTTP finishes
    document.addEventListener('fullscreenchange', this.fullscreenListener);
  }

  ngOnDestroy(): void {
    this.clearHistoryInterval();
    if (this.controlsTimer) {
      clearTimeout(this.controlsTimer);
    }
    document.removeEventListener('fullscreenchange', this.fullscreenListener);
    if (this.player) {
      this.saveHistory(true); // Attempt to save current position before destroying
      this.player.destroy();
      this.player = null;
    }
    this.isStreamInitialized = false;
    this.resumePosition = null;
  }

  loadMedia(): void {
    this.http.get<MediaItem>(`/api/v1/media/${this.mediaId}`).subscribe({
      next: (item) => {
        this.mediaItem = item;

        if (item.transcode_status !== 'done') {
          this.error =
            'This media item has not been optimized for streaming yet. Please initiate optimization from the details page.';
          this.loading = false;
          this.cdr.detectChanges();
          return;
        }

        // Set loading to false and trigger change detection so #videoPlayer is rendered in DOM
        this.loading = false;
        this.cdr.detectChanges();

        // Initialize Player once details are loaded
        setTimeout(() => this.initializePlayer(), 0);
      },
      error: (err) => {
        this.error = 'Could not retrieve media details.';
        this.loading = false;
        this.cdr.detectChanges();
      },
    });
  }

  initializePlayer(): void {
    if (!this.videoElement || !this.mediaItem) return;

    const videoEl = this.videoElement.nativeElement;
    const manifestUrl = `/api/v1/stream/${this.mediaId}/manifest.mpd`;

    // Create dash.js player
    this.player = dashjs.MediaPlayer().create();

    // Authenticate outgoing segment and manifest requests using the addRequestInterceptor API
    const token = this.authService.getToken();
    this.player.addRequestInterceptor((request) => {
      request.headers = request.headers || {};
      request.headers['Authorization'] = `Bearer ${token}`;
      return Promise.resolve(request);
    });

    // Initial config
    this.player.initialize(videoEl, manifestUrl, true);
    this.player.setVolume(this.volume / 100);

    // Bind playback events
    videoEl.addEventListener('play', () => {
      this.isPlaying = true;
      this.cdr.detectChanges();
    });

    videoEl.addEventListener('pause', () => {
      this.isPlaying = false;
      this.cdr.detectChanges();
    });

    videoEl.addEventListener('timeupdate', () => {
      this.updateProgress();
      this.updateTelemetry();
    });

    videoEl.addEventListener('volumechange', () => {
      this.volume = Math.round(videoEl.volume * 100);
      this.isMuted = videoEl.muted;
      this.cdr.detectChanges();
    });

    // Check history and resume
    this.http.get<WatchHistory[]>('/api/v1/history').subscribe({
      next: (historyList) => {
        const startOver = this.route.snapshot.queryParamMap.get('startOver') === 'true';
        if (startOver) {
          this.cdr.detectChanges();
          this.startHistoryInterval();
          return;
        }

        const entry = historyList.find((h) => h.media_item_id === this.mediaId);
        if (entry && !entry.completed && entry.position > 0) {
          // Resume position (if less than duration - 5 seconds)
          const resumeTime = entry.position;
          if (resumeTime < this.mediaItem!.duration - 5) {
            if (this.isStreamInitialized) {
              this.player?.seek(resumeTime);
              this.lastSavedPosition = resumeTime;
            } else {
              this.resumePosition = resumeTime;
            }
          }
        }
        this.cdr.detectChanges();

        // Start watch history sync interval
        this.startHistoryInterval();
      },
      error: () => {
        // Fall back to playing from start
        this.cdr.detectChanges();
        this.startHistoryInterval();
      },
    });

    // Load subtitle and audio tracks when stream is ready
    this.player.on(dashjs.MediaPlayer.events.STREAM_INITIALIZED, () => {
      this.isStreamInitialized = true;
      this.loadTracks();
      if (this.resumePosition !== null && this.resumePosition > 0) {
        this.player?.seek(this.resumePosition);
        this.lastSavedPosition = this.resumePosition;
        this.resumePosition = null;
      }
    });
  }

  loadTracks(): void {
    if (!this.player) return;

    // Subtitles
    const textTracks = this.player.getTracksFor('text') || [];
    this.subtitleTracks = textTracks.map((t) => ({
      index: t.index,
      lang: t.lang || '',
      label: this.getFriendlyLanguageName(t.lang || ''),
    }));

    const activeTextTrack = (this.player as any).getCurrentTrackFor('text');
    this.activeSubtitleTrack = activeTextTrack
      ? this.subtitleTracks.find((t) => t.index === activeTextTrack.index)
      : null;

    // Audio
    const audioTracks = this.player.getTracksFor('audio') || [];
    this.audioTracks = audioTracks.map((t) => ({
      index: t.index,
      lang: t.lang || '',
      label: this.getFriendlyLanguageName(t.lang || ''),
    }));
    const activeAudioTrack = (this.player as any).getCurrentTrackFor('audio');
    this.activeAudioTrack = activeAudioTrack
      ? this.audioTracks.find((t) => t.index === activeAudioTrack.index)
      : null;

    this.cdr.detectChanges();
  }

  getFriendlyLanguageName(code: string): string {
    if (!code) return 'Unknown';
    const clean = code.toLowerCase().trim();
    if (clean === 'eng' || clean === 'en') return 'English';
    if (clean === 'fra' || clean === 'fr') return 'French';
    if (clean === 'spa' || clean === 'es') return 'Spanish';
    if (clean === 'deu' || clean === 'de') return 'German';
    if (clean === 'ita' || clean === 'it') return 'Italian';
    if (clean === 'jpn' || clean === 'ja') return 'Japanese';
    if (clean === 'zho' || clean === 'zh') return 'Chinese';
    if (clean === 'rus' || clean === 'ru') return 'Russian';
    return code.toUpperCase();
  }

  togglePlay(): void {
    if (!this.player) return;
    if (this.isPlaying) {
      this.player.pause();
    } else {
      this.player.play();
    }
  }

  replay10s(): void {
    if (!this.player) return;
    const current = this.player.time();
    this.player.seek(Math.max(0, current - 10));
  }

  forward30s(): void {
    if (!this.player) return;
    const current = this.player.time();
    const duration = this.player.duration();
    this.player.seek(Math.min(duration, current + 30));
  }

  seek(event: MouseEvent): void {
    if (!this.player || !this.videoElement) return;

    let rect = this.timelineRect;
    if (!rect) {
      let target = event.target as HTMLElement;
      while (target && !target.classList.contains('timeline-slider')) {
        target = target.parentElement as HTMLElement;
      }
      if (target) {
        rect = target.getBoundingClientRect();
      }
    }

    if (!rect) return;

    const x = event.clientX - rect.left;
    const width = rect.width;
    const percentage = Math.max(0, Math.min(1, x / width));
    const duration = this.player.duration() || this.mediaItem?.duration || 0;
    this.player.seek(percentage * duration);
  }

  onMouseDown(event: MouseEvent): void {
    this.isDragging = true;

    let target = event.target as HTMLElement;
    while (target && !target.classList.contains('timeline-slider')) {
      target = target.parentElement as HTMLElement;
    }
    if (target) {
      this.timelineRect = target.getBoundingClientRect();
    }

    this.seek(event);
  }

  onMouseMoveDrag(event: MouseEvent): void {
    if (this.isDragging) {
      this.seek(event);
    }
  }

  onMouseUp(): void {
    this.isDragging = false;
    this.timelineRect = null;
  }

  toggleMute(): void {
    if (!this.videoElement) return;
    const videoEl = this.videoElement.nativeElement;
    videoEl.muted = !videoEl.muted;
    this.isMuted = videoEl.muted;
  }

  onVolumeChange(event: Event): void {
    const target = event.target as HTMLInputElement;
    const val = parseInt(target.value, 10);
    this.volume = val;
    if (this.player) {
      this.player.setVolume(val / 100);
    }
    if (this.videoElement) {
      this.videoElement.nativeElement.muted = val === 0;
    }
  }

  toggleSubtitleMenu(event: MouseEvent): void {
    event.stopPropagation();
    this.showSubtitlePanel = !this.showSubtitlePanel;
    this.showAudioPanel = false;
  }

  selectSubtitle(track: any | null): void {
    if (!this.player) return;
    if (track === null) {
      // Subtitles off
      this.player.setTextTrack(-1);
      this.activeSubtitleTrack = null;
    } else {
      this.player.setTextTrack(track.index);
      this.activeSubtitleTrack = track;
    }
    this.showSubtitlePanel = false;
    this.cdr.detectChanges();
  }

  toggleAudioMenu(event: MouseEvent): void {
    event.stopPropagation();
    this.showAudioPanel = !this.showAudioPanel;
    this.showSubtitlePanel = false;
  }

  selectAudio(track: any): void {
    if (!this.player) return;
    this.player.setCurrentTrack(track);
    this.activeAudioTrack = track;
    this.showAudioPanel = false;
    this.cdr.detectChanges();
  }

  toggleTelemetry(): void {
    this.showTelemetry = !this.showTelemetry;
  }

  toggleAspectRatio(): void {
    if (this.aspectRatioMode === 'fit') {
      this.aspectRatioMode = 'fill';
    } else if (this.aspectRatioMode === 'fill') {
      this.aspectRatioMode = 'stretch';
    } else {
      this.aspectRatioMode = 'fit';
    }
  }

  toggleFullscreen(): void {
    const container = document.documentElement;
    if (!document.fullscreenElement) {
      container
        .requestFullscreen()
        .then(() => {
          this.isFullscreen = true;
          this.cdr.detectChanges();
        })
        .catch((err) => {
          console.error('Error attempting to enable fullscreen:', err);
        });
    } else {
      document.exitFullscreen().then(() => {
        this.isFullscreen = false;
        this.cdr.detectChanges();
      });
    }
  }

  goBack(): void {
    if (document.fullscreenElement) {
      document.exitFullscreen().catch(() => {});
    }
    this.location.back();
  }

  getBackdropUrl(): string {
    if (this.mediaItem && this.mediaItem.backdrop_path) {
      return `/api/v1/media/${this.mediaId}/backdrop`;
    }
    return 'https://images.unsplash.com/photo-1574267431629-2e570984a62f?q=80&w=1600&auto=format&fit=crop';
  }

  formatTime(seconds: number): string {
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

  private updateProgress(): void {
    if (!this.player) return;
    const current = this.player.time();
    const duration = this.player.duration() || this.mediaItem?.duration || 0;

    this.currentTimeStr = this.formatTime(current);
    this.totalTimeStr = this.formatTime(duration);

    if (duration > 0) {
      this.progressPercent = (current / duration) * 100;
    }
    this.cdr.detectChanges();
  }

  private updateTelemetry(): void {
    if (!this.player || !this.showTelemetry || !this.videoElement) return;

    const streamInfo = this.player.getActiveStream();
    if (!streamInfo) return;

    const videoInfo = (this.player as any).getCurrentTrackFor('video');
    const bitrateInfoList = (this.player as any).getBitrateInfoListFor('video');
    const quality = (this.player as any).getQualityFor('video');

    if (bitrateInfoList && bitrateInfoList[quality]) {
      const bitRateKbps = bitrateInfoList[quality].bitrate / 1000;
      this.telemetryBitrate = `${(bitRateKbps / 1000).toFixed(1)} Mbps`;
    }

    const bufferVal = this.player.getBufferLength('video');
    this.telemetryBuffer = `${bufferVal.toFixed(1)}s`;

    if (videoInfo) {
      this.telemetryCodec = videoInfo.codec || 'H.264';
    }

    const videoEl = this.videoElement.nativeElement as any;
    if (videoEl.getVideoPlaybackQuality) {
      const qualityObj = videoEl.getVideoPlaybackQuality();
      this.telemetryDropped = qualityObj.droppedVideoFrames || 0;
    }
    this.cdr.detectChanges();
  }

  // Auto-hide UI controls when cursor stops moving
  resetControlsTimer(): void {
    this.controlsVisible = true;
    if (this.controlsTimer) {
      clearTimeout(this.controlsTimer);
    }
    this.controlsTimer = setTimeout(() => {
      if (this.isPlaying) {
        this.controlsVisible = false;
        this.showSubtitlePanel = false;
        this.showAudioPanel = false;
        this.cdr.detectChanges();
      }
    }, 4000);
  }

  onMouseMove(): void {
    this.resetControlsTimer();
  }

  // History sync implementation
  private startHistoryInterval(): void {
    this.clearHistoryInterval();
    this.historyIntervalId = setInterval(() => {
      this.saveHistory();
    }, 10000);
  }

  private clearHistoryInterval(): void {
    if (this.historyIntervalId) {
      clearInterval(this.historyIntervalId);
      this.historyIntervalId = null;
    }
  }

  private saveHistory(force = false): void {
    if (!this.player || !this.mediaItem) return;

    const current = this.player.time();
    const duration = this.player.duration() || this.mediaItem.duration || 0;

    if (duration <= 0) return;

    // Determine if complete (within 5 seconds of the end)
    const completed = duration - current <= 5;

    // Only update if position has moved significantly (at least 1 second) or force is true
    if (Math.abs(current - this.lastSavedPosition) >= 1 || force || completed) {
      this.lastSavedPosition = current;

      this.http
        .put(`/api/v1/history/${this.mediaId}`, {
          position: current,
          completed: completed,
        })
        .subscribe({
          next: () => {
            if (completed) {
              // Stop history sync and close/navigate away or let user close
              this.clearHistoryInterval();
            }
          },
          error: (err) => {
            console.error('Failed to save watch history:', err);
          },
        });
    }
  }

  // Helpers for click-away to close track menus
  closeMenus(): void {
    this.showSubtitlePanel = false;
    this.showAudioPanel = false;
  }

  onSubtitleSettingsClick(): void {
    console.log('Subtitle settings clicked');
  }

  setSubtitleSize(size: 'small' | 'medium' | 'large' | 'xlarge'): void {
    this.subtitleSize = size;
    localStorage.setItem('prism_subtitle_size', size);
    this.cdr.detectChanges();
  }
}
