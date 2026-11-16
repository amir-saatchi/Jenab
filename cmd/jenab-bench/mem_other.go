//go:build !windows

package main

import (
	"errors"
	"time"
)

var errNotWindows = errors.New("jenab-bench measures memory on Windows only for now")

func treeMemory(int) (memory, error) { return memory{}, errNotWindows }

func started(int) (time.Time, error) { return time.Time{}, errNotWindows }
