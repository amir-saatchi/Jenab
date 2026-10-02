// Package id has the typed IDs, the ULID generator and Source, the one
// format for who did something (SPEC 2.5).
package id

import (
	"crypto/rand"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

// Typed IDs, so the compiler catches a chat ID passed as a project ID (Q8).
// TypeScript sees plain strings.
type (
	Project  string
	Chat     string
	Message  string
	Run      string
	Task     string
	Change   string
	Approval string
)

// entropy is crypto/rand rather than the library's default, math/rand seeded
// from the clock, so IDs can't be guessed from the time they were made. IDs
// made in the same millisecond still increase, so New sorts in call order.
var entropy = &ulid.LockedMonotonicReader{MonotonicReader: ulid.Monotonic(rand.Reader, 0)}

// New returns a ULID: 48 bits of Unix milliseconds and 80 random bits, as 26
// characters.
func New() string {
	id, err := ulid.New(ulid.Timestamp(time.Now()), entropy)
	if err != nil { // crypto/rand failed, or 2^48 IDs in one millisecond
		panic("id: " + err.Error())
	}
	return id.String()
}

// Valid reports whether s looks like a ULID made by New, for example a
// project folder name when the registry is rebuilt (SPEC 2.1). New writes
// upper case, so lower case is not valid.
func Valid(s string) bool {
	_, err := ulid.ParseStrict(s)
	return err == nil && strings.ToUpper(s) == s
}

// Source says who did something: "app", "user", "auto", or "<kind>:<id>"
// such as "message:01J…" or "run:01J…" (SPEC 2.5). A kind may gain an ID
// later, so readers accept both forms.
type Source string

// Sources without an ID.
const (
	SourceApp  Source = "app"  // the app itself, e.g. the Mother chat
	SourceUser Source = "user" // the person using the app
	SourceAuto Source = "auto" // the Auto approval level (SPEC 8.8)
)

// SourceOf builds a source; an empty id gives just the kind.
func SourceOf(kind, id string) Source {
	if id == "" {
		return Source(kind)
	}
	return Source(kind + ":" + id)
}

// Kind is the part before the first colon, or the whole source.
func (s Source) Kind() string {
	k, _, _ := strings.Cut(string(s), ":")
	return k
}

// ID is the part after the first colon, or "" when there is none.
func (s Source) ID() string {
	_, v, _ := strings.Cut(string(s), ":")
	return v
}
