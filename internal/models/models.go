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
	TranscodeStatusNone       TranscodeStatus = "none"
	TranscodeStatusPending    TranscodeStatus = "pending"
	TranscodeStatusProcessing TranscodeStatus = "processing"
	TranscodeStatusDone       TranscodeStatus = "done"
	TranscodeStatusFailed     TranscodeStatus = "failed"
)

// SourceStatus tracks file availability.
const (
	SourceStatusAvailable = "available"
	SourceStatusMissing   = "missing"
)

// BundleStatus tracks transcode bundle availability.
const (
	BundleStatusNone      = "none"
	BundleStatusAvailable = "available"
	BundleStatusMissing   = "missing"
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

// Library is a collection of media at one or more paths.
type Library struct {
	ID        uuid.UUID `db:"id" json:"id"`
	Path      string    `db:"path" json:"path"`
	MediaType MediaType `db:"media_type" json:"media_type"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// CastMember represents a person in the cast.
type CastMember struct {
	Name        string `json:"name"`
	Character   string `json:"character"`
	ProfilePath string `json:"profile_path,omitempty"`
}

// TVShow represents a TV series tracked within a tvshow library.
type TVShow struct {
	ID           uuid.UUID    `db:"id" json:"id"`
	LibraryID    uuid.UUID    `db:"library_id" json:"library_id"`
	Name         string       `db:"name" json:"name"`
	TMDBId       *int         `db:"tmdb_id" json:"tmdb_id,omitempty"`
	Overview     *string      `db:"overview" json:"overview,omitempty"`
	PosterPath   *string      `db:"poster_path" json:"poster_path,omitempty"`
	FirstAirYear *int         `db:"first_air_year" json:"first_air_year,omitempty"`
	Director     *string      `db:"director" json:"director,omitempty"`
	Cast         []CastMember `db:"cast_members" json:"cast,omitempty"`
	BackdropPath *string      `db:"backdrop_path" json:"backdrop_path,omitempty"`
	ExtraPosters []string     `db:"extra_posters" json:"extra_posters,omitempty"`
	CreatedAt    time.Time    `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time    `db:"updated_at" json:"updated_at"`
}

// TVSeason represents a single season within a TVShow.
type TVSeason struct {
	ID           uuid.UUID `db:"id" json:"id"`
	TVShowID     uuid.UUID `db:"tv_show_id" json:"tv_show_id"`
	SeasonNumber int       `db:"season_number" json:"season_number"`
	TMDBId       *int      `db:"tmdb_id" json:"tmdb_id,omitempty"`
	Overview     *string   `db:"overview" json:"overview,omitempty"`
	PosterPath   *string   `db:"poster_path" json:"poster_path,omitempty"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at" json:"updated_at"`
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
	TMDBId       *int         `db:"tmdb_id" json:"tmdb_id,omitempty"`
	Year         *int         `db:"year" json:"year,omitempty"`
	Overview     *string      `db:"overview" json:"overview,omitempty"`
	PosterPath   *string      `db:"poster_path" json:"poster_path,omitempty"`
	Director     *string      `db:"director" json:"director,omitempty"`
	Cast         []CastMember `db:"cast_members" json:"cast,omitempty"`
	BackdropPath *string      `db:"backdrop_path" json:"backdrop_path,omitempty"`
	ExtraPosters []string     `db:"extra_posters" json:"extra_posters,omitempty"`
	// TV episode fields (nullable; populated only for MediaTypeEpisode items)
	TVShowID      *uuid.UUID `db:"tv_show_id" json:"tv_show_id,omitempty"`
	TVSeasonID    *uuid.UUID `db:"tv_season_id" json:"tv_season_id,omitempty"`
	SeasonNumber  *int       `db:"season_number" json:"season_number,omitempty"`
	EpisodeNumber *int       `db:"episode_number" json:"episode_number,omitempty"`
	TVShowTitle   *string    `json:"tv_show_title,omitempty"`
	// Transcode
	TranscodeStatus   TranscodeStatus     `db:"transcode_status" json:"transcode_status"`
	TranscodeProgress *float64            `json:"transcode_progress,omitempty"`
	SubJobs           []*TranscodeSubJob  `db:"-" json:"sub_jobs,omitempty"`
	MPDPath           *string             `db:"mpd_path" json:"mpd_path,omitempty"`
	SourceFingerprint *string             `db:"source_fingerprint" json:"source_fingerprint,omitempty"`
	SourceStatus      string              `db:"source_status" json:"source_status"`
	BundleStatus      string              `db:"bundle_status" json:"bundle_status"`
	TranscodeSizes    *TranscodeSizesInfo `db:"transcode_sizes" json:"transcode_sizes,omitempty"`
	CreatedAt         time.Time           `db:"created_at" json:"created_at"`
	UpdatedAt         time.Time           `db:"updated_at" json:"updated_at"`
}

// TranscodeWorker represents a remote transcode worker.
type TranscodeWorker struct {
	ID            uuid.UUID  `db:"id" json:"id"`
	Name          string     `db:"name" json:"name"`
	APIKey        string     `db:"api_key" json:"api_key,omitempty"`
	Threads       int        `db:"threads" json:"threads"`
	HWAccel       string     `db:"hwaccel" json:"hwaccel"`
	Status        string     `db:"status" json:"status"`
	LastHeartbeat *time.Time `db:"last_heartbeat" json:"last_heartbeat,omitempty"`
	IsEphemeral   bool       `db:"is_ephemeral" json:"is_ephemeral"`
	CreatedAt     time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time  `db:"updated_at" json:"updated_at"`
}

// EphemeralWorkerToken is a re-usable token used for registering ephemeral workers.
type EphemeralWorkerToken struct {
	ID        uuid.UUID `db:"id" json:"id"`
	Token     string    `db:"token" json:"token"`
	Name      string    `db:"name" json:"name"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}


// TranscodeJob tracks an FFmpeg transcoding job.
type TranscodeJob struct {
	ID          uuid.UUID       `db:"id" json:"id"`
	MediaItemID uuid.UUID       `db:"media_item_id" json:"media_item_id"`
	WorkerID    *uuid.UUID      `db:"worker_id" json:"worker_id,omitempty"`
	Status      TranscodeStatus `db:"status" json:"status"`
	Progress    float64         `db:"progress" json:"progress"` // 0-100
	Priority    int             `db:"priority" json:"priority"`
	ErrorMsg    *string         `db:"error_msg" json:"error_msg,omitempty"`
	StartedAt   *time.Time      `db:"started_at" json:"started_at,omitempty"`
	FinishedAt  *time.Time      `db:"finished_at" json:"finished_at,omitempty"`
	SubJobs     []*TranscodeSubJob `db:"-" json:"sub_jobs,omitempty"`
	CreatedAt   time.Time       `db:"created_at" json:"created_at"`
}

// TranscodeSubJob tracks a single stream/profile transcoding sub-job.
type TranscodeSubJob struct {
	ID             uuid.UUID       `db:"id" json:"id"`
	JobID          uuid.UUID       `db:"job_id" json:"job_id"`
	MediaItemID    uuid.UUID       `db:"media_item_id" json:"media_item_id"`
	WorkerID       *uuid.UUID      `db:"worker_id" json:"worker_id,omitempty"`
	Type           string          `db:"type" json:"type"` // "video" or "subtitles"
	ProfileID      *uuid.UUID      `db:"profile_id" json:"profile_id,omitempty"`
	ProfileName    *string         `db:"profile_name" json:"profile_name,omitempty"`
	Width          *int            `db:"width" json:"width,omitempty"`
	Height         *int            `db:"height" json:"height,omitempty"`
	VideoBitrateK  *int            `db:"video_bitrate_k" json:"video_bitrate_k,omitempty"`
	AudioBitrateK  *int            `db:"audio_bitrate_k" json:"audio_bitrate_k,omitempty"`
	Codec          *string         `db:"codec" json:"codec,omitempty"`
	Status         TranscodeStatus `db:"status" json:"status"`
	Progress       float64         `db:"progress" json:"progress"` // 0-100
	ErrorMsg       *string         `db:"error_msg" json:"error_msg,omitempty"`
	StartedAt      *time.Time      `db:"started_at" json:"started_at,omitempty"`
	FinishedAt     *time.Time      `db:"finished_at" json:"finished_at,omitempty"`
	CreatedAt      time.Time       `db:"created_at" json:"created_at"`
}

const (
	SubJobTypeVideo     = "video"
	SubJobTypeSubtitles = "subtitles"
	SubJobTypeWhisper   = "whisper"
)

// WatchHistory records playback position per user per item.
type WatchHistory struct {
	ID          uuid.UUID  `db:"id" json:"id"`
	UserID      uuid.UUID  `db:"user_id" json:"user_id"`
	MediaItemID uuid.UUID  `db:"media_item_id" json:"media_item_id"`
	Position    float64    `db:"position" json:"position"` // seconds
	Completed   bool       `db:"completed" json:"completed"`
	UpdatedAt   time.Time  `db:"updated_at" json:"updated_at"`
	Media       *MediaItem `json:"media,omitempty"`
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

// StorageAreaKind classifies storage area purpose.
type StorageAreaKind string

const (
	StorageAreaKindSegments StorageAreaKind = "segments"
)

// StorageArea describes a filesystem path managed by the storage subsystem.
type StorageArea struct {
	ID        uuid.UUID       `db:"id" json:"id"`
	Kind      StorageAreaKind `db:"kind" json:"kind"`
	Path      string          `db:"path" json:"path"`
	Enabled   bool            `db:"enabled" json:"enabled"`
	CreatedAt time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt time.Time       `db:"updated_at" json:"updated_at"`
}

// ArtifactHealth classifies the state of a discovered transcode artifact bundle.
type ArtifactHealth string

const (
	ArtifactHealthUnknown       ArtifactHealth = "unknown"
	ArtifactHealthHealthy       ArtifactHealth = "healthy"
	ArtifactHealthStale         ArtifactHealth = "stale"
	ArtifactHealthMissing       ArtifactHealth = "missing"
	ArtifactHealthMetadataInvalid ArtifactHealth = "metadata_invalid"
	ArtifactHealthUnavailable   ArtifactHealth = "unavailable"
)

// ArtifactMatchedVia classifies how an artifact was linked to a media item.
type ArtifactMatchedVia string

const (
	ArtifactMatchedViaFingerprint ArtifactMatchedVia = "fingerprint"
	ArtifactMatchedViaHeuristic   ArtifactMatchedVia = "heuristic"
	ArtifactMatchedViaManual      ArtifactMatchedVia = "manual"
)

// ArtifactLinkStatus classifies the confidence of an artifact-to-media link.
type ArtifactLinkStatus string

const (
	ArtifactLinkLinked   ArtifactLinkStatus = "linked"
	ArtifactLinkUnmatched ArtifactLinkStatus = "unmatched"
	ArtifactLinkAmbiguous ArtifactLinkStatus = "ambiguous"
)

// ArtifactRecord represents a discovered transcode artifact bundle on disk.
type ArtifactRecord struct {
	ID               uuid.UUID       `db:"id" json:"id"`
	StorageAreaID    uuid.UUID       `db:"storage_area_id" json:"storage_area_id"`
	SourcePath       string          `db:"source_path" json:"source_path"`
	SourceFingerprint *string        `db:"source_fingerprint" json:"source_fingerprint,omitempty"`
	OutputDir        string          `db:"output_dir" json:"output_dir,omitempty"`
	MPDPath          string          `db:"mpd_path" json:"mpd_path,omitempty"`
	Health           ArtifactHealth  `db:"health" json:"health"`
	LastSeenAt       time.Time       `db:"last_seen_at" json:"last_seen_at"`
	RegisteredAt     time.Time       `db:"registered_at" json:"registered_at"`
	UpdatedAt        time.Time       `db:"updated_at" json:"updated_at"`
}

// ArtifactMediaLink associates a discovered artifact with a media item.
type ArtifactMediaLink struct {
	ID          uuid.UUID         `db:"id" json:"id"`
	ArtifactID  uuid.UUID         `db:"artifact_id" json:"artifact_id"`
	MediaItemID uuid.UUID         `db:"media_item_id" json:"media_item_id"`
	MatchedVia  ArtifactMatchedVia `db:"matched_via" json:"matched_via"`
	Status      ArtifactLinkStatus `db:"status" json:"status"`
	CreatedAt   time.Time         `db:"created_at" json:"created_at"`
}

// ArtifactIndexSummary provides counts from an indexing operation.
type ArtifactIndexSummary struct {
	TotalDiscovered   int `json:"total_discovered"`
	RegisteredNew     int `json:"registered_new"`
	UpdatedExisting   int `json:"updated_existing"`
	MarkedMissing     int `json:"marked_missing"`
	Invalid           int `json:"invalid"`
	MediaItemsCreated int `json:"media_items_created"`
}

// ArtifactRelinkSummary provides counts from a relinking operation.
type ArtifactRelinkSummary struct {
	Linked    int `json:"linked"`
	Unmatched int `json:"unmatched"`
	Ambiguous int `json:"ambiguous"`
	Invalid   int `json:"invalid"`
}

// SearchResult represents a single item matched in a global search.
type SearchResult struct {
	ID           uuid.UUID    `json:"id"`
	Title        string       `json:"title"`
	MediaType    string       `json:"media_type"` // "movie" or "tvshow"
	Overview     *string      `json:"overview,omitempty"`
	PosterPath   *string      `json:"poster_path,omitempty"`
	Year         *int         `json:"year,omitempty"`
	Director     *string      `json:"director,omitempty"`
	Cast         []CastMember `json:"cast,omitempty"`
}

// RenditionSize describes a transcode rendition name and its combined size on disk.
type RenditionSize struct {
	Resolution string `json:"resolution"` // e.g., "360p", "480p", etc.
	Size       int64  `json:"size"`       // Total size of all files in the rendition directory in bytes
}

// TranscodeSizesInfo holds details about the transcode bundle resolution sizes and total size.
type TranscodeSizesInfo struct {
	Renditions []RenditionSize `json:"renditions"`
	TotalSize  int64           `json:"total_size"`
}

// TranscodeProfile represents a user-configurable rendition profile.
type TranscodeProfile struct {
	ID            uuid.UUID `db:"id" json:"id"`
	Name          string    `db:"name" json:"name"`
	Width         int       `db:"width" json:"width"`
	Height        int       `db:"height" json:"height"`
	VideoBitrateK int       `db:"video_bitrate_k" json:"video_bitrate_k"`
	AudioBitrateK int       `db:"audio_bitrate_k" json:"audio_bitrate_k"`
	Codec         string    `db:"codec" json:"codec"`
	IsActive      bool      `db:"is_active" json:"is_active"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time `db:"updated_at" json:"updated_at"`
}

// MediaSubtitle represents an uploaded subtitle track.
type MediaSubtitle struct {
	ID              uuid.UUID `db:"id" json:"id"`
	MediaItemID     uuid.UUID `db:"media_item_id" json:"media_item_id"`
	Language        string    `db:"language" json:"language"`
	Label           string    `db:"label" json:"label"`
	VTTContent      string    `db:"vtt_content" json:"vtt_content"`
	SimilarityScore *float64  `db:"similarity_score" json:"similarity_score"`
	SyncOffset      float64   `db:"sync_offset" json:"sync_offset"`
	AlignmentStatus string    `db:"alignment_status" json:"alignment_status"`
	CreatedAt       time.Time `db:"created_at" json:"created_at"`
}

// WhisperTranscription holds the Whisper Speech-to-Text output for a media item.
type WhisperTranscription struct {
	ID          uuid.UUID `db:"id" json:"id"`
	MediaItemID uuid.UUID `db:"media_item_id" json:"media_item_id"`
	Language    string    `db:"language" json:"language"`
	VTTContent  string    `db:"vtt_content" json:"vtt_content"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
}




