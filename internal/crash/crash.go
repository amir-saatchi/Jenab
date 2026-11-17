// Package crash keeps what the Go runtime writes when the app crashes
// (TASK-067): a panic nothing recovers, on any goroutine, or a fatal
// error, with every goroutine's stack. Start sends that output to a file
// in the logs folder; the next start finds the file, removes the secrets
// from it as the other logs do (SPEC 6.7), keeps the last few and tells
// the user. Nothing is sent anywhere.
package crash

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Keep is how many crash logs are kept; older ones are removed.
const Keep = 5

const (
	prefix = "crash-"
	rawExt = ".raw" // the runtime's output, as it wrote it
	logExt = ".log" // the same, without secrets
)

// Start sends the runtime's crash output, with every goroutine's stack, to
// a new file in dir, as well as to stderr. It returns the file's path and
// stop, which ends that at a normal quit and removes the empty file.
//
// Call stop at the end of a normal quit, never in a defer: a panic runs
// the defers of its goroutine before the runtime writes it, so a deferred
// stop would turn the output off first and leave nothing.
func Start(dir string) (path string, stop func(), err error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", nil, fmt.Errorf("crash: %w", err)
	}
	name := fmt.Sprintf("%s%s-%d%s", prefix, time.Now().Format("20060102-150405"), os.Getpid(), rawExt)
	path = filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", nil, fmt.Errorf("crash: %w", err)
	}
	defer f.Close() // the runtime keeps its own copy of the file
	debug.SetTraceback("all")
	if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
		os.Remove(path)
		return "", nil, fmt.Errorf("crash: %w", err)
	}
	stop = func() {
		debug.SetCrashOutput(nil, debug.CrashOptions{})
		if fi, err := os.Stat(path); err == nil && fi.Size() == 0 {
			os.Remove(path)
		}
	}
	return path, stop, nil
}

// A crash's output, which may hold keys, is kept only until its log is
// written. When that fails, it is tried again at the next starts, and
// after MaxTries tries, or once it is MaxAge old, it is removed without a
// log (TASK-073). Variables so tests can change them.
var (
	MaxTries = 3
	MaxAge   = 7 * 24 * time.Hour
)

// triesExt counts the failed tries of a .raw file, next to it.
const triesExt = ".tries"

// Result is what Collect did.
type Result struct {
	// Logs are the new crash logs, oldest first.
	Logs []string
	// Lost are crashes whose log couldn't be written: their output was
	// removed, since it may hold keys. Each is the output file's name.
	Lost []string
}

// Collect turns the crash output left by earlier runs in dir into crash
// logs, passing their text through redact, and keeps the newest Keep. It
// skips current, this run's file. Empty files, from runs that ended
// without a crash but also without stop, are removed. A file whose log
// can't be written stays for the next start, until MaxTries or MaxAge.
func Collect(dir, current string, redact func(string) string) (Result, error) {
	var res Result
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return res, nil
	}
	if err != nil {
		return res, fmt.Errorf("crash: %w", err)
	}
	var errs []error
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, rawExt+triesExt) {
			// The count of a file that is gone.
			if _, err := os.Lstat(filepath.Join(dir, strings.TrimSuffix(name, triesExt))); errors.Is(err, os.ErrNotExist) {
				os.Remove(filepath.Join(dir, name))
			}
			continue
		}
		if e.IsDir() || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, rawExt) {
			continue
		}
		raw := filepath.Join(dir, name)
		if current != "" && filepath.Clean(raw) == filepath.Clean(current) {
			continue
		}
		log, err := writeLog(raw, redact)
		switch {
		case err == nil && log == "": // empty
			os.Remove(raw)
			os.Remove(raw + triesExt)
		case err == nil:
			os.Remove(raw)
			os.Remove(raw + triesExt)
			res.Logs = append(res.Logs, log)
		default:
			errs = append(errs, err)
			if failed(raw) {
				if rerr := os.Remove(raw); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
					errs = append(errs, rerr)
					continue
				}
				os.Remove(raw + triesExt)
				res.Lost = append(res.Lost, name)
			}
		}
	}
	if err := prune(dir); err != nil {
		errs = append(errs, err)
	}
	res.Logs = slices.DeleteFunc(res.Logs, func(p string) bool {
		_, err := os.Stat(p)
		return err != nil // pruned at once, when more than Keep came in
	})
	if err := errors.Join(errs...); err != nil {
		return res, fmt.Errorf("crash: %w", err)
	}
	return res, nil
}

// writeLog writes raw's text, through redact, to its log. It returns ""
// for an empty file.
func writeLog(raw string, redact func(string) string) (string, error) {
	b, err := os.ReadFile(raw)
	if err != nil {
		return "", err
	}
	if len(b) == 0 {
		return "", nil
	}
	text := string(b)
	if redact != nil {
		text = redact(text)
	}
	log := strings.TrimSuffix(raw, rawExt) + logExt
	if err := os.WriteFile(log, []byte(text), 0o600); err != nil {
		// A part written before the failure goes too.
		if fi, serr := os.Lstat(log); serr == nil && fi.Mode().IsRegular() {
			os.Remove(log)
		}
		return "", err
	}
	return log, nil
}

// failed counts a failed try of raw and reports whether it is the last:
// MaxTries tries, or raw is MaxAge old. A count that can't be written
// leaves the age to decide.
func failed(raw string) bool {
	if fi, err := os.Stat(raw); err == nil && time.Since(fi.ModTime()) >= MaxAge {
		return true
	}
	n := 0
	if b, err := os.ReadFile(raw + triesExt); err == nil {
		n, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	}
	n++
	if n >= MaxTries {
		return true
	}
	os.WriteFile(raw+triesExt, []byte(strconv.Itoa(n)), 0o600)
	return false
}

// prune removes all but the newest Keep crash logs. Their names start
// with the time, so the newest sort last.
func prune(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var logs []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) && strings.HasSuffix(e.Name(), logExt) {
			logs = append(logs, e.Name())
		}
	}
	slices.Sort(logs)
	var errs []error
	for len(logs) > Keep {
		if err := os.Remove(filepath.Join(dir, logs[0])); err != nil {
			errs = append(errs, err)
		}
		logs = logs[1:]
	}
	return errors.Join(errs...)
}
