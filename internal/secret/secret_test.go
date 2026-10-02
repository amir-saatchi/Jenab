package secret

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
)

const key = "sk-test-0123456789abcdef" // fake

// memKeyring is a keychain in memory.
type memKeyring map[string]string

func (m memKeyring) Get(name string) (string, error) {
	v, ok := m[name]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}
func (m memKeyring) Set(name, v string) error { m[name] = v; return nil }
func (m memKeyring) Delete(name string) error {
	if _, ok := m[name]; !ok {
		return ErrNotFound
	}
	delete(m, name)
	return nil
}

func get(t *testing.T) Value {
	t.Helper()
	s := New(memKeyring{"provider:test": key})
	v, err := s.Get("provider:test")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestValueNeverPrints(t *testing.T) {
	v := get(t)
	type exported struct{ Key Value }
	type unexported struct{ key Value }

	var out []string
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d"} {
		out = append(out,
			fmt.Sprintf(verb, v),
			fmt.Sprintf(verb, exported{v}),
			fmt.Sprintf(verb, unexported{v}),
			fmt.Sprintf(verb, &exported{v}),
			fmt.Sprintf(verb, []Value{v}),
		)
	}
	out = append(out, v.String(), v.GoString(), fmt.Sprint(v), fmt.Sprintln(v))
	for _, s := range out {
		if strings.Contains(s, key) || strings.Contains(s, fmt.Sprintf("%x", key)) {
			t.Errorf("printed the key: %s", s)
		}
	}
	if got := fmt.Sprintf("%v", v); got != "[secret:provider:test]" {
		t.Errorf("%%v = %q", got)
	}
}

func TestValueInLogsAndJSON(t *testing.T) {
	v := get(t)
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("call", "key", v, "group", slog.GroupValue(slog.Any("k", v)))
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("call", "key", v)
	b, err := json.Marshal(struct{ Key Value }{v})
	if err != nil {
		t.Fatal(err)
	}
	buf.Write(b)
	if strings.Contains(buf.String(), key) {
		t.Errorf("the key reached a log or JSON:\n%s", buf.String())
	}
	if !json.Valid(b) || !strings.Contains(string(b), "[secret:provider:test]") {
		t.Errorf("JSON = %s", b)
	}
}

func TestReveal(t *testing.T) {
	if got := get(t).Reveal(); got != key {
		t.Errorf("Reveal = %q", got)
	}
	if (Value{}).Reveal() != "" {
		t.Error("zero Value should reveal nothing")
	}
}

func TestSetSizes(t *testing.T) {
	s := New(memKeyring{})
	tests := []struct {
		size int
		ok   bool
	}{{0, false}, {1, true}, {MaxSize, true}, {MaxSize + 1, false}}
	for _, tt := range tests {
		err := s.Set("conn:x", strings.Repeat("a", tt.size))
		if (err == nil) != tt.ok {
			t.Errorf("Set with %d bytes: err = %v", tt.size, err)
		}
	}
}

func TestGetMissing(t *testing.T) {
	_, err := New(memKeyring{}).Get("provider:none")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestDeleteMissingIsFine(t *testing.T) {
	if err := New(memKeyring{}).Delete("provider:none"); err != nil {
		t.Errorf("Delete of a missing secret: %v", err)
	}
}

func TestStoreRedact(t *testing.T) {
	kr := memKeyring{}
	s := New(kr)
	if err := s.Set("provider:a", "abcd1234"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("provider:b", "xxabcd1234yy"); err != nil { // contains the first
		t.Fatal(err)
	}
	got := s.Redact(`error: key "xxabcd1234yy" rejected; also abcd1234`)
	want := `error: key "[secret:provider:b]" rejected; also [secret:provider:a]`
	if got != want {
		t.Errorf("Redact =\n%s\nwant\n%s", got, want)
	}

	if err := s.Delete("provider:a"); err != nil {
		t.Fatal(err)
	}
	if got := s.Redact("abcd1234"); got != "abcd1234" {
		t.Errorf("a deleted secret is still redacted: %s", got)
	}
}

// TestOSKeyring writes, reads and deletes one entry in the real keychain,
// under the service "jenab-test". It runs only with JENAB_KEYCHAIN_TEST=1,
// so CI machines without a keychain skip it.
func TestOSKeyring(t *testing.T) {
	if os.Getenv("JENAB_KEYCHAIN_TEST") != "1" {
		t.Skip("set JENAB_KEYCHAIN_TEST=1 to use the real keychain")
	}
	s := New(OSKeyring("jenab-test"))
	const name = "provider:roundtrip"
	value := strings.Repeat("k", MaxSize)
	if err := s.Set(name, value); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Delete(name) })
	v, err := s.Get(name)
	if err != nil || v.Reveal() != value {
		t.Fatalf("Get = %d bytes, %v", len(v.Reveal()), err)
	}
	if err := s.Delete(name); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(name); !errors.Is(err, ErrNotFound) {
		t.Errorf("after Delete: err = %v, want ErrNotFound", err)
	}
}
