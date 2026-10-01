// SPIKE-020: keyless web search for Burrow. See README.md.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var feeds = [][2]string{
	{"coindesk", "https://www.coindesk.com/arc/outboundfeeds/rss/"},
	{"cointelegraph", "https://cointelegraph.com/rss"},
	{"decrypt", "https://decrypt.co/feed"},
}

func main() {
	collect := flag.String("collect", "", "comma list: gdelt-slow,searxng-local,searxng-public,ddg,gdelt,wikipedia,hn,marginalia,rss (Google News RSS is not implemented: robots.txt disallows /rss)")
	local := flag.String("local", "http://127.0.0.1:8888", "local SearXNG base URL")
	rounds := flag.Int("rounds", 3, "rounds of the general query set against local SearXNG")
	public := flag.String("public", "", "comma list of public SearXNG base URLs")
	ddgControl := flag.Bool("ddg-control", false, "one DDG Instant Answer request for a plain entity (Bitcoin), saved to raw/ddg-control.txt")
	listPublic := flag.Bool("list-public", false, "fetch searx.space instance list once and print candidates")
	dump := flag.Bool("dump", false, "write top5.md for hand judging")
	report := flag.Bool("report", false, "write the report (results.md) to stdout")
	only := flag.String("only", "", "comma list of query IDs to run (default all)")
	tag := flag.String("tag", "", "suffix for the raw file name, so a rerun does not overwrite")
	flag.Parse()
	if *only != "" {
		var keep []Query
		for _, q := range queries {
			if strings.Contains(","+*only+",", ","+q.ID+",") {
				keep = append(keep, q)
			}
		}
		queries = keep
	}

	ctx := context.Background()
	c := newClient()
	c.setInterval("api.gdeltproject.org", 10*time.Second) // GDELT asks for at most one request every 5 s; 10 s to be safe
	c.setInterval("127.0.0.1:8888", 3*time.Second)        // each local query fans out to ~8 upstream engines

	switch {
	case *ddgControl:
		r := ddgSearch(ctx, c, Query{ID: "control", Text: "Bitcoin", Lang: "en"})
		b, _ := json.MarshalIndent(r, "", " ")
		must(os.WriteFile("raw/ddg-control.txt", b, 0o644))
		fmt.Println(r.Status, len(r.Results), r.Extra)
		return
	case *listPublic:
		listPublicInstances(ctx, c)
		return
	case *dump:
		must(writeDump(loadAll()))
		return
	case *report:
		writeReport(os.Stdout, loadAll())
		return
	}

	for _, s := range strings.Split(*collect, ",") {
		var recs []Record
		add := func(r Record) {
			fmt.Printf("%-22s %-5s r%d %-7s %4dms n=%-3d %s\n", r.Source, r.QueryID, r.Round, r.Status, r.LatencyMs, len(r.Results), r.Detail)
			recs = append(recs, r)
		}
		switch s {
		case "":
			continue
		case "searxng-local":
			for round := 1; round <= *rounds; round++ {
				for _, q := range queries {
					add(searxSearch(ctx, c, "searxng-local", *local, q, round, "general", true))
				}
			}
			for _, q := range queries {
				add(searxSearch(ctx, c, "searxng-local-news", *local, q, 1, "news", true))
			}
			saveExtra(ctx, c, *local+"/stats/errors", "raw/searxng-local-stats-errors.json")
			saveExtra(ctx, c, *local+"/config", "raw/searxng-local-config.json")
		case "searxng-public":
			for _, inst := range strings.Split(*public, ",") {
				inst = strings.TrimRight(strings.TrimSpace(inst), "/")
				host := strings.TrimPrefix(strings.TrimPrefix(inst, "https://"), "http://")
				c.setInterval(host, 4*time.Second)
				fails := 0
				for i, q := range queries { // at most 10 requests per instance
					r := searxSearch(ctx, c, "searxng-public:"+host, inst, q, 1, "general", false)
					add(r)
					if r.Status != "ok" {
						fails++
					} else {
						fails = 0
					}
					if fails >= 2 { // stop politely after two consecutive refusals
						fmt.Printf("    stopping %s after %d requests\n", host, i+1)
						break
					}
				}
			}
		case "ddg":
			for _, q := range queries {
				add(ddgSearch(ctx, c, q))
			}
		case "gdelt":
			for _, q := range queries {
				add(gdeltSearch(ctx, c, "gdelt", q, ""))
			}
			for _, q := range queries {
				add(gdeltSearch(ctx, c, "gdelt-7d", q, "7d"))
			}
		case "gdelt-slow": // same 7-day search, 30 s apart, no retry: is the 429 about pacing?
			c.setInterval("api.gdeltproject.org", 30*time.Second)
			for _, q := range queries {
				add(gdeltSearchOnce(ctx, c, q))
			}
		case "wikipedia":
			for _, q := range queries {
				add(wikiSearch(ctx, c, q))
			}
		case "hn":
			for _, q := range queries {
				add(hnSearch(ctx, c, q))
			}
		case "marginalia":
			for _, q := range queries {
				add(marginaliaSearch(ctx, c, q))
			}
		case "rss":
			var items []feedItem
			var maxLat int64
			for _, f := range feeds {
				fr, err := c.get(ctx, f[1], nil, true)
				r := base("rss-feed:"+f[0], "feed", 0, fr, err)
				if r.Status == "ok" {
					its, perr := parseFeed(fr.Body)
					if perr != nil {
						r.Status, r.Detail = "error", "parse: "+perr.Error()
					}
					for _, it := range its {
						r.Results = append(r.Results, Result{Title: it.Title, URL: it.Link, Date: it.Date})
					}
					items = append(items, its...)
				}
				maxLat = max(maxLat, r.LatencyMs)
				add(r)
			}
			sort.SliceStable(items, func(i, j int) bool { return items[i].Date > items[j].Date })
			for _, q := range queries {
				r := Record{Source: "crypto-rss", QueryID: q.ID, Time: time.Now().UTC(), HTTPCode: 200, Status: "ok", LatencyMs: maxLat,
					Extra: fmt.Sprintf("local keyword filter over %d feed items", len(items))}
				for _, it := range items {
					if matches(q, it.Title+" "+it.Summary) {
						r.Results = append(r.Results, Result{Title: it.Title, URL: it.Link, Date: it.Date})
					}
				}
				add(r)
			}
		default:
			fmt.Println("unknown source", s)
			os.Exit(2)
		}
		must(save(filepath.Join("raw", s+*tag+".json"), recs))
	}
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func save(path string, v any) error {
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func saveExtra(ctx context.Context, c *politeClient, u, path string) {
	fr, err := c.get(ctx, u, nil, false)
	if err != nil || fr.Code != 200 {
		fmt.Println("could not fetch", u, err, fr.Code)
		return
	}
	_ = os.WriteFile(path, fr.Body, 0o644)
}

func loadAll() []Record {
	var all []Record
	files, _ := filepath.Glob("raw/*.json")
	for _, f := range files {
		if strings.Contains(f, "searxng-local-") || strings.Contains(f, "searx-space") {
			continue
		}
		b, err := os.ReadFile(f)
		must(err)
		var recs []Record
		must(json.Unmarshal(b, &recs))
		all = append(all, recs...)
	}
	return all
}

func listPublicInstances(ctx context.Context, c *politeClient) {
	fr, err := c.get(ctx, "https://searx.space/data/instances.json", nil, false)
	must(err)
	_ = os.WriteFile("raw/searx-space-instances.json", fr.Body, 0o644)
	var d struct {
		Instances map[string]struct {
			NetworkType string `json:"network_type"`
			Version     string `json:"version"`
			HTTP        struct {
				StatusCode int `json:"status_code"`
			} `json:"http"`
			Timing struct {
				Search struct {
					SuccessPercentage float64 `json:"success_percentage"`
					All               struct {
						Median float64 `json:"median"`
					} `json:"all"`
				} `json:"search"`
			} `json:"timing"`
			Uptime struct {
				UptimeMonth float64 `json:"uptimeMonth"`
			} `json:"uptime"`
		} `json:"instances"`
	}
	must(json.Unmarshal(fr.Body, &d))
	type cand struct {
		url           string
		succ, med, up float64
		ver           string
	}
	var cs []cand
	for u, in := range d.Instances {
		if in.NetworkType != "normal" || in.HTTP.StatusCode != 200 {
			continue
		}
		cs = append(cs, cand{u, in.Timing.Search.SuccessPercentage, in.Timing.Search.All.Median, in.Uptime.UptimeMonth, in.Version})
	}
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].succ != cs[j].succ {
			return cs[i].succ > cs[j].succ
		}
		return cs[i].up > cs[j].up
	})
	fmt.Printf("%d instances listed, %d normal+200\n", len(d.Instances), len(cs))
	for i, x := range cs {
		if i >= 30 {
			break
		}
		fmt.Printf("%-45s success=%5.1f%% median=%.2fs uptimeMonth=%.1f%% %s\n", x.url, x.succ, x.med, x.up, x.ver)
	}
}
