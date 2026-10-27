package app

import (
	"context"
	"slices"
	"testing"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
)

func TestServices(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	ps, cs := e.svc.Project, e.svc.Chat

	if _, err := ps.Create(ctx, "  "); err == nil {
		t.Fatal("a project without a name")
	} else {
		uiErr(t, err, KindInvalid)
	}
	op, err := ps.Create(ctx, "Coins")
	if err != nil {
		t.Fatal(err)
	}
	if op.Name != "Coins" || op.Mother == "" || op.Level != "standard" || op.Damage != nil {
		t.Fatalf("created %+v", op)
	}
	list, err := ps.List(ctx)
	if err != nil || len(list) != 1 || list[0].ID != op.ID || list[0].LastOpened == nil {
		t.Fatalf("List = %+v, %v", list, err)
	}
	if again, err := ps.Open(ctx, op.ID); err != nil || again != op {
		t.Fatalf("Open = %+v, %v", again, err)
	}
	if _, err := ps.Open(ctx, id.Project(id.New())); err == nil {
		t.Fatal("opened a project that doesn't exist")
	} else {
		uiErr(t, err, KindNotFound)
	}

	// A message to the Mother chat runs a turn; the snapshot has it.
	e.fp.Push(fake.Text("Hello."))
	if _, err := cs.Send(ctx, op.ID, op.Mother, "  "); err == nil {
		t.Fatal("sent an empty message")
	} else {
		uiErr(t, err, KindInvalid)
	}
	if _, err := cs.Send(ctx, op.ID, op.Mother, "Hi"); err != nil {
		t.Fatal(err)
	}
	e.idle(op.Mother)
	snap, err := cs.Snapshot(ctx, op.ID, op.Mother)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Chat.Kind != chat.KindMother || snap.From != 1 || len(snap.Messages) != 2 || snap.Seq == 0 || snap.Live.Running {
		t.Fatalf("snapshot %+v", snap)
	}
	if got := snap.Messages[1].Parts[0].Text.Text; got != "Hello." {
		t.Errorf("answer %q", got)
	}
	if !slices.Contains(e.sink.names(), EventPart) || !slices.Contains(e.sink.names(), EventStatus) {
		t.Errorf("events %v", e.sink.names())
	}
	if ms, err := cs.Messages(ctx, op.ID, op.Mother, 1, 1); err != nil || len(ms) != 2 {
		t.Errorf("Messages = %d, %v", len(ms), err)
	}
	if ms, err := cs.Messages(ctx, op.ID, op.Mother, 0, 0); err != nil || len(ms) != 2 {
		t.Errorf("Messages from 0 = %d, %v", len(ms), err)
	}

	// A chat of the user's, renamed, with a role and a model.
	c, err := cs.Create(ctx, op.ID, "", "You track prices.")
	if err != nil {
		t.Fatal(err)
	}
	if title, err := cs.Rename(ctx, op.ID, c.ID, "  Prices "); err != nil || title != "Prices" {
		t.Fatalf("Rename = %q, %v", title, err)
	}
	if err := cs.SetRole(ctx, op.ID, op.Mother, "x"); err == nil {
		t.Fatal("Mother got a role")
	} else {
		uiErr(t, err, KindInvalid)
	}
	if err := cs.SetModel(ctx, op.ID, c.ID, "fast"); err != nil {
		t.Fatal(err)
	}
	items, err := cs.List(ctx, op.ID)
	if err != nil || len(items) != 2 || items[0].Chat.Kind != chat.KindMother || items[0].LastActivity == nil ||
		items[1].Chat.Title != "Prices" || !items[1].Chat.TitleFixed || items[1].Chat.Model != "fast" || items[1].LastActivity != nil {
		t.Fatalf("List = %+v, %v", items, err)
	}
	if err := cs.Archive(ctx, op.ID, c.ID, true); err != nil {
		t.Fatal(err)
	}

	// Errors the frontend can act on.
	if err := cs.SetLevel(ctx, op.ID, c.ID, "loose"); err == nil {
		t.Fatal("a bad level")
	} else {
		uiErr(t, err, KindInvalid)
	}
	if err := cs.SetLevel(ctx, op.ID, c.ID, "strict"); err != nil {
		t.Fatal(err)
	}
	if op2, _ := ps.Open(ctx, op.ID); op2.Level != "strict" {
		t.Errorf("level %s", op2.Level)
	}
	if err := cs.Answer(ctx, op.ID, c.ID, agent.Answer{Message: "m", Grant: chat.GrantOnce}); err == nil {
		t.Fatal("an answer with nothing waiting")
	} else {
		uiErr(t, err, KindBusy)
	}
	if err := cs.Retry(ctx, op.ID, c.ID); err == nil {
		t.Fatal("a retry with nothing to retry")
	} else {
		uiErr(t, err, KindBusy)
	}
	if _, err := cs.Snapshot(ctx, op.ID, id.Chat(id.New())); err == nil {
		t.Fatal("a snapshot of no chat")
	} else {
		uiErr(t, err, KindNotFound)
	}
	if err := cs.Stop(ctx, op.ID, c.ID); err != nil {
		t.Fatal(err)
	}

	r, err := cs.Search(ctx, op.ID, SearchRequest{Query: "hello"})
	if err != nil || len(r.Hits) != 1 || r.Hits[0].Chat != op.Mother {
		t.Fatalf("Search = %+v, %v", r, err)
	}
	if err := cs.Clear(ctx, op.ID, op.Mother); err != nil {
		t.Fatal(err)
	}
	if snap, err := cs.Snapshot(ctx, op.ID, op.Mother); err != nil || len(snap.Messages) != 0 || snap.Messages == nil {
		t.Fatalf("after Clear: %+v, %v", snap, err)
	}
	if err := cs.Delete(ctx, op.ID, c.ID); err != nil {
		t.Fatal(err)
	}

	if a, err := ps.Activity(ctx, op.ID); err != nil || a.Project != op.ID || !a.Open {
		t.Errorf("Activity = %+v, %v", a, err)
	}
	if _, err := ps.FolderWarning(ctx); err != nil {
		t.Error(err)
	}
}
