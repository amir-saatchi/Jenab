package main

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"

	"github.com/amir-saatchi/jenab/internal/appinfo"
)

// uninstallKey is the installer's entry for the app, under HKCU.
const uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\` + appinfo.Identifier

// refreshAppsEntry brings the Apps entry's version up to date with this
// exe's, as after an update. With no entry, as for a copy that was never
// installed, it does nothing; it never makes one.
func refreshAppsEntry(version string) (bool, error) {
	exe, err := os.Executable()
	if err != nil {
		return false, err
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, uninstallKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer k.Close()
	return refreshEntry(k, filepath.Dir(exe), version)
}
