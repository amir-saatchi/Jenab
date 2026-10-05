// Package sysmem reads how much memory the OS can still hand out (SPEC
// 8.5, 5.12): the commit headroom on Windows, MemAvailable on Linux and the
// memory pressure level on macOS.
package sysmem

// Level is how tight memory is.
type Level string

const (
	OK       Level = "ok"
	Low      Level = "low"      // under LowBytes free, or the OS warns
	Critical Level = "critical" // under StopBytes free, or the OS is critical
)

const (
	// LowBytes is where the bottom bar turns amber (5.12).
	LowBytes = 1 << 30
	// StopBytes is the hard stop for commands (8.5), where it turns red.
	StopBytes = 500 << 20
)

// Reading is one reading. Free is 0 where the OS gives only a level
// (macOS).
type Reading struct {
	Free  uint64 `json:"free"`  // bytes the OS can still hand out
	Total uint64 `json:"total"` // physical memory
	Level Level  `json:"level"`
}

// Read reads the memory now.
func Read() (Reading, error) { return read() }

// levelOf is the level for free bytes.
func levelOf(free uint64) Level {
	switch {
	case free < StopBytes:
		return Critical
	case free < LowBytes:
		return Low
	}
	return OK
}
