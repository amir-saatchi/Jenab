//go:build !windows

package webui

// systemDark is false off Windows for now: the window opens light and the
// page paints the theme.
func systemDark() bool { return false }
