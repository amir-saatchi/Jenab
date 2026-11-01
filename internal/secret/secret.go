// Package secret reads and writes keys in the OS keychain, and keeps their
// values out of logs, errors and prompts (SPEC 6.7, Q36).
package secret

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
)

// ErrNotFound means the keychain has no entry with that name.
var ErrNotFound = errors.New("secret: not found")

// Size limits for a value: the Windows credential store takes at most
// 2,560 bytes (SPIKE-006).
const (
	MinSize = 1
	MaxSize = 2560
)

// Value is a secret read from the keychain. Every way of printing it shows
// [secret:NAME]; only Reveal returns the value. The value is held by a
// function, because fmt prints a function only as an address: even a struct
// with a Value in an unexported field, which fmt prints by reflection, never
// shows the key.
type Value struct {
	name   string
	reveal func() string
}

// NewValue wraps a value the user just typed, before it is stored, so it is
// handled like a stored one (e.g. a key tried by Connect).
func NewValue(name, v string) Value {
	return Value{name: name, reveal: func() string { return v }}
}

// Name is the keychain name, e.g. "provider:anthropic".
func (s Value) Name() string { return s.name }

// Reveal returns the value. Only the code that sends it to its own host
// (an auth header) calls it.
func (s Value) Reveal() string {
	if s.reveal == nil {
		return ""
	}
	return s.reveal()
}

// Redact replaces the value in text with [secret:NAME], for error bodies
// that echo a key back.
func (s Value) Redact(text string) string {
	if v := s.Reveal(); v != "" {
		return strings.ReplaceAll(text, v, s.String())
	}
	return text
}

func (s Value) String() string                { return "[secret:" + s.name + "]" }
func (s Value) GoString() string              { return s.String() }
func (s Value) LogValue() slog.Value          { return slog.StringValue(s.String()) }
func (s Value) MarshalText() ([]byte, error)  { return []byte(s.String()), nil }
func (s Value) Format(f fmt.State, verb rune) { fmt.Fprint(f, s.String()) } // %v, %+v, %#v, %s, %q, %x…
func (s Value) MarshalJSON() ([]byte, error)  { return json.Marshal(s.String()) }

// Keyring is the OS keychain, behind an interface so tests never touch the
// real one (Q12).
type Keyring interface {
	Get(name string) (string, error) // ErrNotFound when missing
	Set(name, value string) error
	Delete(name string) error // ErrNotFound when missing
}

// Store reads secrets when they are needed and never keeps their values
// (SPEC 6.7). It remembers only the names it has seen, for Redact.
type Store struct {
	kr    Keyring
	mu    sync.Mutex
	names map[string]struct{}
}

// New returns a store over kr.
func New(kr Keyring) *Store {
	return &Store{kr: kr, names: map[string]struct{}{}}
}

// Get reads a secret, e.g. Get("provider:anthropic").
func (s *Store) Get(name string) (Value, error) {
	v, err := s.kr.Get(name)
	if err != nil {
		return Value{}, fmt.Errorf("secret %s: %w", name, err)
	}
	s.remember(name)
	return Value{name: name, reveal: func() string { return v }}, nil
}

// Set stores a secret of 1 to 2,560 bytes.
func (s *Store) Set(name, value string) error {
	if n := len(value); n < MinSize || n > MaxSize {
		return fmt.Errorf("secret %s: %d bytes; it must be %d to %d", name, n, MinSize, MaxSize)
	}
	if err := s.kr.Set(name, value); err != nil {
		return fmt.Errorf("secret %s: %w", name, err)
	}
	s.remember(name)
	return nil
}

// Delete removes a secret; a missing one is not an error.
func (s *Store) Delete(name string) error {
	if err := s.kr.Delete(name); err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("secret %s: %w", name, err)
	}
	s.mu.Lock()
	delete(s.names, name)
	s.mu.Unlock()
	return nil
}

// Redact replaces every secret this store has read or written with
// [secret:NAME], raw or JSON-escaped. It reads them again from the
// keychain (about 0.1 ms each), so no value is kept in memory. Use it on
// text that leaves the app or is logged, such as provider error bodies.
func (s *Store) Redact(text string) string {
	return s.Redactor()(text)
}

// Redactor reads names (a missing one is skipped) and every secret this
// store has seen, once, and returns a function that replaces each with
// [secret:NAME], raw or JSON-escaped. Redact uses it for one text; a long
// job, such as a chat export (SPEC 2.3), keeps one instead of reading the
// keychain for every string. The values live in it until it is dropped, so
// drop it with the job. There is no minimum length: every stored value is
// replaced, since the keychain holds only secrets.
func (s *Store) Redactor(names ...string) func(string) string {
	for _, n := range names {
		_, _ = s.Get(n) // Get remembers the name; a missing one has nothing to hide
	}
	s.mu.Lock()
	all := make([]string, 0, len(s.names))
	for n := range s.names {
		all = append(all, n)
	}
	s.mu.Unlock()
	type pair struct{ old, new string }
	var pairs []pair
	for _, n := range all {
		v, err := s.Get(n)
		if err != nil || v.Reveal() == "" {
			continue
		}
		for _, form := range forms(v.Reveal()) {
			pairs = append(pairs, pair{form, v.String()})
		}
	}
	if len(pairs) == 0 {
		return func(t string) string { return t }
	}
	// Longest first: at one position, the replacer takes the first match
	// in argument order, so a key that contains another is replaced whole.
	slices.SortStableFunc(pairs, func(a, b pair) int { return len(b.old) - len(a.old) })
	args := make([]string, 0, 2*len(pairs))
	for _, p := range pairs {
		args = append(args, p.old, p.new)
	}
	return strings.NewReplacer(args...).Replace
}

// forms is a value as text may hold it: raw, and inside a JSON string with
// and without HTML escaping.
func forms(v string) []string {
	out := []string{v}
	add := func(b []byte) {
		b = bytes.TrimSuffix(b, []byte("\n")) // Encode ends with a newline
		if len(b) >= 2 {
			if f := string(b[1 : len(b)-1]); !slices.Contains(out, f) {
				out = append(out, f)
			}
		}
	}
	if b, err := json.Marshal(v); err == nil {
		add(b)
	}
	var buf bytes.Buffer
	e := json.NewEncoder(&buf)
	e.SetEscapeHTML(false)
	if e.Encode(v) == nil {
		add(buf.Bytes())
	}
	return out
}

func (s *Store) remember(name string) {
	s.mu.Lock()
	s.names[name] = struct{}{}
	s.mu.Unlock()
}
