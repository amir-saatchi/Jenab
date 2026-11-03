// Package version is the app's version (TASK-037). The number comes from
// the VERSION file at the repo root; the Taskfile sets it, with the git
// commit, through -ldflags -X. A build without the flags, such as
// `go run ./cmd/desktop`, is "dev".
package version

import (
	"runtime"
	"runtime/debug"
)

// Set by the Taskfile's -ldflags -X; see Taskfile.yml.
var (
	number   = ""
	commit   = ""
	modified = "" // "true" when the tree had uncommitted changes
)

// Info is what the app tells about its build, for Settings → General and
// bug reports.
type Info struct {
	// Version is the semantic version, such as 0.1.0-dev, or "dev" for a
	// build without the Taskfile.
	Version string `json:"version"`
	// Commit is the git short commit, empty when unknown.
	Commit string `json:"commit"`
	// Modified is set when the tree had changes the commit doesn't have.
	Modified bool `json:"modified"`
	// Go is the Go version the app was built with.
	Go string `json:"go"`
	// Wails is the Wails module's version, empty when the build has no
	// module data.
	Wails string `json:"wails"`
	// OS is GOOS/GOARCH, such as windows/amd64.
	OS string `json:"os"`
}

// Get returns this build's Info.
func Get() Info {
	i := Info{Version: number, Commit: commit, Modified: modified == "true",
		Go: runtime.Version(), OS: runtime.GOOS + "/" + runtime.GOARCH}
	if i.Version == "" {
		i.Version = "dev"
	}
	if b, ok := debug.ReadBuildInfo(); ok {
		for _, d := range b.Deps {
			if d.Path == "github.com/wailsapp/wails/v3" {
				i.Wails = d.Version
				if d.Replace != nil {
					i.Wails = d.Replace.Version
				}
			}
		}
	}
	return i
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
