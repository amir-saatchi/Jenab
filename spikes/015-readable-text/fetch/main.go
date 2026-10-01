// Command fetch downloads the fixture pages once and writes fixtures/<id>.html
// plus fixtures/<id>.json. It checks robots.txt, sends one request per page with
// a plain User-Agent, and skips fixtures that already exist unless -force.
// Derived legacy-encoding fixtures are rebuilt from their source fixture.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"burrow/spikes/readable/corpus"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
)

const (
	userAgent = "BurrowSpikeFetcher/0.1 (readable-text library test; one request per page)"
	uaToken   = "burrowspikefetcher"
	maxBody   = 32 << 20
)

var (
	dir    = flag.String("dir", "fixtures", "fixture directory")
	force  = flag.Bool("force", false, "refetch existing fixtures")
	only   = flag.String("only", "", "comma-separated fixture ids")
	client = &http.Client{Timeout: 90 * time.Second}

	robotsCache = map[string]*robots{}
	lastHit     = map[string]time.Time{}
)

func main() {
	flag.Parse()
	if err := os.MkdirAll(*dir, 0o755); err != nil {
		fatal(err)
	}
	want := map[string]bool{}
	for _, id := range strings.Split(*only, ",") {
		if id != "" {
			want[id] = true
		}
	}
	for _, p := range corpus.Pages {
		if len(want) > 0 && !want[p.ID] {
			continue
		}
		if p.URL == "" {
			continue
		}
		if !*force && exists(p.ID) {
			fmt.Printf("skip  %-32s (exists)\n", p.ID)
			continue
		}
		if err := fetchPage(p); err != nil {
			fmt.Printf("FAIL  %-32s %v\n", p.ID, err)
		}
	}
	for _, p := range corpus.Pages {
		if p.From == "" || (len(want) > 0 && !want[p.ID]) {
			continue
		}
		if err := derive(p); err != nil {
			fmt.Printf("FAIL  %-32s %v\n", p.ID, err)
		}
	}
}

func exists(id string) bool {
	_, err := os.Stat(filepath.Join(*dir, id+".html"))
	return err == nil
}

func fetchPage(p corpus.Page) error {
	u, err := url.Parse(p.URL)
	if err != nil {
		return err
	}
	rb, err := robotsFor(u)
	if err != nil {
		return fmt.Errorf("robots.txt: %w", err)
	}
	path := u.EscapedPath()
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	if !rb.allowed(path) {
		return fmt.Errorf("disallowed by robots.txt")
	}
	wait(u.Host, rb.delay)

	req, _ := http.NewRequest("GET", p.URL, nil)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return err
	}
	ct := resp.Header.Get("Content-Type")
	m := corpus.Meta{
		ID:            p.ID,
		URL:           p.URL,
		FinalURL:      resp.Request.URL.String(),
		Status:        resp.StatusCode,
		FetchedAt:     time.Now().UTC().Format(time.RFC3339),
		ContentType:   ct,
		HeaderCharset: headerCharset(ct),
		MetaCharset:   metaCharset(body),
		Bytes:         len(body),
		Robots:        "allowed (" + rb.group + ")",
	}
	if len(body) == maxBody {
		m.Note = "truncated at 32 MB by the fetcher"
	}
	if err := os.WriteFile(filepath.Join(*dir, p.ID+".html"), body, 0o644); err != nil {
		return err
	}
	fmt.Printf("ok    %-32s %d %s %d bytes\n", p.ID, resp.StatusCode, ct, len(body))
	return corpus.SaveMeta(*dir, m)
}

func wait(host string, delay time.Duration) {
	if delay < 2*time.Second {
		delay = 2 * time.Second
	}
	if t, ok := lastHit[host]; ok {
		if d := time.Until(t.Add(delay)); d > 0 {
			time.Sleep(d)
		}
	}
	lastHit[host] = time.Now()
}

func headerCharset(ct string) string {
	_, params, err := mime.ParseMediaType(ct)
	if err != nil {
		return ""
	}
	return strings.ToLower(params["charset"])
}

var (
	reMetaCharset = regexp.MustCompile(`(?is)<meta[^>]+charset\s*=\s*["']?\s*([a-z0-9_\-:.]+)`)
	reMetaTag     = regexp.MustCompile(`(?is)<meta[^>]*charset\s*=[^>]*>`)
	reHead        = regexp.MustCompile(`(?is)<head[^>]*>`)
)

func metaCharset(b []byte) string {
	if len(b) > 4096 {
		b = b[:4096]
	}
	if m := reMetaCharset.FindSubmatch(b); m != nil {
		return strings.ToLower(string(m[1]))
	}
	return ""
}

// derive builds a legacy-encoded copy of a UTF-8 fixture. Characters the target
// encoding cannot represent become numeric character references (&#NNNN;), as
// browsers do, so the text stays the same after decoding.
func derive(p corpus.Page) error {
	src, err := os.ReadFile(filepath.Join(*dir, p.From+".html"))
	if err != nil {
		return err
	}
	if !utf8.Valid(src) {
		return fmt.Errorf("source %s is not UTF-8", p.From)
	}
	var enc encoding.Encoding
	switch p.Encoding {
	case "windows-1256":
		enc = charmap.Windows1256
	case "windows-1252":
		enc = charmap.Windows1252
	default:
		return fmt.Errorf("unknown encoding %s", p.Encoding)
	}
	s := reMetaTag.ReplaceAllString(string(src), "")
	if p.Meta {
		loc := reHead.FindStringIndex(s)
		if loc == nil {
			return fmt.Errorf("no <head> in source")
		}
		s = s[:loc[1]] + `<meta charset="` + p.Encoding + `">` + s[loc[1]:]
	}
	out := encodeNCR(s, enc)
	if err := os.WriteFile(filepath.Join(*dir, p.ID+".html"), out, 0o644); err != nil {
		return err
	}
	srcMeta, _ := corpus.LoadMeta(*dir, p.From)
	m := corpus.Meta{
		ID:            p.ID,
		URL:           srcMeta.URL,
		FinalURL:      srcMeta.FinalURL,
		Status:        200,
		FetchedAt:     srcMeta.FetchedAt,
		ContentType:   "text/html; charset=" + p.Encoding,
		HeaderCharset: p.Encoding,
		Bytes:         len(out),
		Robots:        "derived, no request",
		DerivedFrom:   p.From,
		Note:          "re-encoded from the UTF-8 fixture; unmappable characters written as &#NNNN;",
	}
	if p.Meta {
		m.MetaCharset = p.Encoding
	} else {
		m.Note += "; <meta charset> removed, only the HTTP header names the encoding"
	}
	fmt.Printf("ok    %-32s derived from %s as %s (%d bytes)\n", p.ID, p.From, p.Encoding, len(out))
	return corpus.SaveMeta(*dir, m)
}

func encodeNCR(s string, enc encoding.Encoding) []byte {
	e := enc.NewEncoder()
	var buf bytes.Buffer
	var one [4]byte
	for _, r := range s {
		n := utf8.EncodeRune(one[:], r)
		b, err := e.Bytes(one[:n])
		if err != nil {
			buf.WriteString("&#" + strconv.Itoa(int(r)) + ";")
			continue
		}
		buf.Write(b)
	}
	return buf.Bytes()
}

// ---- robots.txt ----

type rule struct {
	allow   bool
	pattern string
}

type robots struct {
	rules []rule
	delay time.Duration
	group string
}

func robotsFor(u *url.URL) (*robots, error) {
	key := u.Scheme + "://" + u.Host
	if r, ok := robotsCache[key]; ok {
		return r, nil
	}
	wait(u.Host, 0)
	req, _ := http.NewRequest("GET", key+"/robots.txt", nil)
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	r := &robots{group: "no robots.txt"}
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
		r = parseRobots(body)
	case resp.StatusCode >= 500:
		return nil, fmt.Errorf("status %d, treating as disallowed", resp.StatusCode)
	}
	robotsCache[key] = r
	return r, nil
}

// parseRobots keeps the group for our own token if present, else the "*" group.
func parseRobots(body []byte) *robots {
	type group struct {
		agents []string
		rules  []rule
		delay  time.Duration
	}
	var groups []*group
	var cur *group
	lastWasAgent := false
	sc := bufio.NewScanner(bytes.NewReader(body))
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		switch k {
		case "user-agent":
			if cur == nil || !lastWasAgent {
				cur = &group{}
				groups = append(groups, cur)
			}
			cur.agents = append(cur.agents, strings.ToLower(v))
			lastWasAgent = true
			continue
		case "allow", "disallow":
			if cur != nil && v != "" {
				cur.rules = append(cur.rules, rule{allow: k == "allow", pattern: v})
			}
		case "crawl-delay":
			if cur != nil {
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					cur.delay = time.Duration(f * float64(time.Second))
				}
			}
		}
		lastWasAgent = false
	}
	var star *group
	for _, g := range groups {
		for _, a := range g.agents {
			if a == uaToken {
				return &robots{rules: g.rules, delay: g.delay, group: "own group"}
			}
			if a == "*" && star == nil {
				star = g
			}
		}
	}
	if star == nil {
		return &robots{group: "no matching group"}
	}
	return &robots{rules: star.rules, delay: star.delay, group: "* group"}
}

// allowed applies the longest matching rule; Allow wins a tie.
func (r *robots) allowed(path string) bool {
	best, allow := -1, true
	for _, ru := range r.rules {
		if matchRobots(ru.pattern, path) {
			n := len(ru.pattern)
			if n > best || (n == best && ru.allow) {
				best, allow = n, ru.allow
			}
		}
	}
	return allow
}

func matchRobots(pattern, path string) bool {
	anchored := strings.HasSuffix(pattern, "$")
	pattern = strings.TrimSuffix(pattern, "$")
	var re strings.Builder
	re.WriteString("^")
	for i, part := range strings.Split(pattern, "*") {
		if i > 0 {
			re.WriteString(".*")
		}
		re.WriteString(regexp.QuoteMeta(part))
	}
	if anchored {
		re.WriteString("$")
	}
	ok, _ := regexp.MatchString(re.String(), path)
	return ok
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
