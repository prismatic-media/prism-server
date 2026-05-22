// Shared domain models matching the Go API response shapes.

export interface User {
  id: string;
  username: string;
  email: string;
  is_admin: boolean;
  created_at: string;
}

export interface Library {
  id: string;
  name: string;
  path: string;
  media_type: 'movie' | 'tvshow' | 'episode' | 'music';
  created_at: string;
}

export interface MediaItem {
  id: string;
  library_id: string;
  title: string;
  media_type: string;
  file_path: string;
  file_size: number;
  duration: number;
  width: number;
  height: number;
  video_codec: string;
  audio_codec: string;
  tmdb_id?: number;
  year?: number;
  overview?: string;
  poster_path?: string;
  transcode_status: 'pending' | 'processing' | 'done' | 'failed';
  mpd_path?: string;
  created_at: string;
  updated_at: string;
}

export interface TranscodeJob {
  id: string;
  media_item_id: string;
  status: 'pending' | 'processing' | 'done' | 'failed';
  progress: number;
  error_msg?: string;
  created_at: string;
  started_at?: string;
  finished_at?: string;
}

export interface WatchHistory {
  id: string;
  user_id: string;
  media_item_id: string;
  position: number;
  completed: boolean;
  updated_at: string;
}

export interface LoginRequest {
  username: string;
  password: string;
}

export interface TokenResponse {
  access_token: string;
  refresh_token: string;
}
