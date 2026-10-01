package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Result is what Burrow's web_search tool would return per hit.
type Result struct {
	Title   string   `json:"title"`
	URL     string   `json:"url"`
	Snippet string   `json:"snippet,omitempty"`
	Date    string   `json:"date,omitempty"`     // publication date, RFC 3339
	ModDate string   `json:"mod_date,omitempty"` // last-modified only (Wikipedia)
	Engines []string `json:"engines,omitempty"`  // SearXNG upstream engines
	Lang    string   `json:"lang,omitempty"`
}

// Record is one request (or one locally filtered query for RSS).
type Record struct {
	Source       string     `json:"source"`
	QueryID      string     `json:"query_id"`
	Round        int        `json:"round"`
	Time         time.Time  `json:"time"`
	HTTPCode     int        `json:"http_code"`
	Status       string     `json:"status"` // ok, blocked, error
	Detail       string     `json:"detail,omitempty"`
	LatencyMs    int64      `json:"latency_ms"`
	Retries      int        `json:"retries,omitempty"`
	Results      []Result   `json:"results"`
	Unresponsive [][]string `json:"unresponsive,omitempty"` // SearXNG [engine, reason]
	Extra        string     `json:"extra,omitempty"`
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

func clean(s string) string {
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 300 {
		s = string(r[:300]) + "..."
	}
	return s
}

func snip(b []byte) string {
	s := strings.Join(strings.Fields(string(b)), " ")
	if r := []rune(s); len(r) > 160 {
		s = string(r[:160])
	}
	return s
}

func base(src, qid string, round int, fr fetchResult, err error) Record {
	r := Record{Source: src, QueryID: qid, Round: round, Time: time.Now().UTC(), HTTPCode: fr.Code,
		LatencyMs: fr.Latency.Milliseconds(), Retries: fr.Retries}
	switch {
	case err != nil:
		r.Status, r.Detail = "error", err.Error()
	case fr.Code == 200:
		r.Status = "ok"
	case fr.Code == 403 || fr.Code == 429 || fr.Code == 503:
		r.Status, r.Detail = "blocked", fmt.Sprintf("HTTP %d: %s", fr.Code, snip(fr.Body))
	default:
		r.Status, r.Detail = "error", fmt.Sprintf("HTTP %d: %s", fr.Code, snip(fr.Body))
	}
	return r
}

func parseTime(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "20060102T150405Z",
		time.RFC1123Z, time.RFC1123, "Mon, 2 Jan 2006 15:04:05 -0700", "Mon, 2 Jan 2006 15:04:05 MST", "Mon, 02 Jan 2006 15:04:05 Z", "2006-01-02"}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return ""
}

// ---------- SearXNG ----------

type searxResp struct {
	Results []struct {
		Title         string   `json:"title"`
		URL           string   `json:"url"`
		Content       string   `json:"content"`
		PublishedDate *string  `json:"publishedDate"`
		Engines       []string `json:"engines"`
	} `json:"results"`
	Unresponsive [][]any `json:"unresponsive_engines"`
}

func searxSearch(ctx context.Context, c *politeClient, src, baseURL string, q Query, round int, category string, retry bool) Record {
	v := url.Values{"q": {q.Text}, "format": {"json"}, "categories": {category}, "language": {"auto"}}
	fr, err := c.get(ctx, strings.TrimRight(baseURL, "/")+"/search?"+v.Encode(), nil, retry)
	rec := base(src, q.ID, round, fr, err)
	if rec.Status != "ok" {
		return rec
	}
	var sr searxResp
	if err := json.Unmarshal(fr.Body, &sr); err != nil {
		rec.Status, rec.Detail = "blocked", "not JSON (probably HTML/format disabled): "+snip(fr.Body)
		return rec
	}
	for _, x := range sr.Results {
		r := Result{Title: clean(x.Title), URL: x.URL, Snippet: clean(x.Content), Engines: x.Engines}
		if x.PublishedDate != nil {
			r.Date = parseTime(*x.PublishedDate)
		}
		rec.Results = append(rec.Results, r)
	}
	for _, u := range sr.Unresponsive {
		row := []string{}
		for _, e := range u {
			row = append(row, fmt.Sprint(e))
		}
		rec.Unresponsive = append(rec.Unresponsive, row)
	}
	return rec
}

// ---------- DuckDuckGo Instant Answer ----------

type ddgTopic struct {
	FirstURL string     `json:"FirstURL"`
	Text     string     `json:"Text"`
	Name     string     `json:"Name"`
	Topics   []ddgTopic `json:"Topics"`
}

func ddgSearch(ctx context.Context, c *politeClient, q Query) Record {
	v := url.Values{"q": {q.Text}, "format": {"json"}, "no_html": {"1"}, "skip_disambig": {"1"}, "t": {"burrow-spike"}}
	fr, err := c.get(ctx, "https://api.duckduckgo.com/?"+v.Encode(), nil, true)
	rec := base("ddg-instant-answer", q.ID, 0, fr, err)
	if rec.Status != "ok" {
		return rec
	}
	var d struct {
		Type          string     `json:"Type"`
		Heading       string     `json:"Heading"`
		AbstractText  string     `json:"AbstractText"`
		AbstractURL   string     `json:"AbstractURL"`
		AbstractSrc   string     `json:"AbstractSource"`
		Answer        any        `json:"Answer"`
		Results       []ddgTopic `json:"Results"`
		RelatedTopics []ddgTopic `json:"RelatedTopics"`
	}
	if err := json.Unmarshal(fr.Body, &d); err != nil {
		rec.Status, rec.Detail = "error", "bad JSON: "+snip(fr.Body)
		return rec
	}
	if d.AbstractText != "" && d.AbstractURL != "" {
		rec.Results = append(rec.Results, Result{Title: d.Heading + " (" + d.AbstractSrc + " abstract)", URL: d.AbstractURL, Snippet: clean(d.AbstractText)})
	}
	for _, r := range d.Results {
		rec.Results = append(rec.Results, Result{Title: clean(r.Text), URL: r.FirstURL})
	}
	var walk func([]ddgTopic)
	walk = func(ts []ddgTopic) {
		for _, t := range ts {
			if t.FirstURL != "" {
				rec.Results = append(rec.Results, Result{Title: clean(t.Text), URL: t.FirstURL})
			}
			walk(t.Topics)
		}
	}
	walk(d.RelatedTopics)
	rec.Extra = fmt.Sprintf("Type=%q Heading=%q Answer=%v", d.Type, d.Heading, d.Answer)
	return rec
}

// ---------- GDELT DOC 2.0 ----------

var noRetry bool

func gdeltSearch(ctx context.Context, c *politeClient, src string, q Query, timespan string) Record {
	v := url.Values{"query": {q.Text}, "mode": {"artlist"}, "format": {"json"}, "maxrecords": {"25"}}
	if timespan != "" {
		v.Set("timespan", timespan)
	}
	fr, err := c.get(ctx, "https://api.gdeltproject.org/api/v2/doc/doc?"+v.Encode(), nil, !noRetry)
	rec := base(src, q.ID, 0, fr, err)
	if rec.Status != "ok" {
		return rec
	}
	var d struct {
		Articles []struct {
			URL      string `json:"url"`
			Title    string `json:"title"`
			SeenDate string `json:"seendate"`
			Domain   string `json:"domain"`
			Language string `json:"language"`
		} `json:"articles"`
	}
	if err := json.Unmarshal(fr.Body, &d); err != nil {
		// GDELT answers query-syntax problems with plain text and HTTP 200.
		rec.Status, rec.Detail = "error", "non-JSON answer: "+snip(fr.Body)
		return rec
	}
	for _, a := range d.Articles {
		rec.Results = append(rec.Results, Result{Title: clean(a.Title), URL: a.URL, Date: parseTime(a.SeenDate), Lang: a.Language})
	}
	return rec
}

// ---------- Wikipedia ----------

func wikiSearch(ctx context.Context, c *politeClient, q Query) Record {
	v := url.Values{"action": {"query"}, "list": {"search"}, "srsearch": {q.Text}, "srlimit": {"10"},
		"srprop": {"snippet|timestamp"}, "format": {"json"}, "formatversion": {"2"}, "utf8": {"1"}}
	host := q.Lang + ".wikipedia.org"
	fr, err := c.get(ctx, "https://"+host+"/w/api.php?"+v.Encode(), nil, true)
	rec := base("wikipedia", q.ID, 0, fr, err)
	if rec.Status != "ok" {
		return rec
	}
	var d struct {
		Query struct {
			SearchInfo struct {
				TotalHits int `json:"totalhits"`
			} `json:"searchinfo"`
			Search []struct {
				Title     string `json:"title"`
				Snippet   string `json:"snippet"`
				Timestamp string `json:"timestamp"`
			} `json:"search"`
		} `json:"query"`
	}
	if err := json.Unmarshal(fr.Body, &d); err != nil {
		rec.Status, rec.Detail = "error", "bad JSON: "+snip(fr.Body)
		return rec
	}
	for _, s := range d.Query.Search {
		rec.Results = append(rec.Results, Result{Title: s.Title, URL: "https://" + host + "/wiki/" + url.PathEscape(strings.ReplaceAll(s.Title, " ", "_")),
			Snippet: clean(s.Snippet), ModDate: parseTime(s.Timestamp), Lang: q.Lang})
	}
	rec.Extra = fmt.Sprintf("host=%s totalhits=%d", host, d.Query.SearchInfo.TotalHits)
	return rec
}

// ---------- Hacker News (Algolia) ----------

func hnSearch(ctx context.Context, c *politeClient, q Query) Record {
	v := url.Values{"query": {q.Text}, "hitsPerPage": {"10"}, "tags": {"story"}}
	fr, err := c.get(ctx, "https://hn.algolia.com/api/v1/search?"+v.Encode(), nil, true)
	rec := base("hn-algolia", q.ID, 0, fr, err)
	if rec.Status != "ok" {
		return rec
	}
	var d struct {
		NbHits int `json:"nbHits"`
		Hits   []struct {
			Title     string `json:"title"`
			URL       string `json:"url"`
			CreatedAt string `json:"created_at"`
			ObjectID  string `json:"objectID"`
			Points    int    `json:"points"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(fr.Body, &d); err != nil {
		rec.Status, rec.Detail = "error", "bad JSON: "+snip(fr.Body)
		return rec
	}
	for _, h := range d.Hits {
		u := h.URL
		if u == "" {
			u = "https://news.ycombinator.com/item?id=" + h.ObjectID
		}
		rec.Results = append(rec.Results, Result{Title: clean(h.Title), URL: u, Date: parseTime(h.CreatedAt), Snippet: fmt.Sprintf("%d points", h.Points)})
	}
	rec.Extra = fmt.Sprintf("nbHits=%d", d.NbHits)
	return rec
}

// ---------- Marginalia ----------

func marginaliaSearch(ctx context.Context, c *politeClient, q Query) Record {
	v := url.Values{"query": {q.Text}, "count": {"10"}}
	fr, err := c.get(ctx, "https://api2.marginalia-search.com/search?"+v.Encode(), map[string]string{"API-Key": "public"}, true)
	rec := base("marginalia", q.ID, 0, fr, err)
	if rec.Status != "ok" {
		return rec
	}
	var d struct {
		License string `json:"license"`
		Results []struct {
			URL         string `json:"url"`
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"results"`
	}
	if err := json.Unmarshal(fr.Body, &d); err != nil {
		rec.Status, rec.Detail = "error", "bad JSON: "+snip(fr.Body)
		return rec
	}
	for _, r := range d.Results {
		rec.Results = append(rec.Results, Result{Title: clean(r.Title), URL: r.URL, Snippet: clean(r.Description)})
	}
	rec.Extra = "license=" + d.License
	return rec
}

// ---------- RSS / Atom ----------

type feedItem struct {
	Title, Link, Summary, Date string
}

type rssDoc struct {
	Channel struct {
		Items []struct {
			Title   string `xml:"title"`
			Link    string `xml:"link"`
			Desc    string `xml:"description"`
			PubDate string `xml:"pubDate"`
			DCDate  string `xml:"http://purl.org/dc/elements/1.1/ date"`
		} `xml:"item"`
	} `xml:"channel"`
	Entries []struct {
		Title   string `xml:"title"`
		Summary string `xml:"summary"`
		Updated string `xml:"updated"`
		Publ    string `xml:"published"`
		Links   []struct {
			Href string `xml:"href,attr"`
		} `xml:"link"`
	} `xml:"entry"`
}

func parseFeed(b []byte) ([]feedItem, error) {
	var d rssDoc
	dec := xml.NewDecoder(strings.NewReader(string(b)))
	dec.Strict = false
	dec.CharsetReader = func(label string, r io.Reader) (io.Reader, error) { return r, nil }
	if err := dec.Decode(&d); err != nil {
		return nil, err
	}
	var out []feedItem
	for _, it := range d.Channel.Items {
		dt := it.PubDate
		if dt == "" {
			dt = it.DCDate
		}
		out = append(out, feedItem{clean(it.Title), strings.TrimSpace(it.Link), clean(it.Desc), parseTime(dt)})
	}
	for _, e := range d.Entries {
		l := ""
		if len(e.Links) > 0 {
			l = e.Links[0].Href
		}
		dt := e.Publ
		if dt == "" {
			dt = e.Updated
		}
		out = append(out, feedItem{clean(e.Title), l, clean(e.Summary), parseTime(dt)})
	}
	return out, nil
}

func matches(q Query, text string) bool {
	t := strings.ToLower(text)
	for _, g := range q.Match {
		hit := false
		for _, alt := range g {
			if strings.Contains(t, strings.ToLower(alt)) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return true
}

// gdeltSearchOnce is the 7-day search without the 429 retry.
func gdeltSearchOnce(ctx context.Context, c *politeClient, q Query) Record {
	noRetry = true
	defer func() { noRetry = false }()
	return gdeltSearch(ctx, c, "gdelt-7d-slow", q, "7d")
}
