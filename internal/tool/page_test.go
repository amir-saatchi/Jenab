package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"golang.org/x/net/idna"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/web"
)

type resolver map[string]string

func (r resolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if a, ok := r[host]; ok {
		return []netip.Addr{netip.MustParseAddr(a)}, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

// pageServer serves sample pages as site.test; the env allows site.test
// to reach it on 127.0.0.1.
func pageServer(t *testing.T, env *Env) (Deps, string) {
	t.Helper()
	body := strings.Repeat("<p>The Laptop X has a bright screen and a quiet keyboard, and the battery lasts a full day.</p>\n", 6)
	mux := http.NewServeMux()
	mux.HandleFunc("/review", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<html><head><title>Laptop X "review" | News</title><meta property="og:title" content="Laptop X review"></head>
<body><nav>Home · News · About us</nav><article><h1>Laptop X review</h1>%s</article><footer>Cookie settings</footer></body></html>`, body)
	})
	mux.HandleFunc("/fa", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<html><head><meta charset="utf-8"><title>قیمت بیت%sکوین</title></head><body><article>%s</article></body></html>`,
			zwnj, strings.Repeat("<p>قیمت بیت"+zwnj+"کوین امروز بالا رفت و بازار آرام بود.</p>", 10))
	})
	mux.HandleFunc("/app", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>App</title></head><body><div id="root"></div><script src="a.js"></script></body></html>`)
	})
	mux.HandleFunc("/moved", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/review", http.StatusFound) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	// /away redirects to the same server as other.test, a second host.
	mux.HandleFunc("/away", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://other.test:"+port+"/review", http.StatusFound)
	})
	env.PrivateHosts = func(h string) bool { return h == "site.test" || h == "other.test" }
	c := web.NewClient(web.Options{Resolver: resolver{"site.test": "127.0.0.1", "other.test": "127.0.0.1", "intranet.test": "192.168.0.10"}})
	return Deps{Web: c}, "http://site.test:" + port
}

func TestFetchPage(t *testing.T) {
	env := testEnv(t)
	env.PreviewTokens = 40
	d, base := pageServer(t, env)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(base, "http://"))

	r, err := callWith(t, d, env, "fetch_page", `{"url": "`+base+`/moved"}`)
	if err != nil {
		t.Fatal(err)
	}
	key := r.Ref
	if !strings.HasPrefix(key, "cache/pages/site.test_"+port+"/") || !strings.HasSuffix(key, ".txt") {
		t.Fatalf("ref %q", key)
	}
	head, _, _ := strings.Cut(r.Text, "\n")
	if want := `[page "Laptop X review" — site.test_` + port + ` — `; !strings.HasPrefix(head, want) || !strings.Contains(head, "showing first") {
		t.Errorf("header %s", head)
	}
	contains(t, r.Text, "bright screen", "[use read_ref(ref, offset) or search_ref(ref, query) for more; the next offset is ")
	if strings.Contains(r.Text, "Cookie settings") || strings.Contains(r.Text, "About us") {
		t.Errorf("boilerplate:\n%s", r.Text)
	}
	if Stub(chat.ToolResult{Text: r.Text, Ref: key}) != `[page "Laptop X review" — site.test_`+port+` — ref: `+key+`]` {
		t.Errorf("stub %s", Stub(chat.ToolResult{Text: r.Text, Ref: key}))
	}

	o, rc, err := env.Project.DB.OpenObject(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := io.ReadAll(rc)
	rc.Close()
	var meta map[string]string
	json.Unmarshal(o.Metadata, &meta)
	if o.Source != "http" || o.SourceURL != base+"/review" || meta["title"] != "Laptop X review" || meta["url"] != base+"/review" || o.Expires.IsZero() {
		t.Errorf("object %+v", o)
	}
	if strings.Count(string(stored), "bright screen") != 6 {
		t.Errorf("stored text:\n%s", stored)
	}
	// The same page again is the same key, and read_ref reads it on.
	r2, err := callWith(t, d, env, "fetch_page", `{"url": "`+base+`/review"}`)
	if err != nil || r2.Ref != key {
		t.Errorf("again: %q, %v", r2.Ref, err)
	}
	contains(t, mustText(t, env, "search_ref", `{"ref": "`+key+`", "query": "battery"}`), "6 lines of "+key+" match")

	// The ZWNJ stays in the title.
	r, err = callWith(t, d, env, "fetch_page", `{"url": "`+base+`/fa"}`)
	if err != nil {
		t.Fatal(err)
	}
	contains(t, r.Text, `[page "قیمت بیت`+zwnj+`کوین" — site.test_`+port)

	r, err = callWith(t, d, env, "fetch_page", `{"url": "`+base+`/app"}`)
	if err != nil || r.Ref != "" || !strings.HasPrefix(r.Text, `[page "App" — site.test_`+port+` — needs_javascript: `) {
		t.Errorf("app: %+v, %v", r, err)
	}
}

func TestFetchPageErrors(t *testing.T) {
	env := testEnv(t)
	d, base := pageServer(t, env)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(base, "http://"))
	for _, tc := range []struct{ url, want string }{
		{"http://intranet.test:" + port + "/", "is blocked: a private address (192.168.0.10)"},
		{"http://127.0.0.1:" + port + "/review", "is blocked: a loopback address"},
		{"file:///etc/passwd", "only http and https URLs are allowed"},
		{base + "/missing", "returned 404 Not Found"},
		{"http://nowhere.test/", "no such host"},
	} {
		_, err := callWith(t, d, env, "fetch_page", `{"url": "`+tc.url+`"}`)
		var te *Error
		if !errors.As(err, &te) || !strings.Contains(te.Msg, tc.want) || !strings.HasPrefix(te.Msg, "could not fetch "+tc.url+": ") {
			t.Errorf("%s: got %v, want %q", tc.url, err, tc.want)
		}
	}
	// A cancelled turn is not the model's mistake.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tl := newFetchPage(d.Web)
	_, err := Run(ctx, tl, Call{Args: json.RawMessage(`{"url": "` + base + `/review"}`), Env: env})
	var te *Error
	if !errors.Is(err, context.Canceled) || errors.As(err, &te) {
		t.Errorf("cancelled: %v", err)
	}
}

// A redirect to another host needs that host's approval too (8.8).
func TestFetchPageRedirectHost(t *testing.T) {
	env := testEnv(t)
	d, base := pageServer(t, env)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(base, "http://"))
	ctx := context.Background()
	target := "http://other.test:" + port + "/review"
	_, err := callWith(t, d, env, "fetch_page", `{"url": "`+base+`/away"}`)
	var te *Error
	if !errors.As(err, &te) || !strings.Contains(te.Msg, "redirects to "+target+", on a host not approved") ||
		!strings.Contains(te.Msg, "call fetch_page with that URL") {
		t.Fatalf("got %v", err)
	}
	l, err := env.Project.DB.ListObjects(ctx, "cache/pages/", "", 0)
	if err != nil || len(l.Objects)+len(l.Folders) != 0 {
		t.Errorf("stored %+v, %v", l, err)
	}
	// A redirect within the approved host still works.
	if r, err := callWith(t, d, env, "fetch_page", `{"url": "`+base+`/moved"}`); err != nil || !strings.HasPrefix(r.Ref, "cache/pages/site.test_") {
		t.Errorf("same host: %+v, %v", r, err)
	}
	// Once other.test is approved, the redirect is followed.
	a := store.Approval{ID: id.Approval(id.New()), Kind: "host", Target: "other.test", Answer: chat.GrantAlways, Source: id.SourceUser}
	if err := env.Project.DB.RecordApproval(ctx, a); err != nil {
		t.Fatal(err)
	}
	r, err := callWith(t, d, env, "fetch_page", `{"url": "`+base+`/away"}`)
	if err != nil || !strings.HasPrefix(r.Ref, "cache/pages/other.test_"+port+"/") {
		t.Errorf("approved: %+v, %v", r, err)
	}
}

func TestFetchPagePreflight(t *testing.T) {
	env := testEnv(t)
	pf := newFetchPage(web.NewClient(web.Options{})).(Preflighter)
	n, err := pf.Preflight(context.Background(), Call{Args: json.RawMessage(`{"url": "https://News.Example.com/a?b=c"}`), Env: env})
	if err != nil {
		t.Fatal(err)
	}
	if n.Effects != Network|Bucket|Untrusted || len(n.Approvals) != 1 {
		t.Fatalf("needs %+v", n)
	}
	a := n.Approvals[0]
	if a.Kind != "host" || !strings.Contains(a.Ask, "news.example.com") || a.ID == "" {
		t.Errorf("approval %+v", a)
	}
	if err := (chat.Part{Kind: chat.PartApproval, Approval: &a}).Validate(); err != nil {
		t.Errorf("approval part: %v", err)
	}
	// Spellings of one host ask for it under one name.
	for _, raw := range []string{"http://[0:0::1]/", "http://[::1]:8080/"} {
		n, err := pf.Preflight(context.Background(), Call{Args: json.RawMessage(`{"url": "` + raw + `"}`), Env: env})
		if err != nil || n.Approvals[0].Target != "::1" {
			t.Errorf("%s: %+v, %v", raw, n.Approvals, err)
		}
	}
	for _, args := range []string{`{"url": "ftp://x.org/"}`, `{"url": 5}`, `{}`, `{"url": "http://16843009/"}`, `{"url": "http://0x01010101/"}`, `{"url": "http://1.1/"}`} {
		_, err := pf.Preflight(context.Background(), Call{Args: json.RawMessage(args), Env: env})
		var te *Error
		if !errors.As(err, &te) {
			t.Errorf("%s: %v", args, err)
		}
	}
}

func TestHostKey(t *testing.T) {
	for raw, want := range map[string]string{
		"https://Example.COM/a":       "example.com",
		"http://example.com:8080/":    "example.com_8080",
		"https://bücher.de/x":         "xn--bcher-kva.de",
		"http://[2001:db8::1]:81/":    "2001-db8--1_81",
		"https://a_b.example.org/x/y": "a_b.example.org",
		"https://EXAMPLE.com./":       "example.com",
		"https://example.com../":      "example.com",
		"http://[0:0::1]:81/":         "--1_81",
		"http://[::ffff:1.1.1.1]/":    "1.1.1.1",
	} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		got := hostKey(u)
		if got != want {
			t.Errorf("hostKey(%s) = %q, want %q", raw, got, want)
		}
	}
	// A Persian domain becomes punycode that reads back as itself.
	u, _ := url.Parse("https://خبر.ایران/")
	got := hostKey(u)
	back, err := idna.ToUnicode(got)
	if !strings.HasPrefix(got, "xn--") || err != nil || back != "خبر.ایران" {
		t.Errorf("hostKey(%s) = %q, back %q, %v", u, got, back, err)
	}
}

// A request to a private address the exceptions don't allow asks through
// env.Approve: once fetches, in this project adds the host to the
// exceptions, deny blocks with the note. Where no one can ask, the error
// names the setting (6.7, 8.8).
func TestFetchPagePrivate(t *testing.T) {
	env := testEnv(t)
	d, base := pageServer(t, env)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(base, "http://"))
	ctx := context.Background()
	env.PrivateHosts = func(h string) bool { return h == "site.test" }
	var cards []chat.Approval
	answer := chat.Approval{Answer: chat.GrantOnce}
	env.Approve = func(_ context.Context, a chat.Approval) (chat.Approval, error) {
		cards = append(cards, a)
		a.Answer, a.Note = answer.Answer, answer.Note
		return a, nil
	}
	hosts := func() []string {
		hs, err := env.Project.PrivateHosts(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return hs
	}

	// Once: fetched, and nothing kept.
	url := "http://127.0.0.1:" + port + "/review"
	if r, err := callWith(t, d, env, "fetch_page", `{"url": "`+url+`"}`); err != nil || r.Ref == "" {
		t.Fatalf("once: %+v, %v", r, err)
	}
	if len(cards) != 1 || len(hosts()) != 0 {
		t.Fatalf("cards %+v, hosts %q", cards, hosts())
	}
	c := cards[0]
	if c.Kind != "private" || c.Target != "127.0.0.1" || c.Ask != "Allow requests to 127.0.0.1 on this computer?" ||
		!strings.Contains(c.Details, "127.0.0.1 is an address on this computer") || !strings.Contains(c.Details, "Tool: fetch_page") {
		t.Errorf("card %+v", c)
	}
	if err := (chat.Part{Kind: chat.PartApproval, Approval: &c}).Validate(); err != nil {
		t.Errorf("card part: %v", err)
	}
	var grants []chat.Grant
	for _, o := range c.Options {
		grants = append(grants, o.Grant)
	}
	if fmt.Sprint(grants) != fmt.Sprint([]chat.Grant{chat.GrantOnce, chat.GrantAlways, chat.GrantDeny}) {
		t.Errorf("options %+v", c.Options)
	}

	// In this project: the host joins the exceptions.
	answer.Answer = chat.GrantAlways
	if _, err := callWith(t, d, env, "fetch_page", `{"url": "`+url+`"}`); err != nil {
		t.Fatalf("always: %v", err)
	}
	if hs := hosts(); fmt.Sprint(hs) != "[127.0.0.1]" {
		t.Errorf("hosts after always: %q", hs)
	}

	// Deny: the blocked error, with the note.
	answer = chat.Approval{Answer: chat.GrantDeny, Note: "not that one"}
	_, err := callWith(t, d, env, "fetch_page", `{"url": "http://intranet.test:`+port+`/"}`)
	var te *Error
	if !errors.As(err, &te) || !strings.Contains(te.Msg, "is blocked: a private address (192.168.0.10): the user denied it; their note: not that one") ||
		strings.Contains(te.Msg, "project settings") {
		t.Errorf("deny: %v", err)
	}
	c = cards[len(cards)-1]
	if c.Ask != "Allow requests to intranet.test on your local network?" || !strings.Contains(c.Details, "intranet.test resolves to 192.168.0.10: your local network") ||
		!strings.Contains(c.Risk, "intranet.test is a public name, but it points to your local network.") {
		t.Errorf("name card %+v", c)
	}

	// A redirect names both hosts.
	answer = chat.Approval{Answer: chat.GrantOnce}
	if _, err := callWith(t, d, env, "fetch_page", `{"url": "`+base+`/away"}`); err == nil {
		t.Fatal("the redirect to a host not approved was followed")
	}
	a := store.Approval{ID: id.Approval(id.New()), Kind: "host", Target: "other.test", Answer: chat.GrantAlways, Source: id.SourceUser}
	if err := env.Project.DB.RecordApproval(ctx, a); err != nil {
		t.Fatal(err)
	}
	n := len(cards)
	if _, err := callWith(t, d, env, "fetch_page", `{"url": "`+base+`/away"}`); err != nil {
		t.Fatalf("redirect: %v", err)
	}
	if c := cards[len(cards)-1]; len(cards) != n+1 || c.Ask != "Allow the redirect from site.test to other.test on this computer?" ||
		!strings.Contains(c.Details, "Redirected from: site.test") {
		t.Errorf("redirect card %+v", c)
	}

	// No one to ask, as in a pipeline: blocked, naming the setting.
	env.Approve = nil
	_, err = callWith(t, d, env, "fetch_page", `{"url": "http://other.test:`+port+`/review"}`)
	if !errors.As(err, &te) || !strings.Contains(te.Msg, "is blocked: a loopback address (127.0.0.1); it can't be asked for here: to allow it, add other.test to the private-network exceptions in the project settings") {
		t.Errorf("no card: %v", err)
	}
}

// A card's answer may take longer than the call's timeout: the timeout
// stops while the call waits, and goes on with the time it had left.
func TestPauseStopsTheTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		wait := func(ctx context.Context, ms int) error {
			select {
			case <-ctx.Done():
				return context.Cause(ctx)
			case <-time.After(time.Duration(ms) * time.Millisecond):
				return nil
			}
		}
		type args struct {
			Before int `json:"before"`
			After  int `json:"after"`
		}
		slow := Func(Spec{Name: "slow", Description: "Works, asks, works.", Timeout: time.Second, Schema: json.RawMessage(`{"type": "object"}`)},
			func(ctx context.Context, _ *Env, a args) (Result, error) {
				if err := wait(ctx, a.Before); err != nil {
					return Result{}, err
				}
				resume := Pause(ctx)
				err := wait(ctx, 3600_000) // the user answers after an hour
				resume()
				if err != nil {
					return Result{}, err
				}
				if err := wait(ctx, a.After); err != nil {
					return Result{}, err
				}
				return Result{Text: "done"}, nil
			})
		run := func(a string) (Result, error) {
			return Run(context.Background(), slow, Call{Args: json.RawMessage(a)})
		}
		if r, err := run(`{"before": 400, "after": 400}`); err != nil || r.Text != "done" {
			t.Errorf("800 ms of work around an hour's wait: %+v, %v", r, err)
		}
		var te *Error
		if _, err := run(`{"before": 600, "after": 600}`); !errors.As(err, &te) || te.Msg != "slow timed out after 1s" {
			t.Errorf("1.2 s of work: %v", err)
		}
		// Outside a call, Pause does nothing.
		Pause(context.Background())()
	})
}
