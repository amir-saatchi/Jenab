package main

import (
	"path/filepath"
	"strings"
)

// The Apps entry (TASK-062): the installer writes the app's entry in
// Windows' Installed apps under HKCU\Software\Microsoft\Windows\
// CurrentVersion\Uninstall\<identifier>, with the version it installed.
// MyGo's updater (v0.9.1, still so in v0.10.0) swaps the files but leaves
// the entry, so after an update it showed the old version. Jenab brings
// the version up to date at every start.

// appsEntry is the values of the entry the app reads and writes:
// registry.Key's methods.
type appsEntry interface {
	GetStringValue(name string) (string, uint32, error)
	SetStringValue(name, value string) error
}

// refreshEntry sets the entry's DisplayVersion to version when the entry
// is this copy's: its InstallLocation is dir, the folder of the running
// exe. A copy run from elsewhere, such as a build in the repo, leaves the
// installed app's entry alone, as does a build without a version ("dev").
// It reports whether it wrote the version.
func refreshEntry(e appsEntry, dir, version string) (bool, error) {
	if version == "" || version == "dev" {
		return false, nil
	}
	loc, _, err := e.GetStringValue("InstallLocation")
	if err != nil {
		return false, err
	}
	if !samePath(loc, dir) {
		return false, nil
	}
	if have, _, err := e.GetStringValue("DisplayVersion"); err == nil && have == version {
		return false, nil
	}
	if err := e.SetStringValue("DisplayVersion", version); err != nil {
		return false, err
	}
	return true, nil
}

// samePath reports whether two Windows folders are the same, written with
// any case, quotes or a closing separator.
func samePath(a, b string) bool {
	clean := func(p string) string { return filepath.Clean(strings.Trim(strings.TrimSpace(p), `"`)) }
	return a != "" && b != "" && strings.EqualFold(clean(a), clean(b))
}
