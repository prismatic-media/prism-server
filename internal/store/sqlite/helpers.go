package sqlite

import (
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullUUID(id *uuid.UUID) sql.NullString {
	if id == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: id.String(), Valid: true}
}
