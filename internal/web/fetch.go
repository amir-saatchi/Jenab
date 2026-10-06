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
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html/charset"
	"golang.org/x/net/idna"
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

// Rules are one fetch's project rules.
type Rules struct {
	Private  HostCheck     // hosts that may reach private addresses; nil allows none
	Redirect RedirectCheck // nil follows redirects to any host
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
const DefaultUserAgent = "Mozilla/5.0 (compatible; Jenab; +https://github.com/amir-saatchi/Jenab)"

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
	c.hc = &http.Client{Transport: c.transport(nil)}
	return c
}

func (c *Client) transport(allow HostCheck) *http.Transport {
	return &http.Transport{
		Proxy:                 nil, // a proxy would make the address checks meaningless
		DialContext:           c.dialer(allow),
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
	}
}

// client returns the HTTP client for a fetch with these rules. With
// exceptions, its transport is a new one, closed after the fetch, so a
// connection opened under one project's exception is never reused without
// it.
func (c *Client) client(r Rules) (*http.Client, func()) {
	hc := &http.Client{Transport: c.hc.Transport, CheckRedirect: checkRedirect(r.Redirect)}
	if r.Private == nil {
		return hc, func() {}
	}
	t := c.transport(r.Private)
	hc.Transport = t
	return hc, t.CloseIdleConnections
}

// BlockedError is a request the network rules refused.
type BlockedError struct {
	URL    string
	Reason string
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
// one to another host.
func checkRedirect(check RedirectCheck) func(req *http.Request, via []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) > maxRedirects { // this request is redirect number len(via)
			return fmt.Errorf("web: more than %d redirects", maxRedirects)
		}
		if _, err := CheckURL(req.URL.String()); err != nil {
			return err
		}
		if check != nil && Host(req.URL) != Host(via[len(via)-1].URL) {
			if err := check(req.Context(), req.URL); err != nil {
				return &RedirectError{URL: req.URL.String(), Err: err}
			}
		}
		return nil
	}
}

// dialer connects to one of the host's addresses that the rules allow.
// The check is on the address itself, so a name that resolves to a private
// address is caught, and so is a redirect to one.
func (c *Client) dialer(allow HostCheck) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		var addrs []netip.Addr
		if a, err := netip.ParseAddr(host); err == nil {
			addrs = []netip.Addr{a}
		} else {
			addrs, err = c.resolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
		}
		exception := allow != nil && allow(canonical(host))
		var d net.Dialer
		var lastErr error
		for _, a := range addrs {
			if why := blocked(a); why != "" && !exception {
				lastErr = &BlockedError{URL: host, Reason: why + " (" + a.Unmap().String() + ")"}
				continue
			}
			conn, err := d.DialContext(ctx, network, net.JoinHostPort(a.Unmap().String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("web: %s has no addresses", host)
		}
		return nil, lastErr
	}
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
	if a.Is6() {
		b := a.As16()
		var v4 netip.Addr
		switch {
		case nat64.Contains(a), compat.Contains(a) && !a.IsLoopback() && !a.IsUnspecified():
			v4 = netip.AddrFrom4([4]byte(b[12:16]))
		case sixTo4.Contains(a):
			v4 = netip.AddrFrom4([4]byte(b[2:6]))
		}
		if v4.IsValid() {
			if why := blocked(v4); why != "" {
				return why + " inside an IPv6 address"
			}
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
	hc, done := c.client(r)
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
