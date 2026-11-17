package secret

import (
	"strings"
	"testing"
)

// The keys here are made up.
func TestShapes(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Authorization: Bearer abcDEF0123456789xyz", "Authorization: Bearer [secret]"},
		{`map[Authorization:[Bearer abcDEF0123456789xyz]]`, `map[Authorization:[Bearer [secret]]]`},
		{`{"authorization":"Bearer abcDEF0123456789xyz"}`, `{"authorization":"Bearer [secret]"}`},
		{"x-api-key: 0123456789abcdefABCDEF", "x-api-key: [secret]"},
		{`{"api_key": "0123456789abcdefABCDEF"}`, `{"api_key": "[secret]"}`},
		{"GET https://host.example/v1/models?key=0123456789abcdefABCDEF&alt=sse", "GET https://host.example/v1/models?key=[secret]&alt=sse"},
		{"the call to sk-test-0123456789abcdef broke", "the call to [secret] broke"},
		{"AIzaDummyDummyDummyDummyDummyDummy0123", "[secret]"},
		{"nvapi-0123456789abcdefABCDEF here", "[secret] here"},
		// Left as they are:
		{"Authorization: Bearer [secret:provider:a]", "Authorization: Bearer [secret:provider:a]"},
		{`{"api_key":"[secret:provider:a]"}`, `{"api_key":"[secret:provider:a]"}`},
		{"claude/task-076-review-leftovers-and-more", "claude/task-076-review-leftovers-and-more"},
		{"goroutine 1 [running]:\nmain.main()\n\tD:/src/main.go:12 +0x1d", "goroutine 1 [running]:\nmain.main()\n\tD:/src/main.go:12 +0x1d"},
		{"the key is short: api_key=abc", "the key is short: api_key=abc"},
	} {
		if got := Shapes(c.in); got != c.want {
			t.Errorf("Shapes(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestStoredListsTheKeychain(t *testing.T) {
	if _, err := New(memKeyring{}).Stored(); err != ErrNotListable {
		t.Errorf("a keychain that can't list: %v", err)
	}
	s := New(listKeyring{memKeyring{"provider:gone": key, "provider:kept": "other-key"}})
	names, err := s.Stored()
	if err != nil || len(names) != 2 {
		t.Fatalf("Stored: %v, %v", names, err)
	}
	if got := s.Redactor(names...)("a " + key + " b"); strings.Contains(got, key) || !strings.Contains(got, "[secret:provider:gone]") {
		t.Errorf("redacted: %q", got)
	}
}

type listKeyring struct{ memKeyring }

func (k listKeyring) Names() ([]string, error) {
	var out []string
	for n := range k.memKeyring {
		out = append(out, n)
	}
	return out, nil
}

// Credential Manager's targets are "<service>:<name>"; another app's, or
// one named like the service alone, isn't ours.
func TestNamesUnder(t *testing.T) {
	got := namesUnder("jenab", []string{"jenab:provider:a", "jenab:provider:b:account_id", "jenabx:provider:c", "other:jenab:d", "jenab:", "jenab"})
	if strings.Join(got, ",") != "provider:a,provider:b:account_id" {
		t.Errorf("namesUnder = %q", got)
	}
}
