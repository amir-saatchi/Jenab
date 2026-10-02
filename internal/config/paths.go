// Package config has the app's paths and settings (SPEC 2.1). View, page
// and pipeline configs (SPEC 10) are added in Phase 3.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Paths are the app's folders and files.
type Paths struct {
	Root       string // <user data dir>/<app>: settings, registry and logs, in a fixed place
	Settings   string // Root/config.yaml
	Registry   string // Root/registry.db
	Logs       string // Root/logs
	DataFolder string // ~/<app> unless the settings say otherwise (SPEC 3.9)
	Projects   string // DataFolder/projects
}

// DefaultPaths returns the paths for an app folder name such as "Jenab".
func DefaultPaths(app string) (Paths, error) {
	data, err := userDataDir()
	if err != nil {
		return Paths{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("config: %w", err)
	}
	root := filepath.Join(data, app)
	p := Paths{
		Root:     root,
		Settings: filepath.Join(root, "config.yaml"),
		Registry: filepath.Join(root, "registry.db"),
		Logs:     filepath.Join(root, "logs"),
	}
	return p.WithDataFolder(filepath.Join(home, app)), nil
}

// WithDataFolder moves the data folder to dir; an empty dir keeps it.
func (p Paths) WithDataFolder(dir string) Paths {
	if dir != "" {
		p.DataFolder = dir
		p.Projects = filepath.Join(dir, "projects")
	}
	return p
}

// userDataDir is the per-user folder for app data that is not synced:
// %LOCALAPPDATA% on Windows, ~/Library/Application Support on macOS and
// $XDG_DATA_HOME or ~/.local/share elsewhere. os.UserConfigDir would give
// the roaming %APPDATA% on Windows, which is no place for SQLite files.
func userDataDir() (string, error) {
	switch runtime.GOOS {
	case "windows":
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			return d, nil
		}
		return "", errors.New("config: %LOCALAPPDATA% is not set")
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("config: %w", err)
		}
		return filepath.Join(home, "Library", "Application Support"), nil
	default:
		if d := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(d) {
			return d, nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("config: %w", err)
		}
		return filepath.Join(home, ".local", "share"), nil
	}
}
