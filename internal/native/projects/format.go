package projects

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/amir-saatchi/jenab/internal/chatexport"
	"github.com/amir-saatchi/jenab/internal/project"
)

// The texts and numbers of the exports (shell/export-project.tsx,
// export-chat.tsx, dev/format.ts).

// StepText says what the export is copying.
func StepText(step project.ExportStep) string {
	switch step {
	case project.StepProject:
		return "Copying the project's data…"
	case project.StepChats:
		return "Copying the chats…"
	case project.StepObjects:
		return "Copying the files…"
	case project.StepSkills:
		return "Copying the skills…"
	case project.StepDone:
		return "Finishing…"
	}
	return "Starting…"
}

// Percent is how far the export is, 0 to 100.
func Percent(p *project.ExportProgress) float64 {
	if p == nil || p.Total <= 0 {
		return 0
	}
	return min(100, 100*float64(p.Done)/float64(p.Total))
}

// FormatText says what each format of a chat's export holds.
func FormatText(format chatexport.Format) string {
	if format == chatexport.FormatJSON {
		return "Every message part as stored, thinking included, for keeping everything."
	}
	return "The messages to read: tool calls as one line each, without thinking or full tool results."
}

// num is a whole number with group separators: 7,412.
func num(n int64) string {
	s := strconv.FormatInt(n, 10)
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

// size is a byte count: 312 B, 0.2 KB at 200 bytes and up, 1.5 MB.
func size(bytes int64) string {
	switch {
	case bytes < 200:
		return fmt.Sprintf("%d B", bytes)
	case bytes < 1_000_000:
		return trim(fmt.Sprintf("%.1f", float64(bytes)/1000)) + " KB"
	}
	return trim(fmt.Sprintf("%.1f", float64(bytes)/1_000_000)) + " MB"
}

func trim(s string) string { return strings.TrimSuffix(s, ".0") }

// seconds is how long an export took.
func seconds(ms int64) string {
	if ms < 1000 {
		return "under a second"
	}
	return fmt.Sprintf("%.1f s", float64(ms)/1000)
}

// plural is n with the word for one or for more.
func plural(n int64, one, more string) string {
	if n == 1 {
		return num(n) + " " + one
	}
	return num(n) + " " + more
}

// MissingText says how many files an export left out because the project
// had lost them.
func MissingText(n int) string {
	if n == 1 {
		return "1 file was missing from the project and is not in the export."
	}
	return num(int64(n)) + " files were missing from the project and are not in the export."
}
