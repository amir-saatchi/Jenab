package provider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/amir-saatchi/jenab/internal/secret"
)

// Kind is the API a connection speaks (SPEC 3.9).
type Kind string

const (
	KindAnthropic  Kind = "anthropic"
	KindOpenAI     Kind = "openai"
	KindGemini     Kind = "gemini"
	KindCompatible Kind = "openai_compatible" // any base URL: Z.ai, Groq…
	KindOllama     Kind = "ollama"            // Ollama's native API, local or Ollama Cloud
)

// Kinds lists every kind.
var Kinds = []Kind{KindAnthropic, KindOpenAI, KindGemini, KindCompatible, KindOllama}

// DefaultBaseURL is the kind's official base URL, or "" if the user must
// give one.
func DefaultBaseURL(k Kind) string {
	switch k {
	case KindAnthropic:
		return "https://api.anthropic.com/"
	case KindOpenAI:
		return "https://api.openai.com/v1/"
	case KindGemini:
		return "https://generativelanguage.googleapis.com/v1beta/"
	case KindOllama:
		return "http://localhost:11434/"
	}
	return ""
}

// Connection is what a backend is built from.
type Connection struct {
	Name    string // the user's name for it, e.g. "gemini" or "zai"
	Kind    Kind
	BaseURL string       // never empty when a backend gets it
	Key     secret.Value // empty for a local Ollama
	// Redact removes every known secret from a text (secret.Store.Redact).
	Redact func(string) string
	// HTTP is the client to use; nil means http.DefaultClient's transport.
	// Tests pass a test server's client.
	HTTP *http.Client
}

// Factory builds a backend for a connection.
type Factory func(Connection) (Provider, error)

// CheckBaseURL accepts an https URL, or http for this machine only, so a
// key never travels in clear text over a network. Its errors show the URL
// without its user, query and fragment, which may hold a key.
func CheckBaseURL(raw string) (*url.URL, error) { return checkBaseURL(raw, nil) }

// checkBaseURL is CheckBaseURL with hide also removing secrets from the
// URL shown in its errors.
func checkBaseURL(raw string, hide func(string) string) (*url.URL, error) {
	show := showURL(raw)
	if hide != nil {
		show = hide(show)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("provider: %w: %q is not a URL", ErrBadBaseURL, show)
	}
	switch {
	case u.Scheme == "https":
	case u.Scheme == "http" && isLoopback(u.Hostname()):
	default:
		return nil, fmt.Errorf("provider: %w: %q must use https (http only for this machine)", ErrBadBaseURL, show)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("provider: %w: %q must not hold a user, query or fragment", ErrBadBaseURL, show)
	}
	return u, nil
}

// showURL is raw for an error message: without its user, query and
// fragment. It works on text, so a URL that doesn't parse is cut too.
func showURL(raw string) string {
	s, _, _ := strings.Cut(raw, "#")
	s, _, _ = strings.Cut(s, "?")
	scheme, rest, ok := strings.Cut(s, "://")
	if !ok {
		rest = s
	}
	if i := strings.LastIndex(rest, "@"); i >= 0 {
		rest = rest[i+1:]
	}
	if ok {
		return scheme + "://" + rest
	}
	return rest
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Guard is the HTTP check every backend runs on each request (SPEC 3.8,
// 6.7):
//   - a request goes only to the connection's own scheme and host;
//   - only allowed headers leave, so the SDKs' OPENAI_* environment
//     variables and anything else never reach another host;
//   - an error answer's body is kept, redacted, for Classify.
type Guard struct {
	Scheme, Host string
	Allow        []string // more allowed header names, lower case (e.g. "x-api-key")
	Redact       func(string) string
}

// NewGuard returns the guard for c's base URL.
func NewGuard(c Connection, allow ...string) (*Guard, error) {
	u, err := CheckBaseURL(c.BaseURL)
	if err != nil {
		return nil, err
	}
	r := c.Redact
	if r == nil {
		r = func(s string) string { return s }
	}
	inner, key := r, c.Key
	r = func(s string) string { return key.Redact(inner(s)) }
	return &Guard{Scheme: u.Scheme, Host: u.Host, Allow: allow, Redact: r}, nil
}

// baseHeaders are the headers any backend may send.
var baseHeaders = []string{"authorization", "content-type", "accept", "user-agent", "content-length", "idempotency-key", "accept-encoding"}

func (g *Guard) allowed(name string) bool {
	name = strings.ToLower(name)
	for _, a := range baseHeaders {
		if name == a {
			return true
		}
	}
	for _, a := range g.Allow {
		if name == a {
			return true
		}
	}
	return strings.HasPrefix(name, "x-stainless-") // the SDKs' own client info
}

// Do checks req, sends it with next and keeps an error body.
func (g *Guard) Do(req *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	if req.URL.Scheme != g.Scheme || req.URL.Host != g.Host {
		return nil, fmt.Errorf("provider: refused a request to %s://%s; this connection talks only to %s://%s", req.URL.Scheme, req.URL.Host, g.Scheme, g.Host)
	}
	if req.URL.User != nil {
		return nil, fmt.Errorf("provider: refused a URL with a user")
	}
	for k := range req.Header {
		if !g.allowed(k) {
			req.Header.Del(k)
		}
	}
	resp, err := next(req)
	if err != nil {
		return resp, err
	}
	if c := captureOf(req.Context()); c != nil {
		c.mu.Lock()
		c.Status = resp.StatusCode
		c.Header = http.Header{}
		for _, k := range []string{"retry-after", "retry-after-ms"} {
			if v := resp.Header.Get(k); v != "" {
				c.Header.Set(k, v)
			}
		}
		if resp.StatusCode >= 400 {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
			resp.Body.Close()
			resp.Body = io.NopCloser(bytes.NewReader(b))
			c.Body = g.Redact(string(b))
		}
		c.mu.Unlock()
	}
	return resp, nil
}

// Fail turns a failed request into an *Error: an error answer the Guard
// saw is classified by its status and body; anything else is a transport
// error. The SDK's error is dropped for error answers, since it holds the
// request and its headers.
func (g *Guard) Fail(provider string, c *Capture, err error) *Error {
	if status, h, body := c.Snapshot(); status >= 400 {
		return Classify(provider, status, h, body)
	}
	return TransportError(provider, shorten(g.Redact(err.Error()), 600), err)
}

// Capture holds what Guard saw of one request's answer.
type Capture struct {
	mu     sync.Mutex
	Status int
	Header http.Header // retry-after headers only
	Body   string      // an error answer's body, redacted
}

type captureKey struct{}

// WithCapture returns a context whose requests record their answer in c.
func WithCapture(ctx context.Context) (context.Context, *Capture) {
	c := &Capture{}
	return context.WithValue(ctx, captureKey{}, c), c
}

func captureOf(ctx context.Context) *Capture {
	c, _ := ctx.Value(captureKey{}).(*Capture)
	return c
}

// Snapshot returns the status, headers and body seen.
func (c *Capture) Snapshot() (int, http.Header, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Status, c.Header, c.Body
}

// FieldName is the keychain name of the value for a base URL placeholder,
// e.g. "provider:cloudflare:account_id".
func FieldName(provider, field string) string { return "provider:" + provider + ":" + field }

var (
	rePlaceholder = regexp.MustCompile(`\{([a-z_]+)\}`)
	reFieldValue  = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
)

// Placeholders lists the {name} placeholders in a base URL, such as
// Cloudflare's {account_id}. Their values are kept in the keychain, so the
// settings file holds none of them and errors redact them like keys.
func Placeholders(baseURL string) []string {
	var out []string
	for _, m := range rePlaceholder.FindAllStringSubmatch(baseURL, -1) {
		out = append(out, m[1])
	}
	return out
}

// fill replaces each placeholder with its value from get. A value must be
// one plain path segment, so it can't change the host or the path.
func fill(baseURL string, get func(field string) (string, error)) (string, error) {
	var err error
	out := rePlaceholder.ReplaceAllStringFunc(baseURL, func(m string) string {
		field := m[1 : len(m)-1]
		v, e := get(field)
		switch {
		case e != nil:
			err = fmt.Errorf("no %s is stored for this provider: %w", field, e)
		case !reFieldValue.MatchString(v):
			err = fmt.Errorf("the %s must be letters, digits, - or _", field)
		}
		return v
	})
	if err != nil {
		return "", err
	}
	return out, nil
}
