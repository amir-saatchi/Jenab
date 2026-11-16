package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/amir-saatchi/jenab/internal/appinfo"
)

// ErrRunning is returned by LockRoot when another copy of the app holds
// the lock.
//
//lint:ignore ST1005 the message starts with the app's name
var ErrRunning = errors.New(appinfo.Name + " is already running")

// LockRoot takes the app folder's lock, so only one copy of the app runs:
// two would both move projects and write the registry. The OS drops the
// lock when the process ends, also after a crash. Close releases it.
func LockRoot(root string) (*os.File, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("lock: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(root, "app.lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("lock: %w", err)
	}
	if err := lockFile(f); err != nil {
		f.Close()
		if errors.Is(err, ErrRunning) {
			return nil, err
		}
		return nil, fmt.Errorf("lock: %w", err)
	}
	return f, nil
}

// LockRootWait is LockRoot that waits up to wait for a copy that is
// quitting: the copy an update restarts starts before the old one has
// closed its databases and let go of the lock (TASK-059).
func LockRootWait(root string, wait time.Duration) (*os.File, error) {
	deadline := time.Now().Add(wait)
	for {
		f, err := LockRoot(root)
		if !errors.Is(err, ErrRunning) || time.Now().After(deadline) {
			return f, err
		}
		time.Sleep(100 * time.Millisecond)
	}
}
