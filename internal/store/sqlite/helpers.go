package sqlite

import (
	"errors"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
