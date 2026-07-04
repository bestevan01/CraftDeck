package migrations

import "embed"

// Files holds the embedded migration SQL files, applied in filename order
// by db.Open.
//
//go:embed *.sql
var Files embed.FS
