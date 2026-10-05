//go:build !windows && !linux && !darwin

package sysmem

import "errors"

func read() (Reading, error) { return Reading{}, errors.New("sysmem: not supported on this OS") }
