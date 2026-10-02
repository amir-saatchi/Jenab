package id

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
)

func TestNewIsValidAndSorted(t *testing.T) {
	ids := make([]string, 10000)
	for i := range ids {
		ids[i] = New()
		if !Valid(ids[i]) {
			t.Fatalf("New() = %q, not a valid ULID", ids[i])
		}
	}
	if !slices.IsSorted(ids) {
		t.Error("IDs from New are not in the order they were made")
	}
	if len(slices.Compact(slices.Clone(ids))) != len(ids) {
		t.Error("New returned the same ID twice")
	}
}

func TestNewTime(t *testing.T) {
	before := time.Now().UnixMilli()
	s := New()
	after := time.Now().UnixMilli()
	got := decodeMillis(t, s)
	if got < before || got > after {
		t.Errorf("time part of %q = %d, want between %d and %d", s, got, before, after)
	}
}

// decodeMillis reads the time part of a ULID.
func decodeMillis(t *testing.T, s string) int64 {
	t.Helper()
	id, err := ulid.ParseStrict(s)
	if err != nil {
		t.Fatal(err)
	}
	return int64(id.Time())
}

func TestNewConcurrent(t *testing.T) {
	const workers, each = 8, 1000
	out := make(chan string, workers*each)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for range each {
				out <- New()
			}
		})
	}
	wg.Wait()
	close(out)
	seen := map[string]bool{}
	for s := range out {
		if seen[s] {
			t.Fatalf("New returned %q twice", s)
		}
		seen[s] = true
	}
}

func TestValid(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"01J9ZQ3M5X8K2V7R4T6W0Y1N3B", true},
		{"01j9zq3m5x8k2v7r4t6w0y1n3b", false}, // lowercase
		{"01J9ZQ3M5X8K2V7R4T6W0Y1N3", false},  // 25 characters
		{"81J9ZQ3M5X8K2V7R4T6W0Y1N3B", false}, // more than 128 bits
		{"01J9ZQ3M5X8K2V7R4T6W0Y1N3I", false}, // I is not in the alphabet
		{"", false},
	}
	for _, tt := range tests {
		if got := Valid(tt.in); got != tt.want {
			t.Errorf("Valid(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestSource(t *testing.T) {
	tests := []struct {
		src      Source
		kind, id string
	}{
		{SourceApp, "app", ""},
		{SourceUser, "user", ""},
		{"user:u_01", "user", "u_01"},
		{SourceOf("message", "01J9"), "message", "01J9"},
		{SourceOf("run", ""), "run", ""},
		{"undo:change:7", "undo", "change:7"}, // only the first colon splits
	}
	for _, tt := range tests {
		if tt.src.Kind() != tt.kind || tt.src.ID() != tt.id {
			t.Errorf("%q: Kind, ID = %q, %q; want %q, %q", tt.src, tt.src.Kind(), tt.src.ID(), tt.kind, tt.id)
		}
	}
}
