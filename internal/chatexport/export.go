// Package chatexport saves one chat as a file, as Markdown for reading or
// as JSON for keeping everything (SPEC 2.3). It reads only stored data and
// never calls a model. Turns are read and written in batches, so a long
// chat is never held in memory whole.
package chatexport

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
)

// Format is the kind of file an export writes.
type Format string

const (
	FormatMarkdown Format = "markdown"
	FormatJSON     Format = "json"
)

// FormatName and Version head every JSON export. Version goes up when the
// shape changes.
const (
	FormatName = "jenab.chat"
	Version    = 1
)

// Batch is how many turns are read at a time.
const Batch = 50

var (
	// ErrFormat means the format is neither Markdown nor JSON.
	ErrFormat = errors.New("chatexport: the format must be markdown or json")
	// ErrWrite wraps a failure to write the file, such as a folder the
	// user can't write to; a failure to read the chat is not one.
	ErrWrite = errors.New("chatexport: the file can't be written")
)

// Source is the chat an export reads.
type Source struct {
	Project     id.Project
	ProjectName string
	Chat        chat.Chat
	// Model is how the header names the chat's model, e.g.
	// "default (gemini-2.5-flash)"; "" uses Chat.Model.
	Model    string
	Notes    chat.SessionNote
	LastTurn int // the chat's last turn when the export starts; later turns are left out
	// Turns returns the messages of turns from through to, in order.
	Turns func(ctx context.Context, from, to int) ([]chat.Message, error)
	// Redact replaces secrets with [secret:NAME] (SPEC 6.7); nil
	// replaces nothing. Every string goes through it before it is cut or
	// written.
	Redact func(string) string
	Now    time.Time      // the export's time; zero means now
	Loc    *time.Location // for the Markdown's times; nil means local
}

// Result is what an export wrote.
type Result struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`     // bytes
	Messages int    `json:"messages"` // messages written
}

// Write writes the chat to w in format f.
func Write(ctx context.Context, w io.Writer, f Format, s Source) (int, error) {
	if s.Redact == nil {
		s.Redact = func(t string) string { return t }
	}
	if s.Now.IsZero() {
		s.Now = time.Now()
	}
	if s.Loc == nil {
		s.Loc = time.Local
	}
	switch f {
	case FormatMarkdown:
		return writeMarkdown(ctx, w, s)
	case FormatJSON:
		return writeJSON(ctx, w, s)
	}
	return 0, ErrFormat
}

// WriteFile writes the chat to path. It writes a temporary file next to
// path and renames it when complete, so a failed export leaves nothing
// behind, and a file already at path is replaced only by a complete one.
func WriteFile(ctx context.Context, path string, f Format, s Source) (Result, error) {
	if f != FormatMarkdown && f != FormatJSON {
		return Result{}, ErrFormat
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %w", ErrWrite, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return Result{}, fmt.Errorf("%w: %w", ErrWrite, err)
	}
	ok := false
	defer func() {
		if !ok {
			tmp.Close()
			os.Remove(tmp.Name()) // our own partial file
		}
	}()
	bw := bufio.NewWriterSize(fileWriter{tmp}, 64<<10)
	n, err := Write(ctx, bw, f, s)
	if err != nil {
		return Result{}, err
	}
	if err := bw.Flush(); err != nil {
		return Result{}, err
	}
	if err := tmp.Sync(); err != nil {
		return Result{}, fmt.Errorf("%w: %w", ErrWrite, err)
	}
	st, err := tmp.Stat()
	if err != nil {
		return Result{}, fmt.Errorf("%w: %w", ErrWrite, err)
	}
	if err := tmp.Close(); err != nil {
		return Result{}, fmt.Errorf("%w: %w", ErrWrite, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return Result{}, fmt.Errorf("%w: %w", ErrWrite, err)
	}
	ok = true
	return Result{Path: path, Size: st.Size(), Messages: n}, nil
}

// fileWriter marks the file's write errors with ErrWrite, so they are told
// apart from errors reading the chat.
type fileWriter struct{ f *os.File }

func (w fileWriter) Write(b []byte) (int, error) {
	n, err := w.f.Write(b)
	if err != nil {
		err = fmt.Errorf("%w: %w", ErrWrite, err)
	}
	return n, err
}

// eachBatch calls fn with the messages of every Batch turns up to
// s.LastTurn.
func eachBatch(ctx context.Context, s Source, fn func([]chat.Message) error) error {
	for from := 1; from <= s.LastTurn; from += Batch {
		if err := ctx.Err(); err != nil {
			return err
		}
		ms, err := s.Turns(ctx, from, min(from+Batch-1, s.LastTurn))
		if err != nil {
			return err
		}
		if err := fn(ms); err != nil {
			return err
		}
	}
	return nil
}
