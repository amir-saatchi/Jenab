package state

// shell/private-hosts.test.ts, ported, and the store's calls.

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/amir-saatchi/jenab/internal/id"
)

func TestHostProblem(t *testing.T) {
	for _, ok := range []string{"127.0.0.1", "nas.local", " localhost ", "::1", "[::1]", "fe80::1", "رایانه.local"} {
		if p := HostProblem(ok); p != "" {
			t.Errorf("HostProblem(%q) = %q", ok, p)
		}
	}
	for in, want := range map[string]string{
		"  ":                      "Type a host.",
		"nas.local printer.local": "Add one host at a time.",
		"http://127.0.0.1":        "Give the host alone, without http:// or a path.",
		"127.0.0.1/crypto":        "Give the host alone, without http:// or a path.",
		"127.0.0.1:8099":          "Give the host without a port.",
		"nas.local:":              "Give the host without a port.",
		"[::1]:80":                "Give the host without a port.",
	} {
		if got := HostProblem(in); got != want {
			t.Errorf("HostProblem(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWithHostAndWithoutHost(t *testing.T) {
	same(t, WithHost(nil, " 127.0.0.1 "), []string{"127.0.0.1"})
	same(t, WithHost([]string{"127.0.0.1"}, "127.0.0.1"), []string{"127.0.0.1"})
	same(t, WithHost([]string{"127.0.0.1"}, "nas.local"), []string{"127.0.0.1", "nas.local"})
	same(t, WithoutHost([]string{"127.0.0.1", "nas.local"}, "127.0.0.1"), []string{"nas.local"})
	same(t, WithoutHost([]string{"nas.local"}, "other"), []string{"nas.local"})
	hosts := []string{"a", "b"}
	WithoutHost(hosts, "a")
	same(t, hosts, []string{"a", "b"})
}

type fakeHosts struct {
	mu     sync.Mutex
	stored []string
	err    error
}

func (f *fakeHosts) PrivateHosts(context.Context, id.Project) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.stored), nil
}

func (f *fakeHosts) SetPrivateHosts(_ context.Context, _ id.Project, hosts []string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	f.stored = slices.Clone(hosts)
	return slices.Clone(hosts), nil
}

func TestHostsChangeStartsFromTheStoredList(t *testing.T) {
	l := newLoop()
	f := &fakeHosts{}
	h := NewHosts(f, l.post, func(err error) { t.Error(err) })
	h.Load("p")
	until(t, l, func() bool { return h.ByProject["p"] != nil })
	same(t, h.ByProject["p"], []string{})

	// A card adds a host after the page read the list.
	f.stored = []string{"nas.local"}
	done := make(chan error, 1)
	h.Change("p", func(hs []string) []string { return WithHost(hs, "127.0.0.1") }, func(err error) { done <- err })
	if !h.Busy["p"] {
		t.Error("not busy while the change runs")
	}
	if err := await(t, l, done); err != nil {
		t.Fatal(err)
	}
	same(t, h.ByProject["p"], []string{"nas.local", "127.0.0.1"})
	if h.Busy["p"] {
		t.Error("still busy")
	}

	f.err = errors.New("refused")
	h.Change("p", func(hs []string) []string { return WithoutHost(hs, "nas.local") }, func(err error) { done <- err })
	if err := await(t, l, done); err == nil {
		t.Error("no error")
	}
	same(t, h.ByProject["p"], []string{"nas.local", "127.0.0.1"})
}
