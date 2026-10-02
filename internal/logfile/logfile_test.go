package logfile

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestOpenLevels(t *testing.T) {
	for _, debug := range []bool{false, true} {
		dir := t.TempDir()
		log, closeLog, err := Open(dir, debug)
		if err != nil {
			t.Fatal(err)
		}
		log.Debug("detail", "chat", "c_01")
		log.Info("turn done", "chat", "c_01", "ms", 412)
		if err := closeLog(); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(dir, Name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), `msg="turn done" chat=c_01 ms=412`) {
			t.Errorf("debug=%v: info line missing:\n%s", debug, b)
		}
		if got := strings.Contains(string(b), "detail"); got != debug {
			t.Errorf("debug=%v: debug line written = %v", debug, got)
		}
	}
}

func TestRotate(t *testing.T) {
	dir := t.TempDir()
	w, err := newWriter(dir, 100)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 40 {
		fmt.Fprintf(w, "line %02d %s\n", i, strings.Repeat("x", 30)) // 40 bytes each
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	names, _ := filepath.Glob(filepath.Join(dir, Name+"*"))
	if len(names) != maxFiles {
		t.Fatalf("got %d files, want %d: %v", len(names), maxFiles, names)
	}
	for _, n := range names {
		if st, _ := os.Stat(n); st.Size() > 100 {
			t.Errorf("%s is %d bytes, over the limit", filepath.Base(n), st.Size())
		}
	}
	cur, _ := os.ReadFile(filepath.Join(dir, Name))
	if !strings.Contains(string(cur), "line 39") {
		t.Errorf("the newest line is not in the current file:\n%s", cur)
	}
	if _, err := fmt.Fprintln(w, "after close"); err == nil {
		t.Error("write after Close should fail")
	}
}

func TestTail(t *testing.T) {
	dir := t.TempDir()
	w, err := newWriter(dir, 200)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 30 {
		chat := "c_01"
		if i%3 == 0 {
			chat = "c_02"
		}
		fmt.Fprintf(w, "n=%02d chat=%s\n", i, chat)
	}
	w.Close()

	got, err := Tail(dir, "chat=c_02")
	if err != nil {
		t.Fatal(err)
	}
	// Rotation keeps 5 files of at most 200 bytes (11 lines of 18 bytes), so
	// the oldest lines are gone; what is left is in order and only c_02.
	if len(got) == 0 || !slices.IsSorted(got) {
		t.Fatalf("Tail = %v", got)
	}
	for _, l := range got {
		if !strings.HasSuffix(l, "chat=c_02") {
			t.Errorf("Tail returned a line that doesn't match: %q", l)
		}
	}
	if last := got[len(got)-1]; last != "n=27 chat=c_02" {
		t.Errorf("last line = %q, want n=27", last)
	}
}

func TestTailLimit(t *testing.T) {
	var lines []string
	var b strings.Builder
	for i := range 1000 {
		fmt.Fprintf(&b, "%04d\n", i)
	}
	lines, err := appendMatches(lines, strings.NewReader(b.String()), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != maxTail || lines[0] != "0800" || lines[maxTail-1] != "0999" {
		t.Errorf("got %d lines, %s … %s", len(lines), lines[0], lines[len(lines)-1])
	}
}

func TestTailNoFiles(t *testing.T) {
	got, err := Tail(t.TempDir(), "x")
	if err != nil || len(got) != 0 {
		t.Errorf("Tail of an empty folder = %v, %v", got, err)
	}
}
