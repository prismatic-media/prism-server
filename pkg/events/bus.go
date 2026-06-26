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
	Progress    float64                   `json:"progress"`
	Done        bool                      `json:"done"`
	Error       string                    `json:"error,omitempty"`
	SubJobs     []*models.TranscodeSubJob `json:"sub_jobs,omitempty"`
}

// MediaUpdatedPayload is published when transcode_status changes.
type MediaUpdatedPayload struct {
	MediaItemID     uuid.UUID `json:"media_item_id"`
	LibraryID       uuid.UUID `json:"library_id"`
	TranscodeStatus string    `json:"transcode_status"`
}

// MediaCreatedPayload is published when a media item is first discovered.
type MediaCreatedPayload struct {
	MediaItemID uuid.UUID `json:"media_item_id"`
	LibraryID   uuid.UUID `json:"library_id"`
	Title       string    `json:"title"`
}

// MediaEnrichedPayload is published when TMDB metadata has been fetched and
// a poster image is available for a media item.
type MediaEnrichedPayload struct {
	MediaItemID uuid.UUID `json:"media_item_id"`
	LibraryID   uuid.UUID `json:"library_id"`
	PosterPath  string    `json:"poster_path"`
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
