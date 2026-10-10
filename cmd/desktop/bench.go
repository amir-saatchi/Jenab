//go:build bench

package main

import (
	"fmt"
	"os"
	"strconv"
)

// The bench build (go build -tags production,bench) opens WebView2's
// DevTools port when JENAB_DEVTOOLS_PORT is set, so cmd/jenab-bench can
// drive and measure the app (P1-18). Shipped builds have no such port:
// WebView2 ignores WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS once the app
// passes arguments of its own, and Wails always does.
func init() {
	benchArgs = func() []string {
		port, err := strconv.Atoi(os.Getenv("JENAB_DEVTOOLS_PORT"))
		if err != nil || port <= 0 {
			return nil
		}
		return []string{fmt.Sprintf("--remote-debugging-port=%d", port)}
	}
}
