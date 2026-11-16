//go:build !windows

package lifecycle

import (
	"fmt"
	"os"
)

// Fatal tells the user why the app can't start.
func Fatal(msg string) { fmt.Fprintln(os.Stderr, msg) }
