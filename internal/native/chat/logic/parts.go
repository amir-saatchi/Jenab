package logic

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/amir-saatchi/jenab/internal/chat"
)

// firstLine is a short preview of s.
func firstLine(s string, most int) string {
	line, _, _ := strings.Cut(jsTrim(s), "\n")
	if utf16Len(line) > most {
		return utf16Cut(line, most-1) + "…"
	}
	return line
}

// argsValue reads a call's arguments as the React app does: JSON, or a
// JSON string holding JSON. text is what String(args) would show when
// they don't read.
func argsValue(args json.RawMessage) (v jsValue, text string, err error) {
	v, err = parseJSON(args)
	if err != nil {
		return jsValue{}, string(args), err
	}
	if v.kind == jsString {
		text = v.s
		v, err = parseJSON([]byte(v.s))
	}
	return v, text, err
}

// ArgsText is a call's arguments for its unfolded chip: indented JSON, cut
// past 4,000 characters.
func ArgsText(args json.RawMessage) string {
	v, text, err := argsValue(args)
	if err != nil {
		return text
	}
	s := v.stringify("  ")
	if utf16Len(s) > 4000 {
		return utf16Cut(s, 4000) + "\n…"
	}
	return s
}

// argsPreview is the call's arguments in one line, for its chip.
func argsPreview(args json.RawMessage) string {
	v, _, err := argsValue(args)
	if err != nil || (v.kind != jsObject && v.kind != jsArray) {
		// shown in full when unfolded
		return ""
	}
	_, vals := v.entries()
	var out []string
	for _, x := range vals {
		if x.kind == jsString || x.kind == jsNumber {
			out = append(out, x.String())
		}
	}
	return strings.Join(out, " · ")
}

// ChipState is how a tool call stands on its chip.
type ChipState string

const (
	ChipDone    ChipState = "done"
	ChipRunning ChipState = "running"
	ChipFailed  ChipState = "failed"
)

// Chip is what a tool call's chip shows: its state, and the line after
// the tool's name. Desktop is a desktop control step's chip, which doesn't
// unfold.
type Chip struct {
	State   ChipState
	Detail  string
	Desktop bool
}

// ToolChip is a tool call's chip, with its result if there is one. A
// desktop control step (SPEC 8.10) shows what it did, with the lengths of
// the texts, not the texts read from or typed into the window. Errors are
// Jenab's own and show in full.
func ToolChip(call chat.ToolCall, result *chat.ToolResult, running bool) Chip {
	c := Chip{State: ChipFailed, Desktop: strings.HasPrefix(call.Name, "desktop_")}
	switch {
	case result != nil && !result.IsError:
		c.State = ChipDone
	case result == nil && running:
		c.State = ChipRunning
	}
	if !c.Desktop {
		switch {
		case result != nil:
			c.Detail = firstLine(result.Text, 120)
		case running:
			c.Detail = argsPreview(call.Args)
			if c.Detail == "" {
				c.Detail = "running"
			}
		default:
			c.Detail = "not run"
		}
		return c
	}
	if result != nil && result.IsError {
		c.Detail = firstLine(result.Text, 120)
		return c
	}
	what := "not run"
	switch {
	case result != nil:
		what = characters(utf16Len(result.Text))
	case running:
		what = "running"
	}
	if args := DesktopArgs(call.Args); args != "" {
		what = args + " · " + what
	}
	c.Detail = what
	return c
}

func characters(n int) string {
	if n == 1 {
		return "1 character"
	}
	return strconv.Itoa(n) + " characters"
}

// DesktopArgs is a desktop call's arguments in one line: the action, key
// and element as they are, any other text by its length.
func DesktopArgs(args json.RawMessage) string {
	v, _, err := argsValue(args)
	if err != nil || (v.kind != jsObject && v.kind != jsArray) {
		// nothing to show
		return ""
	}
	keys, vals := v.entries()
	out := make([]string, len(keys))
	for n, k := range keys {
		x := vals[n]
		switch {
		case x.kind == jsArray:
			// A macro's params and log hold values typed into the window: only their count.
			out[n] = k + ": " + strconv.Itoa(len(x.items))
		case x.kind != jsString:
			out[n] = k + " " + x.String()
		case slices.Contains([]string{"id", "action", "key", "name"}, k):
			out[n] = x.s
		default:
			out[n] = k + ": " + characters(utf16Len(x.s))
		}
	}
	return strings.Join(out, " · ")
}

// NoticeText is a notice as the user reads it: the app writes them for the
// model, in brackets.
func NoticeText(text string) string {
	if len(text) >= 2 && strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]") {
		return text[1 : len(text)-1]
	}
	return text
}
