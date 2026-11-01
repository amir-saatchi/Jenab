package app

import (
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

// markDownloaded sets the quarantine attribute, as a browser does for a
// download, so Gatekeeper checks anything in the copy before it runs.
func markDownloaded(file string) {
	v := fmt.Sprintf("0081;%08x;Jenab;", time.Now().Unix())
	_ = unix.Setxattr(file, "com.apple.quarantine", []byte(v), 0)
}
