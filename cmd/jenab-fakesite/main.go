// Command jenab-fakesite serves the scenario pages of daily crypto closes
// as a small web site on this machine, for manual runs that need pages
// that are fixed, offline and free (TASK-034):
//
//	go run ./cmd/jenab-fakesite [-addr 127.0.0.1:8099] [-pages dir]...
//
// It serves / (an index of the days), /crypto/<YYYY-MM-DD> (one day's
// page as HTML) and a 404 page for anything else, and logs each request.
// The pages are the scenarios' Markdown files, read at start; where two
// folders have the same day, the files must be the same.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// defaultPages are the scenario sets with daily pages, from the
// repository root.
var defaultPages = []string{
	filepath.Join("scenarios", "bench-bitcoin", "pages"),
	filepath.Join("scenarios", "bench-bitcoin-long", "pages"),
}

func main() {
	var dirs []string
	addr := flag.String("addr", "127.0.0.1:8099", "`address` to listen on; the host must be a loopback address")
	flag.Func("pages", "a `folder` of YYYY-MM-DD.md pages; repeat for more (default: the bench-bitcoin sets)", func(s string) error {
		dirs = append(dirs, s)
		return nil
	})
	flag.Parse()
	if flag.NArg() > 0 {
		flag.Usage()
		os.Exit(2)
	}
	if len(dirs) == 0 {
		dirs = defaultPages
	}
	if err := run(*addr, dirs); err != nil {
		fmt.Fprintln(os.Stderr, "jenab-fakesite:", err)
		os.Exit(1)
	}
}

func run(addr string, dirs []string) error {
	s, err := load(dirs)
	if err != nil {
		return err
	}
	if err := loopback(addr); err != nil {
		return err
	}
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	logger := log.New(os.Stderr, "", log.LstdFlags)
	logger.Printf("serving %d days, %s to %s, at http://%s/", len(s.days), s.days[0], s.days[len(s.days)-1], l.Addr())
	srv := &http.Server{Handler: logged(logger, s.handler()), ReadHeaderTimeout: 10 * time.Second}
	return srv.Serve(l)
}

// loopback accepts an address whose host is a loopback IP address, so the
// site is never reachable from other machines.
func loopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("-addr %q: %w", addr, err)
	}
	if a, err := netip.ParseAddr(host); err != nil || !a.IsLoopback() {
		return fmt.Errorf("-addr %q: the host must be a loopback address, such as 127.0.0.1", addr)
	}
	return nil
}

var dayFile = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\.md$`)

// site is the pages, rendered.
type site struct {
	days  []string          // sorted
	pages map[string][]byte // day → HTML
	index []byte
}

// source is one day's Markdown and the file it came from.
type source struct {
	file string
	md   []byte
}

// load reads the YYYY-MM-DD.md pages in dirs. A day found in more than one
// folder must have the same text in each (line endings aside); if not,
// load names the days that differ.
func load(dirs []string) (*site, error) {
	got := map[string]source{}
	var differ []string
	for _, d := range dirs {
		es, err := os.ReadDir(d)
		if err != nil {
			return nil, err
		}
		n := 0
		for _, e := range es {
			m := dayFile.FindStringSubmatch(e.Name())
			if m == nil || e.IsDir() {
				continue
			}
			if _, err := time.Parse(time.DateOnly, m[1]); err != nil {
				return nil, fmt.Errorf("%s: not a date: %w", filepath.Join(d, e.Name()), err)
			}
			n++
			f := filepath.Join(d, e.Name())
			md, err := os.ReadFile(f)
			if err != nil {
				return nil, err
			}
			md = bytes.ReplaceAll(md, []byte("\r\n"), []byte("\n"))
			if old, ok := got[m[1]]; ok {
				if !bytes.Equal(old.md, md) {
					differ = append(differ, fmt.Sprintf("%s: %s and %s differ", m[1], old.file, f))
				}
				continue
			}
			got[m[1]] = source{file: f, md: md}
		}
		if n == 0 {
			return nil, fmt.Errorf("%s: no YYYY-MM-DD.md pages in it", d)
		}
	}
	if len(differ) > 0 {
		return nil, fmt.Errorf("the pages must be the same where folders share a day:\n  %s", strings.Join(differ, "\n  "))
	}
	s := &site{pages: map[string][]byte{}}
	var entries []entry
	for day, src := range got {
		body, title := toHTML(string(src.md))
		if title == "" {
			title = day
		}
		var b bytes.Buffer
		if err := dayPage.Execute(&b, struct {
			Title string
			Body  template.HTML
		}{title, body}); err != nil {
			return nil, err
		}
		s.pages[day] = b.Bytes()
		s.days = append(s.days, day)
		entries = append(entries, entry{Day: day, Title: title})
	}
	slices.Sort(s.days)
	slices.SortFunc(entries, func(a, b entry) int { return strings.Compare(a.Day, b.Day) })
	var b bytes.Buffer
	if err := indexPage.Execute(&b, entries); err != nil {
		return nil, err
	}
	s.index = b.Bytes()
	return s, nil
}

type entry struct{ Day, Title string }

func (s *site) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { write(w, http.StatusOK, s.index) })
	mux.HandleFunc("GET /crypto/{day}", func(w http.ResponseWriter, r *http.Request) {
		if p, ok := s.pages[r.PathValue("day")]; ok {
			write(w, http.StatusOK, p)
			return
		}
		write(w, http.StatusNotFound, notFound)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { write(w, http.StatusNotFound, notFound) })
	return mux
}

func write(w http.ResponseWriter, status int, page []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	w.Write(page)
}

// logged logs each request's path and status; the logger adds the time.
func logged(l *log.Logger, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &recorder{ResponseWriter: w, status: http.StatusOK}
		h.ServeHTTP(rw, r)
		l.Printf("%s %s %d", r.Method, r.URL.RequestURI(), rw.status)
	})
}

type recorder struct {
	http.ResponseWriter
	status int
}

func (r *recorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

var (
	dayPage = template.Must(template.New("day").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>{{.Title}} - Markets Daily</title>
</head>
<body>
<header><nav><a href="/">Markets Daily: all days</a></nav></header>
<main>
<article>
{{.Body}}</article>
</main>
</body>
</html>
`))
	indexPage = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Markets Daily</title>
</head>
<body>
<main>
<h1>Markets Daily</h1>
<p>One page a day with the closing prices of the top 10 coins and the day's headlines.</p>
<ul>
{{range .}}<li><a href="/crypto/{{.Day}}">{{.Day}}</a>: {{.Title}}</li>
{{end}}</ul>
</main>
</body>
</html>
`))
	notFound = []byte(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Not found - Markets Daily</title>
</head>
<body>
<main>
<h1>Not found</h1>
<p>There is no page here. The <a href="/">index</a> lists the days there are pages for.</p>
</main>
</body>
</html>
`)
)
