// internal/storage/database/migrations/migrations.go
package migrations

import (
	"embed"
)

//go:embed *.sql
var Migrations embed.FS
