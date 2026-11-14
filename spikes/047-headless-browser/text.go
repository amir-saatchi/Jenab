package main

// Part A.3: text from JavaScript-only pages, through the readable-text path.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

// textOf runs HTML through the readable-text path (3.7).
func textOf(html, pageURL string) (title, text string) {
	u, _ := url.Parse(pageURL)
	t, x, err := readable([]byte(html), u)
	if err != nil {
		return "", "error: " + err.Error()
	}
	return t, x
}

func chars(s string) int { return utf8.RuneCountInString(s) }

// rendered reads the page's HTML and runs it through the path.
func rendered(ctx context.Context, a *AB, pageURL string) (title, text string, r Result) {
	r = a.Run(ctx, 20*time.Second, "get", "html", "html")
	if !r.Success {
		return "", "", r
	}
	t, x := textOf("<!doctype html><html>"+r.field("html")+"</html>", pageURL)
	return t, x, r
}

// plain is what fetch_page's own HTTP path gets.
func plain(u string, host string) (string, string, int) {
	req, _ := http.NewRequest("GET", u, nil)
	if host != "" {
		req.Host = host
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Jenab; +https://github.com/amir-saatchi/Jenab)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "error: " + err.Error(), 0
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	t, x := textOf(string(b), u)
	return t, x, resp.StatusCode
}

type timing struct {
	open, idle, total time.Duration
	load, idleC, set  int
	text              string
}

// visit opens a page and reads its text three ways: right after open
// (the load event), after network idle, and after it stops changing.
func visit(ctx context.Context, a *AB, u string) (timing, Result) {
	var t timing
	t0 := time.Now()
	r := a.Run(ctx, 60*time.Second, "open", u)
	t.open = time.Since(t0)
	if !r.Success {
		return t, r
	}
	_, x, _ := rendered(ctx, a, u)
	t.load = chars(x)
	a.Run(ctx, 20*time.Second, "wait", "--load", "networkidle", "--timeout", "10000")
	_, x, _ = rendered(ctx, a, u)
	t.idle = time.Since(t0)
	t.idleC = chars(x)
	prev, same := -1, 0
	for i := 0; i < 12 && same < 2; i++ {
		time.Sleep(500 * time.Millisecond)
		_, x, _ = rendered(ctx, a, u)
		if chars(x) == prev {
			same++
		} else {
			same = 0
		}
		prev = chars(x)
	}
	t.set = chars(x)
	t.text = x
	t.total = time.Since(t0)
	return t, r
}

func phrasesHit(text string, ps []string) string {
	n := 0
	for _, ph := range ps {
		if strings.Contains(text, ph) {
			n++
		}
	}
	return fmt.Sprintf("%d/%d", n, len(ps))
}

func ms(d time.Duration) string { return fmt.Sprintf("%.1f s", d.Seconds()) }

func partText(ctx context.Context) {
	e, err := newEnv()
	if err != nil {
		p("setup: %v", err)
		return
	}
	proxyAP := netip.MustParseAddrPort(e.PAddr)
	p("## 3. Text from JavaScript-only pages (spike's own pages)\n")
	p("Plain = fetch_page's HTTP path today. Load = right after `open`; idle = after `wait --load networkidle`; settled = after the text stopped changing (polled every 0.5 s, up to 6 s). Times are to the idle read (Jenab's likely recipe: open, wait for network idle, get html), cold = a fresh daemon and browser, warm = the same page again on the running browser. Memory and CPU are peaks over cold and warm for the whole tree (CLI, daemon, browser and its children).\n")
	for _, b := range browsers() {
		p("### %s\n", b.ID)
		p("| Page | What | Plain | Load | Idle | Settled | Phrases | Cold | Warm | Peak working set | Peak private | Peak CPU | Processes |")
		p("|---|---|---|---|---|---|---|---|---|---|---|---|---|")
		for _, id := range jsMain {
			pg := jsPages[id]
			u := e.S.Base() + "/js/" + id + pg.Hash
			_, px, _ := plain(fmt.Sprintf("http://127.0.0.1:%d/js/%s", e.S.Port, id), "")
			a := e.ab(b, "text")
			a.Setup()
			s := NewSampler(root, proxyAP)
			s.Start()
			cold, r := visit(ctx, a, u)
			warm, _ := visit(ctx, a, u)
			s.Stop()
			a.Close(ctx)
			waitGone(10 * time.Second)
			if !r.Success {
				p("| %s | %s | %d | open failed: %s |||||||||", id, pg.What, chars(px), clip(r.Error, 80))
				continue
			}
			p("| %s | %s | %d | %d | %d | %d | %s | %s | %s | %s | %s | %.0f%% | %d |", id, pg.What, chars(px), cold.load, cold.idleC, cold.set,
				phrasesHit(cold.text, pg.Phrases), ms(cold.idle), ms(warm.idle), mb(s.PeakWS), mb(s.PeakPriv), s.PeakCPU, s.MaxProcs)
		}
		p("")
	}
}

func partLive(ctx context.Context) {
	e, err := newEnv()
	if err != nil {
		p("setup: %v", err)
		return
	}
	proxyAP := netip.MustParseAddrPort(e.PAddr)
	p("## 3b. SPIKE-015 corpus pages, live\n")
	b := browsers()[0]
	for _, kv := range strings.Split(*flagLive, ",") {
		id, u, _ := strings.Cut(kv, "=")
		pu, _ := url.Parse(u)
		host := pu.Hostname()
		pt, px, code := plain(u, "")
		p("### %s (%s)\n", id, host)
		p("- Plain fetch: HTTP %d, title %q, %d characters", code, pt, chars(px))
		for _, strict := range []bool{false, true} {
			if strict {
				e.P.Approved = func(h string) bool { return h == host }
			} else {
				e.P.Approved = nil
			}
			a := e.ab(b, "live")
			a.Idle = "60s"
			a.Setup()
			e.P.Reset()
			s := NewSampler(root, proxyAP)
			s.Start()
			cold, r := visit(ctx, a, u)
			warm, _ := visit(ctx, a, u)
			title, _, _ := rendered(ctx, a, u)
			s.Stop()
			a.Close(ctx)
			waitGone(10 * time.Second)
			es := e.P.Reset()
			hosts := map[string]int{}
			blocked := 0
			for _, en := range es {
				hosts[en.Host]++
				if en.Decision != "allowed" {
					blocked++
				}
			}
			mode := "every public host allowed (private check only)"
			if strict {
				mode = "only " + host + " approved"
			}
			if !r.Success {
				p("- Rendered, %s: open failed: %s", mode, clip(r.Error, 200))
				continue
			}
			p("- Rendered, %s (%s): title %q; load %d, idle %d, settled %d characters; cold %s, warm %s; %d proxy requests to %d hosts, %d refused; peak %s working set, %s private, CPU %.0f%%",
				mode, b.ID, title, cold.load, cold.idleC, cold.set, ms(cold.idle), ms(warm.idle), len(es), len(hosts), blocked, mb(s.PeakWS), mb(s.PeakPriv), s.PeakCPU)
			p("  - Outside sockets: %s", orNone(s.Outside()))
			p("  - Consent words in the text: %s", consentWords(cold.text))
			p("  - Text starts: %q", clip(strings.Join(strings.Fields(cold.text), " "), 400))
		}
		p("")
	}
}

var consentList = []string{"cookie", "consent", "privacy", "einwillig", "zustimm", "akzeptier", "datenschutz", "partner", "verify you are human", "captcha"}

func consentWords(t string) string {
	l := strings.ToLower(t)
	var out []string
	for _, w := range consentList {
		if n := strings.Count(l, w); n > 0 {
			out = append(out, fmt.Sprintf("%s ×%d", w, n))
		}
	}
	return orNone(out)
}
