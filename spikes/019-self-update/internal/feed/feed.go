// Package feed is the local update feed: an endpoint-manifest host and a fake
// of the GitHub releases API, on 127.0.0.1 only, with every request logged
// (URL, query, all headers).
package feed

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"burrow/spikes/selfupdate/internal/spike"
)

type Route struct {
	Body        []byte
	ContentType string
	Status      int
	Cut         bool   // announce the full Content-Length, send half, drop the connection
	Redirect    string // 302 to this URL
	RateBps     int    // >0: throttle the body to about this many bytes per second
}

type Server struct {
	mu       sync.Mutex
	routes   map[string]Route
	scenario string
	log      *spike.Logger
	HTTPURL  string // http://127.0.0.1:port
	HTTPSURL string // https://127.0.0.1:port (self-signed)
	CertPEM  []byte
	srv, tls *http.Server
}

func Start(logPath string) (*Server, error) {
	s := &Server{routes: map[string]Route{}, log: spike.NewLogger(logPath, map[string]any{"src": "feed"})}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s.HTTPURL = "http://" + ln.Addr().String()
	s.srv = &http.Server{Handler: s}
	go s.srv.Serve(ln)

	cert, pemBytes, err := selfSigned()
	if err != nil {
		return nil, err
	}
	s.CertPEM = pemBytes
	tln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s.HTTPSURL = "https://" + tln.Addr().String()
	s.tls = &http.Server{Handler: s, TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}}}
	go s.tls.Serve(tls.NewListener(tln, s.tls.TLSConfig))
	return s, nil
}

func (s *Server) Close() {
	_ = s.srv.Close()
	_ = s.tls.Close()
}

func (s *Server) SetScenario(name string) {
	s.mu.Lock()
	s.scenario = name
	s.mu.Unlock()
}

func (s *Server) Reset() {
	s.mu.Lock()
	s.routes = map[string]Route{}
	s.mu.Unlock()
}

func (s *Server) Set(path string, r Route) {
	s.mu.Lock()
	s.routes[path] = r
	s.mu.Unlock()
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	rt, ok := s.routes[r.URL.Path]
	sc := s.scenario
	s.mu.Unlock()
	hdr := map[string]string{}
	for k, v := range r.Header {
		hdr[k] = joinVals(v)
	}
	q := map[string]string{}
	for k, v := range r.URL.Query() {
		q[k] = joinVals(v)
	}
	status := http.StatusNotFound
	if ok {
		status = rt.Status
		if status == 0 {
			status = http.StatusOK
		}
	}
	s.log.Log("request", map[string]any{
		"sc": sc, "method": r.Method, "url": r.URL.String(), "path": r.URL.Path, "query": q, "headers": hdr,
		"host": r.Host, "remote": r.RemoteAddr, "proto": r.Proto, "tls": r.TLS != nil, "status": status, "cut": rt.Cut,
		"bodyLen": r.ContentLength,
	})
	if !ok {
		http.NotFound(w, r)
		return
	}
	if rt.Redirect != "" {
		http.Redirect(w, r, rt.Redirect, http.StatusFound)
		return
	}
	if rt.ContentType != "" {
		w.Header().Set("Content-Type", rt.ContentType)
	}
	if rt.Cut {
		w.Header().Set("Content-Length", strconv.Itoa(len(rt.Body)))
		w.WriteHeader(status)
		_, _ = w.Write(rt.Body[:len(rt.Body)/2])
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		if hj, ok := w.(http.Hijacker); ok {
			if c, _, err := hj.Hijack(); err == nil {
				_ = c.Close()
			}
		}
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(rt.Body)))
	w.WriteHeader(status)
	if rt.RateBps > 0 {
		const chunk = 64 << 10
		for off := 0; off < len(rt.Body); off += chunk {
			end := min(off+chunk, len(rt.Body))
			if _, err := w.Write(rt.Body[off:end]); err != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(time.Duration(float64(end-off) / float64(rt.RateBps) * float64(time.Second)))
		}
		return
	}
	_, _ = w.Write(rt.Body)
}

func joinVals(v []string) string {
	out := ""
	for i, s := range v {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}

func selfSigned() (tls.Certificate, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "burrow-spike019 local feed"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:         true, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	kb, _ := x509.MarshalECPrivateKey(key)
	cert, err := tls.X509KeyPair(pemBytes, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}))
	return cert, pemBytes, err
}

func WriteFile(path string, b []byte) error { return os.WriteFile(path, b, 0o644) }
