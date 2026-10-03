package project

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCheckFolderOnWindows checks the real paths: the OneDrive folder and a
// network share.
func TestCheckFolderOnWindows(t *testing.T) {
	if od := os.Getenv("OneDrive"); od == "" {
		t.Log("OneDrive is not set up here; skipped")
	} else if w := CheckFolder(filepath.Join(od, "Jenab")); w == nil || w.Service != "OneDrive" || w.Network {
		t.Errorf("OneDrive folder: %+v", w)
	}
	share := `\\localhost\C$\Windows`
	if w := CheckFolder(share); w == nil || !w.Network {
		t.Errorf("%s: %+v", share, w)
	}
	// GetDriveType itself, which catches drive letters mapped to a share:
	// remote for the share's root, fixed for the system drive.
	if root, ok := volumeRoot(share); !ok || driveType(root) != driveRemote {
		t.Errorf("volume root of %s = %q (%v), type %d", share, root, ok, driveType(root))
	}
	sys := os.Getenv("SystemRoot")
	if root, ok := volumeRoot(sys); !ok || driveType(root) != 3 || isNetwork(sys) {
		t.Errorf("%s: root %q, type %d", sys, root, driveType(root))
	}
}
