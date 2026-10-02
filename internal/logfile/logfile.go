// Package logfile writes the app log: slog's text handler into
// <data>/logs/, rotated by size (Q36). Log IDs, kinds, timings and errors;
// never message content, prompts or table data.
package logfile

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Name is the current log file; older ones are Name.1 (newest) to Name.4.
const Name = "jenab.log"

const (
	maxSize  = 10 << 20 // bytes per file
	maxFiles = 5        // the current file plus 4 old ones
	maxTail  = 200      // lines returned by Tail
)

// Open starts logging into dir at Info, or Debug when debug is set. The
// returned function closes the file.
func Open(dir string, debug bool) (*slog.Logger, func() error, error) {
	w, err := newWriter(dir, maxSize)
	if err != nil {
		return nil, nil, err
	}
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})), w.Close, nil
}

// writer appends to dir/Name and rotates before a write would pass limit.
type writer struct {
	mu    sync.Mutex
	dir   string
	limit int64
	f     *os.File
	size  int64
}

func newWriter(dir string, limit int64) (*writer, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("logfile: %w", err)
	}
	w := &writer{dir: dir, limit: limit}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *writer) open() error {
	f, err := os.OpenFile(filepath.Join(w.dir, Name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("logfile: %w", err)
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return fmt.Errorf("logfile: %w", err)
	}
	w.f, w.size = f, st.Size()
	return nil
}

func (w *writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return 0, os.ErrClosed
	}
	if w.size > 0 && w.size+int64(len(p)) > w.limit {
		w.rotate()
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	return n, err
}

// rotate shifts Name.3 → Name.4 … Name → Name.1. If a rename fails (on
// Windows, while another program has the file open), it keeps writing to
// the current file and tries again on a later write.
func (w *writer) rotate() {
	if err := w.f.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "logfile:", err)
	}
	path := filepath.Join(w.dir, Name)
	os.Remove(path + "." + strconv.Itoa(maxFiles-1))
	for i := maxFiles - 2; i >= 1; i-- {
		os.Rename(path+"."+strconv.Itoa(i), path+"."+strconv.Itoa(i+1))
	}
	if err := os.Rename(path, path+".1"); err != nil {
		fmt.Fprintln(os.Stderr, "logfile: rotate:", err)
	}
	if err := w.open(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		w.f = nil
	}
}

func (w *writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return nil
	}
	err := w.f.Close()
	w.f = nil
	return err
}

// Tail returns the last 200 lines across the log files that contain match,
// oldest first, for *Copy details*. An empty match returns the last lines.
func Tail(dir, match string) ([]string, error) {
	path := filepath.Join(dir, Name)
	var lines []string
	for i := maxFiles - 1; i >= 0; i-- {
		p := path
		if i > 0 {
			p += "." + strconv.Itoa(i)
		}
		f, err := os.Open(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("logfile: %w", err)
		}
		lines, err = appendMatches(lines, f, match)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("logfile: %w", err)
		}
	}
	return lines, nil
}

func appendMatches(lines []string, r io.Reader, match string) ([]string, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		if strings.Contains(sc.Text(), match) {
			lines = append(lines, sc.Text())
			if len(lines) > 2*maxTail {
				lines = append(lines[:0], lines[len(lines)-maxTail:]...)
			}
		}
	}
	if len(lines) > maxTail {
		lines = lines[len(lines)-maxTail:]
	}
	return lines, sc.Err()
}
