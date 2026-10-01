// Package frontend embeds the built UI, because go:embed can't reach a
// parent folder from cmd/desktop (Q1). Run `bun run build` here first.
package frontend

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Assets returns the built UI with dist/ as its root.
func Assets() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}
