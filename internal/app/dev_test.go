package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
)

// The inspector shows the blocks the provider got, and neither screen
// shows a key: not one the user pasted into the chat, a provider's error
// or the model's answer (P1-17).
func TestDevScreensShowWhatWasSentWithoutSecrets(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	op, err := e.svc.Project.Create(ctx, "Keys")
	if err != nil {
		t.Fatal(err)
	}
	// Turn 1 fails with the key in the provider's error.
	e.fp.Push(fake.Fail(&provider.Error{Provider: "p", Kind: provider.BadRequest, Message: "bad key " + testKey}))
	if _, err := e.svc.Chat.Send(ctx, op.ID, op.Mother, "my key is "+testKey); err != nil {
		t.Fatal(err)
	}
	e.idle(op.Mother)
	// Turn 2 hangs while it streams the key back.
	e.fp.Push(fake.Reply{Events: []provider.Event{{Kind: provider.EventDelta, PartKind: chat.PartText, Text: "it is " + testKey}}, Hang: true})
	if _, err := e.svc.Chat.Send(ctx, op.ID, op.Mother, "again"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for e.orch.Live(op.ID, op.Mother).Text == "" {
		if time.Now().After(deadline) {
			t.Fatal("the turn never streamed")
		}
		time.Sleep(5 * time.Millisecond)
	}

	var screens []any
	turns, err := e.svc.Dev.Turns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || turns[0].Turn != 2 || !turns[0].Running || turns[1].Running || turns[0].ProjectName != "Keys" || turns[0].ChatTitle == "" {
		t.Fatalf("Turns = %+v", turns)
	}
	screens = append(screens, turns)
	calls := e.fp.Calls()
	n := 0
	for _, tn := range []int{1, 2} {
		v, err := e.svc.Dev.Turn(ctx, op.ID, op.Mother, tn)
		if err != nil {
			t.Fatal(err)
		}
		screens = append(screens, v)
		for i, r := range v.Requests {
			sent := calls[n]
			n++
			if len(r.Blocks) != len(agent.Blocks(sent, tn)) {
				t.Fatalf("turn %d request %d: %d blocks, want %d", tn, i, len(r.Blocks), len(agent.Blocks(sent, tn)))
			}
			for j := range r.Blocks {
				got, err := e.svc.Dev.Block(ctx, op.ID, op.Mother, tn, i, j)
				if err != nil {
					t.Fatal(err)
				}
				want, _ := BlockText(sent, tn, j)
				if got != e.secrets.Redact(want) {
					t.Errorf("turn %d request %d block %q:\n%s\nwant\n%s", tn, i, r.Blocks[j].Name, got, want)
				}
				screens = append(screens, got)
			}
		}
		if tn == 1 && (len(v.Requests) != 1 || v.Requests[0].Err == "") {
			t.Errorf("turn 1 requests %+v", v.Requests)
		}
	}
	if n != len(calls) {
		t.Errorf("%d requests shown, the provider got %d", n, len(calls))
	}
	first, _ := e.svc.Dev.Turn(ctx, op.ID, op.Mother, 1)
	bs := first.Requests[0].Blocks
	last, _ := e.svc.Dev.Block(ctx, op.ID, op.Mother, 1, 0, len(bs)-1)
	if bs[len(bs)-1].Name != "This turn" {
		t.Errorf("blocks %+v", bs)
	}
	if !strings.Contains(last, "my key is [secret:") {
		t.Errorf("this turn's block %q, want the message with the key redacted", last)
	}

	rt, err := e.svc.Dev.Runtime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	screens = append(screens, rt)
	if len(rt.Projects) != 1 || rt.Projects[0].Name != "Keys" || len(rt.Projects[0].Writers) != 2 || rt.Stuck != 0 {
		t.Fatalf("Runtime = %+v", rt)
	}
	if w := rt.Projects[0].Work; len(w) != 1 || w[0].Kind != "turn" || w[0].LimitMS != provider.FirstEvent.Milliseconds() || w[0].Stuck {
		t.Errorf("work %+v", w)
	}
	if rt.Calls.Size != 8 || rt.Calls.InUse != 1 || len(rt.Providers) != 1 || rt.Providers[0].LastProblem == "" {
		t.Errorf("calls %+v, providers %+v", rt.Calls, rt.Providers)
	}

	b, err := json.Marshal(screens)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), testKey) {
		t.Errorf("a screen shows the key:\n%s", b)
	}
	if err := e.svc.Chat.Stop(ctx, op.ID, op.Mother); err != nil {
		t.Fatal(err)
	}
	e.idle(op.Mother)
}

func TestDevUnknownTurn(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	_, err := e.svc.Dev.Turn(ctx, "p1", "c1", 1)
	uiErr(t, err, KindNotFound)
	_, err = e.svc.Dev.Block(ctx, "p1", "c1", 1, 0, 0)
	uiErr(t, err, KindNotFound)
}

// A negative block is not found, not a panic.
func TestBlockTextBounds(t *testing.T) {
	req := provider.Request{System: []provider.Block{{Text: "s"}}}
	for _, r := range []provider.Request{req, {System: req.System, Tools: []provider.ToolDef{{Name: "t"}}}} {
		for _, i := range []int{-1, -2, 99} {
			if _, ok := BlockText(r, 1, i); ok {
				t.Errorf("block %d with %d tools found", i, len(r.Tools))
			}
		}
	}
}

func TestCacheHits(t *testing.T) {
	bs := []agent.TraceBlock{{Tokens: 100}, {Tokens: 100}, {Tokens: 200}}
	// The provider counted twice the estimate: 800 tokens, 300 from cache.
	got := cacheHits(bs, chat.Usage{Input: 500, CacheRead: 300})
	if want := []string{"hit", "part", "miss"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("cacheHits = %v, want %v", got, want)
	}
	if got := cacheHits(bs, chat.Usage{}); got[0] != "" {
		t.Errorf("no usage: %v", got)
	}
}
