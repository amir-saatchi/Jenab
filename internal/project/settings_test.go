package project

import (
	"context"
	"slices"
	"testing"
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
	if err := p.SetPrivateHosts(ctx, []string{" NAS.local ", "رایانه.local", "", "nas.local"}); err != nil {
		t.Fatal(err)
	}
	hs, err := p.PrivateHosts(ctx)
	if want := []string{"nas.local", "xn--mgba2a5ff90f.local"}; err != nil || !slices.Equal(hs, want) {
		t.Errorf("PrivateHosts = %q, %v; want %q", hs, err, want)
	}
}
