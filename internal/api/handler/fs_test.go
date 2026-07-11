package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/prismatic-media/prism-server/internal/api/handler"
)

func TestFsHandler_BrowseDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "prism-fs-test")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sub1 := filepath.Join(tmpDir, "subdir1")
	sub2 := filepath.Join(tmpDir, "subdir2")
	nested1 := filepath.Join(sub1, "nested1")

	if err := os.MkdirAll(nested1, 0755); err != nil {
		t.Fatalf("MkdirAll nested1: %v", err)
	}
	if err := os.MkdirAll(sub2, 0755); err != nil {
		t.Fatalf("MkdirAll sub2: %v", err)
	}

	h := handler.NewFsHandler()

	tests := []struct {
		name         string
		pathParam    string
		expectedDirs []string
	}{
		{
			name:      "Browse temp dir root (no trailing slash)",
			pathParam: tmpDir,
			expectedDirs: []string{
				tmpDir,
			},
		},
		{
			name:      "Browse temp dir root (with trailing slash)",
			pathParam: tmpDir + "/",
			expectedDirs: []string{
				sub1,
				sub2,
			},
		},
		{
			name:      "Browse temp dir root (with trailing backslash)",
			pathParam: tmpDir + "\\",
			expectedDirs: []string{
				sub1,
				sub2,
			},
		},
		{
			name:      "Browse subdir1 (with trailing backslash)",
			pathParam: sub1 + "\\",
			expectedDirs: []string{
				nested1,
			},
		},
		{
			name:      "Browse sub1 (with trailing slash)",
			pathParam: sub1 + "/",
			expectedDirs: []string{
				nested1,
			},
		},
		{
			name:      "Browse relative path dot-slash",
			pathParam: "./",
			// This resolves to the root directory `/`
			expectedDirs: nil, // we won't assert exact dirs since root contents vary by system
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/fs:browse?path="+tt.pathParam, nil)
			rec := httptest.NewRecorder()

			h.BrowseDir(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", rec.Code)
			}

			var resp map[string][]string
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}

			dirs := resp["dirs"]
			if tt.expectedDirs == nil {
				return
			}

			if len(dirs) != len(tt.expectedDirs) {
				t.Errorf("expected %d dirs, got %d. Dirs: %v, Expected: %v", len(tt.expectedDirs), len(dirs), dirs, tt.expectedDirs)
				return
			}

			for i, d := range dirs {
				if d != tt.expectedDirs[i] {
					t.Errorf("at index %d: expected %q, got %q", i, tt.expectedDirs[i], d)
				}
			}
		})
	}
}
