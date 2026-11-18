package project

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func TestLevel(t *testing.T) {
	e := newEnv(t, t.TempDir())
	defer e.stop(t)
	ctx := context.Background()
	level := Auto
	e.m.d.Level = func() Level { return level }
	p := open(t, e.m, create(t, e.m, "Auto"))
	defer p.Release()
	if l, err := p.Level(ctx); err != nil || l != Auto {
		t.Errorf("a new project's level = %q, %v; want the default, auto", l, err)
	}
	level = Strict // a new default doesn't change existing projects
	if l, _ := p.Level(ctx); l != Auto {
		t.Errorf("level after the default changed = %q", l)
	}
	if err := p.SetLevel(ctx, Strict); err != nil {
		t.Fatal(err)
	}
	if l, _ := p.Level(ctx); l != Strict {
		t.Errorf("level after SetLevel = %q", l)
	}
	if err := p.SetLevel(ctx, "yolo"); err == nil {
		t.Error("an unknown level was stored")
	}
	if err := p.DB.SetMeta(ctx, map[string]string{metaLevel: ""}); err != nil {
		t.Fatal(err)
	}
	if l, _ := p.Level(ctx); l != Standard {
		t.Errorf("level with none stored = %q, want standard", l)
	}

	level = "yolo" // a bad default gives Standard
	q := open(t, e.m, create(t, e.m, "Bad default"))
	defer q.Release()
	if l, _ := q.Level(ctx); l != Standard {
		t.Errorf("level from a bad default = %q", l)
	}
}

func TestPrivateHosts(t *testing.T) {
	e := newEnv(t, t.TempDir())
	defer e.stop(t)
	ctx := context.Background()
	p := open(t, e.m, create(t, e.m, "Hosts"))
	defer p.Release()
	if hs, err := p.PrivateHosts(ctx); err != nil || len(hs) != 0 {
		t.Errorf("a new project's exceptions = %v, %v", hs, err)
	}
	if _, err := p.SetPrivateHosts(ctx, []string{" NAS.local ", "رایانه.local", "", "nas.local"}); err != nil {
		t.Fatal(err)
	}
	hs, err := p.PrivateHosts(ctx)
	if want := []string{"nas.local", "xn--mgba2a5ff90f.local"}; err != nil || !slices.Equal(hs, want) {
		t.Errorf("PrivateHosts = %q, %v; want %q", hs, err, want)
	}
}

// The exceptions take a name or an address alone; anything else is a
// *HostError and changes nothing. A card's "Allow in this project" adds
// one (6.7, 8.8).
func TestPrivateHostsChecked(t *testing.T) {
	e := newEnv(t, t.TempDir())
	defer e.stop(t)
	ctx := context.Background()
	p := open(t, e.m, create(t, e.m, "Checked"))
	defer p.Release()
	got, err := p.SetPrivateHosts(ctx, []string{"127.0.0.1", "[0:0::1]", "Localhost."})
	if want := []string{"127.0.0.1", "::1", "localhost"}; err != nil || !slices.Equal(got, want) {
		t.Errorf("SetPrivateHosts = %q, %v; want %q", got, err, want)
	}
	for _, bad := range []string{"http://127.0.0.1", "127.0.0.1:8099", "nas.local/x"} {
		_, err := p.SetPrivateHosts(ctx, []string{"nas.local", bad})
		var he *HostError
		if !errors.As(err, &he) || he.Host != bad {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if hs, _ := p.PrivateHosts(ctx); len(hs) != 3 {
		t.Errorf("a refused list changed the hosts: %q", hs)
	}
	for _, h := range []string{"NAS.local", "nas.local", "127.0.0.1"} {
		if err := p.AddPrivateHost(ctx, h); err != nil {
			t.Fatal(err)
		}
	}
	if hs, _ := p.PrivateHosts(ctx); !slices.Equal(hs, []string{"127.0.0.1", "::1", "localhost", "nas.local"}) {
		t.Errorf("after AddPrivateHost: %q", hs)
	}
	if err := p.AddPrivateHost(ctx, "nas.local:80"); err == nil {
		t.Error("a host with a port was added")
	}
	if got, err := p.SetPrivateHosts(ctx, nil); err != nil || got == nil || len(got) != 0 {
		t.Errorf("cleared: %q, %v", got, err)
	}
}

// A card's AddPrivateHost that comes while an edit holds the list waits
// for the edit's save, and is kept (TASK-079). The edit lets it run for a
// while: had the edit read and then saved the list apart, the card's host
// would be saved over.
func TestAnEditKeepsACardsHost(t *testing.T) {
	e := newEnv(t, t.TempDir())
	defer e.stop(t)
	ctx := context.Background()
	p := open(t, e.m, create(t, e.m, "Edit"))
	defer p.Release()
	if _, err := p.SetPrivateHosts(ctx, []string{"nas.local"}); err != nil {
		t.Fatal(err)
	}
	added := make(chan error, 1)
	got, err := p.EditPrivateHosts(ctx, func(hs []string) []string {
		go func() { added <- p.AddPrivateHost(ctx, "printer.local") }()
		select {
		case err := <-added: // it didn't wait
			added <- err
		case <-time.After(300 * time.Millisecond):
		}
		return append(hs, "127.0.0.1")
	})
	if want := []string{"127.0.0.1", "nas.local"}; err != nil || !slices.Equal(got, want) {
		t.Errorf("EditPrivateHosts = %q, %v; want %q", got, err, want)
	}
	if err := <-added; err != nil {
		t.Fatal(err)
	}
	if hs, _ := p.PrivateHosts(ctx); !slices.Equal(hs, []string{"127.0.0.1", "nas.local", "printer.local"}) {
		t.Errorf("after the edit and the card: %q", hs)
	}

	// A host that isn't one changes nothing.
	_, err = p.EditPrivateHosts(ctx, func(hs []string) []string { return append(hs, "nas.local:80") })
	var he *HostError
	if !errors.As(err, &he) || he.Host != "nas.local:80" {
		t.Errorf("a bad host: %v", err)
	}
	if hs, _ := p.PrivateHosts(ctx); len(hs) != 3 {
		t.Errorf("a refused edit changed the hosts: %q", hs)
	}
}
