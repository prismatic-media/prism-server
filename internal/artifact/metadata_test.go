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

func TestGetTranscodeSizesInfo(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Create a dummy transcode output structure.
	// We need resolution subdirectories, e.g. 360p and 480p.
	// Inside each, we write files with specific sizes to verify summation.
	r360 := tmpDir + "/360p"
	r480 := tmpDir + "/480p"
	notRendition := tmpDir + "/temp" // directory that doesn't match rendition format

	if err := os.MkdirAll(r360, 0755); err != nil {
		t.Fatalf("failed to create 360p dir: %v", err)
	}
	if err := os.MkdirAll(r480, 0755); err != nil {
		t.Fatalf("failed to create 480p dir: %v", err)
	}
	if err := os.MkdirAll(notRendition, 0755); err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Write files to 360p: total 30 bytes
	_ = os.WriteFile(r360+"/init.mp4", make([]byte, 10), 0644)
	_ = os.WriteFile(r360+"/seg_00001.m4s", make([]byte, 20), 0644)

	// Write files to 480p: total 100 bytes
	_ = os.WriteFile(r480+"/init.mp4", make([]byte, 40), 0644)
	_ = os.WriteFile(r480+"/seg_00001.m4s", make([]byte, 60), 0644)

	// Write files to temp: should be ignored
	_ = os.WriteFile(notRendition+"/ignored.mp4", make([]byte, 1000), 0644)

	// Write a file directly to outputDir: 5 bytes
	_ = os.WriteFile(tmpDir+"/manifest.mpd", make([]byte, 5), 0644)

	mpdPath := tmpDir + "/manifest.mpd"

	info := GetTranscodeSizesInfo(mpdPath)

	if len(info.Renditions) != 2 {
		t.Fatalf("expected 2 rendition sizes, got %d", len(info.Renditions))
	}

	// Order is not guaranteed since ReadDir sorts alphabetically.
	// But "360p" and "480p" are sorted alphabetically, so info.Renditions[0] should be 360p, info.Renditions[1] should be 480p.
	if info.Renditions[0].Resolution != "360p" || info.Renditions[0].Size != 30 {
		t.Errorf("expected 360p to have size 30, got %s size %d", info.Renditions[0].Resolution, info.Renditions[0].Size)
	}
	if info.Renditions[1].Resolution != "480p" || info.Renditions[1].Size != 100 {
		t.Errorf("expected 480p to have size 100, got %s size %d", info.Renditions[1].Resolution, info.Renditions[1].Size)
	}

	// Total size: 30 (360p) + 100 (480p) + 5 (manifest.mpd) = 135 bytes
	if info.TotalSize != 135 {
		t.Errorf("expected total size 135, got %d", info.TotalSize)
	}
}

