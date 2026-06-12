package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const (
	PathStatusOK               = "ok"
	PathStatusDisabled         = "disabled"
	PathStatusMissing          = "missing"
	PathStatusStatError        = "stat_error"
	PathStatusPermissionDenied = "permission_denied"
	PathStatusUnwritable       = "unwritable"
	PathStatusBelowReserve     = "below_reserve"
)

type PathMetrics struct {
	TotalBytes      uint64
	UsedBytes       uint64
	FreeBytes       uint64
	UtilizationPct  float64
	Status          string
	Error           string
	EligibleSegment bool
}

func CollectPathMetrics(path string, minFreeBytes uint64, considerReserve bool) PathMetrics {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PathMetrics{Status: PathStatusMissing, Error: err.Error()}
		}
		if errors.Is(err, os.ErrPermission) {
			return PathMetrics{Status: PathStatusPermissionDenied, Error: err.Error()}
		}
		return PathMetrics{Status: PathStatusStatError, Error: err.Error()}
	}
	if !info.IsDir() {
		return PathMetrics{Status: PathStatusStatError, Error: "path is not a directory"}
	}

	stats, err := statfs(path)
	if err != nil {
		return PathMetrics{Status: PathStatusStatError, Error: err.Error()}
	}
	if err := writable(path); err != nil {
		if errors.Is(err, os.ErrPermission) {
			stats.Status = PathStatusPermissionDenied
		} else {
			stats.Status = PathStatusUnwritable
		}
		stats.Error = err.Error()
		return stats
	}

	stats.Status = PathStatusOK
	stats.EligibleSegment = true
	if considerReserve && stats.FreeBytes <= minFreeBytes {
		stats.Status = PathStatusBelowReserve
		stats.EligibleSegment = false
	}

	return stats
}

func statfs(path string) (PathMetrics, error) {
	var fs syscall.Statfs_t
	if err := syscall.Statfs(path, &fs); err != nil {
		return PathMetrics{}, fmt.Errorf("statfs %s: %w", path, err)
	}
	blockSize := uint64(fs.Bsize)
	total := fs.Blocks * blockSize
	free := fs.Bavail * blockSize
	used := total - free
	util := 0.0
	if total > 0 {
		util = float64(used) / float64(total) * 100
	}
	return PathMetrics{TotalBytes: total, UsedBytes: used, FreeBytes: free, UtilizationPct: util}, nil
}

func writable(path string) error {
	f, err := os.CreateTemp(path, ".prism-writecheck-*")
	if err != nil {
		return err
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(filepath.Clean(name))
	return nil
}
