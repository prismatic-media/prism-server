package artifact

import (
	"os"
	"testing"
)

func TestWriteAndReadSidecar(t *testing.T) {
	tmpDir := t.TempDir()

	meta := &SidecarMetadata{
		MediaItemID:       "test-media-id",
		SourcePath:        "movies/hero.mp4",
		SourceFingerprint: "abc123",
		OutputDir:         tmpDir,
		MPDPath:           "manifest.mpd",
		Duration:          7200.5,
	}

	if err := WriteSidecar(tmpDir, meta); err != nil {
		t.Fatalf("WriteSidecar: %v", err)
	}

	// Verify sidecar file exists.
	sidecarPath := tmpDir + "/artifact.json"
	if _, err := os.Stat(sidecarPath); err != nil {
		t.Errorf("sidecar file not created: %v", err)
	}

	// Verify read back.
	read, err := ReadSidecar(tmpDir)
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
	if read.Duration != meta.Duration {
		t.Errorf("Duration: got %f, want %f", read.Duration, meta.Duration)
	}
}

func TestValidateBundle(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a minimal segment file.
	_ = os.WriteFile(tmpDir+"/media_0.mp4", []byte("fake"), 0644)
	_ = os.WriteFile(tmpDir+"/manifest.mpd", []byte("<MPD/>"), 0644)

	v, err := ValidateBundle(tmpDir)
	if err != nil {
		t.Fatalf("ValidateBundle: %v", err)
	}

	if !v.MPDExists {
		t.Error("MPD should exist")
	}
	if !v.SegmentsExist {
		t.Error("segments should exist")
	}
	if v.IsBundleHealthy() {
		t.Error("bundle should not be healthy without sidecar")
	}

	// Add sidecar.
	_ = WriteSidecar(tmpDir, &SidecarMetadata{MediaItemID: "test"})
	v, err = ValidateBundle(tmpDir)
	if err != nil {
		t.Fatalf("ValidateBundle after sidecar: %v", err)
	}
	if !v.IsBundleHealthy() {
		t.Error("bundle should be healthy with sidecar")
	}
}

func TestSidecarV1Compatibility(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Write a raw V1 JSON sidecar.
	v1JSON := `{
		"v": 1,
		"media_item_id": "test-v1-uuid",
		"source_path": "movies/v1.mp4",
		"source_fingerprint": "v1hash",
		"output_dir": "/dummy",
		"mpd_path": "manifest.mpd",
		"duration": 120.0
	}`
	sidecarPath := tmpDir + "/artifact.json"
	if err := os.WriteFile(sidecarPath, []byte(v1JSON), 0644); err != nil {
		t.Fatalf("failed to write v1 sidecar: %v", err)
	}

	// 2. Read it back.
	meta, err := ReadSidecar(tmpDir)
	if err != nil {
		t.Fatalf("failed to read v1 sidecar: %v", err)
	}
	if meta.Version != 1 {
		t.Errorf("expected Version 1, got %d", meta.Version)
	}
	if meta.MediaItemID != "test-v1-uuid" {
		t.Errorf("expected MediaItemID test-v1-uuid, got %s", meta.MediaItemID)
	}
}
