package migrations

import "embed"

// FS holds all goose migration SQL files compiled into the binary.
//
//go:embed *.sql
var FS embed.FS
