package scanner

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prismatic-media/prism-server/internal/artifact"
)

func TestWriteAndReadSidecar(t *testing.T) {
	dir := t.TempDir()

	// Create a test sidecar.
	profiles := []artifact.RenditionInfo{
		{Name: "720p", Height: 720, Width: 1280, VideoBitrateK: 4000, AudioBitrateK: 128},
		{Name: "480p", Height: 480, Width: 854, VideoBitrateK: 2000, AudioBitrateK: 96},
	}
	meta := &artifact.SidecarMetadata{
		Version:             1,
		MediaItemID:         "test-media-id",
		SourcePath:          "videos/test.mp4",
		SourceFingerprint:   "abc123def456",
		OutputDir:           dir,
		MPDPath:             "manifest.mpd",
		Profiles:            profiles,
		Duration:            3600.5,
		WrittenAt:           time.Now().UTC(),
	}

	if err := artifact.WriteSidecar(dir, meta); err != nil {
		t.Fatalf("WriteSidecar: %v", err)
	}

	// Verify the sidecar file exists.
	sidecarPath := filepath.Join(dir, artifact.SidecarFilename)
	if _, err := os.Stat(sidecarPath); os.IsNotExist(err) {
		t.Fatalf("sidecar file not found at %s", sidecarPath)
	}

	// Read the sidecar back.
	read, err := artifact.ReadSidecar(dir)
	if err != nil {
		t.Fatalf("ReadSidecar: %v", err)
	}

	if read.MediaItemID != meta.MediaItemID {
		t.Errorf("MediaItemID: got %q, want %q", read.MediaItemID, meta.MediaItemID)
	}
	if read.SourcePath != meta.SourcePath {
		t.Errorf("SourcePath: got %q, want %q", read.SourcePath, meta.SourcePath)
	}
	if read.SourceFingerprint != meta.SourceFingerprint {
		t.Errorf("SourceFingerprint: got %q, want %q", read.SourceFingerprint, meta.SourceFingerprint)
	}
	if read.OutputDir != meta.OutputDir {
		t.Errorf("OutputDir: got %q, want %q", read.OutputDir, meta.OutputDir)
	}
	if read.MPDPath != meta.MPDPath {
		t.Errorf("MPDPath: got %q, want %q", read.MPDPath, meta.MPDPath)
	}
	if len(read.Profiles) != len(profiles) {
		t.Errorf("Profiles: got %d, want %d", len(read.Profiles), len(profiles))
	}
	if read.Duration != meta.Duration {
		t.Errorf("Duration: got %f, want %f", read.Duration, meta.Duration)
	}
}

func TestValidateBundle(t *testing.T) {
	dir := t.TempDir()

	// Create MPD file.
	mpdPath := filepath.Join(dir, "manifest.mpd")
	if err := os.WriteFile(mpdPath, []byte("<MPD></MPD>"), 0644); err != nil {
		t.Fatalf("WriteFile MPD: %v", err)
	}

	// Create segment file (must be media_*.mp4 for ValidateBundle).
	segPath := filepath.Join(dir, "media_0001.mp4")
	if err := os.WriteFile(segPath, []byte("segment"), 0644); err != nil {
		t.Fatalf("WriteFile segment: %v", err)
	}

	// Validate.
	v, err := artifact.ValidateBundle(dir)
	if err != nil {
		t.Fatalf("ValidateBundle: %v", err)
	}

	if !v.MPDExists {
		t.Error("MPDExists should be true")
	}
	if !v.SegmentsExist {
		t.Error("SegmentsExist should be true")
	}
	if v.IsBundleHealthy() {
		t.Error("IsBundleHealthy should be false (no sidecar)")
	}

	// Add sidecar.
	meta := &artifact.SidecarMetadata{
		Version:             1,
		MediaItemID:         "test",
		SourcePath:          "test.mp4",
		SourceFingerprint:   "abc123",
		OutputDir:           dir,
		MPDPath:             "manifest.mpd",
		WrittenAt:           time.Now().UTC(),
	}
	if err := artifact.WriteSidecar(dir, meta); err != nil {
		t.Fatalf("WriteSidecar: %v", err)
	}

	v, err = artifact.ValidateBundle(dir)
	if err != nil {
		t.Fatalf("ValidateBundle: %v", err)
	}
	if !v.IsBundleHealthy() {
		t.Error("IsBundleHealthy should be true (all files present)")
	}
}
