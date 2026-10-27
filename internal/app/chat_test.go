package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
)

// A snapshot holds the last SnapshotTurns turns; older ones come with
// Messages.
func TestSnapshotWindow(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	op, err := e.svc.Project.Create(ctx, "Long")
	if err != nil {
		t.Fatal(err)
	}
	for i := range SnapshotTurns + 1 {
		e.fp.Push(fake.Text("ok"))
		if _, err := e.svc.Chat.Send(ctx, op.ID, op.Mother, fmt.Sprint("turn ", i+1)); err != nil {
			t.Fatal(err)
		}
		e.idle(op.Mother)
	}
	snap, err := e.svc.Chat.Snapshot(ctx, op.ID, op.Mother)
	if err != nil {
		t.Fatal(err)
	}
	if snap.From != 2 || len(snap.Messages) != 2*SnapshotTurns || snap.Messages[0].Parts[0].Text.Text != "turn 2" {
		t.Fatalf("From %d, %d messages", snap.From, len(snap.Messages))
	}
	ms, err := e.svc.Chat.Messages(ctx, op.ID, op.Mother, 1, 1)
	if err != nil || len(ms) != 2 || ms[0].Parts[0].Text.Text != "turn 1" {
		t.Fatalf("older turn: %+v, %v", ms, err)
	}
}

// A rename during a turn moves the turn's later events past the
// snapshot's sequence number, so the frontend keeps them (Q32).
func TestWriteDuringTurn(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	op, err := e.svc.Project.Create(ctx, "Live")
	if err != nil {
		t.Fatal(err)
	}
	delta := func(text string) provider.Event {
		return provider.Event{Kind: provider.EventDelta, PartKind: chat.PartText, Text: text}
	}
	e.fp.Push(fake.Reply{Wait: 400 * time.Millisecond, Events: []provider.Event{delta("a"), delta("b")}, Hang: true})
	if _, err := e.svc.Chat.Send(ctx, op.ID, op.Mother, "Go"); err != nil {
		t.Fatal(err)
	}
	deltas := func() []chat.Delta {
		e.sink.mu.Lock()
		defer e.sink.mu.Unlock()
		var ds []chat.Delta
		for _, ev := range e.sink.events {
			if d, ok := ev.data.(chat.Delta); ok {
				ds = append(ds, d)
			}
		}
		return ds
	}
	wait := func(n int) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for len(deltas()) < n {
			if time.Now().After(deadline) {
				t.Fatalf("%d deltas; want %d", len(deltas()), n)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	wait(1)
	if _, err := e.svc.Chat.Rename(ctx, op.ID, op.Mother, "Renamed"); err != nil {
		t.Fatal(err)
	}
	snap, err := e.svc.Chat.Snapshot(ctx, op.ID, op.Mother)
	if err != nil {
		t.Fatal(err)
	}
	ps, err := e.svc.Settings.Providers(ctx)
	if err != nil || len(ps) != 1 || ps[0].Running != 1 {
		t.Errorf("Providers during the turn = %+v, %v", ps, err)
	}
	wait(2)
	if d := deltas()[1]; d.Seq < snap.Seq {
		t.Errorf("delta after the rename has seq %d; the snapshot has %d", d.Seq, snap.Seq)
	}
}

// A chat that asks the user is in Waiting and shows as waiting in List.
func TestWaiting(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	cs := e.svc.Chat
	if ws, err := cs.Waiting(ctx); err != nil || ws == nil || len(ws) != 0 {
		t.Fatalf("Waiting = %+v, %v", ws, err)
	}
	op, err := e.svc.Project.Create(ctx, "Coins")
	if err != nil {
		t.Fatal(err)
	}
	form := map[string]any{"questions": []map[string]any{
		{"header": "Currency", "question": "Which currency?", "options": []map[string]any{{"label": "EUR"}, {"label": "USD"}}},
	}}
	e.fp.Push(fake.ToolCall("c1", "ask_user", form), fake.Text("ok"))
	if _, err := cs.Send(ctx, op.ID, op.Mother, "set up prices"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for e.orch.State(op.ID, op.Mother) != chat.StateWaiting {
		if time.Now().After(deadline) {
			t.Fatal("the chat never waited")
		}
		time.Sleep(5 * time.Millisecond)
	}
	ws, err := cs.Waiting(ctx)
	if err != nil || len(ws) != 1 || ws[0].Project != op.ID || ws[0].Chat != op.Mother || ws[0].Title == "" ||
		ws[0].Waiting.Kind != chat.PartQuestion || ws[0].Waiting.Text != "Which currency?" {
		t.Fatalf("Waiting = %+v, %v", ws, err)
	}
	items, err := cs.List(ctx, op.ID)
	if err != nil || items[0].State != chat.StateWaiting {
		t.Fatalf("List = %+v, %v", items, err)
	}
	if err := cs.Stop(ctx, op.ID, op.Mother); err != nil {
		t.Fatal(err)
	}
	e.idle(op.Mother)
	if items, err := cs.List(ctx, op.ID); err != nil || items[0].State != chat.StateIdle {
		t.Fatalf("List after Stop = %+v, %v", items, err)
	}
}
