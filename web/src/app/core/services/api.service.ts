import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { environment } from '../../../environments/environment';
import type {
  Library,
  MediaItem,
  TranscodeJob,
  WatchHistory,
  User,
} from '../models';

@Injectable({ providedIn: 'root' })
export class ApiService {
  private readonly http = inject(HttpClient);
  private readonly base = environment.apiBase;

  // --- Users ---
  getMe() {
    return this.http.get<User>(`${this.base}/me`);
  }

  updateMe(data: Partial<User>) {
    return this.http.put<User>(`${this.base}/me`, data);
  }

  createUser(data: { username: string; email: string; password: string; is_admin?: boolean }) {
    return this.http.post<User>(`${this.base}/users`, data);
  }

  // --- Libraries ---
  listLibraries() {
    return this.http.get<Library[]>(`${this.base}/libraries`);
  }

  getLibrary(id: string) {
    return this.http.get<Library>(`${this.base}/libraries/${id}`);
  }

  createLibrary(data: { name: string; path: string; media_type: string }) {
    return this.http.post<Library>(`${this.base}/libraries`, data);
  }

  deleteLibrary(id: string) {
    return this.http.delete<void>(`${this.base}/libraries/${id}`);
  }

  scanLibrary(id: string) {
    return this.http.post<void>(`${this.base}/libraries/${id}/scan`, null);
  }

  // --- Media items ---
  listMedia(libraryId?: string) {
    const params: Record<string, string> | undefined = libraryId ? { library_id: libraryId } : undefined;
    return this.http.get<MediaItem[]>(`${this.base}/media`, { params });
  }

  getMedia(id: string) {
    return this.http.get<MediaItem>(`${this.base}/media/${id}`);
  }

  deleteMedia(id: string) {
    return this.http.delete<void>(`${this.base}/media/${id}`);
  }

  posterUrl(id: string): string {
    return `${this.base}/media/${id}/poster`;
  }

  // --- Transcode jobs ---
  enqueueTranscode(mediaId: string) {
    return this.http.post<TranscodeJob>(`${this.base}/media/${mediaId}/transcode`, null);
  }

  listJobs() {
    return this.http.get<TranscodeJob[]>(`${this.base}/jobs`);
  }

  getJob(id: string) {
    return this.http.get<TranscodeJob>(`${this.base}/jobs/${id}`);
  }

  // --- Streaming ---
  manifestUrl(mediaId: string): string {
    return `${this.base}/stream/${mediaId}/manifest.mpd`;
  }

  // --- Watch history ---
  getHistory() {
    return this.http.get<WatchHistory[]>(`${this.base}/history`);
  }

  upsertHistory(mediaId: string, position: number, completed = false) {
    return this.http.put<WatchHistory>(`${this.base}/history/${mediaId}`, {
      position,
      completed,
    });
  }
}
