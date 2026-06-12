package artifact

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SidecarMetadata describes a transcode artifact bundle for persistence.
// It is written as a JSON file alongside the MPD manifest so that artifact
// identity survives database loss.
type SidecarMetadata struct {
	// Version is the schema version of the sidecar format.
	Version int `json:"v"`
	// MediaItemID is the UUID of the media item this bundle belongs to.
	MediaItemID string `json:"media_item_id"`
	// SourcePath is the normalized path of the source video relative to
	// the storage area root.
	SourcePath string `json:"source_path"`
	// SourceFingerprint is the SHA-256 hash of the first 64 KB of the
	// source file.
	SourceFingerprint string `json:"source_fingerprint"`
	// OutputDir is the absolute path where segments and manifest were written.
	OutputDir string `json:"output_dir"`
	// MPDPath is the path to the generated MPD manifest relative to output_dir.
	MPDPath string `json:"mpd_path"`
	// Profiles lists the rendition profiles used for this transcode.
	Profiles []RenditionInfo `json:"profiles"`
	// Duration is the source duration in seconds.
	Duration float64 `json:"duration"`
	// WrittenAt is when the sidecar was written.
	WrittenAt time.Time `json:"written_at"`
}

// RenditionInfo describes a single transcode rendition.
type RenditionInfo struct {
	Name          string `json:"name"`
	Height        int    `json:"height"`
	Width         int    `json:"width"`
	VideoBitrateK int    `json:"video_bitrate_k"`
	AudioBitrateK int    `json:"audio_bitrate_k"`
}

// SidecarFilename is the name of the sidecar metadata file.
const SidecarFilename = "artifact.json"

// WriteSidecar writes the sidecar metadata to the output directory.
func WriteSidecar(outputDir string, meta *SidecarMetadata) error {
	meta.WrittenAt = time.Now().UTC()
	meta.Version = 2

	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshaling sidecar metadata: %w", err)
	}

	// Ensure output directory exists.
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output dir for sidecar: %w", err)
	}

	sidecarPath := filepath.Join(outputDir, SidecarFilename)
	if err := os.WriteFile(sidecarPath, data, 0644); err != nil {
		return fmt.Errorf("writing sidecar metadata: %w", err)
	}
	return nil
}

// ReadSidecar reads and parses the sidecar metadata from the output directory.
func ReadSidecar(outputDir string) (*SidecarMetadata, error) {
	sidecarPath := filepath.Join(outputDir, SidecarFilename)

	data, err := os.ReadFile(sidecarPath)
	if err != nil {
		return nil, fmt.Errorf("reading sidecar metadata: %w", err)
	}

	var meta SidecarMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing sidecar metadata: %w", err)
	}
	return &meta, nil
}

// ValidateBundle checks that the expected bundle files exist in outputDir.
type BundleValidation struct {
	MPDExists     bool
	SegmentsExist bool
	SidecarExists bool
	Subtitles     []string
}

// ValidateBundle checks that the expected DASH bundle files exist.
func ValidateBundle(outputDir string) (*BundleValidation, error) {
	v := &BundleValidation{}

	mpdPath := filepath.Join(outputDir, "manifest.mpd")
	if _, err := os.Stat(mpdPath); err == nil {
		v.MPDExists = true
	}

	// Check for segment files (media_*.mp4).
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return nil, fmt.Errorf("reading output dir for validation: %w", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if name == SidecarFilename {
			v.SidecarExists = true
			continue
		}
		if name == "manifest.mpd" {
			continue
		}
		if filepath.Ext(name) == ".mp4" && len(name) > 5 && name[:5] == "media" {
			v.SegmentsExist = true
		}
		if filepath.Ext(name) == ".vtt" {
			v.Subtitles = append(v.Subtitles, filepath.Join(outputDir, name))
		}
	}

	return v, nil
}

// IsBundleHealthy checks that all critical bundle files exist.
func (v *BundleValidation) IsBundleHealthy() bool {
	return v.MPDExists && v.SegmentsExist && v.SidecarExists
}
