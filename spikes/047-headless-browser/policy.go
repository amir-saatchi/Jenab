package main

// Part A.4, second look: the action policy as an allow list, and where
// --max-output applies.
//
// The first safety run used category names ("eval", "network", "state")
// in "deny"; agent-browser matches its internal action names instead
// ("evaluate", "route", "state_save"), so only read, download and upload
// were denied. This part denies by default and allows only what
// fetch_page needs.

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// allowFetch is what fetch_page's rendering needs: open, wait for load,
// read the HTML, the accessibility tree, the URL and title, scroll, and
// turn the stream server off.
var allowFetch = []string{"navigate", "waitforloadstate", "content", "innerhtml", "snapshot", "url", "title", "scroll", "wait", "stream_disable", "close"}

func partPolicy(ctx context.Context) {
	e, err := newEnv()
	if err != nil {
		p("setup: %v", err)
		return
	}
	b := browsers()[0]
	p("## 4b. Action policy as an allow list (%s)\n", b.ID)
	a := e.ab(b, "policy")
	a.Boundaries = true
	a.MaxOutput = 300
	a.Policy = map[string]any{"default": "deny", "allow": allowFetch}
	a.Setup()
	pol, _ := os.ReadFile(filepath.Join(a.Home, "policy.json"))
	p("Policy: `%s`; max output 300 in the config file.\n", strings.Join(strings.Fields(string(pol)), " "))
	u := e.S.Base() + "/js/spa-inline"
	up := filepath.Join(a.Home, "up.txt")
	os.WriteFile(up, []byte("x"), 0o600)
	tests := [][]string{
		{"open", u},
		{"wait", "--load", "networkidle", "--timeout", "5000"},
		{"wait", "300"},
		{"get", "html", "html"},
		{"get", "text", "body"},
		{"get", "url"},
		{"get", "title"},
		{"snapshot"},
		{"snapshot", "-i"},
		{"scroll", "down", "500"},
		{"eval", "document.title"},
		{"wait", "--fn", "true"},
		{"network", "route", "**/*", "--abort"},
		{"network", "requests"},
		{"state", "save", filepath.Join(a.Home, "state.json")},
		{"cookies", "get"},
		{"cookies", "set", "a", "b"},
		{"storage", "local"},
		{"read", u},
		{"click", "h1"},
		{"fill", "input", "x"},
		{"type", "h1", "x"},
		{"press", "Enter"},
		{"upload", "#f", up},
		{"download", "#dl", filepath.Join(a.Home, "downloads", "x.txt")},
		{"screenshot", filepath.Join(a.Home, "shots", "s.png")},
		{"tab", "new"},
		{"stream", "disable"},
	}
	p("| Command | Success | Error, or the size of the output |")
	p("|---|---|---|")
	for _, tc := range tests {
		r := a.Run(ctx, 20*time.Second, tc...)
		res := r.Error
		if r.Success {
			res = "ok, " + itoaLen(r.Raw) + " bytes of JSON"
			if h := r.field("html"); h != "" {
				res += ", html " + itoaLen(h) + " chars"
			}
			if t := r.field("text"); t != "" {
				res += ", text " + itoaLen(t) + " chars"
			}
			if s := r.field("snapshot"); s != "" {
				res += ", snapshot " + itoaLen(s) + " chars"
			}
		}
		p("| `%s` | %v | %s |", strings.Join(tc, " "), r.Success, clip(strings.ReplaceAll(res, "\n", " "), 160))
	}
	a.Close(ctx)
	waitGone(10 * time.Second)

	// --max-output as a flag and as the environment variable
	p("\n### Where --max-output applies\n")
	for _, how := range []string{"flag", "env"} {
		a := e.ab(b, "maxout-"+how)
		a.Setup()
		if how == "env" {
			a.Env = append(a.Env, "AGENT_BROWSER_MAX_OUTPUT=300")
		}
		a.Run(ctx, 60*time.Second, "open", u)
		var g, t Result
		if how == "flag" {
			g = a.Run(ctx, 20*time.Second, "--max-output", "300", "get", "html", "html")
			t = a.Run(ctx, 20*time.Second, "--max-output", "300", "snapshot")
		} else {
			g = a.Run(ctx, 20*time.Second, "get", "html", "html")
			t = a.Run(ctx, 20*time.Second, "snapshot")
		}
		p("- %s: get html gave %s chars of html (%v %s); snapshot %s chars", how, itoaLen(g.field("html")), g.Success, clip(g.Error, 100), itoaLen(t.field("snapshot")))
		a.Close(ctx)
		waitGone(10 * time.Second)
	}

	// the daemon's port and the session name
	p("\n### The daemon's port\n")
	for _, name := range []string{"", "jenab-a1", "jenab-b2"} {
		a := e.ab(b, "port")
		a.Session = name
		a.Setup()
		a.Run(ctx, 60*time.Second, "open", u)
		es, _ := os.ReadDir(filepath.Join(a.Home, "sock"))
		var ports []string
		for _, f := range es {
			if strings.HasSuffix(f.Name(), ".port") {
				x, _ := os.ReadFile(filepath.Join(a.Home, "sock", f.Name()))
				ports = append(ports, f.Name()+"="+strings.TrimSpace(string(x)))
			}
		}
		p("- session %q: %s", name, strings.Join(ports, ", "))
		a.Close(ctx)
		waitGone(10 * time.Second)
	}
}

func itoaLen(s string) string { return strconv.Itoa(chars(s)) }
