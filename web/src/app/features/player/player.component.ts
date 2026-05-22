import {
  Component,
  OnInit,
  OnDestroy,
  AfterViewInit,
  ElementRef,
  ViewChild,
  inject,
  signal,
} from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { CommonModule } from '@angular/common';
import * as dashjs from 'dashjs';
import { ApiService } from '../../core/services/api.service';
import { AuthService } from '../../core/services/auth.service';

const REPORT_INTERVAL_MS = 10_000;

/** Common ISO 639-2/B and 639-1 codes → English display names. */
const ISO_LANG_NAMES: Record<string, string> = {
  eng: 'English', en: 'English',
  fra: 'French',  fr: 'French',
  fre: 'French',
  deu: 'German',  de: 'German',
  ger: 'German',
  spa: 'Spanish', es: 'Spanish',
  ita: 'Italian', it: 'Italian',
  por: 'Portuguese', pt: 'Portuguese',
  rus: 'Russian', ru: 'Russian',
  jpn: 'Japanese', ja: 'Japanese',
  zho: 'Chinese', chi: 'Chinese', zh: 'Chinese',
  kor: 'Korean',  ko: 'Korean',
  ara: 'Arabic',  ar: 'Arabic',
  nld: 'Dutch',   nl: 'Dutch',
  dut: 'Dutch',
  pol: 'Polish',  pl: 'Polish',
  swe: 'Swedish', sv: 'Swedish',
  nor: 'Norwegian', no: 'Norwegian',
  dan: 'Danish',  da: 'Danish',
  fin: 'Finnish', fi: 'Finnish',
  hun: 'Hungarian', hu: 'Hungarian',
  ces: 'Czech',   cze: 'Czech', cs: 'Czech',
  slk: 'Slovak',  slo: 'Slovak', sk: 'Slovak',
  ron: 'Romanian', rum: 'Romanian', ro: 'Romanian',
  tur: 'Turkish', tr: 'Turkish',
  heb: 'Hebrew',  he: 'Hebrew',
  tha: 'Thai',    th: 'Thai',
  vie: 'Vietnamese', vi: 'Vietnamese',
  ind: 'Indonesian', id: 'Indonesian',
  und: 'Undetermined',
};

function langLabel(code: string): string {
  if (!code) return 'Unknown';
  return ISO_LANG_NAMES[code.toLowerCase()] ?? code.toUpperCase();
}

@Component({
  selector: 'app-player',
  standalone: true,
  imports: [CommonModule],
  template: `
    <div class="player-wrapper">
      <div class="player-toolbar">
        <button class="back-btn" (click)="goBack()">← Back</button>
        <span class="title-label">{{ title() }}</span>
        <div class="controls-right">
          <select class="quality-sel" (change)="setQuality($event)">
            <option value="-1">Auto quality</option>
            @for (q of qualities(); track q.index) {
              <option [value]="q.index">{{ q.height }}p</option>
            }
          </select>
          @if (subtitles().length > 0) {
            <select class="sub-sel" (change)="setSubtitle($event)">
              <option value="-1">Off</option>
              @for (s of subtitles(); track s.index) {
                <option [value]="s.index">{{ s.lang }}</option>
              }
            </select>
          }
        </div>
      </div>

      <video
        #videoEl
        class="video"
        controls
        autoplay
        playsinline
      ></video>

      @if (error()) {
        <div class="error-banner">{{ error() }}</div>
      }
    </div>
  `,
  styleUrl: './player.component.scss',
})
export class PlayerComponent implements OnInit, AfterViewInit, OnDestroy {
  @ViewChild('videoEl') videoRef!: ElementRef<HTMLVideoElement>;

  private readonly api = inject(ApiService);
  private readonly auth = inject(AuthService);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);

  title = signal('');
  error = signal('');
  qualities = signal<{ index: number; height: number }[]>([]);
  subtitles = signal<{ index: number; lang: string }[]>([]);

  private player!: dashjs.MediaPlayerClass;
  private mediaId = '';
  private startPosition = 0;
  private reportTimer?: ReturnType<typeof setInterval>;

  ngOnInit() {
    this.mediaId = this.route.snapshot.paramMap.get('id')!;
    this.api.getMedia(this.mediaId).subscribe((m) => this.title.set(m.title));

    // Resume from history if available.
    this.api.getHistory().subscribe((items) => {
      const h = items.find((x) => x.media_item_id === this.mediaId);
      this.startPosition = h?.position ?? 0;
    });
  }

  ngAfterViewInit() {
    this.initPlayer();
  }

  ngOnDestroy() {
    this.stopReporting();
    if (this.player) {
      // Save final position before destroying.
      const pos = this.player.time();
      const dur = this.player.duration();
      const completed = dur > 0 && pos >= dur - 5;
      this.api.upsertHistory(this.mediaId, pos, completed).subscribe();
      this.player.destroy();
    }
  }

  private initPlayer() {
    const video = this.videoRef.nativeElement;
    const mpdUrl = this.api.manifestUrl(this.mediaId);

    this.player = dashjs.MediaPlayer().create();

    // Attach the JWT to every request dash.js makes (manifest + segments).
    const token = this.auth.accessToken();
    if (token) {
      this.player.addRequestInterceptor((req: any) => {
        req.headers = { ...(req.headers ?? {}), Authorization: `Bearer ${token}` };
        return Promise.resolve(req);
      });
    }

    this.player.initialize(video, mpdUrl, true);

    if (this.startPosition > 0) {
      this.player.seek(this.startPosition);
    }

    // Populate quality list once stream initialises.
    this.player.on(dashjs.MediaPlayer.events.STREAM_INITIALIZED, () => {
      const reps = this.player.getRepresentationsByType('video' as dashjs.MediaType) ?? [];
      this.qualities.set(
        reps.map((r) => ({ index: r.index, height: r.height })),
      );
    });

    // Populate subtitle list once dash.js has loaded the text track metadata.
    // TEXT_TRACKS_ADDED fires after STREAM_INITIALIZED and guarantees the
    // tracks are present on the <video> element.
    this.player.on((dashjs.MediaPlayer.events as any).TEXT_TRACKS_ADDED ?? 'textTracksAdded', () => {
      const textTracks: TextTrack[] = Array.from(video.textTracks ?? []);
      this.subtitles.set(
        textTracks.map((t, i) => ({
          index: i,
          lang: langLabel(t.language || t.label || `Track ${i + 1}`),
        })),
      );
    });

    this.player.on(dashjs.MediaPlayer.events.ERROR, (e: any) => {
      this.error.set(`Playback error: ${e?.error?.message ?? 'unknown'}`);
    });

    this.startReporting();
  }

  setQuality(event: Event) {
    const val = parseInt((event.target as HTMLSelectElement).value, 10);
    if (val === -1) {
      this.player.updateSettings({ streaming: { abr: { autoSwitchBitrate: { video: true } } } });
    } else {
      this.player.updateSettings({ streaming: { abr: { autoSwitchBitrate: { video: false } } } });
      this.player.setRepresentationForTypeByIndex('video' as dashjs.MediaType, val);
    }
  }

  setSubtitle(event: Event) {
    const val = parseInt((event.target as HTMLSelectElement).value, 10);
    const tracks = Array.from(this.videoRef.nativeElement.textTracks);
    tracks.forEach((t, i) => {
      t.mode = i === val ? 'showing' : 'hidden';
    });
  }

  private startReporting() {
    this.reportTimer = setInterval(() => {
      if (!this.player) return;
      const pos = this.player.time();
      if (pos > 0) {
        this.api.upsertHistory(this.mediaId, pos, false).subscribe();
      }
    }, REPORT_INTERVAL_MS);
  }

  private stopReporting() {
    if (this.reportTimer) clearInterval(this.reportTimer);
  }

  goBack() {
    this.router.navigate(['/media', this.mediaId]);
  }
}
