// Package version is the app's version (TASK-037, TASK-061). The number is
// mygo.json's version, the one place it is set: the release build
// (go tool mygo build) writes it into the exe and the installer, and this
// package reads it from the copy the root package embeds. The commit, and
// whether the tree had changes, come from Go's build info, which go build
// stamps in a git checkout (vcs.revision, vcs.modified). Tests have no
// build info, so there the commit is empty.
//
// -ldflags -X still overrides each, such as
// -X github.com/amir-saatchi/jenab/internal/version.commit=d449b7e, for a
// build from outside a git checkout.
package version

import (
	"encoding/json"
	"runtime"
	"runtime/debug"

	"github.com/amir-saatchi/jenab"
)

// Overrides, set with -ldflags -X.
var (
	number   = ""
	commit   = ""
	modified = "" // "true" when the tree had uncommitted changes
)

// Info is what the app tells about its build, for Settings → General and
// bug reports.
type Info struct {
	// Version is the semantic version, such as 0.1.0-dev, or "dev" when
	// mygo.json has none.
	Version string `json:"version"`
	// Commit is the git short commit, empty when unknown.
	Commit string `json:"commit"`
	// Modified is set when the tree had changes the commit doesn't have.
	Modified bool `json:"modified"`
	// Go is the Go version the app was built with.
	Go string `json:"go"`
	// OS is GOOS/GOARCH, such as windows/amd64.
	OS string `json:"os"`
}

// Get returns this build's Info.
func Get() Info {
	i := Info{Version: number, Commit: commit, Modified: modified == "true",
		Go: runtime.Version(), OS: runtime.GOOS + "/" + runtime.GOARCH}
	if i.Version == "" {
		i.Version = configVersion(jenab.MyGoJSON)
	}
	if i.Commit == "" {
		if b, ok := debug.ReadBuildInfo(); ok {
			i.Commit, i.Modified = vcs(b.Settings)
		}
	}
	return i
}

// configVersion is mygo.json's version, or "dev" when it has none.
func configVersion(b []byte) string {
	var c struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(b, &c) != nil || c.Version == "" {
		return "dev"
	}
	return c.Version
}

// vcs is the short commit and the modified flag Go stamped into the build.
func vcs(settings []debug.BuildSetting) (commit string, modified bool) {
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			commit = s.Value
			if len(commit) > 7 {
				commit = commit[:7]
			}
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	return commit, modified
}

// String is the version with the commit, as in "0.1.0-dev (d449b7e)".
func (i Info) String() string {
	s := i.Version
	if i.Commit != "" {
		c := i.Commit
		if i.Modified {
			c += ", modified"
		}
		s += " (" + c + ")"
	}
	return s
}
