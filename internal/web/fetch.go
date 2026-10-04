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
)

// MaxBody caps a response body (SPEC 6.6); the rest is not read.
const MaxBody = 5 << 20

// maxRedirects bounds the redirects a fetch follows.
const maxRedirects = 10

// HostCheck reports whether host may reach private addresses: the
// exceptions in the project settings (SPEC 6.7).
type HostCheck func(host string) bool

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
	c.hc = &http.Client{Transport: c.transport(nil), CheckRedirect: checkRedirect}
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

// client returns the HTTP client for a fetch with these exceptions. With
// some, it is a new one, closed after the fetch, so a connection opened
// under one project's exception is never reused without it.
func (c *Client) client(allow HostCheck) (*http.Client, func()) {
	if allow == nil {
		return c.hc, func() {}
	}
	t := c.transport(allow)
	return &http.Client{Transport: t, CheckRedirect: checkRedirect}, t.CloseIdleConnections
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
	if u.Hostname() == "" {
		return nil, fmt.Errorf("web: URL %q has no host", raw)
	}
	if u.User != nil {
		return nil, &BlockedError{URL: raw, Reason: "URLs with a user name or password are not allowed"}
	}
	return u, nil
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > maxRedirects { // this request is redirect number len(via)
		return fmt.Errorf("web: more than %d redirects", maxRedirects)
	}
	_, err := CheckURL(req.URL.String())
	return err
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
		exception := allow != nil && allow(host)
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

// blocked says why a is not allowed, or "" if it is.
func blocked(a netip.Addr) string {
	a = a.Unmap()
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

// Fetch gets rawURL and turns it into readable text (SPEC 3.7). allow
// names the hosts that may reach private addresses; nil allows none.
func (c *Client) Fetch(ctx context.Context, rawURL string, allow HostCheck) (Page, error) {
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
	hc, done := c.client(allow)
	defer done()
	resp, err := hc.Do(req)
	if err != nil {
		var b *BlockedError
		if errors.As(err, &b) {
			return Page{}, b
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
