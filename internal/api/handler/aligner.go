package handler

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math"

	"github.com/google/uuid"

	"github.com/prismatic-media/prism-server/internal/store/sqlite"
	"github.com/prismatic-media/prism-server/pkg/events"
	"github.com/prismatic-media/prism-server/pkg/subtitle"
)

// AlignPendingSubtitles automatically triggers alignment for all pending subtitles of a media item.
func AlignPendingSubtitles(ctx context.Context, db *sql.DB, bus *events.Bus, mediaItemID uuid.UUID) error {
	slog.Info("aligning all pending subtitles for media item", "media_item_id", mediaItemID)

	// 1. Check if Whisper transcription exists
	hasWhisper, err := sqlite.HasWhisperTranscription(ctx, db, mediaItemID)
	if err != nil {
		return fmt.Errorf("checking whisper transcription: %w", err)
	}
	if !hasWhisper {
		slog.Info("no whisper transcription available yet, skipping auto-alignment", "media_item_id", mediaItemID)
		return nil
	}

	// 2. Fetch all subtitles
	subs, err := sqlite.ListMediaSubtitles(ctx, db, mediaItemID)
	if err != nil {
		return fmt.Errorf("listing media subtitles: %w", err)
	}

	for _, sub := range subs {
		if sub.AlignmentStatus == "pending" {
			go RunSubtitleAlignment(context.Background(), db, bus, sub.ID)
		}
	}
	return nil
}

// RunSubtitleAlignment performs similarity checking and auto-sync alignment.
// It is intended to run asynchronously in a background goroutine.
func RunSubtitleAlignment(ctx context.Context, db *sql.DB, bus *events.Bus, subtitleID uuid.UUID) {
	slog.Info("starting subtitle alignment background job", "subtitle_id", subtitleID)

	// 1. Set status to processing
	err := sqlite.UpdateMediaSubtitleStatus(ctx, db, subtitleID, "processing")
	if err != nil {
		slog.Error("failed to update subtitle status to processing", "subtitle_id", subtitleID, "error", err)
		return
	}

	// Helper to fail the job cleanly
	failJob := func(errReason error) {
		slog.Error("subtitle alignment failed", "subtitle_id", subtitleID, "error", errReason)
		_ = sqlite.UpdateMediaSubtitleStatus(ctx, db, subtitleID, "failed")
		bus.Publish(events.EventSubtitleAligned, events.SubtitleAlignedPayload{
			SubtitleID:      subtitleID,
			AlignmentStatus: "failed",
			Error:           errReason.Error(),
		})
	}

	// 2. Fetch subtitle details
	sub, err := sqlite.GetMediaSubtitleByID(ctx, db, subtitleID)
	if err != nil {
		failJob(fmt.Errorf("fetching subtitle: %w", err))
		return
	}

	// 4. Fetch Whisper transcription
	transcription, err := sqlite.GetWhisperTranscriptionByMediaItem(ctx, db, sub.MediaItemID)
	if err != nil {
		failJob(fmt.Errorf("fetching whisper transcription from DB: %w", err))
		return
	}

	// 5. Parse Whisper transcription blocks
	refBlocks, err := subtitle.ParseSubtitleBlocks(transcription.VTTContent)
	if err != nil {
		failJob(fmt.Errorf("parsing whisper transcription VTT: %w", err))
		return
	}

	// 6. Perform alignment
	uploadedBlocks, err := subtitle.ParseSubtitleBlocks(sub.VTTContent)
	if err != nil {
		failJob(fmt.Errorf("parsing uploaded subtitle VTT content: %w", err))
		return
	}

	offset, score := subtitle.AlignSubtitles(refBlocks, uploadedBlocks)
	slog.Info("subtitle alignment completed", "subtitle_id", subtitleID, "offset_seconds", offset, "similarity_score", score)

	// 7. Apply offset to shift VTT timestamps
	shiftedVTT := sub.VTTContent
	if math.Abs(offset) > 0.001 {
		shiftedVTT, err = subtitle.ShiftVTT(sub.VTTContent, offset)
		if err != nil {
			failJob(fmt.Errorf("shifting subtitle timestamps: %w", err))
			return
		}
	}

	// 8. Update subtitle in DB
	err = sqlite.UpdateMediaSubtitleAlignment(ctx, db, subtitleID, "completed", &score, offset, shiftedVTT)
	if err != nil {
		failJob(fmt.Errorf("saving alignment results to database: %w", err))
		return
	}

	// 9. Sync to disk output directory and regenerate DASH manifest
	err = syncSubtitlesAndRegenerateMPD(ctx, db, sub.MediaItemID)
	if err != nil {
		slog.Warn("syncing subtitles/regenerating manifest failed after alignment", "subtitle_id", subtitleID, "error", err)
	}

	// 10. Publish event to WebSocket clients
	bus.Publish(events.EventSubtitleAligned, events.SubtitleAlignedPayload{
		SubtitleID:      subtitleID,
		MediaItemID:     sub.MediaItemID,
		SimilarityScore: &score,
		SyncOffset:      offset,
		AlignmentStatus: "completed",
	})
}
