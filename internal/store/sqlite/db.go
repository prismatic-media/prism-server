package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/ringmaster217/galactic-media-server/migrations"
)

// Open opens (or creates) the SQLite database at path and configures it for
// safe concurrent use within a single process.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// SQLite allows only one writer at a time; a pool size of 1 avoids
	// "database is locked" errors while still permitting concurrent reads
	// under WAL mode.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`
		PRAGMA foreign_keys  = ON;
		PRAGMA journal_mode  = WAL;
		PRAGMA busy_timeout  = 5000;
		PRAGMA synchronous   = NORMAL;
	`); err != nil {
		return nil, fmt.Errorf("setting pragmas: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return db, nil
}

// Migrate runs any pending goose migrations against db. Migration SQL files
// are compiled into the binary via the migrations package embed.
func Migrate(db *sql.DB) error {
	goose.SetBaseFS(migrations.FS)

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("setting dialect: %w", err)
	}

	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	return nil
}
