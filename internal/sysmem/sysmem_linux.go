package sysmem

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strconv"
)

func read() (Reading, error) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return Reading{}, fmt.Errorf("sysmem: %w", err)
	}
	return parseMeminfo(b)
}

// parseMeminfo reads MemTotal and MemAvailable, given in kB.
func parseMeminfo(b []byte) (Reading, error) {
	var r Reading
	var haveFree, haveTotal bool
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		f := bytes.Fields(sc.Bytes())
		if len(f) < 2 {
			continue
		}
		n, err := strconv.ParseUint(string(f[1]), 10, 64)
		if err != nil {
			continue
		}
		switch string(f[0]) {
		case "MemTotal:":
			r.Total, haveTotal = n*1024, true
		case "MemAvailable:":
			r.Free, haveFree = n*1024, true
		}
	}
	if !haveFree || !haveTotal {
		return Reading{}, fmt.Errorf("sysmem: no MemAvailable or MemTotal in /proc/meminfo")
	}
	r.Level = levelOf(r.Free)
	return r, nil
}
