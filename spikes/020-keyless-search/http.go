package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const userAgent = "Burrow-spike/0.1 (research; contact via github)"

// politeClient enforces a minimum interval per host and one backoff retry on 429.
type politeClient struct {
	hc       *http.Client
	mu       sync.Mutex
	last     map[string]time.Time
	interval map[string]time.Duration
	def      time.Duration
}

func newClient() *politeClient {
	return &politeClient{
		hc:       &http.Client{Timeout: 60 * time.Second, Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, TLSHandshakeTimeout: 30 * time.Second, ResponseHeaderTimeout: 45 * time.Second, IdleConnTimeout: 90 * time.Second, MaxIdleConnsPerHost: 2}},
		last:     map[string]time.Time{},
		interval: map[string]time.Duration{},
		def:      1100 * time.Millisecond,
	}
}

func (c *politeClient) setInterval(host string, d time.Duration) {
	c.mu.Lock()
	c.interval[host] = d
	c.mu.Unlock()
}

func (c *politeClient) wait(host string) {
	c.mu.Lock()
	d, ok := c.interval[host]
	if !ok {
		d = c.def
	}
	next := c.last[host].Add(d)
	c.mu.Unlock()
	if w := time.Until(next); w > 0 {
		time.Sleep(w)
	}
	c.mu.Lock()
	c.last[host] = time.Now()
	c.mu.Unlock()
}

type fetchResult struct {
	Code    int
	Body    []byte
	Latency time.Duration
	Retries int
	Header  http.Header
}

// get fetches u. retry429 allows a single retry after Retry-After (capped at 60 s, default 15 s).
func (c *politeClient) get(ctx context.Context, u string, hdr map[string]string, retry429 bool) (fetchResult, error) {
	pu, err := url.Parse(u)
	if err != nil {
		return fetchResult{}, err
	}
	var fr fetchResult
	for attempt := 0; attempt < 2; attempt++ {
		c.wait(pu.Host)
		req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "application/json, application/rss+xml, application/xml;q=0.9, */*;q=0.5")
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		start := time.Now()
		resp, err := c.hc.Do(req)
		if err != nil {
			return fetchResult{Latency: time.Since(start), Retries: attempt}, err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		fr = fetchResult{Code: resp.StatusCode, Body: body, Latency: time.Since(start), Retries: attempt, Header: resp.Header}
		if err != nil {
			return fr, err
		}
		if resp.StatusCode == 429 && retry429 && attempt == 0 {
			wait := 15 * time.Second
			if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
				wait = time.Duration(min(s, 60)) * time.Second
			}
			fmt.Printf("    429 from %s, backing off %s\n", pu.Host, wait)
			time.Sleep(wait)
			continue
		}
		break
	}
	return fr, nil
}
