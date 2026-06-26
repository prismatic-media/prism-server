package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/prismatic-media/prism-server/internal/store/sqlite"
	"github.com/prismatic-media/prism-server/pkg/events"
	"github.com/prismatic-media/prism-server/pkg/ffmpeg"
	"github.com/prismatic-media/prism-server/pkg/subtitle"
)

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

	// 3. Fetch corresponding MediaItem
	item, err := sqlite.GetMediaItemByID(ctx, db, sub.MediaItemID)
	if err != nil {
		failJob(fmt.Errorf("fetching media item: %w", err))
		return
	}

	// Check if source file exists
	if _, err := os.Stat(item.FilePath); os.IsNotExist(err) {
		failJob(fmt.Errorf("media source file not found: %s", item.FilePath))
		return
	}

	// Get Whisper binary and model paths from DB settings
	whisperBin, err := sqlite.GetSetting(ctx, db, "whisper_binary_path")
	if err != nil || whisperBin == "" {
		whisperBin = "whisper-cli"
	}
	whisperModel, err := sqlite.GetSetting(ctx, db, "whisper_model_path")
	if err != nil {
		whisperModel = ""
	}

	var refBlocks []subtitle.SubtitleBlock

	// 4. Try extracting embedded subtitles first
	probeRes, err := ffmpeg.Probe(ctx, "ffprobe", item.FilePath)
	if err != nil {
		slog.Warn("ffprobe failed on source file, attempting fallback to Whisper", "file", item.FilePath, "error", err)
	}

	hasEmbedded := false
	var selectedStreamIndex int
	if probeRes != nil && len(probeRes.SubtitleStreams) > 0 {
		// Look for a text subtitle matching the uploaded subtitle's language
		for _, s := range probeRes.SubtitleStreams {
			// Skip bitmap subtitles which can't be converted
			if _, isBitmap := bitmapSubtitleCodecs[s.Codec]; isBitmap {
				continue
			}
			// Exact match or fallback to first subtitle stream
			if strings.EqualFold(s.Language, sub.Language) || s.Language == "" {
				selectedStreamIndex = s.Index
				hasEmbedded = true
				break
			}
		}
		// If no language match, fall back to the first non-bitmap text subtitle
		if !hasEmbedded {
			for _, s := range probeRes.SubtitleStreams {
				if _, isBitmap := bitmapSubtitleCodecs[s.Codec]; !isBitmap {
					selectedStreamIndex = s.Index
					hasEmbedded = true
					break
				}
			}
		}
	}

	if hasEmbedded {
		slog.Info("found embedded subtitle stream, extracting for alignment reference", "subtitle_id", subtitleID, "stream_index", selectedStreamIndex)
		tmpVTT, err := os.CreateTemp("", "prism-extracted-*.vtt")
		if err == nil {
			tmpVTT.Close()
			defer os.Remove(tmpVTT.Name())

			args := []string{
				"-y", "-i", item.FilePath,
				"-map", fmt.Sprintf("0:%d", selectedStreamIndex),
				"-c:s", "webvtt",
				tmpVTT.Name(),
			}
			cmd := exec.CommandContext(ctx, "ffmpeg", args...)
			if out, err := cmd.CombinedOutput(); err != nil {
				slog.Warn("failed to extract embedded subtitle via ffmpeg, falling back to Whisper", "error", err, "output", string(out))
				hasEmbedded = false
			} else {
				vttBytes, err := os.ReadFile(tmpVTT.Name())
				if err != nil {
					slog.Warn("failed to read extracted subtitle temp file, falling back to Whisper", "error", err)
					hasEmbedded = false
				} else {
					refBlocks, err = subtitle.ParseSubtitleBlocks(string(vttBytes))
					if err != nil || len(refBlocks) == 0 {
						slog.Warn("failed to parse extracted embedded subtitle blocks, falling back to Whisper", "error", err)
						hasEmbedded = false
					}
				}
			}
		} else {
			slog.Warn("failed to create temp file for embedded subtitle extraction, falling back to Whisper", "error", err)
			hasEmbedded = false
		}
	}

	// 5. Fallback to Whisper Speech-to-Text if embedded subtitles aren't available
	if !hasEmbedded {
		slog.Info("no embedded subtitles available, transcribing full audio using Whisper", "subtitle_id", subtitleID)

		if whisperModel == "" {
			failJob(errors.New("alignment failed: Whisper model path is not configured in Admin Settings"))
			return
		}

		// Ensure Whisper binary exists/is executable
		if _, err := exec.LookPath(whisperBin); err != nil {
			failJob(fmt.Errorf("alignment failed: Whisper CLI executable %q not found on system PATH: %w", whisperBin, err))
			return
		}

		tmpWav, err := os.CreateTemp("", "prism-audio-*.wav")
		if err != nil {
			failJob(fmt.Errorf("creating temp wav file: %w", err))
			return
		}
		tmpWav.Close()
		defer os.Remove(tmpWav.Name())

		// Extract full 16kHz mono WAV audio
		ffmpegArgs := []string{
			"-y",
			"-i", item.FilePath,
			"-ar", "16000",
			"-ac", "1",
			"-c:a", "pcm_s16le",
			tmpWav.Name(),
		}
		ffmpegCmd := exec.CommandContext(ctx, "ffmpeg", ffmpegArgs...)
		if out, err := ffmpegCmd.CombinedOutput(); err != nil {
			failJob(fmt.Errorf("extracting full audio via ffmpeg: %w (output: %s)", err, string(out)))
			return
		}

		// Run Whisper transcription
		tmpOutPrefix := filepath.Join(os.TempDir(), fmt.Sprintf("whisper-out-%s", uuid.NewString()))
		tmpVTTFile := tmpOutPrefix + ".vtt"
		defer os.Remove(tmpVTTFile)

		langCode := map3To2Lang(sub.Language)
		whisperArgs := []string{
			"-m", whisperModel,
			"-f", tmpWav.Name(),
			"-ovtt",
			"-of", tmpOutPrefix,
			"-l", langCode,
		}
		whisperCmd := exec.CommandContext(ctx, whisperBin, whisperArgs...)
		if out, err := whisperCmd.CombinedOutput(); err != nil {
			failJob(fmt.Errorf("transcribing full audio via Whisper: %w (output: %s)", err, string(out)))
			return
		}

		// Parse VTT output
		vttBytes, err := os.ReadFile(tmpVTTFile)
		if err != nil {
			failJob(fmt.Errorf("reading Whisper output file: %w", err))
			return
		}

		refBlocks, err = subtitle.ParseSubtitleBlocks(string(vttBytes))
		if err != nil {
			failJob(fmt.Errorf("parsing Whisper VTT output: %w", err))
			return
		}
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

// Map of bitmap subtitle codecs to exclude from extraction (copied from ffmpeg package since private)
var bitmapSubtitleCodecs = map[string]struct{}{
	"dvd_subtitle":      {},
	"dvdsub":            {},
	"pgssub":            {},
	"hdmv_pgs_subtitle": {},
	"xsub":              {},
	"dvb_subtitle":      {},
	"dvb_teletext":      {},
}

func map3To2Lang(lang string) string {
	lang = strings.ToLower(lang)
	switch lang {
	case "eng":
		return "en"
	case "spa":
		return "es"
	case "fra", "fre":
		return "fr"
	case "deu", "ger":
		return "de"
	case "zho", "chi":
		return "zh"
	case "jpn":
		return "ja"
	case "kor":
		return "ko"
	case "rus":
		return "ru"
	case "por":
		return "pt"
	case "ita":
		return "it"
	default:
		if len(lang) >= 2 {
			return lang[:2]
		}
		return "auto"
	}
}
