// Package jenab holds what the Go code reads from the repo root: mygo.json,
// the release build's config, which sets the app's version (TASK-061).
package jenab

import _ "embed"

// MyGoJSON is mygo.json. internal/version reads the version from it, so
// the version is set in one place.
//
//go:embed mygo.json
var MyGoJSON []byte
