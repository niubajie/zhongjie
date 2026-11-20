package migrations

import "embed"

// FS contains the versioned SQL migrations. Only additive migrations are
// allowed: never edit a file that has already been applied.
//
//go:embed *.sql
var FS embed.FS
