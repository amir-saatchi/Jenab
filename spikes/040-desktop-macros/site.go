package main

import (
	"fmt"
	"hash/fnv"
	"html"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// A stand-in search site on 127.0.0.1, for the browser breaks after Google's second CAPTCHA:
// a cookie dialog (on or off), a search box and a results page, in English or German. It is
// built like Google's: the dialog sits on the same page, the box is a combobox-like input,
// results are links with a title and a site. The results are made from the query, so the
// grader knows them.

const sitePort = 8740

var siteURL = fmt.Sprintf("http://127.0.0.1:%d/", sitePort)

type siteMode struct {
	mu      sync.Mutex
	consent bool
	lang    string // en, de
}

var site = &siteMode{lang: "en"}

var siteWords = map[string]map[string]string{
	"en": {"title": "Findit", "search": "Search", "button": "Search", "results": "Results", "about": "About %d results",
		"dialog": "Before you continue to Findit", "reject": "Reject all", "accept": "Accept all", "cookies": "We use cookies to run this site."},
	"de": {"title": "Findit", "search": "Suche", "button": "Suchen", "results": "Ergebnisse", "about": "Ungefähr %d Ergebnisse",
		"dialog": "Bevor Sie zu Findit weitergehen", "reject": "Alle ablehnen", "accept": "Alle akzeptieren", "cookies": "Wir verwenden Cookies."},
}

var siteOnce sync.Once

func startSite() error {
	var err error
	siteOnce.Do(func() {
		var ln net.Listener
		ln, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", sitePort))
		if err != nil {
			return
		}
		mux := http.NewServeMux()
		mux.HandleFunc("/", siteHandler)
		go http.Serve(ln, mux)
	})
	return err
}

func siteResults(q string) []Result {
	words := []string{"guide", "tutorial", "reference", "notes", "benchmarks", "FAQ", "examples", "overview"}
	hosts := []string{"docs.example.org", "blog.example.net", "wiki.example.com", "dev.example.org", "forum.example.net", "learn.example.com", "news.example.org", "code.example.net"}
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(strings.TrimSpace(q))))
	k := int(h.Sum32())
	var out []Result
	for i := 0; i < 8; i++ {
		w := words[(k+i*3)%len(words)]
		host := hosts[(k/7+i)%len(hosts)]
		out = append(out, Result{Title: q + " " + w, Site: host, Cite: "https://" + host})
	}
	return out
}

func siteHandler(w http.ResponseWriter, r *http.Request) {
	site.mu.Lock()
	consent, lang := site.consent, site.lang
	site.mu.Unlock()
	t := siteWords[lang]
	if r.URL.Path == "/consent" {
		http.SetCookie(w, &http.Cookie{Name: "c", Value: "reject", Path: "/"})
		back := r.URL.Query().Get("back")
		if !strings.HasPrefix(back, "/") {
			back = "/"
		}
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	if r.URL.Path != "/" && r.URL.Path != "/search" {
		http.NotFound(w, r)
		return
	}
	_, err := r.Cookie("c")
	showDialog := consent && err != nil
	q := r.URL.Query().Get("q")
	var b strings.Builder
	fmt.Fprintf(&b, `<!doctype html><html lang="%s"><head><meta charset="utf-8"><title>%s</title></head><body>`, lang, t["title"])
	if showDialog {
		back := html.EscapeString(r.URL.RequestURI())
		fmt.Fprintf(&b, `<div role="dialog" aria-modal="true" aria-label="%s" style="position:fixed;inset:0;background:#fff;padding:2em">
<h1>%s</h1><p>%s</p>
<form action="/consent" method="get"><input type="hidden" name="back" value="%s"><button>%s</button></form>
<form action="/consent" method="get"><input type="hidden" name="back" value="%s"><button>%s</button></form></div>`,
			t["dialog"], t["dialog"], t["cookies"], back, t["reject"], back, t["accept"])
	}
	fmt.Fprintf(&b, `<header><h1>%s</h1><form action="/search" method="get" role="search">
<input type="search" name="q" aria-label="%s" value="%s"><button>%s</button></form></header>`, t["title"], t["search"], html.EscapeString(q), t["button"])
	if r.URL.Path == "/search" && q != "" {
		res := siteResults(q)
		fmt.Fprintf(&b, `<main><h2>%s</h2><p>%s</p><ol>`, t["results"], fmt.Sprintf(t["about"], 1000+len(q)*37))
		for _, x := range res {
			fmt.Fprintf(&b, `<li><a href="%s/%d"><h3>%s</h3><cite>%s</cite></a></li>`, x.Cite, len(x.Title), html.EscapeString(x.Title), x.Cite)
		}
		b.WriteString(`</ol></main>`)
	}
	b.WriteString(`</body></html>`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(b.String()))
}

func init() {
	tasks["site"] = Task{
		Name:       "site",
		Goal:       `In the open Chrome window, go to ` + siteURL + `, search for "{query}", and tell me the first five results (title and URL), in order. Reading them is enough; don't open them.`,
		Browser:    true,
		ParamOrder: []string{"query"},
		Params: []map[string]string{
			{"query": "Wails v3 Go desktop framework"},
			{"query": "SQLite WAL mode"},
			{"query": "Go generics tutorial"},
			{"query": "Tauri vs Electron"},
			{"query": "Windows UI Automation API"},
			{"query": "Rust async runtime comparison"},
		},
		Launch: func(cfg *Config, f agentFlags) (*App, error) {
			if err := startSite(); err != nil {
				return nil, err
			}
			site.mu.Lock()
			site.consent, site.lang = f.siteConsent, f.siteLang
			site.mu.Unlock()
			p := f.chromeProfile
			if p == "" {
				p = filepath.Join(cfg.Scratch, "chrome-"+time.Now().Format("150405"))
			}
			return launchChrome(cfg.CLI, ChromeOpts{Profile: p, UILang: f.chromeUI})
		},
		// Pass: the first five titles in order (what the page served for this query).
		Check: func(s *Session, p map[string]string, answer string) (bool, string) {
			ref := siteResults(p["query"])[:5]
			s.br.Reference = ref
			writeJSON(s.refPath, ref)
			s.br.Matched, s.br.HostsGiven = gradeResults(ref, answer)
			return s.br.Matched == 5, fmt.Sprintf("titles %d/5 in order, hosts %d/5", s.br.Matched, s.br.HostsGiven)
		},
	}
}
