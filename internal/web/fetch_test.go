package web

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// fakeResolver maps names to addresses; other names don't resolve.
type fakeResolver map[string][]netip.Addr

func (f fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if a, ok := f[host]; ok {
		return a, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

var zwnj = string(rune(0x200C))

const article = `<!doctype html><html><head><meta charset="utf-8">
<title>Laptop X review | Example News</title>
<meta property="og:title" content="Laptop X review">
</head><body>
<nav><a href="/">Home</a> <a href="/news">News</a> <a href="/about">About us</a></nav>
<article><h1>Laptop X review</h1>
<p>The Laptop X is a thin machine with a bright screen. We used it for two weeks of travel and daily work, and the battery lasted a full day of writing and browsing.</p>
<p>Its keyboard is quiet and firm. The trackpad is large and precise, and the speakers are better than most at this price. The fan stays silent under light load.</p>
<table><tr><th>Model</th><th>Price</th></tr><tr><td>Laptop X 13</td><td>999 EUR</td></tr><tr><td>Laptop X 15</td><td>1,299 EUR</td></tr></table>
<p>See the <a href="/specs">full specs</a> for details.</p>
</article>
<footer>Cookie settings · Imprint · Newsletter signup</footer>
</body></html>`

// server serves a few pages; its address is 127.0.0.1, reached as site.test.
func server(t *testing.T) (*httptest.Server, *Client, string) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/article", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.UserAgent(), "Jenab") {
			http.Error(w, "no agent", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, article)
	})
	mux.HandleFunc("/js", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>App</title><script src="app.js"></script></head><body><div id="root">Loading…</div></body></html>`)
	})
	mux.HandleFunc("/fa", func(w http.ResponseWriter, r *http.Request) {
		// windows-1256, named only in <meta>. It has no Persian yeh, so
		// such pages use the Arabic one.
		page := `<html><head><meta http-equiv="Content-Type" content="text/html; charset=windows-1256"><title>قيمت بيت` + zwnj + `کوين</title></head><body><article><p>` +
			strings.Repeat("قيمت بيت"+zwnj+"کوين امروز بالا رفت و بازار ارز ديجيتال آرام بود. ", 8) + `</p></article></body></html>`
		b, err := charmap.Windows1256.NewEncoder().Bytes([]byte(page))
		if err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write(b)
	})
	mux.HandleFunc("/text", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, "plain words")
	})
	mux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		chunk := strings.Repeat("x", 1<<16)
		for range (6 << 20) / len(chunk) {
			fmt.Fprint(w, chunk)
		}
	})
	mux.HandleFunc("/pdf", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		fmt.Fprint(w, "%PDF-1.7")
	})
	mux.HandleFunc("/missing", http.NotFound)
	mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/loop", http.StatusFound) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	loop := netip.MustParseAddr("127.0.0.1")
	c := NewClient(Options{Resolver: fakeResolver{
		"site.test":    {loop},
		"private.test": {netip.MustParseAddr("10.1.2.3")},
		"local.test":   {loop},
	}})
	return srv, c, port
}

// onlySite is the project exception for the test server's name.
func onlySite(host string) bool { return host == "site.test" }

func TestFetchArticle(t *testing.T) {
	_, c, port := server(t)
	p, err := c.Fetch(t.Context(), "http://site.test:"+port+"/article", onlySite)
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "Laptop X review" {
		t.Errorf("title %q", p.Title)
	}
	for _, want := range []string{"bright screen", "keyboard is quiet", "| Laptop X 15 | 1,299 EUR |", "(http://site.test:" + port + "/specs)"} {
		if !strings.Contains(p.Text, want) {
			t.Errorf("text lacks %q:\n%s", want, p.Text)
		}
	}
	for _, leak := range []string{"Cookie settings", "About us"} {
		if strings.Contains(p.Text, leak) {
			t.Errorf("text has %q:\n%s", leak, p.Text)
		}
	}
	if p.NeedsJavaScript || p.Truncated || p.MIME != "text/html" {
		t.Errorf("page %+v", p)
	}
}

func TestFetchCharset(t *testing.T) {
	_, c, port := server(t)
	p, err := c.Fetch(t.Context(), "http://site.test:"+port+"/fa", onlySite)
	if err != nil {
		t.Fatal(err)
	}
	if want := "قيمت بيت" + zwnj + "کوين"; p.Title != want {
		t.Errorf("title %q, want %q", p.Title, want)
	}
	if !strings.Contains(p.Text, "بيت"+zwnj+"کوين امروز بالا رفت") || strings.ContainsRune(p.Text, 0xFFFD) {
		t.Errorf("text %q", p.Text)
	}
}

func TestFetchKinds(t *testing.T) {
	_, c, port := server(t)
	base := "http://site.test:" + port
	p, err := c.Fetch(t.Context(), base+"/js", onlySite)
	if err != nil || !p.NeedsJavaScript {
		t.Errorf("js page: %+v, %v", p, err)
	}
	p, err = c.Fetch(t.Context(), base+"/text", onlySite)
	if err != nil || p.Text != "plain words" || p.NeedsJavaScript || p.MIME != "text/plain" {
		t.Errorf("text: %+v, %v", p, err)
	}
	p, err = c.Fetch(t.Context(), base+"/big", onlySite)
	if err != nil || !p.Truncated || len(p.Text) != MaxBody {
		t.Errorf("big: %d bytes, truncated %v, %v", len(p.Text), p.Truncated, err)
	}
	if _, err := c.Fetch(t.Context(), base+"/pdf", onlySite); !errors.Is(err, ErrNotPage) {
		t.Errorf("pdf: %v", err)
	}
	var se *StatusError
	if _, err := c.Fetch(t.Context(), base+"/missing", onlySite); !errors.As(err, &se) || se.Status != 404 {
		t.Errorf("missing: %v", err)
	}
	if _, err := c.Fetch(t.Context(), base+"/loop", onlySite); err == nil || !strings.Contains(err.Error(), "redirects") {
		t.Errorf("loop: %v", err)
	}
}

func TestFetchBlocked(t *testing.T) {
	srv, c, port := server(t)
	mux := srv.Config.Handler.(*http.ServeMux)
	mux.HandleFunc("/to-loopback", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:"+port+"/article", http.StatusFound)
	})
	mux.HandleFunc("/to-local", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://local.test:"+port+"/article", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/to-file", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///C:/Windows/win.ini", http.StatusFound)
	})
	base := "http://site.test:" + port
	for _, tc := range []struct{ name, url string }{
		{"redirect to 127.0.0.1", base + "/to-loopback"},
		{"redirect to a name for 127.0.0.1", base + "/to-local"},
		{"redirect to file", base + "/to-file"},
		{"name for a private address", "http://private.test:" + port + "/article"},
		{"name for loopback", "http://local.test:" + port + "/article"},
		{"loopback address", "http://127.0.0.1:" + port + "/article"},
		{"IPv6 loopback", "http://[::1]:" + port + "/article"},
		{"mapped loopback", "http://[::ffff:127.0.0.1]:" + port + "/article"},
		{"file URL", "file:///C:/Windows/win.ini"},
		{"ftp URL", "ftp://site.test/x"},
		{"user in URL", "http://user:pw@site.test:" + port + "/article"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.Fetch(t.Context(), tc.url, onlySite)
			var b *BlockedError
			if !errors.As(err, &b) {
				t.Fatalf("got %v, want a BlockedError", err)
			}
		})
	}
	// Without the exception the test server itself is blocked.
	if _, err := c.Fetch(t.Context(), base+"/article", nil); err == nil {
		t.Error("site.test without an exception was fetched")
	}
}

func TestBlockedAddresses(t *testing.T) {
	for _, tc := range []struct {
		addr    string
		blocked bool
	}{
		{"127.0.0.1", true}, {"127.8.9.10", true}, {"::1", true}, {"::ffff:127.0.0.1", true},
		{"10.0.0.1", true}, {"172.16.5.4", true}, {"172.31.255.255", true}, {"192.168.1.1", true},
		{"169.254.169.254", true}, {"fe80::1", true}, {"fc00::1", true}, {"fd12:3456::1", true},
		{"0.0.0.0", true}, {"::", true}, {"::ffff:0.0.0.0", true}, {"224.0.0.1", true}, {"ff02::1", true},
		{"8.8.8.8", false}, {"172.32.0.1", false}, {"2001:4860:4860::8888", false}, {"::ffff:8.8.8.8", false},
	} {
		if got := blocked(netip.MustParseAddr(tc.addr)) != ""; got != tc.blocked {
			t.Errorf("%s: blocked %v, want %v", tc.addr, got, tc.blocked)
		}
	}
}

func TestReadableDropsData(t *testing.T) {
	b64 := strings.Repeat("iVBORw0KGgo", 1000)
	p := strings.Repeat("<p>The widget runs on the frontend and the check runs on the backend of the site.</p>\n", 5)
	page := `<html><head><title>Flow</title></head><body><article><h1>Flow</h1>` + p +
		`<p><img src="data:image/png;base64,` + b64 + `" alt="Flow diagram"> and <img src=" DATA:image/gif;base64,R0lG"></p>` +
		`<p><a href="data:text/html,hi">a data link</a> and <a href="https://example.com/docs">the docs</a>.</p></article></body></html>`
	u, _ := url.Parse("https://site.test/a")
	_, text, err := readable([]byte(page), u)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(text), "data:") || strings.Contains(text, "iVBOR") {
		t.Errorf("data URL kept:\n%s", text)
	}
	for _, want := range []string{"Flow diagram", "a data link", "[the docs](https://example.com/docs)"} {
		if !strings.Contains(text, want) {
			t.Errorf("no %q in:\n%s", want, text)
		}
	}
}

// FuzzReadable: hostile HTML doesn't panic, and the page comes back as
// valid UTF-8 with a one-line title.
func FuzzReadable(f *testing.F) {
	for _, s := range []string{
		"",
		"<html><head><title>T</title></head><body><article><p>Hi</p></article></body></html>",
		"<table><tr><td>a<br>b<td rowspan=999999 colspan=999999>c</table>",
		"<p>\xff\xfe broken \xc3 utf8</p><title>\x80</title>",
		"<meta charset=windows-1256><title>\xed\xc7</title><p>\xc8\xe5</p>",
		"<img src=data:image/png;base64,AAAA alt=x><a href=data:,x>y</a><svg><foreignObject><math><mi><style><!--</style>",
		strings.Repeat("<div>", 3000) + "deep" + strings.Repeat("</div>", 3000),
		"<base href=javascript:alert(1)><a href=//x>z</a><a href=\"http://[::1\">bad</a>",
	} {
		f.Add([]byte(s))
	}
	u, _ := url.Parse("https://site.test/a")
	f.Fuzz(func(t *testing.T, body []byte) {
		text, err := decode(body, "text/html")
		if err != nil {
			return
		}
		title, md, err := readable(text, u)
		if err != nil {
			return
		}
		if !utf8.ValidString(title) || !utf8.ValidString(md) || strings.Contains(title, "\n") {
			t.Errorf("title %q, text %q", title, md)
		}
	})
}

func TestBoundTables(t *testing.T) {
	intro := strings.Repeat("<p>Prices of the coins this week, from the exchange, in euros and dollars.</p>\n", 4)
	row := func(cells ...string) string { return "<tr><td>" + strings.Join(cells, "</td><td>") + "</td></tr>" }
	for _, tc := range []struct {
		name, table string
		md          bool   // kept as a Markdown table
		want        string // text that must stay
	}{
		{"plain", "<table>" + row("Coin", "Price") + row("Bitcoin", "60,000") + "</table>", true, "| Bitcoin | 60,000 |"},
		{"spans", `<table><tr><th>Coin</th><th colspan="2">Price</th></tr><tr><td rowspan="2">Bitcoin</td><td>EUR</td><td>60,000</td></tr>` +
			row("USD", "65,000") + "</table>", true, "| Bitcoin | USD | 65,000 |"},
		{"huge rowspan", `<table><tr><td rowspan="999999" colspan="999999">Bitcoin</td><td>60,000</td></tr></table>`, false, "Bitcoin 60,000"},
		{"wide", "<table>" + row(strings.Split(strings.Repeat("x,", 1200)+"Bitcoin", ",")...) + "</table>", false, "x x Bitcoin"},
		{"ragged", "<table>" + row(strings.Split(strings.Repeat("x,", 600)+"Bitcoin", ",")...) + strings.Repeat(row("y"), 500) + "</table>", false, "Bitcoin"},
		{"span text", `<table>` + row("Coin", "Note") + `<tr><td>Bitcoin</td><td colspan="900">` + strings.Repeat("long note ", 400) + `</td></tr></table>`, false, "Bitcoin long note"},
		// Row 1 has 600 cells of its own and 600 reaching down from row 0.
		{"rowspans widen", "<table><tr>" + strings.Repeat(`<td rowspan="2">x</td>`, 600) + "<td>Bitcoin</td></tr>" + row(strings.Split(strings.Repeat("y,", 599)+"y", ",")...) + "</table>", false, "x x Bitcoin"},
		{"nested", "<table><tr><td>Outer</td><td><table>" + row(`<span>Bitcoin</span>`, "60,000") + `<tr><td rowspan="999999">deep</td></tr></table></td></tr></table>`, true, "| Outer | Bitcoin 60,000"}, // only the inner table is flattened
	} {
		page := "<html><head><title>Coins</title></head><body><article><h1>Coins</h1>" + intro + tc.table + intro + "</article></body></html>"
		u, _ := url.Parse("https://site.test/a")
		_, text, err := readable([]byte(page), u)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if strings.Contains(text, "|---") != tc.md || !strings.Contains(text, tc.want) {
			t.Errorf("%s: markdown table %v, want %v, %q:\n%.600s", tc.name, strings.Contains(text, "|---"), tc.md, tc.want, text)
		}
	}
}

func TestFetchRedirectsAndKinds(t *testing.T) {
	srv, c, port := server(t)
	mux := srv.Config.Handler.(*http.ServeMux)
	// /hops/n redirects n more times, then serves text.
	mux.HandleFunc("/hops/{n}", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.PathValue("n"))
		if n == 0 {
			fmt.Fprint(w, "arrived")
			return
		}
		http.Redirect(w, r, "/hops/"+strconv.Itoa(n-1), http.StatusFound)
	})
	serve := func(path, ctype, body string) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", ctype)
			fmt.Fprint(w, body)
		})
	}
	article := `<html><head><title>Coins</title></head><body><article>` +
		strings.Repeat("<p>Bitcoin and Ether prices moved little this week, and trading was quiet on most exchanges.</p>", 5) + `</article></body></html>`
	serve("/xhtml", "application/xhtml+xml; charset=utf-8", article)
	serve("/feed", "application/atom+xml", "<feed><title>Coins</title></feed>")
	serve("/data", "application/ld+json", `{"name": "Coins"}`)
	base := "http://site.test:" + port

	p, err := c.Fetch(t.Context(), base+"/hops/10", onlySite)
	if err != nil || p.Text != "arrived" || p.URL != base+"/hops/0" {
		t.Errorf("10 redirects: %+v, %v", p, err)
	}
	if _, err := c.Fetch(t.Context(), base+"/hops/11", onlySite); err == nil || !strings.Contains(err.Error(), "more than 10 redirects") {
		t.Errorf("11 redirects: %v", err)
	}
	p, err = c.Fetch(t.Context(), base+"/xhtml", onlySite)
	if err != nil || p.Title != "Coins" || !strings.Contains(p.Text, "trading was quiet") || strings.Contains(p.Text, "<p>") {
		t.Errorf("xhtml: %+v, %v", p, err)
	}
	for _, path := range []string{"/feed", "/data"} {
		p, err := c.Fetch(t.Context(), base+path, onlySite)
		if err != nil || p.Title != "" || !strings.Contains(p.Text, "Coins") {
			t.Errorf("%s: %+v, %v", path, p, err)
		}
	}
}
