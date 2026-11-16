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

// Collect turns the crash output left by earlier runs in dir into crash
// logs, passing their text through redact, and keeps the newest Keep. It
// skips current, this run's file. It returns the new logs, oldest first.
// Empty files, from runs that ended without a crash but also without stop,
// are removed.
func Collect(dir, current string, redact func(string) string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("crash: %w", err)
	}
	var found []string
	var errs []error
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, rawExt) {
			continue
		}
		raw := filepath.Join(dir, name)
		if current != "" && filepath.Clean(raw) == filepath.Clean(current) {
			continue
		}
		b, err := os.ReadFile(raw)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if len(b) == 0 {
			os.Remove(raw)
			continue
		}
		text := string(b)
		if redact != nil {
			text = redact(text)
		}
		log := strings.TrimSuffix(raw, rawExt) + logExt
		if err := os.WriteFile(log, []byte(text), 0o600); err != nil {
			errs = append(errs, err)
			continue
		}
		os.Remove(raw)
		found = append(found, log)
	}
	if err := prune(dir); err != nil {
		errs = append(errs, err)
	}
	found = slices.DeleteFunc(found, func(p string) bool {
		_, err := os.Stat(p)
		return err != nil // pruned at once, when more than Keep came in
	})
	if err := errors.Join(errs...); err != nil {
		return found, fmt.Errorf("crash: %w", err)
	}
	return found, nil
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
