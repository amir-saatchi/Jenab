//go:build windows

package desktop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestCLILive runs the real program, when JENAB_AGENT_DESKTOP names it:
// the hash check, a new private state folder, and one command that reads
// nothing of the user's windows.
func TestCLILive(t *testing.T) {
	path := os.Getenv("JENAB_AGENT_DESKTOP")
	if path == "" {
		t.Skip("set JENAB_AGENT_DESKTOP to agent-desktop.exe to run it")
	}
	c := &CLI{Path: path, Home: filepath.Join(t.TempDir(), "agent-desktop")}
	data, err := c.Run(context.Background(), callTimeout, "version")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &v); err != nil || v.Version != Version {
		t.Fatalf("version: %s (%v)", data, err)
	}
	if err := prepareHome(c.Home); err != nil {
		t.Fatalf("the folder it made isn't private: %v", err)
	}
}
