package agent

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
)

// One card for a new private host (TASK-041, SPEC 6.7, 8.8): where the
// host card would ask, the private card asks alone and its answer covers
// the host too.

// hostRecords are the recorded host decisions on target, oldest first.
func (h *harness) hostRecords(target string) []string {
	h.t.Helper()
	var out []string
	for _, a := range h.approvals() {
		if a.Kind == "host" && a.Target == target {
			out = append(out, string(a.Answer)+" by "+string(a.Source))
		}
	}
	slices.Reverse(out) // Approvals lists the newest first
	return out
}

// hostApproved reports whether target is approved as a host.
func (h *harness) hostApproved(target string) bool {
	h.t.Helper()
	var ok bool
	h.open(func(p *project.Project) {
		var err error
		if ok, err = p.DB.Approved(context.Background(), "host", target); err != nil {
			h.t.Fatal(err)
		}
	})
	return ok
}

// askOne fetches url and checks that the one card is the private one,
// naming the new host; it returns the card's wait.
func (h *harness) askOne(c id.Chat, url string) chat.Waiting {
	h.t.Helper()
	h.fetch(c, url)
	w := h.waitingFor(c)
	if w.Kind != chat.PartApproval || w.Text != "Allow requests to 127.0.0.1 on this computer?" {
		h.t.Fatalf("waiting %+v", w)
	}
	return w
}

// At Strict and Standard a new private host gets one card. Allow once
// fetches and approves nothing; the next request asks the one card again.
// Allow in this project adds the exception and approves the host, so the
// next request asks nothing.
func TestNewPrivateHostOneCard(t *testing.T) {
	for _, l := range []project.Level{project.Strict, project.Standard} {
		t.Run(string(l), func(t *testing.T) {
			h, port := siteHarness(t)
			defer h.stop()
			h.setLevel(l)
			url := "http://127.0.0.1:" + port + "/crypto/2026-10-05"
			c := h.newChat(chat.Chat{Title: "Prices"})

			w := h.askOne(c.ID, url)
			h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantOnce})
			h.wait()
			ms := h.messages(c.ID)
			if r := lastResult(ms); !strings.Contains(r, "Bitcoin closed at 62,154.30 USD") {
				t.Fatalf("once: result %q", r)
			}
			cs := cards(ms)
			if len(cs) != 1 || cs[0].Kind != "private" || cs[0].Why != "I'll read the price page for 5 October." ||
				!strings.Contains(cs[0].Details, "New host for this project: Allow in this project also approves it as a host") {
				t.Fatalf("once: cards %+v", cs)
			}
			if h.hostApproved("127.0.0.1") || len(h.hostRecords("127.0.0.1")) != 0 || len(h.privateHosts()) != 0 {
				t.Errorf("once kept the host: %q, %q", h.hostRecords("127.0.0.1"), h.privateHosts())
			}

			w = h.askOne(c.ID, url)
			h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantAlways})
			h.wait()
			if r := lastResult(h.messages(c.ID)); !strings.Contains(r, "Bitcoin closed") {
				t.Fatalf("in this project: result %q", r)
			}
			if !h.hostApproved("127.0.0.1") || !slices.Equal(h.hostRecords("127.0.0.1"), []string{"always by user"}) ||
				!slices.Equal(h.privateHosts(), []string{"127.0.0.1"}) {
				t.Errorf("in this project: %q, %q", h.hostRecords("127.0.0.1"), h.privateHosts())
			}

			h.fetch(c.ID, url)
			h.wait()
			ms = h.messages(c.ID)
			if r := lastResult(ms); !strings.Contains(r, "Bitcoin closed") || len(cards(ms)) != 2 {
				t.Errorf("after: %q, %d cards", r, len(cards(ms)))
			}
		})
	}
}

// Deny denies both: the request is blocked with the note, the host's
// denial is recorded, and the next request asks the one card again.
func TestNewPrivateHostDeny(t *testing.T) {
	h, port := siteHarness(t)
	defer h.stop()
	url := "http://127.0.0.1:" + port + "/crypto/2026-10-05"
	c := h.newChat(chat.Chat{Title: "Prices"})
	w := h.askOne(c.ID, url)
	h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantDeny, Note: "not now"})
	h.wait()
	ms := h.messages(c.ID)
	if r := lastResult(ms); !strings.Contains(r, "the user denied it; their note: not now") {
		t.Errorf("result %q", r)
	}
	if cs := cards(ms); len(cs) != 1 || cs[0].Kind != "private" || cs[0].Answer != chat.GrantDeny {
		t.Errorf("cards %+v", cs)
	}
	if !slices.Equal(h.hostRecords("127.0.0.1"), []string{"deny by user"}) || len(h.privateHosts()) != 0 {
		t.Errorf("recorded %q, exceptions %q", h.hostRecords("127.0.0.1"), h.privateHosts())
	}
	w = h.askOne(c.ID, url)
	h.o.Stop(h.pid, c.ID)
	h.wait()
	if cs := cards(h.messages(c.ID)); len(cs) != 2 || cs[1].Kind != "private" || !cs[1].Stopped {
		t.Errorf("again: cards %+v", cs)
	}
	if got := h.hostRecords("127.0.0.1"); len(got) != 1 {
		t.Errorf("a stopped card recorded %q", got)
	}
}

// A public host asks the host card alone, with the agent's reason, as
// before; denied, nothing is fetched or connected.
func TestNewPublicHostUnchanged(t *testing.T) {
	h, port := siteHarness(t)
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Prices"})
	h.fetch(c.ID, "http://public.test:"+port+"/crypto/2026-10-05")
	w := h.waitingFor(c.ID)
	if w.Text != "Allow requests to public.test for this project?" {
		t.Errorf("waiting %+v", w)
	}
	h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantDeny})
	h.wait()
	ms := h.messages(c.ID)
	if cs := cards(ms); len(cs) != 1 || cs[0].Kind != "host" || cs[0].Why != "I'll read the price page for 5 October." {
		t.Errorf("cards %+v", cs)
	}
	if r := lastResult(ms); !strings.HasPrefix(r, "not run: the user denied it") {
		t.Errorf("result %q", r)
	}
}

// An exception host that isn't approved as a host asks the host card
// alone, as before.
func TestExceptionHostAsksHostCard(t *testing.T) {
	h, port := siteHarness(t)
	defer h.stop()
	h.open(func(p *project.Project) {
		if _, err := p.SetPrivateHosts(context.Background(), []string{"127.0.0.1"}); err != nil {
			t.Fatal(err)
		}
	})
	c := h.newChat(chat.Chat{Title: "Prices"})
	h.fetch(c.ID, "http://127.0.0.1:"+port+"/crypto/2026-10-05")
	w := h.waitingFor(c.ID)
	if w.Text != "Allow requests to 127.0.0.1 for this project?" {
		t.Errorf("waiting %+v", w)
	}
	h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantAlways})
	h.wait()
	ms := h.messages(c.ID)
	if cs := cards(ms); len(cs) != 1 || cs[0].Kind != "host" {
		t.Errorf("cards %+v", cs)
	}
	if r := lastResult(ms); !strings.Contains(r, "Bitcoin closed") {
		t.Errorf("result %q", r)
	}
}

// Auto is unchanged where the level approves the host: its chip, then
// the private card, which still asks. After the turn read a page, the
// level would ask for the next new host: that one gets one card.
func TestNewPrivateHostAuto(t *testing.T) {
	h, port := siteHarness(t)
	defer h.stop()
	h.setLevel(project.Auto)
	c := h.newChat(chat.Chat{Title: "Prices"})
	call := func(id, host string) fake.Reply {
		return fake.ToolCall(id, "fetch_page", map[string]string{"url": "http://" + host + ":" + port + "/crypto/2026-10-05"})
	}
	h.fp.Push(call("c1", "127.0.0.1"), call("c2", "nas.local"), fake.Text("ok"))
	h.send(c.ID, "read both")
	w := h.waitingFor(c.ID)
	if w.Text != "Allow requests to 127.0.0.1 on this computer?" {
		t.Fatalf("first: waiting %+v", w)
	}
	h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantOnce})
	w = h.waitingFor(c.ID)
	if w.Text != "Allow requests to nas.local on this computer?" {
		t.Fatalf("second: waiting %+v", w)
	}
	h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantAlways})
	h.wait()
	var got []string
	for _, a := range cards(h.messages(c.ID)) {
		got = append(got, a.Kind+" "+a.Target+" "+string(a.Answer)+" by "+string(a.By))
	}
	want := []string{"host 127.0.0.1 always by auto", "private 127.0.0.1 once by user", "private nas.local always by user"}
	if !slices.Equal(got, want) {
		t.Errorf("cards %q, want %q", got, want)
	}
	if !h.hostApproved("nas.local") || !slices.Equal(h.privateHosts(), []string{"nas.local"}) {
		t.Errorf("nas.local: %q, %q", h.hostRecords("nas.local"), h.privateHosts())
	}
}

// When the model wrote nothing before the call, the host and private
// cards say so instead of showing no reason.
func TestCardWithoutReason(t *testing.T) {
	h, port := siteHarness(t)
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Prices"})
	for _, host := range []string{"127.0.0.1", "public.test"} {
		h.fp.Push(fake.ToolCall("c1", "fetch_page", map[string]string{"url": "http://" + host + ":" + port + "/crypto/2026-10-05"}), fake.Text("ok"))
		h.send(c.ID, "read it")
		w := h.waitingFor(c.ID)
		h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantDeny})
		h.wait()
	}
	cs := cards(h.messages(c.ID))
	if len(cs) != 2 || cs[0].Kind != "private" || cs[1].Kind != "host" {
		t.Fatalf("cards %+v", cs)
	}
	for _, a := range cs {
		if a.Why != noReason {
			t.Errorf("%s card: why %q", a.Kind, a.Why)
		}
	}
}
