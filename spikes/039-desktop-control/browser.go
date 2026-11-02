package main

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// The browser view: what the model sees of a browser window is the address bar (as an origin)
// and the page, not the rest of the frame (tabs, toolbar, bookmarks, profile). Google's results
// sit deeper than agent-desktop's default depth of 10, so browser snapshots use --max-depth 40
// and --compact (single-child unnamed nodes collapsed).

var browserSnapArgs = []string{"--timeout-ms", "15000", "--max-depth", "40", "--compact"}

// BrowserNotes is what a browser run records next to the usual counts.
type BrowserNotes struct {
	Origins       []string // each origin the page showed, in order
	ConsentSeen   bool     // a snapshot had a reject button
	RejectClicked bool
	AcceptRefused int
	Presses       int
	PressMissed   int // a press after which the focused element was not the one named
	Foreground    int // presses after which the foreground window changed
	Captcha       bool
	Reference     []Result
	Matched       int // reference titles found in the answer, in order
	HostsGiven    int // reference hosts found in the answer
}

type Result struct {
	Title, Site, Cite string
}

var (
	reRejectBtn = regexp.MustCompile(`(?i)^(reject all|alle ablehnen|decline all|ablehnen)$`)
	reCaptcha   = regexp.MustCompile(`(?i)unusual traffic|ungewöhnlichen datenverkehr|not a robot|kein roboter|recaptcha`)
)

// pageNode is the tab's page: the document with an address. Chrome's own UI (side panel, menus)
// has documents too, without one.
func pageNode(n SnapNode) *SnapNode {
	return findNode(n, func(n SnapNode) bool { return n.Role == "document" && n.Value != "" })
}

func omnibox(n SnapNode) *SnapNode {
	return findNode(n, func(n SnapNode) bool { return n.Ref != "" && strings.Contains(n.Name, "Address and search") })
}

// origin keeps scheme and host of a URL, or the text as it is when it isn't one.
func origin(v string) string {
	if u, err := url.Parse(v); err == nil && u.Host != "" {
		return u.Scheme + "://" + u.Host
	}
	if u, err := url.Parse("https://" + v); err == nil && strings.Contains(u.Host, ".") && !strings.Contains(v, " ") {
		return u.Host
	}
	return v
}

// browserSnapshot takes an interactive snapshot of the locked browser window, waiting (8 s at
// most) while the page's document has no children, which is how a page looks before Chrome
// has built its tree.
func (s *Session) browserSnapshot() string {
	var snap *SnapData
	start := time.Now()
	for {
		var err error
		snap, _, _, err = s.cli.snapshot(s.Win.ID, !s.full, browserSnapArgs...)
		if err != nil {
			s.errors = append(s.errors, "snapshot: "+err.Error())
			return "error: " + err.Error()
		}
		d := pageNode(snap.Tree)
		if d == nil || !strings.HasPrefix(d.Value, "http") || len(d.Children) > 0 || time.Since(start) > 8*time.Second {
			break
		}
		time.Sleep(time.Second)
	}
	if snap.Window.ID != s.Win.ID {
		return "error: the snapshot was not of the chosen window"
	}
	s.snapN++
	doc := pageNode(snap.Tree)
	page := "(none)"
	if doc != nil {
		page = origin(doc.Value)
	}
	if n := len(s.br.Origins); n == 0 || s.br.Origins[n-1] != page {
		s.br.Origins = append(s.br.Origins, page)
	}
	// The page counts as a CAPTCHA if its text says so (Google's "unusual traffic" page is /sorry/).
	if doc != nil && (strings.Contains(doc.Value, "/sorry/") || reCaptcha.MatchString(allText(*doc))) {
		s.br.Captcha = true
		s.stop = "captcha"
		return "stopped: the page asks to prove you are not a robot. The user has to handle this."
	}
	if doc != nil && findNode(*doc, func(n SnapNode) bool { return n.Role == "button" && reRejectBtn.MatchString(n.Name) }) != nil {
		s.br.ConsentSeen = true
	}
	var b strings.Builder
	fmt.Fprintf(&b, "browser window (snapshot %d), page: %s\n", s.snapN, page)
	var roots []SnapNode
	if o := omnibox(snap.Tree); o != nil {
		ob := *o
		ob.Children = nil
		ob.Value = origin(ob.Value)
		roots = append(roots, ob)
	}
	if doc != nil {
		d := *doc
		d.Value = page
		roots = append(roots, d)
	}
	c := compactNodes(roots, s.snapN)
	for k, v := range c.IDs {
		s.ids[k] = v
		s.nodes[k] = c.Nodes[k]
	}
	b.WriteString(c.Text)
	return b.String()
}

func allText(n SnapNode) string {
	var b strings.Builder
	var walk func(SnapNode)
	walk = func(n SnapNode) {
		b.WriteString(n.Name + " ")
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// googleResults lists the organic results of a Google results page, in order: links whose
// children are the title, an empty node and the site's name. Ads ("Gesponsert", "Sponsored"),
// video cards, sub-links and "Translate this page" links don't have that shape or are skipped.
func googleResults(doc SnapNode) []Result {
	var out []Result
	var walk func(n SnapNode, ad bool)
	walk = func(n SnapNode, ad bool) {
		if strings.Contains(n.Name, "Gesponsert") || strings.Contains(n.Name, "Sponsored") || strings.Contains(n.Name, "Anzeige") {
			ad = true
		}
		// A sub-link (Reddit's "more results") has the same shape, but its empty children are
		// bold words, so its name doesn't start with "title site".
		if n.Role == "link" && !ad && len(n.Children) >= 3 && n.Children[0].Name != "" && n.Children[1].Name == "" &&
			n.Children[2].Name != "" && strings.HasPrefix(norm(n.Name), norm(n.Children[0].Name+" "+n.Children[2].Name)) &&
			!strings.Contains(n.Value, "/aclk") && !strings.Contains(n.Value, "googleadservices") {
			r := Result{Title: n.Children[0].Name, Site: n.Children[2].Name}
			if i := strings.Index(n.Name, "http"); i >= 0 {
				r.Cite = strings.TrimSpace(n.Name[i:])
			}
			out = append(out, r)
			return
		}
		for _, c := range n.Children {
			walk(c, ad)
		}
	}
	walk(doc, false)
	return out
}

var reSpace = regexp.MustCompile(`\s+`)

func norm(s string) string {
	s = strings.ToLower(strings.ReplaceAll(s, " ", " "))
	s = strings.TrimRight(strings.TrimSpace(s), ".… ")
	return reSpace.ReplaceAllString(s, " ")
}

// gradeResults counts how many of the first five reference titles appear in the answer, in
// order, and how many of their hosts the answer names.
func gradeResults(ref []Result, answer string) (matched, hosts int) {
	a := norm(answer)
	pos := 0
	for i, r := range ref {
		if i == 5 {
			break
		}
		t := norm(r.Title)
		if j := strings.Index(a[pos:], t); j >= 0 {
			matched++
			pos += j + len(t)
		}
		if r.Cite != "" {
			host := strings.Fields(strings.TrimPrefix(strings.TrimPrefix(r.Cite, "https://"), "http://"))[0]
			if strings.Contains(a, strings.ToLower(host)) {
				hosts++
			}
		}
	}
	return
}
