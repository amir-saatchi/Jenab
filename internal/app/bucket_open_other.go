//go:build !windows && !darwin

package app

// markDownloaded does nothing: Linux desktops have no download mark.
func markDownloaded(string) {}
