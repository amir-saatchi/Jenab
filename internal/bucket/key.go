// Package bucket holds a project's object bytes, named by their SHA-256,
// and the rules for keys, links and serving untrusted files (SPEC 4). The
// index of keys and versions is in store, which calls this package.
package bucket

import (
	"errors"
	"fmt"
	"strings"

	"github.com/amir-saatchi/jenab/internal/id"
)

// MaxKey is the longest key, in bytes (SPEC 4.1).
const MaxKey = 512

var (
	ErrBadKey   = errors.New("bucket: bad key")
	ErrNotFound = errors.New("bucket: no such object")
	ErrTooLarge = errors.New("bucket: the file is too large")
)

// CheckKey checks a key: segments of [A-Za-z0-9._-] separated by "/", at
// most 512 characters, no leading "/", no empty segments, and no "." or ".."
// segments (SPEC 4.1).
func CheckKey(key string) error {
	if key == "" {
		return fmt.Errorf("%w: it is empty", ErrBadKey)
	}
	return checkPath(key, false)
}

// CheckPrefix checks a list prefix: "" or the start of a key, which may end
// in "/" or in the middle of a segment.
func CheckPrefix(prefix string) error {
	if prefix == "" {
		return nil
	}
	return checkPath(prefix, true)
}

func checkPath(s string, prefix bool) error {
	if len(s) > MaxKey {
		return fmt.Errorf("%w: longer than %d characters", ErrBadKey, MaxKey)
	}
	segs := strings.Split(s, "/")
	for i, seg := range segs {
		last := i == len(segs)-1
		switch {
		case seg == "" && !(prefix && last && i > 0):
			return fmt.Errorf("%w: %q has an empty segment", ErrBadKey, s)
		case (seg == "." || seg == "..") && !(prefix && last):
			return fmt.Errorf("%w: %q has a %q segment", ErrBadKey, s, seg)
		}
		for j := 0; j < len(seg); j++ {
			if !keyByte(seg[j]) {
				return fmt.Errorf("%w: %q may hold only letters, digits, '.', '_', '-' and '/'", ErrBadKey, s)
			}
		}
	}
	return nil
}

func keyByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-'
}

// KeyEnd is greater than every key that starts with a given prefix:
// prefix+KeyEnd bounds a range scan, since every key byte is below '{'.
const KeyEnd = "{"

const linkScheme = "jenab://"

// Link is an object's link, jenab://<project_id>/<key> (SPEC 4.5).
func Link(p id.Project, key string) string { return linkScheme + string(p) + "/" + key }

// ParseLink splits a link into its project and key, checking both.
func ParseLink(s string) (id.Project, string, error) {
	rest, ok := strings.CutPrefix(s, linkScheme)
	if !ok {
		return "", "", fmt.Errorf("bucket: %q is not a jenab:// link", s)
	}
	pid, key, _ := strings.Cut(rest, "/")
	if !id.Valid(pid) {
		return "", "", fmt.Errorf("bucket: %q has no valid project ID", s)
	}
	if err := CheckKey(key); err != nil {
		return "", "", err
	}
	return id.Project(pid), key, nil
}
