package app

import (
	"context"
	"testing"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/store"
)

// configEvents takes the project:configs events the sink got so far.
func (s *sink) configEvents() []project.ConfigsChanged {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []project.ConfigsChanged
	kept := s.events[:0]
	for _, e := range s.events {
		if c, ok := e.data.(project.ConfigsChanged); ok && e.name == EventConfigs {
			out = append(out, c)
			continue
		}
		kept = append(kept, e)
	}
	s.events = kept
	return out
}

// The service lists and reads views and pages; saves, deletes, Undo turn
// and rewind's Undo changes send project:configs with their IDs.
func TestConfigService(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	cfg := e.svc.Config
	op, err := e.svc.Project.Create(ctx, "Coins")
	if err != nil {
		t.Fatal(err)
	}
	if l, err := cfg.List(ctx, op.ID, ""); err != nil || l == nil || len(l) != 0 {
		t.Fatalf("empty list %#v, %v", l, err)
	}
	_, err = cfg.List(ctx, op.ID, "pipeline")
	uiErr(t, err, KindInvalid)

	// Two turns, each saving a config as the tools will (P3-03).
	turnSource := func(turn int) id.Source {
		t.Helper()
		snap, err := e.svc.Chat.Snapshot(ctx, op.ID, op.Mother)
		if err != nil {
			t.Fatal(err)
		}
		for i := len(snap.Messages) - 1; i >= 0; i-- {
			if m := snap.Messages[i]; m.Turn == turn && m.Role == chat.RoleAssistant {
				return id.SourceOf("message", string(m.ID))
			}
		}
		t.Fatalf("no answer in turn %d", turn)
		return ""
	}
	save := func(s store.ConfigSave) {
		t.Helper()
		if err := open(ctx, e.pm, op.ID, func(p *project.Project) error {
			_, err := p.DB.SaveConfig(ctx, s)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	e.fp.Push(fake.Text("Built the view."))
	if _, err := e.svc.Chat.Send(ctx, op.ID, op.Mother, "Show prices", nil); err != nil {
		t.Fatal(err)
	}
	e.idle(op.Mother)
	viewYAML := "version: 1\nid: btc_prices\ntitle: BTC price\ntype: table\nquery: SELECT 1\n"
	save(store.ConfigSave{Kind: store.KindView, ID: "btc_prices", Type: "table", Title: "BTC price", YAML: viewYAML, Source: turnSource(1)})
	e.fp.Push(fake.Text("Built the page."))
	if _, err := e.svc.Chat.Send(ctx, op.ID, op.Mother, "Make a page", nil); err != nil {
		t.Fatal(err)
	}
	e.idle(op.Mother)
	pageYAML := "version: 1\nid: bitcoin\ntitle: Bitcoin\nrows:\n  - [ { view: btc_prices } ]\n"
	save(store.ConfigSave{Kind: store.KindPage, ID: "bitcoin", Title: "Bitcoin", YAML: pageYAML, Source: turnSource(2)})

	if ev := e.sink.configEvents(); len(ev) != 2 || ev[0].Project != op.ID ||
		ev[0].Changes[0] != (store.ConfigChange{Kind: store.KindView, ID: "btc_prices", Op: "save", Revision: 1}) ||
		ev[1].Changes[0].Kind != store.KindPage {
		t.Fatalf("save events %+v", ev)
	}
	l, err := cfg.List(ctx, op.ID, "")
	if err != nil || len(l) != 2 || l[0].Kind != store.KindView || l[0].Title != "BTC price" || l[1].ID != "bitcoin" || l[1].Type != "page" {
		t.Fatalf("list %+v, %v", l, err)
	}
	if l, err := cfg.List(ctx, op.ID, store.KindPage); err != nil || len(l) != 1 || l[0].ID != "bitcoin" {
		t.Fatalf("pages %+v, %v", l, err)
	}
	if c, err := cfg.Read(ctx, op.ID, "", "bitcoin"); err != nil || c.YAML != pageYAML || c.Kind != store.KindPage {
		t.Fatalf("read %+v, %v", c, err)
	}
	_, err = cfg.Read(ctx, op.ID, store.KindView, "nothing")
	uiErr(t, err, KindNotFound)

	// Undo turn 2: the page is gone, and the event says so.
	if r, err := e.svc.Chat.UndoTurn(ctx, op.ID, op.Mother, agent.UndoRequest{Turn: 2}); err != nil || !r.Done {
		t.Fatalf("undo turn %+v, %v", r, err)
	}
	if ev := e.sink.configEvents(); len(ev) != 1 || ev[0].Changes[0] != (store.ConfigChange{Kind: store.KindPage, ID: "bitcoin", Op: "delete", Revision: 2}) {
		t.Fatalf("undo events %+v", ev)
	}
	_, err = cfg.Read(ctx, op.ID, store.KindPage, "bitcoin")
	uiErr(t, err, KindNotFound)
	if r, err := e.svc.Chat.UndoTurn(ctx, op.ID, op.Mother, agent.UndoRequest{Turn: 2, Redo: true}); err != nil || !r.Done {
		t.Fatalf("redo turn %+v, %v", r, err)
	}
	if ev := e.sink.configEvents(); len(ev) != 1 || ev[0].Changes[0].Op != "save" {
		t.Fatalf("redo events %+v", ev)
	}

	// Rewind to turn 1, then undo the changes of the turn that left.
	r, err := e.svc.Chat.Rewind(ctx, op.ID, op.Mother, 1)
	if err != nil || r.Rewind == 0 {
		t.Fatalf("rewind %+v, %v", r, err)
	}
	if ev := e.sink.configEvents(); len(ev) != 0 {
		t.Fatalf("the rewind itself sent %+v", ev)
	}
	if u, err := e.svc.Chat.UndoTurn(ctx, op.ID, op.Mother, agent.UndoRequest{Rewind: r.Rewind}); err != nil || !u.Done {
		t.Fatalf("undo the rewound changes %+v, %v", u, err)
	}
	if ev := e.sink.configEvents(); len(ev) != 1 || ev[0].Changes[0].ID != "bitcoin" || ev[0].Changes[0].Op != "delete" {
		t.Fatalf("rewind undo events %+v", ev)
	}
	if l, err := cfg.List(ctx, op.ID, ""); err != nil || len(l) != 1 || l[0].ID != "btc_prices" {
		t.Fatalf("list after the rewind's undo %+v, %v", l, err)
	}
}
