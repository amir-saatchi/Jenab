package dev

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/id"
)

// The formatting helpers of the developer tools, ported from
// frontend/src/dev/format.ts with its tests.

// Num is a whole number with group separators: 7,412.
func Num(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// Duration is a time span in milliseconds, in its natural unit: 38 ms,
// 1.2 s, 3 min 4 s.
func Duration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%d ms", max(0, ms))
	}
	if ms < 60_000 {
		return fixed1(float64(ms)/1000) + " s"
	}
	s := int64(math.Round(float64(ms) / 1000))
	if s%60 != 0 {
		return fmt.Sprintf("%d min %d s", s/60, s%60)
	}
	return fmt.Sprintf("%d min", s/60)
}

// Size is a byte count: 312 B, 0.2 KB at 200 bytes and up, 1.5 MB.
func Size(bytes int) string {
	switch {
	case bytes < 200:
		return fmt.Sprintf("%d B", bytes)
	case bytes < 1_000_000:
		return fixed1(float64(bytes)/1000) + " KB"
	}
	return fixed1(float64(bytes)/1_000_000) + " MB"
}

// fixed1 writes x with one decimal, as JavaScript's toFixed(1), without a
// trailing ".0".
func fixed1(x float64) string {
	return strings.TrimSuffix(strconv.FormatFloat(x, 'f', 1, 64), ".0")
}

// Scale is the timeline's span and its ticks, in milliseconds: at most
// six, at 1, 2 or 5 times a power of ten.
type Scale struct {
	Span  int64
	Step  int64
	Ticks []int64
}

// NewScale is the scale of a timeline total milliseconds long.
func NewScale(total int64) Scale {
	t := max(total, 1)
	var step int64
find:
	for p := int64(1); ; p *= 10 {
		for _, s := range []int64{p, 2 * p, 5 * p} {
			if float64(t)/float64(s) <= 5 {
				step = s
				break find
			}
		}
	}
	span := (t + step - 1) / step * step
	ticks := make([]int64, 0, span/step+1)
	for v := int64(0); v <= span; v += step {
		ticks = append(ticks, v)
	}
	return Scale{Span: span, Step: step, Ticks: ticks}
}

// Tick is a tick's label: in seconds when the step is a second or more.
func Tick(ms, step int64) string {
	if step >= 1000 {
		return strconv.FormatFloat(float64(ms)/1000, 'f', -1, 64) + " s"
	}
	return fmt.Sprintf("%d ms", ms)
}

// ChatOption is a chat with recorded turns, for the inspector's picker.
type ChatOption struct {
	Key     string
	Project id.Project
	Chat    id.Chat
	Label   string
}

// ChatKey names a chat in the pickers.
func ChatKey(p id.Project, c id.Chat) string { return string(p) + "/" + string(c) }

// ChatOptions lists the chats of the turns, in the order of the turns,
// which are the most recent first.
func ChatOptions(turns []app.TurnItem) []ChatOption {
	var out []ChatOption
	for _, t := range turns {
		key := ChatKey(t.Project, t.Chat)
		if slices.ContainsFunc(out, func(o ChatOption) bool { return o.Key == key }) {
			continue
		}
		project := t.ProjectName
		if project == "" {
			project = "Closed project"
		}
		title := t.ChatTitle
		if title == "" {
			title = "Untitled chat"
		}
		out = append(out, ChatOption{Key: key, Project: t.Project, Chat: t.Chat, Label: project + " · " + title})
	}
	return out
}

var partNames = map[string]string{
	"text":      "answer",
	"thinking":  "thinking",
	"tool_call": "tool call",
	"image":     "image",
}

// Parts names what a request got back, once each: "thinking · answer".
func Parts(kinds []string) string {
	var out []string
	for _, k := range kinds {
		name, ok := partNames[k]
		if !ok {
			name = strings.ReplaceAll(k, "_", " ")
		}
		if !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return strings.Join(out, " · ")
}

// Share is the part of a prompt read from the cache: "6,120 cached
// (83 %)"; "" for no prompt.
func Share(cached, prompt int64) string {
	if prompt <= 0 {
		return ""
	}
	return fmt.Sprintf("%s cached (%d %%)", Num(cached), int64(math.Round(float64(cached)/float64(prompt)*100)))
}

// The helpers of inspector.tsx and runtime.tsx, which have no tests of
// their own there.

// TurnLabel is a turn in the turn picker: "14:05 · turn 3 · 2 requests";
// clock is its start in the user's format.
func TurnLabel(clock string, t app.TurnItem) string {
	what := "running"
	if !t.Running {
		what = plural(t.Requests, "request", "requests")
	}
	return fmt.Sprintf("%s · turn %d · %s", clock, t.Turn, what)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// Prompt is the tokens a request sent: those read fresh, from the cache
// and written to it.
func Prompt(r app.RequestView) int64 {
	return int64(r.Usage.Input + r.Usage.CacheRead + r.Usage.CacheWrite)
}

// Counted is a request's token count, or "–" before the provider sent its
// counts.
func Counted(r app.RequestView, n int64) string {
	if Prompt(r)+int64(r.Usage.Output) == 0 {
		return "–"
	}
	return Num(n)
}

// ProviderState is a provider's pause and its last problem, or "OK".
func ProviderState(p app.ProviderStatus) string {
	var out []string
	if p.PausedMS > 0 {
		out = append(out, "Paused for "+Duration(p.PausedMS))
	}
	if p.LastProblem != "" {
		out = append(out, p.LastProblem)
	}
	if len(out) == 0 {
		return "OK"
	}
	return strings.Join(out, " · ")
}

// ProviderLimit is a provider's limit, with the limit now when a rate
// limit halved it: "2 of 4".
func ProviderLimit(p app.ProviderStatus) string {
	if p.Current == p.Limit {
		return strconv.Itoa(p.Limit)
	}
	return fmt.Sprintf("%d of %d", p.Current, p.Limit)
}

// StuckTitle is the runtime's warning over what hasn't moved.
func StuckTitle(n int) string {
	if n == 1 {
		return "1 thing hasn't moved for longer than its limit"
	}
	return fmt.Sprintf("%d things haven't moved for longer than their limit", n)
}

// ProjectLine is a project's line in the runtime: its work and its leases.
func ProjectLine(work, leases int) string {
	running := "Nothing running."
	if work > 0 {
		running = fmt.Sprintf("%d running.", work)
	}
	return fmt.Sprintf("%s Held open by %s.", running, plural(leases, "lease", "leases"))
}
