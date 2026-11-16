package projects

// export-project.test.ts's "percent and stepText" and export-chat.test.ts's
// "formatText", ported; the file names and paths are state's
// (exports_test.go). Then the numbers of dev/format.ts the dialogs use.

import (
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chatexport"
	"github.com/amir-saatchi/jenab/internal/project"
)

func TestPercentAndStepText(t *testing.T) {
	p := func(done, total int64) *project.ExportProgress {
		return &project.ExportProgress{Done: done, Total: total}
	}
	for _, c := range []struct {
		p    *project.ExportProgress
		want float64
	}{{nil, 0}, {p(5, 0), 0}, {p(25, 100), 25}, {p(200, 100), 100}} {
		if got := Percent(c.p); got != c.want {
			t.Errorf("Percent(%+v) = %v, want %v", c.p, got, c.want)
		}
	}
	if got := StepText(project.StepChats); got != "Copying the chats…" {
		t.Errorf("StepText(chats) = %q", got)
	}
	if got := StepText(""); got != "Starting…" {
		t.Errorf("StepText() = %q", got)
	}
}

func TestFormatText(t *testing.T) {
	if !strings.Contains(FormatText(chatexport.FormatJSON), "as stored") {
		t.Error("JSON's text")
	}
	if !strings.Contains(FormatText(chatexport.FormatMarkdown), "to read") {
		t.Error("Markdown's text")
	}
}

func TestNumbers(t *testing.T) {
	for in, want := range map[int64]string{0: "0", 999: "999", 7412: "7,412", 1234567: "1,234,567", -1000: "-1,000"} {
		if got := num(in); got != want {
			t.Errorf("num(%d) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[int64]string{312: "0.3 KB", 199: "199 B", 200: "0.2 KB", 1000: "1 KB", 1234: "1.2 KB", 1_500_000: "1.5 MB", 2_000_000: "2 MB"} {
		if got := size(in); got != want {
			t.Errorf("size(%d) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[int64]string{0: "under a second", 999: "under a second", 1000: "1.0 s", 1540: "1.5 s"} {
		if got := seconds(in); got != want {
			t.Errorf("seconds(%d) = %q, want %q", in, got, want)
		}
	}
	if got := MissingText(1); got != "1 file was missing from the project and is not in the export." {
		t.Errorf("MissingText(1) = %q", got)
	}
	if got := MissingText(1200); got != "1,200 files were missing from the project and are not in the export." {
		t.Errorf("MissingText(1200) = %q", got)
	}
}
