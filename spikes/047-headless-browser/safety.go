package main

// Part A.4: safety settings, and what consent walls and CAPTCHAs look like.

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// policy is the first try, with category names. agent-browser matches
// internal action names, so only read, download and upload are denied
// here (results.md, section 4); policy.go has the allow list that works.
var policy = map[string]any{
	"default": "allow",
	"deny":    []string{"eval", "download", "upload", "read", "network", "state"},
}

func partSafety(ctx context.Context) {
	e, err := newEnv()
	if err != nil {
		p("setup: %v", err)
		return
	}
	b := browsers()[0]
	p("## 4. Safety (%s)\n", b.ID)
	a := e.ab(b, "safety")
	a.Boundaries = true
	a.MaxOutput = 200000
	a.Policy = policy
	a.Setup()
	pol, _ := os.ReadFile(filepath.Join(a.Home, "policy.json"))
	p("Action policy: `%s`; content boundaries on; max output 200,000.\n", strings.Join(strings.Fields(string(pol)), " "))
	u := e.S.Base() + "/js/spa-inline"
	r := a.Run(ctx, 60*time.Second, "open", u)
	p("- open: %v", r.Success)
	g := a.Run(ctx, 20*time.Second, "get", "html", "html")
	p("- get html, raw output starts: `%s`", clip(strings.ReplaceAll(g.Raw, "`", "'"), 400))
	p("- get html, raw output ends: `%s`", tail(strings.ReplaceAll(strings.TrimSpace(g.Raw), "`", "'"), 300))
	t := a.Run(ctx, 20*time.Second, "get", "text", "body")
	p("- get text body, raw output starts: `%s`", clip(strings.ReplaceAll(t.Raw, "`", "'"), 300))
	tests := [][]string{
		{"eval", "document.title"},
		{"wait", "--fn", "document.title.length > 0"},
		{"read"},
		{"network", "route", "**/*", "--abort"},
		{"state", "save", filepath.Join(a.Home, "state.json")},
		{"cookies", "set", "a", "b"},
		{"snapshot", "-i"},
		{"get", "title"},
	}
	p("\n| Command | Success | Error / result |")
	p("|---|---|---|")
	for _, tc := range tests {
		r := a.Run(ctx, 20*time.Second, tc...)
		res := r.Error
		if r.Success {
			res = clip(strings.ReplaceAll(string(r.Data), "|", "/"), 120)
		}
		p("| `%s` | %v | %s |", strings.Join(tc, " "), r.Success, clip(strings.ReplaceAll(res, "\n", " "), 160))
	}
	a.Run(ctx, 30*time.Second, "open", e.S.Base()+"/upload")
	up := filepath.Join(a.Home, "up.txt")
	os.WriteFile(up, []byte("x"), 0o600)
	for _, tc := range [][]string{{"download", "#dl", filepath.Join(a.Home, "downloads", "x.txt")}, {"upload", "#f", up}, {"click", "#dl"}} {
		r := a.Run(ctx, 20*time.Second, tc...)
		p("| `%s` | %v | %s |", strings.Join(tc, " "), r.Success, clip(strings.ReplaceAll(r.Error+string(r.Data), "\n", " "), 160))
	}
	time.Sleep(2 * time.Second)
	es, _ := os.ReadDir(filepath.Join(a.Home, "downloads"))
	var names []string
	for _, e := range es {
		names = append(names, e.Name())
	}
	p("\nFiles in the download folder after `click` on a download link: %v", names)
	r = a.Run(ctx, 20*time.Second, "open", "file:///C:/Windows/win.ini")
	g = a.Run(ctx, 20*time.Second, "get", "text", "body")
	p("\n- `open file:///C:/Windows/win.ini`: %v %s; body has \"[fonts]\": %v", r.Success, clip(r.Error, 100), strings.Contains(strings.ToLower(g.Raw), "[fonts]"))
	st := a.Run(ctx, 10*time.Second, "stream", "status")
	p("- `stream status`: %s", clip(string(st.Data)+st.Error, 200))
	sd := a.Run(ctx, 10*time.Second, "stream", "disable")
	s := NewSampler(root, netip.AddrPort{})
	s.Start()
	time.Sleep(300 * time.Millisecond)
	s.Stop()
	p("- `stream disable`: %v %s; listening afterwards: %s", sd.Success, clip(sd.Error, 100), strings.Join(s.Listening(), "; "))
	a.Close(ctx)
	waitGone(10 * time.Second)

	// max output
	a = e.ab(b, "maxout")
	a.Boundaries = true
	a.MaxOutput = 500
	a.Policy = policy
	a.Setup()
	a.Run(ctx, 60*time.Second, "open", u)
	g = a.Run(ctx, 20*time.Second, "get", "html", "html")
	p("\n- With max output 500: get html returned %d characters of html; ends: `%s`", chars(g.field("html")), tail(strings.ReplaceAll(g.field("html"), "`", "'"), 160))
	a.Close(ctx)
	waitGone(10 * time.Second)

	// consent and CAPTCHA pages
	p("\n### Consent walls and CAPTCHAs (spike's pages)\n")
	a = e.ab(b, "consent")
	a.Policy = policy
	a.Setup()
	for _, id := range jsSafety {
		u := e.S.Base() + "/js/" + id
		tm, _ := visit(ctx, a, u)
		title, _, _ := rendered(ctx, a, u)
		p("- **%s** (%s): title %q, %d characters, article phrases %s; consent words: %s. Text: %q", id, jsPages[id].What, title, tm.set,
			phrasesHit(tm.text, jsPages[id].Phrases), consentWords(tm.text), clip(strings.Join(strings.Fields(tm.text), " "), 300))
	}
	a.Close(ctx)
	waitGone(10 * time.Second)
	_ = fmt.Sprint
}

func tail(s string, n int) string {
	if len(s) > n {
		return "…" + s[len(s)-n:]
	}
	return s
}
