package agent

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
	"github.com/amir-saatchi/jenab/internal/web"
)

// Requests to private network addresses (SPEC 6.7, 8.8). The page server
// is real, on 127.0.0.1, so these tests run outside a synctest bubble.

type lookup map[string]string

func (n lookup) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if a, ok := n[host]; ok {
		return []netip.Addr{netip.MustParseAddr(a)}, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

// privateHarness serves a day's price page on 127.0.0.1, as jenab-fakesite
// does, under the names below; it returns the harness, with fetch_page
// alone, and the server's port. The hosts are approved for the project,
// so the cards are the private-network ones.
func privateHarness(t *testing.T) (*harness, string) {
	t.Helper()
	return siteHarness(t, "127.0.0.1", "nas.local", "prices.example.com", "gate.test")
}

// siteHarness is privateHarness with only the hosts given approved.
// public.test resolves to a public address, which no test connects to.
func siteHarness(t *testing.T, approved ...string) (*harness, string) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/crypto/2026-10-05", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<html><head><title>Crypto prices, 5 October 2026</title></head><body><article>"+
			strings.Repeat("<p>Bitcoin closed at 62,154.30 USD on 5 October 2026, up 1.2% on the day.</p>", 4)+"</article></body></html>")
	})
	mux.HandleFunc("/away", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://nas.local:"+r.Host[strings.LastIndexByte(r.Host, ':')+1:]+"/crypto/2026-10-05", http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	c := web.NewClient(web.Options{Resolver: lookup{"nas.local": "127.0.0.1", "prices.example.com": "127.0.0.1", "gate.test": "127.0.0.1", "public.test": "203.0.113.7"}})
	pages := slices.DeleteFunc(tool.Builtin(tool.Deps{Web: c}), func(tl tool.Tool) bool { return tl.Spec().Name != "fetch_page" })
	h := newHarness(t, t.TempDir(), "", testSettings(), pages...)
	h.open(func(p *project.Project) {
		for _, host := range approved {
			a := store.Approval{ID: id.Approval(id.New()), Kind: "host", Target: host, Answer: chat.GrantAlways, Source: id.SourceUser, CreatedAt: time.Now()}
			if err := p.DB.RecordApproval(context.Background(), a); err != nil {
				t.Fatal(err)
			}
		}
	})
	return h, port
}

// waitingFor polls until the chat waits for a card, in real time.
func (h *harness) waitingFor(c id.Chat) chat.Waiting {
	h.t.Helper()
	for end := time.Now().Add(30 * time.Second); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
		if l := h.o.Live(h.pid, c); l.Waiting != nil {
			return *l.Waiting
		}
	}
	h.t.Fatal("the chat doesn't wait")
	return chat.Waiting{}
}

func (h *harness) privateHosts() []string {
	h.t.Helper()
	var hs []string
	h.open(func(p *project.Project) {
		var err error
		if hs, err = p.PrivateHosts(context.Background()); err != nil {
			h.t.Fatal(err)
		}
	})
	return hs
}

// fetch makes the model fetch url, saying why first, and sends the turn.
func (h *harness) fetch(c id.Chat, url string) {
	h.t.Helper()
	call := fake.ToolCall("c1", "fetch_page", map[string]string{"url": url})
	call.Events = append([]provider.Event{{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartText,
		Text: &chat.Text{Text: "I'll read the price page\nfor 5 October."}}}}, call.Events...)
	h.fp.Push(call, fake.Text("ok"))
	h.send(c, "read "+url)
}

// lastResult is the chat's last tool result.
func lastResult(ms []chat.Message) string {
	got := texts(ms)
	for i := len(got) - 1; i >= 0; i-- {
		if s, ok := strings.CutPrefix(got[i], "tool: result "); ok {
			return s
		}
	}
	return ""
}

// The card asks at every level, Auto too; Deny blocks with the note.
func TestPrivateCardAtEveryLevel(t *testing.T) {
	h, port := privateHarness(t)
	defer h.stop()
	url := "http://127.0.0.1:" + port + "/crypto/2026-10-05"
	for _, l := range []project.Level{project.Strict, project.Standard, project.Auto} {
		h.setLevel(l)
		c := h.newChat(chat.Chat{Title: "Prices " + string(l)})
		h.fetch(c.ID, url)
		w := h.waitingFor(c.ID)
		if w.Kind != chat.PartApproval || w.Text != "Allow requests to 127.0.0.1 on this computer?" {
			t.Errorf("%s: waiting %+v", l, w)
		}
		h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantDeny, Note: "not now"})
		h.wait()
		ms := h.messages(c.ID)
		if r := lastResult(ms); !strings.Contains(r, "127.0.0.1 is blocked: a loopback address (127.0.0.1): the user denied it; their note: not now") {
			t.Errorf("%s: result %q", l, r)
		}
		cs := cards(ms)
		if len(cs) != 1 || cs[0].Kind != "private" || cs[0].Answer != chat.GrantDeny || cs[0].By != id.SourceUser ||
			cs[0].Why != "I'll read the price page for 5 October." {
			t.Errorf("%s: cards %+v", l, cs)
		}
	}
	if hs := h.privateHosts(); len(hs) != 0 {
		t.Errorf("a denial added %q", hs)
	}
}

// Allow once fetches and keeps nothing; Allow in this project adds the
// host, so the next request doesn't ask; removed in the settings, it asks
// again. The answers are recorded, but the exceptions decide.
func TestPrivateOnceAndInThisProject(t *testing.T) {
	h, port := privateHarness(t)
	defer h.stop()
	h.setLevel(project.Auto)
	url := "http://127.0.0.1:" + port + "/crypto/2026-10-05"
	c := h.newChat(chat.Chat{Title: "Prices"})

	h.fetch(c.ID, url)
	w := h.waitingFor(c.ID)
	h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantOnce})
	h.wait()
	if r := lastResult(h.messages(c.ID)); !strings.Contains(r, "Bitcoin closed at 62,154.30 USD") {
		t.Fatalf("once: result %q", r)
	}
	if hs := h.privateHosts(); len(hs) != 0 {
		t.Errorf("once kept %q", hs)
	}

	h.fetch(c.ID, url)
	w = h.waitingFor(c.ID)
	h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantAlways})
	h.wait()
	if r := lastResult(h.messages(c.ID)); !strings.Contains(r, "Bitcoin closed") {
		t.Fatalf("in this project: result %q", r)
	}
	if hs := h.privateHosts(); !slices.Equal(hs, []string{"127.0.0.1"}) {
		t.Errorf("exceptions %q", hs)
	}

	h.fetch(c.ID, url)
	h.wait()
	ms := h.messages(c.ID)
	if r := lastResult(ms); !strings.Contains(r, "Bitcoin closed") || len(cards(ms)) != 2 {
		t.Errorf("with the exception: %q, %d cards", r, len(cards(ms)))
	}

	// Removed in the settings, the next request asks again, though the
	// record says Always.
	h.open(func(p *project.Project) {
		if _, err := p.SetPrivateHosts(context.Background(), nil); err != nil {
			t.Fatal(err)
		}
	})
	h.fetch(c.ID, url)
	w = h.waitingFor(c.ID)
	h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantDeny})
	h.wait()
	var kinds []string
	for _, a := range h.approvals() {
		if a.Kind == "private" {
			kinds = append(kinds, string(a.Answer))
		}
	}
	if !slices.Equal(kinds, []string{"deny", "always", "once"}) {
		t.Errorf("recorded %q", kinds)
	}
}

// A public name that resolves to a private address says so; a redirect to
// a private address names both hosts.
func TestPrivateNameAndRedirect(t *testing.T) {
	h, port := privateHarness(t)
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Prices"})

	h.fetch(c.ID, "http://prices.example.com:"+port+"/crypto/2026-10-05")
	w := h.waitingFor(c.ID)
	h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantOnce})
	h.wait()
	a := cards(h.messages(c.ID))[0]
	if a.Ask != "Allow requests to prices.example.com on this computer?" ||
		!strings.HasPrefix(a.Risk, "prices.example.com is a public name, but it points to this computer.") ||
		!strings.Contains(a.Details, "prices.example.com resolves to 127.0.0.1: this computer") {
		t.Errorf("name card %+v", a)
	}

	// gate.test is an exception; it redirects to nas.local, which isn't.
	h.open(func(p *project.Project) {
		if _, err := p.SetPrivateHosts(context.Background(), []string{"gate.test"}); err != nil {
			t.Fatal(err)
		}
	})
	h.fetch(c.ID, "http://gate.test:"+port+"/away")
	w = h.waitingFor(c.ID)
	if w.Text != "Allow the redirect from gate.test to nas.local on this computer?" {
		t.Errorf("redirect: waiting %+v", w)
	}
	h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantOnce})
	h.wait()
	ms := h.messages(c.ID)
	if r := lastResult(ms); !strings.Contains(r, "Bitcoin closed") {
		t.Errorf("redirect: result %q", r)
	}
	if a := cards(ms)[1]; !strings.Contains(a.Details, "Redirected from: gate.test") || a.Risk == "" ||
		strings.Contains(a.Risk, "public name") {
		t.Errorf("redirect card %+v", a)
	}
}

// Stop ends the wait: the card is closed as stopped and nothing is
// fetched or kept.
func TestPrivateCardStop(t *testing.T) {
	h, port := privateHarness(t)
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Prices"})
	h.fetch(c.ID, "http://127.0.0.1:"+port+"/crypto/2026-10-05")
	h.waitingFor(c.ID)
	h.o.Stop(h.pid, c.ID)
	h.wait()
	ms := h.messages(c.ID)
	if cs := cards(ms); len(cs) != 1 || !cs[0].Stopped {
		t.Errorf("cards %+v", cs)
	}
	if r := lastResult(ms); r != "cancelled by user" {
		t.Errorf("result %q", r)
	}
	if l := h.o.Live(h.pid, c.ID); l.Waiting != nil {
		t.Errorf("still waiting: %+v", l.Waiting)
	}
}
