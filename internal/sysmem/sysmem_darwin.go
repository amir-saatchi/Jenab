package sysmem

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// macOS has no free-memory number that means "can still be handed out", so
// the reading is the kernel's memory pressure level.
func read() (Reading, error) {
	total, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return Reading{}, fmt.Errorf("sysmem: hw.memsize: %w", err)
	}
	p, err := unix.SysctlUint32("kern.memorystatus_vm_pressure_level")
	if err != nil {
		return Reading{}, fmt.Errorf("sysmem: pressure level: %w", err)
	}
	return Reading{Total: total, Level: pressureLevel(p)}, nil
}

// pressureLevel maps the kernel's 1 (normal), 2 (warn) and 4 (critical).
func pressureLevel(p uint32) Level {
	switch {
	case p >= 4:
		return Critical
	case p >= 2:
		return Low
	}
	return OK
}
