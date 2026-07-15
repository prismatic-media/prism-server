// Package events provides a global fan-out event bus used to push real-time
// updates (transcode progress, new media items, etc.) to WebSocket clients.
package events

import (
	"encoding/json"
	"sync"

	"github.com/google/uuid"
	"github.com/prismatic-media/prism-server/internal/models"
)

// EventType is the discriminator field on every Event.
type EventType string

const (
	// EventJobProgress is published on every transcode progress tick and on completion.
	EventJobProgress EventType = "job.progress"
	// EventMediaUpdated is published when a media item's transcode_status changes.
	EventMediaUpdated EventType = "media.updated"
	// EventMediaCreated is published when a new media item is discovered by the scanner.
	EventMediaCreated EventType = "media.created"
	// EventMediaEnriched is published when TMDB metadata (including a poster) is
	// fetched for a media item after it was first scanned.
	EventMediaEnriched EventType = "media.enriched"
	// EventSubtitleAligned is published when subtitle auto-alignment is completed or failed.
	EventSubtitleAligned EventType = "subtitle.aligned"
	// EventTVShowCreated is published when a TV show is discovered.
	EventTVShowCreated EventType = "tvshow.created"
	// EventTVShowUpdated is published when TV show metadata is updated/enriched.
	EventTVShowUpdated EventType = "tvshow.updated"
	// EventJobCreated is published when a job is enqueued.
	EventJobCreated EventType = "job.created"
	// EventJobUpdated is published when job status changes.
	EventJobUpdated EventType = "job.updated"
)

// SubtitleAlignedPayload carries alignment results.
type SubtitleAlignedPayload struct {
	SubtitleID      uuid.UUID `json:"subtitle_id"`
	MediaItemID     uuid.UUID `json:"media_item_id"`
	SimilarityScore *float64  `json:"similarity_score"`
	SyncOffset      float64   `json:"sync_offset"`
	AlignmentStatus string    `json:"alignment_status"`
	Error           string    `json:"error,omitempty"`
}

// Event is the wire-format envelope sent to WebSocket clients.
type Event struct {
	Type    EventType       `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// JobProgressPayload carries transcode progress for one job.
type JobProgressPayload struct {
	JobID       uuid.UUID                 `json:"job_id"`
	MediaItemID uuid.UUID                 `json:"media_item_id"`
	WorkerID    *uuid.UUID                `json:"worker_id,omitempty"`
	Progress    float64                   `json:"progress"`
	Done        bool                      `json:"done"`
	Error       string                    `json:"error,omitempty"`
	SubJobs     []*models.TranscodeSubJob `json:"sub_jobs,omitempty"`
}

// MediaUpdatedPayload carries the full updated media item.
type MediaUpdatedPayload struct {
	MediaItem *models.MediaItem `json:"media_item"`
}

// MediaCreatedPayload carries the full created media item.
type MediaCreatedPayload struct {
	MediaItem *models.MediaItem `json:"media_item"`
}

// MediaEnrichedPayload carries the full enriched media item.
type MediaEnrichedPayload struct {
	MediaItem *models.MediaItem `json:"media_item"`
}

// TVShowCreatedPayload carries the full created TV show.
type TVShowCreatedPayload struct {
	TVShow *models.TVShow `json:"tv_show"`
}

// TVShowUpdatedPayload carries the full updated TV show.
type TVShowUpdatedPayload struct {
	TVShow *models.TVShow `json:"tv_show"`
}

// JobCreatedPayload carries the full created transcode job.
type JobCreatedPayload struct {
	Job *models.TranscodeJob `json:"job"`
}

// JobUpdatedPayload carries the full updated transcode job.
type JobUpdatedPayload struct {
	Job *models.TranscodeJob `json:"job"`
}

// Bus is a goroutine-safe broadcast bus. All registered subscribers receive
// every published event. Slow subscribers are silently dropped (non-blocking
// send), so channel buffers should be appropriately sized.
type Bus struct {
	mu   sync.RWMutex
	subs map[string]chan Event
}

// NewBus creates an empty Bus ready for use.
func NewBus() *Bus {
	return &Bus{subs: make(map[string]chan Event)}
}

// Subscribe registers a new subscriber and returns its ID (needed to
// unsubscribe) and a read-only channel of events.
func (b *Bus) Subscribe() (id string, ch <-chan Event) {
	subID := uuid.NewString()
	c := make(chan Event, 64)
	b.mu.Lock()
	b.subs[subID] = c
	b.mu.Unlock()
	return subID, c
}

// Unsubscribe removes a subscriber and closes its channel.
func (b *Bus) Unsubscribe(id string) {
	b.mu.Lock()
	if c, ok := b.subs[id]; ok {
		delete(b.subs, id)
		close(c)
	}
	b.mu.Unlock()
}

// Publish marshals payload and fans the event out to all current subscribers.
// It never blocks; slow subscribers have their events dropped.
func (b *Bus) Publish(t EventType, payload any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	evt := Event{Type: t, Payload: raw}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, c := range b.subs {
		select {
		case c <- evt:
		default:
		}
	}
}
