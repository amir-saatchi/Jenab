package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/id"
)

// The config stamp rises after every undo and redo of a config change,
// so the next turn's card is rebuilt (P3-03).
func TestConfigStampAfterUndo(t *testing.T) {
	ctx := context.Background()
	p, _ := openTestProject(t)
	stamp := func() int64 {
		t.Helper()
		s, err := p.ConfigStamp(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	t1, t2 := []id.Source{msg("t1")}, []id.Source{msg("t2")}
	saveView(t, p, t1[0], "btc_prices", priceView, 0)
	saveView(t, p, t2[0], "btc_prices", strings.Replace(priceView, "90 days", "30 days", 1), 1)
	last := stamp()
	for i, step := range []struct {
		turn []id.Source
		opts UndoOptions
	}{{t2, UndoOptions{}}, {t2, UndoOptions{Redo: true}}, {t2, UndoOptions{}}, {t1, UndoOptions{}}} {
		undoOK(t, p, step.turn, step.opts)
		if s := stamp(); s <= last {
			t.Fatalf("step %d: stamp %d, was %d", i+1, s, last)
		} else {
			last = s
		}
	}
	// The undo of the first save deleted the view.
	if _, err := p.Config(ctx, KindView, "btc_prices"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after undoing the first save: %v", err)
	}
}

// A check refuses a save or a delete inside its write, wrapped in
// ErrInvalid, and sees every live view and page.
func TestConfigCheck(t *testing.T) {
	ctx := context.Background()
	p, _ := openTestProject(t)
	saveView(t, p, msg("t1"), "btc_prices", priceView, 0)
	refused := errors.New("refused")
	var seen []string
	check := func(all []Config) error {
		seen = seen[:0]
		for _, c := range all {
			seen = append(seen, string(c.Kind)+":"+c.ID)
		}
		return refused
	}
	_, err := p.SaveConfig(ctx, ConfigSave{Kind: KindPage, ID: "home", YAML: "id: home", Source: msg("t2"), Check: check})
	if !errors.Is(err, ErrInvalid) || !errors.Is(err, refused) {
		t.Fatalf("save: %v", err)
	}
	if len(seen) != 1 || seen[0] != "view:btc_prices" {
		t.Errorf("seen %v", seen)
	}
	if _, err := p.Config(ctx, KindPage, "home"); !errors.Is(err, ErrNotFound) {
		t.Errorf("saved: %v", err)
	}
	_, err = p.DeleteConfig(ctx, ConfigDelete{Kind: KindView, ID: "btc_prices", Source: msg("t2"), Check: check})
	if !errors.Is(err, ErrInvalid) || !errors.Is(err, refused) {
		t.Fatalf("delete: %v", err)
	}
	if _, err := p.Config(ctx, KindView, "btc_prices"); err != nil {
		t.Errorf("deleted: %v", err)
	}
	// The writer goes on.
	saveView(t, p, msg("t3"), "btc_prices", priceView+"# again\n", 1)
}
