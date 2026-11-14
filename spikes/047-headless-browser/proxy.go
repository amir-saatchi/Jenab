package main

// A forward proxy that applies SPEC 6.7 to every request the browser
// makes: host approval, and the private-network check on the address it
// connects to, after DNS. Plain http requests (and WebSocket upgrades) come
// as absolute URLs, https as CONNECT host:port. Where Jenab would ask, the
// proxy records the question and denies, as a Deny on the card would.

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// Entry is one request the proxy saw.
type Entry struct {
	At       time.Duration // since the proxy's log was reset
	Kind     string        // CONNECT, GET, POST, UPGRADE…
	Host     string
	Port     string
	Path     string // "" for CONNECT
	Decision string // allowed, ask-host, ask-private, refused
	Reason   string
	Addr     string // the address it connected to
}

type Proxy struct {
	// Approved reports whether a host is approved for the project; nil
	// approves every host (only the private-network rule applies).
	Approved func(host string) bool
	// Exceptions are the project's private-network exceptions.
	Exceptions map[string]bool
	// Fake maps test names to addresses, in place of DNS.
	Fake map[string]netip.Addr

	ln    net.Listener
	mu    sync.Mutex
	log   []Entry
	start time.Time
}

func (p *Proxy) Start() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	p.ln = ln
	p.start = time.Now()
	go http.Serve(ln, p)
	return ln.Addr().String(), nil
}

func (p *Proxy) Reset() []Entry {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := p.log
	p.log = nil
	p.start = time.Now()
	return out
}

func (p *Proxy) Entries() []Entry {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Entry(nil), p.log...)
}

func (p *Proxy) add(e Entry) {
	p.mu.Lock()
	e.At = time.Since(p.start)
	p.log = append(p.log, e)
	p.mu.Unlock()
}

// check applies 6.7 and returns the address to dial.
func (p *Proxy) check(ctx context.Context, e *Entry) (netip.Addr, bool) {
	h := strings.TrimSuffix(strings.ToLower(e.Host), ".")
	h = strings.TrimSuffix(strings.TrimPrefix(h, "["), "]")
	if p.Approved != nil && !p.Approved(h) {
		e.Decision, e.Reason = "ask-host", "host not approved for the project"
		return netip.Addr{}, false
	}
	var addrs []netip.Addr
	name := false
	if a, err := netip.ParseAddr(h); err == nil {
		addrs = []netip.Addr{a}
	} else if a, ok := p.Fake[h]; ok {
		addrs, name = []netip.Addr{a}, true
	} else {
		name = true
		lctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		as, err := net.DefaultResolver.LookupNetIP(lctx, "ip", h)
		cancel()
		if err != nil || len(as) == 0 {
			e.Decision, e.Reason = "refused", fmt.Sprintf("lookup failed: %v", err)
			return netip.Addr{}, false
		}
		addrs = as
	}
	for _, a := range addrs {
		a = a.Unmap()
		if why := blocked(a); why != "" && !p.Exceptions[h] {
			e.Decision = "ask-private"
			e.Reason = why + " (" + a.String() + ")"
			if name {
				e.Reason += ", a name that resolves to it"
			}
			return netip.Addr{}, false
		}
	}
	e.Decision = "allowed"
	e.Addr = addrs[0].Unmap().String()
	return addrs[0].Unmap(), true
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.connect(w, r)
		return
	}
	e := Entry{Kind: r.Method, Host: r.URL.Hostname(), Port: r.URL.Port(), Path: r.URL.RequestURI()}
	if e.Port == "" {
		e.Port = "80"
	}
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		e.Kind = "UPGRADE"
	}
	if r.URL.Scheme != "http" {
		e.Decision, e.Reason = "refused", "scheme "+r.URL.Scheme
		p.add(e)
		http.Error(w, "only http and https", http.StatusBadRequest)
		return
	}
	a, ok := p.check(r.Context(), &e)
	p.add(e)
	if !ok {
		http.Error(w, "blocked by the network rules: "+e.Reason, http.StatusForbidden)
		return
	}
	dst := net.JoinHostPort(a.String(), e.Port)
	if e.Kind == "UPGRADE" {
		p.tunnelRequest(w, r, dst)
		return
	}
	t := &http.Transport{
		Proxy:             nil,
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, dst)
		},
	}
	out := r.Clone(r.Context())
	out.RequestURI = ""
	for _, k := range []string{"Proxy-Connection", "Proxy-Authorization", "Connection", "Keep-Alive", "Te", "Trailer", "Transfer-Encoding"} {
		out.Header.Del(k)
	}
	resp, err := t.RoundTrip(out)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// tunnelRequest sends an upgrade request on and joins the two connections.
func (p *Proxy) tunnelRequest(w http.ResponseWriter, r *http.Request, dst string) {
	up, err := net.DialTimeout("tcp", dst, 10*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	out := r.Clone(r.Context())
	out.Header.Del("Proxy-Connection")
	if err := out.Write(up); err != nil {
		up.Close()
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		up.Close()
		return
	}
	c, buf, err := hj.Hijack()
	if err != nil {
		up.Close()
		return
	}
	if buf.Reader.Buffered() > 0 {
		b, _ := buf.Reader.Peek(buf.Reader.Buffered())
		up.Write(b)
	}
	pipe(c, up)
}

func (p *Proxy) connect(w http.ResponseWriter, r *http.Request) {
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil {
		host, port = r.Host, "443"
	}
	e := Entry{Kind: "CONNECT", Host: host, Port: port}
	a, ok := p.check(r.Context(), &e)
	p.add(e)
	if !ok {
		http.Error(w, "blocked by the network rules: "+e.Reason, http.StatusForbidden)
		return
	}
	up, err := net.DialTimeout("tcp", net.JoinHostPort(a.String(), port), 10*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		up.Close()
		return
	}
	c, buf, err := hj.Hijack()
	if err != nil {
		up.Close()
		return
	}
	c.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if buf.Reader.Buffered() > 0 {
		b, _ := buf.Reader.Peek(buf.Reader.Buffered())
		up.Write(b)
	}
	pipe(c, up)
}

func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	go func() { io.Copy(a, b); done <- struct{}{} }()
	go func() { io.Copy(b, a); done <- struct{}{} }()
	<-done
	a.Close()
	b.Close()
}
