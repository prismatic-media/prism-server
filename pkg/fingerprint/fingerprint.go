package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// GenerateDeterministic creates a SHA-256 fingerprint of the first 64 KB of a
// file. The fingerprint is deterministic: the same file always produces the
// same hash regardless of modification time, permissions, or other metadata.
//
// The first 64 KB are chosen because they typically contain enough file-format
// header data (e.g. for video containers, MP4/MP4 atoms, Matroska EBML headers)
// to uniquely identify the source.
func GenerateDeterministic(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("opening file for fingerprint: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.CopyN(h, f, 64*1024); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			// File is smaller than 64 KB — hash what we have.
		} else {
			return "", fmt.Errorf("reading file for fingerprint: %w", err)
		}
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// GenerateDeterministicN reads from the first n bytes of the file.
func GenerateDeterministicN(filePath string, n int64) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("opening file for fingerprint: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.CopyN(h, f, n); err != nil {
		return "", fmt.Errorf("reading file for fingerprint: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// SourcePath returns a normalized source path suitable for artifact indexing.
// The path is relative to the storage area root so that it can be resolved
// across different mount points or re-indexing operations.
func SourcePath(storageAreaPath, fullPath string) string {
	rel, err := filepath.Rel(storageAreaPath, fullPath)
	if err != nil {
		// Fall back to the absolute path if relative path fails.
		return fullPath
	}
	return filepath.ToSlash(rel)
}

// ResolvePath joins a storage area path with a normalized source path.
func ResolvePath(storageAreaPath, sourcePath string) string {
	return filepath.Join(storageAreaPath, sourcePath)
}
