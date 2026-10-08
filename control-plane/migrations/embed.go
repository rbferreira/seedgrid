// Package migrations embeds the SQL migration files (goose) so they can be
// applied by the Control Plane binary at boot, without relying on external files.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
