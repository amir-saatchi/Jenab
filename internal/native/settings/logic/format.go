// Package logic is the Settings screen's pure logic, ported from
// frontend/src/settings (format.ts, desktop.tsx's helpers) and
// frontend/src/chat/provider-choice.ts with their tests: no MyGo, no
// stores.
package logic

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/version"
)

// AppName is the app's name as the About line writes it.
const AppName = "Jenab"

// Tokens is a token count in short form, as Intl's compact notation with
// one decimal writes it: 950, 12.5K, 1.2M.
func Tokens(n float64) string {
	if math.Abs(n) < 999.5 {
		return strconv.FormatFloat(math.Round(n), 'f', -1, 64)
	}
	units := []string{"K", "M", "B", "T"}
	v, i := n/1000, 0
	for i < len(units)-1 && math.Abs(math.Round(v*10)/10) >= 1000 {
		v /= 1000
		i++
	}
	return strconv.FormatFloat(math.Round(v*10)/10, 'f', -1, 64) + units[i]
}

// Total is a line's tokens, the cache's included.
func Total(l app.UsageLine) int { return l.Input + l.Output + l.CacheRead + l.CacheWrite }

// Cost is a line's cost in US dollars. Tokens without a price add a "+";
// a line with only such tokens has no cost.
func Cost(l app.UsageLine) string {
	if l.Cost == 0 && l.Unpriced > 0 {
		return "—"
	}
	c := fmt.Sprintf("$%.2f", l.Cost)
	if l.Cost > 0 && l.Cost < 0.01 {
		c = "<$0.01"
	}
	if l.Unpriced > 0 {
		c += "+"
	}
	return c
}

// ContextSize is a context window in short form, e.g. "1M" or "128K";
// empty when unknown.
func ContextSize(n int) string {
	if n <= 0 {
		return ""
	}
	return Tokens(float64(n))
}

// LimitDefault is the background limit a provider gets with no setting of
// its own: 1 for a local Ollama, else the global limit (SPEC 7.6).
func LimitDefault(kind, baseURL string, global int) int {
	if kind == "ollama" && isLocal(baseURL) {
		return 1
	}
	return global
}

func isLocal(baseURL string) bool {
	if baseURL == "" {
		return true
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		return false
	}
	h := u.Hostname()
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

var placeholder = regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`)

// Placeholders are the {field} names in a base URL, e.g. account_id.
func Placeholders(baseURL string) []string {
	out := []string{}
	for _, m := range placeholder.FindAllStringSubmatch(baseURL, -1) {
		out = append(out, m[1])
	}
	return out
}

// VersionLine is the About line: "Jenab 0.1.0-dev (d449b7e)", with
// ", modified" when the build had uncommitted changes.
func VersionLine(v version.Info) string {
	commit := ""
	if v.Commit != "" {
		mod := ""
		if v.Modified {
			mod = ", modified"
		}
		commit = " (" + v.Commit + mod + ")"
	}
	return AppName + " " + v.Version + commit
}

// BuildLine is the rest of the build, for bug reports: the Go version,
// the Wails version when the build has one, and the OS.
func BuildLine(v version.Info) string {
	parts := []string{"Go " + strings.TrimPrefix(v.Go, "go")}
	if v.Wails != "" {
		parts = append(parts, "Wails "+v.Wails)
	}
	if v.OS != "" {
		parts = append(parts, v.OS)
	}
	return strings.Join(parts, ", ")
}

// SamePath compares folders the way Windows and macOS do: without case
// and a trailing separator.
func SamePath(a, b string) bool {
	norm := func(s string) string { return strings.ToLower(strings.TrimRight(s, `\/`)) }
	return norm(a) == norm(b)
}

// Thousands writes n with commas, as toLocaleString in English.
func Thousands(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
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
