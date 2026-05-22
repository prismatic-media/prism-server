package models

import (
	"time"

	"github.com/google/uuid"
)

// MediaType classifies the kind of media item.
type MediaType string

const (
	MediaTypeMovie   MediaType = "movie"
	MediaTypeTVShow  MediaType = "tvshow"
	MediaTypeEpisode MediaType = "episode"
	MediaTypeMusic   MediaType = "music"
)

// TranscodeStatus tracks the state of a transcode job.
type TranscodeStatus string

const (
	TranscodeStatusPending    TranscodeStatus = "pending"
	TranscodeStatusProcessing TranscodeStatus = "processing"
	TranscodeStatusDone       TranscodeStatus = "done"
	TranscodeStatusFailed     TranscodeStatus = "failed"
)

// User represents an application user.
type User struct {
	ID           uuid.UUID `db:"id" json:"id"`
	Username     string    `db:"username" json:"username"`
	Email        string    `db:"email" json:"email"`
	PasswordHash string    `db:"password_hash" json:"-"`
	IsAdmin      bool      `db:"is_admin" json:"is_admin"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at" json:"updated_at"`
}

// Library is a named collection of media at one or more paths.
type Library struct {
	ID        uuid.UUID `db:"id" json:"id"`
	Name      string    `db:"name" json:"name"`
	Path      string    `db:"path" json:"path"`
	MediaType MediaType `db:"media_type" json:"media_type"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// MediaItem represents a single piece of media on disk.
type MediaItem struct {
	ID         uuid.UUID `db:"id" json:"id"`
	LibraryID  uuid.UUID `db:"library_id" json:"library_id"`
	Title      string    `db:"title" json:"title"`
	MediaType  MediaType `db:"media_type" json:"media_type"`
	FilePath   string    `db:"file_path" json:"file_path"`
	FileSize   int64     `db:"file_size" json:"file_size"`
	Duration   float64   `db:"duration" json:"duration"` // seconds
	Width      int       `db:"width" json:"width"`
	Height     int       `db:"height" json:"height"`
	VideoCodec string    `db:"video_codec" json:"video_codec"`
	AudioCodec string    `db:"audio_codec" json:"audio_codec"`
	// Enriched metadata (nullable)
	TMDBId     *int    `db:"tmdb_id" json:"tmdb_id,omitempty"`
	Year       *int    `db:"year" json:"year,omitempty"`
	Overview   *string `db:"overview" json:"overview,omitempty"`
	PosterPath *string `db:"poster_path" json:"poster_path,omitempty"`
	// Transcode
	TranscodeStatus TranscodeStatus `db:"transcode_status" json:"transcode_status"`
	MPDPath         *string         `db:"mpd_path" json:"mpd_path,omitempty"`
	CreatedAt       time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time       `db:"updated_at" json:"updated_at"`
}

// TranscodeJob tracks an FFmpeg transcoding job.
type TranscodeJob struct {
	ID          uuid.UUID       `db:"id" json:"id"`
	MediaItemID uuid.UUID       `db:"media_item_id" json:"media_item_id"`
	Status      TranscodeStatus `db:"status" json:"status"`
	Progress    float64         `db:"progress" json:"progress"` // 0-100
	ErrorMsg    *string         `db:"error_msg" json:"error_msg,omitempty"`
	StartedAt   *time.Time      `db:"started_at" json:"started_at,omitempty"`
	FinishedAt  *time.Time      `db:"finished_at" json:"finished_at,omitempty"`
	CreatedAt   time.Time       `db:"created_at" json:"created_at"`
}

// WatchHistory records playback position per user per item.
type WatchHistory struct {
	ID          uuid.UUID `db:"id" json:"id"`
	UserID      uuid.UUID `db:"user_id" json:"user_id"`
	MediaItemID uuid.UUID `db:"media_item_id" json:"media_item_id"`
	Position    float64   `db:"position" json:"position"` // seconds
	Completed   bool      `db:"completed" json:"completed"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

// RefreshToken is a persisted, hashed refresh token used for JWT rotation.
// The raw token value is never stored; only its SHA-256 hash is kept.
type RefreshToken struct {
	ID        uuid.UUID `db:"id" json:"id"`
	UserID    uuid.UUID `db:"user_id" json:"user_id"`
	TokenHash string    `db:"token_hash" json:"-"`
	ExpiresAt time.Time `db:"expires_at" json:"expires_at"`
	Revoked   bool      `db:"revoked" json:"revoked"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
