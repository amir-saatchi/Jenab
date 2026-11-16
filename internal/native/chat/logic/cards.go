package logic

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/amir-saatchi/jenab/internal/chat"
)

// CardKey is a card waiting for the user, by "message:index", so the
// waiting bar can tell whether one is on screen and scroll to it.
func CardKey(message string, index int) string {
	return fmt.Sprintf("%s:%d", message, index)
}

var cardKinds = map[string]string{
	"host":           "New host",
	"migration":      "Schema change",
	"schema":         "Schema change",
	"starlark":       "Script",
	"connection":     "Connection",
	"mcp":            "MCP server",
	"command":        "Command",
	"memory":         "Memory edit",
	"private":        "Private network",
	"desktop_window": "Other app's window",
	"desktop_action": "Other app's window",
	"desktop_key":    "Key press",
	"desktop_macro":  "Macro",
}

// CardKind is what an approval card's kicker names after "Approval · ":
// "Refresh memory" for a refresh's card, else its kind's name, or the kind
// itself when it has none.
func CardKind(a chat.Approval) string {
	if strings.HasPrefix(a.Target, "refresh:") {
		return "Refresh memory"
	}
	if k, ok := cardKinds[a.Kind]; ok {
		return k
	}
	return a.Kind
}

// CardTime is when a card was answered, as its record shows it: "10:14" in
// t's own zone, or "" when it wasn't.
func CardTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("15:04")
}

// RecordText is how an answered or stopped card ended, for its one-line
// record after what was asked, e.g. "Approved 10:14". at is CardTime's.
func RecordText(a chat.Approval, at string) string {
	var result string
	switch {
	case a.Answer == chat.GrantDeny && a.Note != "":
		result = fmt.Sprintf("Denied %s · “%s”", at, a.Note)
	case a.Answer == chat.GrantDeny:
		result = "Denied " + at
	case a.Answer != "" && IsPick(a.Answer):
		label := ""
		if o, ok := a.Option(a.Answer); ok {
			label = o.Label
		}
		result = fmt.Sprintf("Picked %s %s", label, at)
	case a.Answer != "" && a.By == "auto":
		result = "Approved by the Auto level " + at
	case a.Answer != "":
		result = "Approved " + at
	default:
		result = "Stopped without an answer"
	}
	return jsTrim(result)
}

// MemoryTitle names a memory section by its target, memory:<scope>/<section>.
func MemoryTitle(target string) string {
	rest := strings.TrimPrefix(target, "memory:")
	scope, section, ok := strings.Cut(rest, "/")
	if !ok {
		// As rest.slice(0, -1) and rest.slice(0) are for indexOf's -1.
		_, n := utf8.DecodeLastRuneInString(rest)
		scope, section = rest[:len(rest)-n], rest
	}
	if scope == "user" {
		return "User memory · " + section
	}
	return "Project memory · " + section
}
