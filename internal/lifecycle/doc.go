// Package lifecycle is what the desktop app (cmd/desktop, on MyGo) does
// around the UI: one copy at a time (LockRoot), the quit order (Shutdown,
// Q30) and the message when the app can't start (Fatal).
package lifecycle
