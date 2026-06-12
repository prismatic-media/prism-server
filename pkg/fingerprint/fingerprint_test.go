package fingerprint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateDeterministic(t *testing.T) {
	// Create a temporary file with known content.
	tmpDir := t.TempDir()
	content := []byte("hello world, this is a test file for fingerprinting")
	if err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), content, 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	got, err := GenerateDeterministic(filepath.Join(tmpDir, "test.txt"))
	if err != nil {
		t.Fatalf("GenerateDeterministic: %v", err)
	}

	// Verify determinism: calling again should produce the same hash.
	got2, err := GenerateDeterministic(filepath.Join(tmpDir, "test.txt"))
	if err != nil {
		t.Fatalf("GenerateDeterministic second call: %v", err)
	}
	if got != got2 {
		t.Errorf("non-deterministic: got %s, want %s", got, got2)
	}

	// Verify different content produces different hash.
	different := []byte("different content")
	if err := os.WriteFile(filepath.Join(tmpDir, "test2.txt"), different, 0644); err != nil {
		t.Fatalf("write temp file 2: %v", err)
	}
	got3, err := GenerateDeterministic(filepath.Join(tmpDir, "test2.txt"))
	if err != nil {
		t.Fatalf("GenerateDeterministic different file: %v", err)
	}
	if got == got3 {
		t.Errorf("same hash for different files: %s", got)
	}
}

func TestGenerateDeterministic_NilFile(t *testing.T) {
	_, err := GenerateDeterministic("/nonexistent/file.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestSourcePath(t *testing.T) {
	tests := []struct {
		name       string
		storageDir string
		fullPath   string
		wantRel    string
	}{
		{
			name:       "simple relative",
			storageDir: "/data/videos",
			fullPath:   "/data/videos/movies/hero.mp4",
			wantRel:    "movies/hero.mp4",
		},
		{
			name:       "nested relative",
			storageDir: "/data/videos",
			fullPath:   "/data/videos/movies/drama/hero.mp4",
			wantRel:    "movies/drama/hero.mp4",
		},
		{
			name:       "same directory",
			storageDir: "/data/videos",
			fullPath:   "/data/videos/hero.mp4",
			wantRel:    "hero.mp4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SourcePath(tt.storageDir, tt.fullPath)
			if got != tt.wantRel {
				t.Errorf("SourcePath(%q, %q) = %q, want %q", tt.storageDir, tt.fullPath, got, tt.wantRel)
			}
		})
	}
}

func TestResolvePath(t *testing.T) {
	tests := []struct {
		name        string
		storageDir  string
		sourcePath  string
		wantAbs     string
	}{
		{
			name:       "simple resolve",
			storageDir: "/data/videos",
			sourcePath: "movies/hero.mp4",
			wantAbs:    "/data/videos/movies/hero.mp4",
		},
		{
			name:       "root file",
			storageDir: "/data/videos",
			sourcePath: "hero.mp4",
			wantAbs:    "/data/videos/hero.mp4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolvePath(tt.storageDir, tt.sourcePath)
			if got != tt.wantAbs {
				t.Errorf("ResolvePath(%q, %q) = %q, want %q", tt.storageDir, tt.sourcePath, got, tt.wantAbs)
			}
		})
	}
}
