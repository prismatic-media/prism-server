package sqlite_test

import (
	"database/sql"
	"testing"

	"github.com/ringmaster217/galactic-media-server/internal/store/sqlite"
)

// openTestDB opens an in-memory SQLite database, runs all migrations, and
// registers a cleanup function to close it.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("openTestDB: Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := sqlite.Migrate(db); err != nil {
		t.Fatalf("openTestDB: Migrate: %v", err)
	}

	return db
}
