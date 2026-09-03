// Package static embeds every asset under web/static/ into the binary so a
// built pluris-console serves its CSS/JS from any working directory.
//
// Before this, /static was served straight off disk with a process-cwd-relative
// path (console/server/server.go's e.Static("/static", "web/static")) — a binary
// launched anywhere but the repo root (or `make dev`) 404'd on every stylesheet
// and script. That silently strips styling and disables all list/detail JS
// (search, filters, inline edit, condition builder, code editor, ...) with no
// error surfaced anywhere but the browser console.
//
// Mirrors the fix already applied to migrations in db/schema/embed.go.
package static

import "embed"

//go:embed *.js *.css vendor
var Files embed.FS
