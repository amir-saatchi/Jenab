// Package secret reads and writes keys in the OS keychain, and keeps their
// values out of logs, errors and prompts (SPEC 6.7, Q36).
package secret

import (
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
// [secret:NAME]. It reads them again from the keychain (about 0.1 ms each),
// so no value is kept in memory. Use it on text that leaves the app or is
// logged, such as provider error bodies.
func (s *Store) Redact(text string) string {
	s.mu.Lock()
	names := make([]string, 0, len(s.names))
	for n := range s.names {
		names = append(names, n)
	}
	s.mu.Unlock()
	vals := make([]Value, 0, len(names))
	for _, n := range names {
		if v, err := s.Get(n); err == nil {
			vals = append(vals, v)
		}
	}
	// Longest first, so a key that contains another is replaced whole.
	slices.SortFunc(vals, func(a, b Value) int { return len(b.Reveal()) - len(a.Reveal()) })
	for _, v := range vals {
		text = v.Redact(text)
	}
	return text
}

func (s *Store) remember(name string) {
	s.mu.Lock()
	s.names[name] = struct{}{}
	s.mu.Unlock()
}
