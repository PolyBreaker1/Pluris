// Package schema embeds the SQL migration files into the binary so a
// built pluris-console can migrate a fresh database from any working
// directory. Before this, migrations were read from disk relative to
// the repo root and a binary run elsewhere silently booted with no
// schema at all.
package schema

import "embed"

//go:embed *.sql
var Files embed.FS
