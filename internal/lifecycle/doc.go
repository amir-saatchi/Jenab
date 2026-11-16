// Package lifecycle is what both desktop apps (cmd/desktop on Wails,
// cmd/desktop-native on MyGo) do around the UI: one copy at a time
// (LockRoot), the quit order (Shutdown, Q30) and the message when the app
// can't start (Fatal).
package lifecycle
