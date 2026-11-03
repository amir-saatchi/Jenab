// Package web fetches web pages as readable text, under the network rules
// of SPEC 6.7. Search and feeds come in Phase 4.
package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html/charset"
	"golang.org/x/net/idna"

	"github.com/amir-saatchi/jenab/internal/appinfo"
)

// MaxBody caps a response body (SPEC 6.6); the rest is not read.
const MaxBody = 5 << 20

// maxRedirects bounds the redirects a fetch follows.
const maxRedirects = 10

// HostCheck reports whether host may reach private addresses: the
// exceptions in the project settings (SPEC 6.7).
type HostCheck func(host string) bool

// RedirectCheck is asked before a fetch follows a redirect to another
// host, such as one the project hasn't approved (SPEC 8.8). An error stops
// the fetch with a *RedirectError.
type RedirectCheck func(ctx context.Context, to *url.URL) error

// PrivateRequest is a connection to private addresses that the
// exceptions don't allow, for a PrivateAsk.
type PrivateRequest struct {
	Host  string       // as Host spells it
	Addrs []netip.Addr // the private addresses it resolved to
	// Name: the host is a name, so a lookup gave the addresses.
	Name bool
	// Local: every address is on this computer, such as loopback; else
	// they are on the local network.
	Local bool
	From  string // the host that redirected here; "" for the URL's own
}

// PrivateAsk is asked before a fetch connects to private addresses that
// the exceptions don't allow (SPEC 6.7, 8.8). nil lets this fetch connect
// to those addresses; an error refuses them, and the fetch returns it in
// a *BlockedError.
type PrivateAsk func(ctx context.Context, r PrivateRequest) error

// Rules are one fetch's project rules.
type Rules struct {
	Private HostCheck // hosts that may reach private addresses; nil allows none
	// AskPrivate is asked about the other hosts that resolve to private
	// addresses; nil blocks them.
	AskPrivate PrivateAsk
	Redirect   RedirectCheck // nil follows redirects to any host
}

// Resolver finds a host's addresses. net.DefaultResolver is one; tests use
// a fake.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Options configure a Client.
type Options struct {
	Resolver  Resolver // nil: net.DefaultResolver
	UserAgent string   // "" means DefaultUserAgent
}

// DefaultUserAgent names the app, with a link to it.
const DefaultUserAgent = "Mozilla/5.0 (compatible; " + appinfo.Name + "; +https://github.com/amir-saatchi/Jenab)"

// Client is one HTTP client with the network rules: http and https only,
// no proxy, and no private addresses unless the host is an exception,
// checked on the address it connects to, so on every redirect and after
// DNS.
type Client struct {
	hc       *http.Client // pooled, with no exceptions
	resolver Resolver
	agent    string
}

// NewClient returns a Client.
func NewClient(o Options) *Client {
	c := &Client{resolver: o.Resolver, agent: o.UserAgent}
	if c.resolver == nil {
		c.resolver = net.DefaultResolver
	}
	if c.agent == "" {
		c.agent = DefaultUserAgent
	}
	c.hc = &http.Client{Transport: c.transport(nil, nil)}
	return c
}

func (c *Client) transport(allow HostCheck, f *fetchState) *http.Transport {
	return &http.Transport{
		Proxy:                 nil, // a proxy would make the address checks meaningless
		DialContext:           c.dialer(allow, f),
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
	}
}

// client returns the HTTP client for a fetch with these rules. With
// exceptions or a PrivateAsk, its transport is a new one, closed after
// the fetch, so a connection opened under one project's exception or one
// fetch's answer is never reused without it.
func (c *Client) client(ctx context.Context, r Rules) (*http.Client, func()) {
	if r.Private == nil && r.AskPrivate == nil {
		return &http.Client{Transport: c.hc.Transport, CheckRedirect: checkRedirect(r.Redirect, nil)}, func() {}
	}
	fctx, cancel := context.WithCancel(ctx)
	f := &fetchState{ctx: fctx, ask: r.AskPrivate, from: map[string]string{}, allowed: map[string][]netip.Addr{}}
	t := c.transport(r.Private, f)
	return &http.Client{Transport: t, CheckRedirect: checkRedirect(r.Redirect, f)}, func() {
		cancel()
		f.end()
		t.CloseIdleConnections()
	}
}

// fetchState is one fetch's answers about private addresses.
type fetchState struct {
	// ctx is the fetch's, cancelled when it returns: a dial's own is
	// detached from the request, so Stop wouldn't end the question.
	ctx context.Context
	ask PrivateAsk

	mu      sync.Mutex
	from    map[string]string       // a host the fetch was redirected to → the host before it
	allowed map[string][]netip.Addr // private addresses this fetch may connect to, by host
	ended   bool
	asking  sync.WaitGroup
}

// begin starts a question; false once the fetch has returned. The HTTP
// client returns on Stop without waiting for its dial, so end waits for
// the question: its card is closed before the fetch returns.
func (f *fetchState) begin() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ended {
		return false
	}
	f.asking.Add(1)
	return true
}

func (f *fetchState) end() {
	f.mu.Lock()
	f.ended = true
	f.mu.Unlock()
	f.asking.Wait()
}

// question asks f.ask about r, if the fetch is still running.
func (f *fetchState) question(r PrivateRequest) error {
	if !f.begin() {
		return context.Canceled
	}
	defer f.asking.Done()
	return f.ask(f.ctx, r)
}

func (f *fetchState) redirected(to, from string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.from[to]; !ok {
		f.from[to] = from
	}
}

func (f *fetchState) cameFrom(host string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.from[host]
}

// mayConnect reports whether this fetch's answer allowed host to reach a.
// The answer holds for the addresses it named, so a name that resolves
// to another one later asks again.
func (f *fetchState) mayConnect(host string, a netip.Addr) bool {
	if f == nil {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.allowed[host], a.Unmap())
}

func (f *fetchState) allow(host string, as []netip.Addr) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.allowed[host] = append(f.allowed[host], as...)
}

// BlockedError is a request the network rules refused.
type BlockedError struct {
	URL    string
	Reason string
	// Private: it went to a private address that the host's exceptions
	// don't allow (6.7).
	Private bool
}

func (e *BlockedError) Error() string { return "web: " + e.URL + " is blocked: " + e.Reason }

// StatusError is a response that wasn't a success.
type StatusError struct {
	URL    string
	Status int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("web: %s returned %d %s", e.URL, e.Status, http.StatusText(e.Status))
}

// RedirectError is a redirect that the fetch's RedirectCheck refused.
type RedirectError struct {
	URL string // where the redirect goes
	Err error
}

func (e *RedirectError) Error() string { return "web: redirected to " + e.URL + ": " + e.Err.Error() }
func (e *RedirectError) Unwrap() error { return e.Err }

// ErrNotPage is returned for content that isn't a page or text, such as a
// PDF or an image.
var ErrNotPage = errors.New("web: not a web page or text")

// CheckURL accepts absolute http and https URLs with a host.
func CheckURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("web: bad URL %q: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, &BlockedError{URL: raw, Reason: "only http and https URLs are allowed"}
	}
	h := Host(u)
	if h == "" {
		return nil, fmt.Errorf("web: URL %q has no host", raw)
	}
	if u.User != nil {
		return nil, &BlockedError{URL: raw, Reason: "URLs with a user name or password are not allowed"}
	}
	if _, err := netip.ParseAddr(h); err != nil && numeric(h) {
		return nil, &BlockedError{URL: raw, Reason: "a numeric host must be an IPv4 address in four dotted parts"}
	}
	return u, nil
}

// Host is u's host in one spelling, so a host is approved or revoked
// under one name: lower case, ASCII (punycode), without trailing dots, and
// an IP address in its standard form.
func Host(u *url.URL) string { return canonical(u.Hostname()) }

func canonical(h string) string {
	if a, err := netip.ParseAddr(h); err == nil {
		return a.Unmap().String()
	}
	h = strings.ToLower(h)
	if a, err := idna.Lookup.ToASCII(h); err == nil {
		h = a
	}
	h = strings.TrimRight(h, ".")
	if a, err := netip.ParseAddr(h); err == nil {
		return a.Unmap().String()
	}
	return h
}

// numeric reports whether host h ends in a number, such as 16843009,
// 0x01010101 or 1.1, which resolvers may read as an IPv4 address.
func numeric(h string) bool {
	last := h[strings.LastIndexByte(h, '.')+1:]
	digits := "0123456789"
	if x, ok := strings.CutPrefix(last, "0x"); ok {
		if x == "" {
			return true
		}
		last, digits = x, "0123456789abcdef"
	}
	return last != "" && strings.Trim(last, digits) == ""
}

// checkRedirect checks each redirect as a new URL, and asks check about
// one to another host. f, if set, learns where each host came from.
func checkRedirect(check RedirectCheck, f *fetchState) func(req *http.Request, via []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) > maxRedirects { // this request is redirect number len(via)
			return fmt.Errorf("web: more than %d redirects", maxRedirects)
		}
		if _, err := CheckURL(req.URL.String()); err != nil {
			return err
		}
		to, from := Host(req.URL), Host(via[len(via)-1].URL)
		if to == from {
			return nil
		}
		if check != nil {
			if err := check(req.Context(), req.URL); err != nil {
				return &RedirectError{URL: req.URL.String(), Err: err}
			}
		}
		if f != nil {
			f.redirected(to, from)
		}
		return nil
	}
}

// dialer connects to one of the host's addresses that the rules allow.
// The check is on the address itself, so a name that resolves to a private
// address is caught, and so is a redirect to one. A private address the
// exceptions don't allow is asked about (f.ask), after the others are
// tried; the answer holds for the addresses asked about, which are the
// ones it connects to.
func (c *Client) dialer(allow HostCheck, f *fetchState) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		var addrs []netip.Addr
		a, err := netip.ParseAddr(host)
		name := err != nil
		if !name {
			addrs = []netip.Addr{a}
		} else {
			addrs, err = c.resolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
		}
		h := canonical(host)
		exception := allow != nil && allow(h)
		var d net.Dialer
		var lastErr error
		var held []netip.Addr // private, and not allowed yet
		dial := func(a netip.Addr) (net.Conn, error) {
			return d.DialContext(ctx, network, net.JoinHostPort(a.Unmap().String(), port))
		}
		for _, a := range addrs {
			if why := blocked(a); why != "" && !exception && !f.mayConnect(h, a) {
				held = append(held, a.Unmap())
				lastErr = &BlockedError{URL: host, Reason: why + " (" + a.Unmap().String() + ")", Private: true}
				continue
			}
			conn, err := dial(a)
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		if len(held) > 0 && f != nil && f.ask != nil {
			r := PrivateRequest{Host: h, Addrs: held, Name: name, Local: true, From: f.cameFrom(h)}
			for _, a := range held {
				r.Local = r.Local && onThisComputer(a)
			}
			if err := f.question(r); err != nil {
				if f.ctx.Err() != nil {
					return nil, err
				}
				return nil, &BlockedError{URL: host, Reason: blocked(held[0]) + " (" + held[0].String() + "): " + err.Error(), Private: true}
			}
			f.allow(h, held)
			for _, a := range held {
				conn, err := dial(a)
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("web: %s has no addresses", host)
		}
		return nil, lastErr
	}
}

// ParseHost checks a host for the exceptions (6.7): a name or an address,
// without a scheme, path or port. It returns the host as Host spells it.
// An IPv6 address may be in brackets.
func ParseHost(s string) (string, error) {
	s = strings.TrimSpace(s)
	bad := func(why string) (string, error) { return "", fmt.Errorf("web: %q is not a host: %s", s, why) }
	if s == "" {
		return bad("it is empty")
	}
	if strings.ContainsAny(s, "/\\?#@ \t") {
		return bad("give the name or address alone, without a scheme or path")
	}
	inner := s
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		inner = s[1 : len(s)-1]
		if a, err := netip.ParseAddr(inner); err != nil || !a.Is6() {
			return bad("brackets hold an IPv6 address")
		}
	}
	if a, err := netip.ParseAddr(inner); err == nil {
		if a.Zone() != "" {
			return bad("an address with a zone isn't allowed")
		}
		return a.Unmap().String(), nil
	}
	if strings.Contains(s, ":") {
		return bad("give it without a port")
	}
	// As canonical spells it; an ASCII name that IDNA refuses, such as
	// one with "_", stays as it is, and the label check decides.
	h := strings.TrimRight(strings.ToLower(s), ".")
	if a, err := idna.Lookup.ToASCII(h); err == nil {
		h = a
	} else if strings.ContainsFunc(h, func(r rune) bool { return r >= utf8.RuneSelf }) {
		return bad("it isn't a valid name")
	}
	if h == "" || len(h) > 253 {
		return bad("it isn't a valid name")
	}
	if numeric(h) {
		return bad("a numeric host must be an IPv4 address in four dotted parts")
	}
	for _, l := range strings.Split(h, ".") {
		if l == "" || len(l) > 63 || strings.Trim(l, "abcdefghijklmnopqrstuvwxyz0123456789-_") != "" {
			return bad("it isn't a valid name")
		}
	}
	return h, nil
}

// onThisComputer reports whether a reaches this computer: a loopback or
// unspecified address, or one carried in an IPv6 address.
func onThisComputer(a netip.Addr) bool {
	a = a.Unmap()
	if v4 := carried(a); v4.IsValid() && onThisComputer(v4) {
		return true
	}
	return a.IsLoopback() || a.IsUnspecified() || thisNet.Contains(a)
}

// carried is the IPv4 address that an IPv6 address carries, as NAT64,
// 6to4 and IPv4-compatible addresses do; not valid if none.
func carried(a netip.Addr) netip.Addr {
	if !a.Is6() {
		return netip.Addr{}
	}
	b := a.As16()
	switch {
	case nat64.Contains(a), compat.Contains(a) && !a.IsLoopback() && !a.IsUnspecified():
		return netip.AddrFrom4([4]byte(b[12:16]))
	case sixTo4.Contains(a):
		return netip.AddrFrom4([4]byte(b[2:6]))
	}
	return netip.Addr{}
}

var (
	shared   = netip.MustParsePrefix("100.64.0.0/10") // carrier-grade NAT
	thisNet  = netip.MustParsePrefix("0.0.0.0/8")
	nat64    = netip.MustParsePrefix("64:ff9b::/96")
	nat64Own = netip.MustParsePrefix("64:ff9b:1::/48") // local-use NAT64
	sixTo4   = netip.MustParsePrefix("2002::/16")
	compat   = netip.MustParsePrefix("::/96") // IPv4-compatible
)

// blocked says why a is not allowed, or "" if it is. An IPv6 address that
// carries an IPv4 one, as NAT64, 6to4 and IPv4-compatible addresses do, is
// also checked by that one.
func blocked(a netip.Addr) string {
	a = a.Unmap()
	if v4 := carried(a); v4.IsValid() {
		if why := blocked(v4); why != "" {
			return why + " inside an IPv6 address"
		}
	}
	switch {
	case !a.IsValid():
		return "not an address"
	case a.IsLoopback():
		return "a loopback address"
	case a.IsPrivate():
		return "a private address"
	case a.IsLinkLocalUnicast(), a.IsLinkLocalMulticast():
		return "a link-local address"
	case a.IsUnspecified():
		return "an unspecified address"
	case a.IsMulticast(), a.IsInterfaceLocalMulticast():
		return "a multicast address"
	case shared.Contains(a):
		return "a shared (carrier-grade NAT) address"
	case thisNet.Contains(a), nat64Own.Contains(a):
		return "a reserved address"
	}
	return ""
}

// Page is a fetched page as readable text.
type Page struct {
	URL       string // after redirects
	Title     string
	Text      string // Markdown for HTML pages, else the text as sent
	MIME      string // the response's type, without parameters
	Truncated bool   // the body was cut at MaxBody
	// NeedsJavaScript is set for an HTML page with almost no text without
	// JavaScript, which v1 doesn't run (SPEC 3.7).
	NeedsJavaScript bool
}

// minText is the text length below which an HTML page needs JavaScript.
const minText = 200

// Fetch gets rawURL and turns it into readable text (SPEC 3.7), under the
// project rules r.
func (c *Client) Fetch(ctx context.Context, rawURL string, r Rules) (Page, error) {
	u, err := CheckURL(rawURL)
	if err != nil {
		return Page{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Page{}, err
	}
	req.Header.Set("User-Agent", c.agent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.5")
	hc, done := c.client(ctx, r)
	defer done()
	resp, err := hc.Do(req)
	if err != nil {
		var b *BlockedError
		if errors.As(err, &b) {
			return Page{}, b
		}
		var re *RedirectError
		if errors.As(err, &re) {
			return Page{}, re
		}
		return Page{}, err
	}
	defer resp.Body.Close()
	final := resp.Request.URL
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Page{}, &StatusError{URL: final.String(), Status: resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBody+1))
	if err != nil {
		return Page{}, err
	}
	p := Page{URL: final.String()}
	if len(body) > MaxBody {
		body, p.Truncated = body[:MaxBody], true
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = http.DetectContentType(body)
	}
	p.MIME, _, _ = mime.ParseMediaType(ct)
	if p.MIME == "" {
		p.MIME = "application/octet-stream"
	}
	kind := kindOf(p.MIME)
	if kind == notText {
		return Page{}, fmt.Errorf("%w: %s is %s", ErrNotPage, p.URL, p.MIME)
	}
	text, err := decode(body, ct)
	if err != nil {
		return Page{}, err
	}
	if kind == plain {
		p.Text = string(text)
		return p, nil
	}
	p.Title, p.Text, err = readable(text, final)
	if err != nil {
		return Page{}, err
	}
	p.NeedsJavaScript = len([]rune(strings.TrimSpace(p.Text))) < minText
	return p, nil
}

type textKind int

const (
	notText textKind = iota
	plain
	htmlPage
)

func kindOf(m string) textKind {
	switch {
	case m == "text/html", m == "application/xhtml+xml":
		return htmlPage
	case strings.HasPrefix(m, "text/"), m == "application/json", m == "application/xml",
		strings.HasSuffix(m, "+json"), strings.HasSuffix(m, "+xml"):
		return plain
	}
	return notText
}

// decode turns the body into UTF-8 before any library sees it (SPEC 3.7):
// the libraries guess charsets and turn pages such as windows-1256 into
// garbage. The charset comes from the byte-order mark, the Content-Type
// header, then <meta>, as browsers do. Bytes that aren't valid in the
// charset become U+FFFD.
func decode(body []byte, contentType string) ([]byte, error) {
	if len(body) == 0 {
		return body, nil
	}
	r, err := charset.NewReader(bytes.NewReader(body), contentType)
	if err != nil {
		return nil, fmt.Errorf("web: decoding the page: %w", err)
	}
	text, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("web: decoding the page: %w", err)
	}
	return bytes.ToValidUTF8(text, []byte(string(utf8.RuneError))), nil
}
