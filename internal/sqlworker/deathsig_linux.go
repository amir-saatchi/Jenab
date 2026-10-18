package sqlworker

import (
	"os/exec"
	"syscall"
)

// deathSignal has Linux kill the worker when the app ends.
func deathSignal(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
}
