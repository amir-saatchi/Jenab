package main

// Part A.2: the network rules (6.7) through the proxy.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type variant struct {
	Name    string
	Proxy   bool
	Bypass  bool
	Strict  bool
	Allowed bool
	Direct  bool // open the page at 127.0.0.1 instead of the test name
	Args    []string
}

var variants = []variant{
	{Name: "V0 no proxy (the probes work)", Direct: true},
	{Name: "V1 proxy, private check only, no bypass flag", Proxy: true},
	{Name: "V2 proxy, private check only, <-loopback>", Proxy: true, Bypass: true},
	{Name: "V3 proxy, host approval too, <-loopback>", Proxy: true, Bypass: true, Strict: true},
	{Name: "V4a proxy, --allowed-domains, no browser args", Proxy: true, Allowed: true},
	{Name: "V4b V2 + --allowed-domains", Proxy: true, Bypass: true, Allowed: true},
	{Name: "V5 V2 + WebRTC limited to proxied TCP", Proxy: true, Bypass: true, Args: []string{webrtcPolicy}},
	{Name: "V6 V5 + host approval", Proxy: true, Bypass: true, Strict: true, Args: []string{webrtcPolicy}},
	{Name: "V7 V6 + no DNS in the browser", Proxy: true, Bypass: true, Strict: true, Args: []string{webrtcPolicy, noDNS}},
}

// noDNS makes the browser's own resolver fail for every name but the
// proxy's address (without the EXCLUDE the proxy itself can't be reached:
// ERR_PROXY_CONNECTION_FAILED). A test only: the resolver requests in the
// net log come from the network quality estimator and use only the host
// cache and the hosts file (task types 4 and 9), so no DNS leaves anyway.
const noDNS = "--host-resolver-rules=MAP * ~NOTFOUND, EXCLUDE 127.0.0.1"

const webrtcPolicy = "--force-webrtc-ip-handling-policy=disable_non_proxied_udp"

func partNet(ctx context.Context) {
	e, err := newEnv()
	if err != nil {
		p("setup: %v", err)
		return
	}
	p("## 2. Network rules (6.7)\n")
	p("Canaries: 127.0.0.1 and [::1] port %d, %s port %d (LAN), STUN/udp %s:%d and 127.0.0.1:%d, TURN/tcp %s:%d. The site is %s, a private-network exception.\n",
		e.C.Loop, "the LAN address", e.C.LanTCP, "LAN", e.C.STUN, e.C.STUNLoop, "LAN", e.C.TURN, siteName)
	proxyAP := netip.MustParseAddrPort(e.PAddr)
	for _, b := range browsers() {
		p("### %s\n", b.ID)
		for _, v := range variants {
			if *flagVariants != "" && !strings.Contains(","+*flagVariants+",", ","+strings.Fields(v.Name)[0]+",") {
				continue
			}
			runVariant(ctx, e, b, v, proxyAP)
		}
		if *flagVariants == "" {
			navTests(ctx, e, b, proxyAP)
		}
	}
}

func (e *env) setupVariant(b browser, v variant, name string) *AB {
	a := e.ab(b, name)
	a.Bypass = v.Bypass
	a.Args = v.Args
	if !v.Proxy {
		a.Proxy = ""
	}
	if v.Allowed {
		a.Allowed = []string{siteName}
	}
	if v.Strict {
		e.P.Approved = func(h string) bool { return h == siteName || h == "approved2.spike047.test" }
	} else {
		e.P.Approved = nil
	}
	a.Setup()
	return a
}

func runVariant(ctx context.Context, e *env, b browser, v variant, proxyAP netip.AddrPort) {
	a := e.setupVariant(b, v, "net")
	if !v.Allowed {
		a.Args = append(a.Args, "--log-net-log="+filepath.Join(a.Home, "netlog.json"))
		a.Setup()
	}
	url := e.S.Base() + "/net"
	if v.Direct {
		url = fmt.Sprintf("http://127.0.0.1:%d/net", e.S.Port)
	}
	e.C.Take()
	e.P.Reset()
	s := NewSampler(root, proxyAP)
	s.Start()
	r := a.Run(ctx, 60*time.Second, "open", url)
	time.Sleep(6 * time.Second)
	g := a.Run(ctx, 20*time.Second, "get", "text", "#out")
	s.Stop()
	a.Close(ctx)
	waitGone(10 * time.Second)
	time.Sleep(500 * time.Millisecond)
	hits := e.C.Take()
	entries := e.P.Reset()
	p("#### %s — %s\n", b.ID, v.Name)
	if !r.Success {
		p("open failed: %s\n", clip(strings.ReplaceAll(r.Error, "\n", " "), 300))
	}
	p("- **Got past the proxy (canary hits):** %s", hitList(hits))
	p("- Proxy, other hosts: %s", entryList(entries, true))
	p("- In-page results: `%s`", strings.ReplaceAll(strings.TrimSpace(g.field("text")), "\n", "` · `"))
	p("- Sockets of the tree outside the proxy: %s", orNone(s.Outside()))
	if !v.Allowed {
		nl := readNetLog(filepath.Join(a.Home, "netlog.json"))
		p("- Chrome's net log: names it resolved itself: %s; UDP sockets it connected: %s; QUIC sessions: %d\n", orNone(nl.Resolved), orNone(nl.UDP), nl.QUIC)
	} else {
		p("")
	}
}

func navTests(ctx context.Context, e *env, b browser, proxyAP netip.AddrPort) {
	v := variant{Name: "nav", Proxy: true, Bypass: true, Strict: true}
	a := e.setupVariant(b, v, "nav")
	loop := fmt.Sprintf("127.0.0.1:%d", e.C.Loop)
	lan := fmt.Sprintf("%s:%d", e.C.LAN, e.C.LanTCP)
	tests := []struct{ name, url string }{
		{"open a loopback URL", "http://" + loop + "/n1-open-loopback"},
		{"open a LAN URL", "http://" + lan + "/n1b-open-lan"},
		{"open a public name for loopback", fmt.Sprintf("http://localtest.me:%d/n1c-open-name", e.C.Loop)},
		{"a page redirects (302) to loopback", e.S.Base() + "/redirect?to=http://" + loop + "/n2-redirect-loopback"},
		{"a page redirects (302) to an unapproved public host", e.S.Base() + "/redirect?to=https://example.com/"},
		{"a page redirects (302) to an approved host", e.S.Base() + "/redirect?to=http://approved2.spike047.test:" + fmt.Sprint(e.S.Port) + "/"},
		{"script navigates to the LAN", e.S.Base() + "/jsnav?to=http://" + lan + "/n3-jsnav-lan"},
		{"script navigates to an unapproved public host", e.S.Base() + "/jsnav?to=https://example.com/"},
	}
	p("#### %s — top-level navigation (host approval on, <-loopback>)\n", b.ID)
	p("| Test | open | URL after | Canary hits | Proxy |")
	p("|---|---|---|---|---|")
	for _, t := range tests {
		e.C.Take()
		e.P.Reset()
		r := a.Run(ctx, 30*time.Second, "open", t.url)
		time.Sleep(1500 * time.Millisecond)
		u := a.Run(ctx, 10*time.Second, "get", "url")
		p("| %s | %v %s | %s | %s | %s |", t.name, r.Success, clip(strings.ReplaceAll(r.Error, "\n", " "), 80), clip(u.field("url"), 70), hitList(e.C.Take()), entryList(e.P.Reset(), false))
	}
	// agent-browser's own requests: read <url> runs in the CLI or daemon
	for _, u := range []string{"http://" + loop + "/r1-read-loopback", e.S.Base() + "/js/spa-inline"} {
		e.C.Take()
		e.P.Reset()
		r := a.Run(ctx, 20*time.Second, "read", u)
		p("| `read %s` | %v %s | — | %s | %s |", strings.Replace(u, e.C.LAN, "LAN", 1), r.Success, clip(strings.ReplaceAll(r.Error, "\n", " "), 80), hitList(e.C.Take()), entryList(e.P.Reset(), false))
	}
	a.Close(ctx)
	waitGone(10 * time.Second)
	p("")
}

func hitList(hs []Hit) string {
	if len(hs) == 0 {
		return "none"
	}
	seen := map[string]bool{}
	var out []string
	for _, h := range hs {
		k := h.Listener + " " + h.What
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return "**" + strings.Join(out, "; ") + "**"
}

func entryList(es []Entry, skipSite bool) string {
	seen := map[string]int{}
	var order []string
	for _, e := range es {
		if skipSite && e.Host == siteName && e.Decision == "allowed" {
			continue
		}
		k := fmt.Sprintf("%s %s:%s%s → %s", e.Kind, e.Host, e.Port, clip(e.Path, 40), e.Decision)
		if e.Decision != "allowed" {
			k += " (" + e.Reason + ")"
		}
		if seen[k] == 0 {
			order = append(order, k)
		}
		seen[k]++
	}
	if len(order) == 0 {
		return "none"
	}
	for i, k := range order {
		if seen[k] > 1 {
			order[i] = fmt.Sprintf("%s ×%d", k, seen[k])
		}
	}
	return strings.Join(order, "; ")
}

func orNone(s []string) string {
	if len(s) == 0 {
		return "none"
	}
	return strings.Join(s, "; ")
}

// netLog is what Chrome's own net log says about DNS, UDP and QUIC.
type netLog struct {
	Resolved []string
	UDP      []string
	QUIC     int
}

func readNetLog(path string) netLog {
	var nl netLog
	b, err := os.ReadFile(path)
	if err != nil {
		nl.Resolved = []string{"(no net log: " + err.Error() + ")"}
		return nl
	}
	s := strings.TrimSpace(string(b))
	if !strings.HasSuffix(s, "}") {
		s = strings.TrimRight(s, ",") + "]}"
	}
	var d struct {
		Constants struct {
			LogEventTypes map[string]int `json:"logEventTypes"`
		} `json:"constants"`
		Events []struct {
			Type   int            `json:"type"`
			Params map[string]any `json:"params"`
		} `json:"events"`
	}
	if err := json.Unmarshal([]byte(s), &d); err != nil {
		nl.Resolved = []string{"(net log unreadable: " + err.Error() + ")"}
		return nl
	}
	names := map[int]string{}
	for k, v := range d.Constants.LogEventTypes {
		names[v] = k
	}
	res, udp := map[string]bool{}, map[string]bool{}
	for _, e := range d.Events {
		t := names[e.Type]
		if strings.Contains(t, "HOST_RESOLVER") {
			if h, ok := e.Params["host"].(string); ok {
				res[h] = true
			}
		}
		if strings.HasPrefix(t, "UDP_") {
			if a, ok := e.Params["address"].(string); ok {
				udp[a] = true
			}
		}
		if t == "QUIC_SESSION" {
			nl.QUIC++
		}
	}
	for k := range res {
		nl.Resolved = append(nl.Resolved, k)
	}
	for k := range udp {
		nl.UDP = append(nl.UDP, k)
	}
	sort.Strings(nl.Resolved)
	sort.Strings(nl.UDP)
	return nl
}
