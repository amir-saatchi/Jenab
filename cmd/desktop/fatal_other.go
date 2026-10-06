//go:build !windows

package main

import (
	"fmt"
	"os"
)

// fatal tells the user why the app can't start.
func fatal(msg string) { fmt.Fprintln(os.Stderr, msg) }
