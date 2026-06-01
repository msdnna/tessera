// Package migrations embeds the SQL migration files so the migrate CLI and the
// production image can apply them without the source tree present.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
