// Package embedded exposes the repo's runtime assets compiled into the
// binary, so a deployed binary works standalone — without templates/,
// migrations/ or audios/ shipped alongside it. Callers still prefer the
// on-disk copies when present (cloned-repo workflow keeps live edits).
package embedded

import "embed"

//go:embed all:templates
var Templates embed.FS

//go:embed migrations/*.sql
var Migrations embed.FS

//go:embed audios/*.wav
var Audios embed.FS
