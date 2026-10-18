//go:build unix && !linux

package sqlworker

import "os/exec"

// deathSignal: other systems have none; the worker exits when its stdin
// closes.
func deathSignal(*exec.Cmd) {}
