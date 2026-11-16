// Package logic holds the chat screen's logic that draws nothing
// (SPIKE-048): what frontend/src/chat computes for the React app, as plain
// Go, on the Go types the services return. Their tests are ported with
// them and are the spec.
//
// Strings the user reads are the same as the React app's, Unicode
// isolates and en dashes included. Lengths count UTF-16 code units, as
// JavaScript's do, so a count shown to the user matches. Nothing here
// imports MyGo.
package logic
