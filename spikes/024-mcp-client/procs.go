package main

import (
	"fmt"
	"strings"
)

// proc is one running process.
type proc struct {
	PID, Parent int
	Exe         string
	Created     uint64 // start time, to tell a reused PID apart
}

func procList(ps []proc) string {
	if len(ps) == 0 {
		return "none"
	}
	var b []string
	for _, p := range ps {
		b = append(b, fmt.Sprintf("%s(%d)", p.Exe, p.PID))
	}
	return strings.Join(b, ", ")
}
