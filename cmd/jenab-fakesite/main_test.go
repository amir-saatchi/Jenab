package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/web"
)

func repoPages() []string {
	var out []string
	for _, d := range defaultPages {
		out = append(out, filepath.Join("..", "..", d))
	}
	return out
}

// serve starts the site on the repository's pages.
func serve(t *testing.T) (*httptest.Server, *bytes.Buffer) {
	t.Helper()
	s, err := load(repoPages())
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	srv := httptest.NewServer(logged(log.New(&logs, "", 0), s.handler()))
	t.Cleanup(srv.Close)
	return srv, &logs
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("%s: Content-Type %q", url, ct)
	}
	return resp.StatusCode, string(b)
}

func TestAllDays(t *testing.T) {
	s, err := load(repoPages())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.days) != 19 || s.days[0] != "2026-10-05" || s.days[18] != "2026-10-23" {
		t.Errorf("days = %v; want the 19 days from 2026-10-05 to 2026-10-23", s.days)
	}
}

func TestIndex(t *testing.T) {
	srv, logs := serve(t)
	status, body := get(t, srv.URL+"/")
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	for _, want := range []string{
		`<a href="/crypto/2026-10-05">2026-10-05</a>: Daily crypto close: Monday 5 October 2026`,
		`<a href="/crypto/2026-10-16">`,
		`<a href="/crypto/2026-10-23">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("index lacks %q:\n%s", want, body)
		}
	}
	if n := strings.Count(body, `<a href="/crypto/`); n != 19 {
		t.Errorf("index links %d days; want 19", n)
	}
	if got := logs.String(); got != "GET / 200\n" {
		t.Errorf("log = %q", got)
	}
}

func TestDay(t *testing.T) {
	srv, logs := serve(t)
	status, body := get(t, srv.URL+"/crypto/2026-10-05")
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	for _, want := range []string{
		"<title>Daily crypto close: Monday 5 October 2026 - Markets Daily</title>",
		"<h1>Daily crypto close: Monday 5 October 2026</h1>",
		"<h2>Top 10 by market value</h2>",
		"<th>Close (USD)</th>",
		"<tr><td>1</td><td>Bitcoin (BTC)</td><td>62,418</td><td>+1.4%</td><td>28.4 bn</td><td>1.23 tn</td></tr>",
		"<li><strong>Bitcoin starts October above $62,000 as traders return from holidays</strong> (Block Times).",
		"<p>The total market value rose 1.4% over the day.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q:\n%s", want, body)
		}
	}
	if got := logs.String(); got != "GET /crypto/2026-10-05 200\n" {
		t.Errorf("log = %q", got)
	}
}

func TestNotFound(t *testing.T) {
	srv, logs := serve(t)
	paths := []string{"/crypto/2026-10-24", "/crypto/2026-10-05/", "/crypto/", "/crypto/x", "/daily/2026-10-05", "/favicon.ico"}
	for _, p := range paths {
		status, body := get(t, srv.URL+p)
		if status != http.StatusNotFound || !strings.Contains(body, "<h1>Not found</h1>") {
			t.Errorf("%s: status %d, body:\n%s", p, status, body)
		}
	}
	if got := strings.Count(logs.String(), " 404\n"); got != len(paths) {
		t.Errorf("log has %d 404s; want %d:\n%s", got, len(paths), logs.String())
	}
}

func TestSameFiles(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	page := "# Day\n\n| a | b |\n|---|---|\n| 1 | 2 |\n"
	for _, f := range []struct{ dir, name, text string }{
		{a, "2026-10-05.md", page},
		{b, "2026-10-05.md", strings.ReplaceAll(page, "\n", "\r\n")}, // line endings aside, the same
		{a, "2026-10-06.md", page},
		{b, "2026-10-06.md", page},
		{b, "2026-10-07.md", page},
		{b, "notes.txt", "not a page"},
	} {
		if err := os.WriteFile(filepath.Join(f.dir, f.name), []byte(f.text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := load([]string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(s.days, " ") != "2026-10-05 2026-10-06 2026-10-07" {
		t.Errorf("days = %v", s.days)
	}

	if err := os.WriteFile(filepath.Join(b, "2026-10-06.md"), []byte(strings.Replace(page, "2 |", "3 |", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = load([]string{a, b})
	if err == nil || !strings.Contains(err.Error(), "2026-10-06: ") || strings.Contains(err.Error(), "2026-10-05") {
		t.Errorf("err = %v; want 2026-10-06 named as different, and only it", err)
	}

	if _, err := load([]string{a, t.TempDir()}); err == nil {
		t.Error("a folder without pages was accepted")
	}
}

func TestLoopback(t *testing.T) {
	for addr, ok := range map[string]bool{
		"127.0.0.1:8099": true, "127.0.0.1:0": true, "[::1]:8099": true,
		"0.0.0.0:8099": false, ":8099": false, "localhost:8099": false, "192.168.1.2:8099": false, "8099": false,
	} {
		if err := loopback(addr); (err == nil) != ok {
			t.Errorf("loopback(%q) = %v", addr, err)
		}
	}
}

func TestEscapes(t *testing.T) {
	body, title := toHTML("# A <b> & `x<y>`\n\nSome *text* and **<i>bold</i>**.\n")
	want := "<h1>A &lt;b&gt; &amp; <code>x&lt;y&gt;</code></h1>\n<p>Some *text* and <strong>&lt;i&gt;bold&lt;/i&gt;</strong>.</p>\n"
	if string(body) != want || title != "A <b> & `x<y>`" {
		t.Errorf("toHTML = %q, %q; want %q", body, title, want)
	}
}

// escaped matches the Markdown escapes the page reader writes, such as \#.
var escaped = regexp.MustCompile(`\\([[:punct:]])`)

// TestPageReader fetches every day with the app's page reader, as
// fetch_page does with 127.0.0.1 allowed. Its text, escapes aside, is the
// page's Markdown without the first heading, which is the page's title.
func TestPageReader(t *testing.T) {
	srv, _ := serve(t)
	c := web.NewClient(web.Options{})
	if _, err := c.Fetch(context.Background(), srv.URL+"/crypto/2026-10-05", web.Rules{}); err == nil {
		t.Error("127.0.0.1 was fetched without the exception")
	}
	allow := web.Rules{Private: func(h string) bool { return h == "127.0.0.1" }}
	s, err := load(repoPages())
	if err != nil {
		t.Fatal(err)
	}
	for _, day := range s.days {
		p, err := c.Fetch(context.Background(), srv.URL+"/crypto/"+day, allow)
		if err != nil {
			t.Fatal(err)
		}
		md, err := os.ReadFile(filepath.Join(repoPages()[1], day+".md"))
		if err != nil {
			t.Fatal(err)
		}
		h1, rest, _ := strings.Cut(strings.ReplaceAll(string(md), "\r\n", "\n"), "\n")
		text := escaped.ReplaceAllString(p.Text, "$1")
		if want := strings.TrimSpace(rest); text != want {
			t.Errorf("%s: the reader's text is not the page's Markdown:\n%s\n\nwant:\n%s", day, text, want)
		}
		if want := strings.TrimPrefix(h1, "# "); !strings.HasPrefix(p.Title, want) {
			t.Errorf("%s: title %q; want %q", day, p.Title, want)
		}
		if p.NeedsJavaScript || p.Truncated {
			t.Errorf("%s: NeedsJavaScript %v, Truncated %v", day, p.NeedsJavaScript, p.Truncated)
		}
		if day == "2026-10-05" && !strings.Contains(text, "| 1 | Bitcoin (BTC) | 62,418 |") {
			t.Errorf("Bitcoin's close of 5 October is not in the reader's text:\n%s", text)
		}
	}
}
