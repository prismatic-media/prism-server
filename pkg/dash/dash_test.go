package dash

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateMPD_Width(t *testing.T) {
	tempDir := t.TempDir()
	mpdPath := filepath.Join(tempDir, "manifest.mpd")

	renditions := []RenditionInfo{
		{
			Name:          "720p",
			Width:         1280,
			Height:        720,
			VideoBitrateK: 4000,
			AudioBitrateK: 128,
			Codec:         "h264",
		},
		{
			Name:          "480p",
			Width:         0, // Test fallback logic
			Height:        480,
			VideoBitrateK: 2000,
			AudioBitrateK: 96,
			Codec:         "h264",
		},
	}

	err := GenerateMPD(tempDir, mpdPath, renditions, nil, 120.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(mpdPath)
	if err != nil {
		t.Fatalf("failed to read generated MPD: %v", err)
	}

	xmlStr := string(content)

	// Check that 720p has width="1280"
	if !strings.Contains(xmlStr, `width="1280"`) {
		t.Errorf("expected MPD to contain width=\"1280\", got:\n%s", xmlStr)
	}

	// Check that 480p fallback has width="853" (480*16/9)
	if !strings.Contains(xmlStr, `width="853"`) {
		t.Errorf("expected MPD to contain width=\"853\" for fallback, got:\n%s", xmlStr)
	}

	// Ensure there are no instances of width="auto"
	if strings.Contains(xmlStr, `width="auto"`) {
		t.Errorf("expected MPD to not contain width=\"auto\", got:\n%s", xmlStr)
	}
}
