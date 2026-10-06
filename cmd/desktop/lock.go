package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// errRunning is returned by lockRoot when another copy of the app holds
// the lock.
var errRunning = errors.New(appName + " is already running")

// lockRoot takes the app folder's lock, so only one copy of the app runs:
// two would both move projects and write the registry. The OS drops the
// lock when the process ends, also after a crash. Close releases it.
func lockRoot(root string) (*os.File, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("lock: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(root, "app.lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("lock: %w", err)
	}
	if err := lockFile(f); err != nil {
		f.Close()
		if errors.Is(err, errRunning) {
			return nil, err
		}
		return nil, fmt.Errorf("lock: %w", err)
	}
	return f, nil
}
