package main

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// scenarioDef is one request and its answer key (the ticket's seven).
type scenarioDef struct {
	ID   string
	Name string
	Lang string // the user's language: "en" or "fa"
	// Ask asks Mother for an agent (conditions 2–3). Unasked describes the
	// lasting job without saying "agent" (condition 4). Plain is a one-off
	// message sent to Mother before it in condition 4, which needs no new
	// chat. In the write scenario Ask and Unasked are the same write.
	Ask, Unasked, Plain string
	Write               bool // the request is itself a write that Mother can't do (scenario 7)
	// Need are the groups the agent needs, Forbid those it must not have.
	// Other groups are neither: picking one costs precision only.
	Need, Forbid []string
	// The key (condition 1): the chat the user would set up in the panel.
	KeyTitle  string
	KeyRole   string
	KeyGroups []string
	// Task is the follow-up the user sends the agent; Check reads the end
	// state.
	Task  string
	Check func(s *state) (bool, string)
	// JobWords: the custom prompt states the job when it has one of them.
	JobWords []string
}

var scenarios = []scenarioDef{
	{
		ID: "reviewer", Name: "Pipeline reviewer", Lang: "en",
		Ask:     "Make an agent that reviews our pipelines. When I ask, it checks a pipeline for cost and for missing error handling, and tells me what to fix.",
		Unasked: "From now on I want each pipeline in this project checked for cost and for missing error handling before I rely on it. I'll add more pipelines in the coming weeks.",
		Plain:   "Which pipelines does this project have?",
		Need:    []string{"read_data", "views_pipelines"}, Forbid: []string{"change_data", "web"},
		KeyTitle: "Pipeline reviewer",
		KeyRole: "You review this project's pipelines. When asked to review one, read its config, and dry-run it if that helps. Check its cost (LLM steps, how many items they get, how often it runs) " +
			"and its error handling (retries on network steps, timeouts, continue_on_error, a notification when a run fails). Don't change the pipeline yourself. " +
			"Reply with a short list of problems, most important first, each with its fix as a YAML snippet.",
		KeyGroups: []string{"read_data", "views_pipelines"},
		Task:      "Please review the weekly_summary pipeline.",
		Check: func(s *state) (bool, string) {
			read := slices.ContainsFunc(s.Calls, func(c call) bool {
				return !c.Error && (c.Tool == "read_config" || c.Tool == "dry_run" || c.Tool == "validate") && strings.Contains(c.Args, "weekly_summary")
			})
			hits := words(s.Reply, "retry", "retries", "error", "fail", "notify", "notification", "alert", "timeout")
			return read && hits > 0, fmt.Sprintf("read the config %v; error-handling words in the reply %d", read, hits)
		},
		JobWords: []string{"review", "pipeline"},
	},
	{
		ID: "eth", Name: "ETH tracker", Lang: "en",
		Ask:      "Make an agent that tracks the Ethereum price for me every day, like we do for Bitcoin.",
		Unasked:  "I'd like to follow Ethereum as well from now on: its price every day, like Bitcoin.",
		Plain:    "What was the Bitcoin price yesterday?",
		Need:     []string{"web", "views_pipelines", "change_data"},
		KeyTitle: "ETH tracker",
		KeyRole: "You track the Ethereum (ETH) price in this project, the way daily_btc tracks Bitcoin. Keep a daily ETH price in a table: ask for the table with request_schema_change, " +
			"find a free price API, and save a pipeline that adds the price every day; dry-run it before you save it. Later, fix the pipeline when a run fails. " +
			"Report what you set up: the table, the pipeline's id and schedule, and the API it uses.",
		KeyGroups: []string{"read_data", "change_data", "views_pipelines", "web"},
		Task:      "Set up the daily ETH price: a table for it, and a pipeline like daily_btc that adds the price every day. Use a free price API.",
		Check: func(s *state) (bool, string) {
			for id, y := range s.Pipelines {
				low := strings.ToLower(y)
				if !strings.Contains(low, "eth") || !strings.Contains(low, "schedule") {
					continue
				}
				for _, t := range insertTables(y) {
					if slices.Contains(s.Tables, t) {
						return true, fmt.Sprintf("pipeline %s writes %s", id, t)
					}
				}
				return false, fmt.Sprintf("pipeline %s writes no table that exists (%v)", id, insertTables(y))
			}
			return false, fmt.Sprintf("no saved ETH pipeline with a schedule; tables %v", s.Tables)
		},
		JobWords: []string{"ethereum", "eth"},
	},
	{
		ID: "entry", Name: "Data entry", Lang: "en",
		Ask:     "Make an agent for data entry: I'll paste Bitcoin closing prices I note down by hand, and it adds them to the price table.",
		Unasked: "Every now and then I'll paste Bitcoin closing prices I note down by hand. They need to go into the price table.",
		Plain:   "How many days of prices do we have?",
		Need:    []string{"read_data", "change_data"}, Forbid: []string{"web"},
		KeyTitle: "Price entry",
		KeyRole: "You enter Bitcoin closing prices that the user pastes into btc_prices: the date as YYYY-MM-DD and the price in USD as a number. Check the dates and numbers before you insert them, " +
			"and ask when a line is unclear. Don't overwrite a date that already has a price without asking. Reply with how many rows you added, and any you skipped and why.",
		KeyGroups: []string{"read_data", "change_data"},
		Task:      "Here are five closing prices to add:\n2026-09-01  57,310.20\n2026-09-02  57,902.75\n2026-09-03  56,880.10\n2026-09-04  56,115.00\n2026-09-05  56,740.55",
		Check: func(s *state) (bool, string) {
			want := map[string]float64{"2026-09-01": 57310.20, "2026-09-02": 57902.75, "2026-09-03": 56880.10, "2026-09-04": 56115.00, "2026-09-05": 56740.55}
			n := 0
			for d, p := range want {
				if got, ok := s.Prices[d]; ok && math.Abs(got-p) < 0.01 {
					n++
				}
			}
			return n == len(want), fmt.Sprintf("%d of 5 rows right", n)
		},
		JobWords: []string{"price", "enter", "entry", "add"},
	},
	{
		ID: "docs", Name: "Docs writer", Lang: "en",
		Ask:     "Make an agent that keeps this project's documentation: it writes docs about our tables, pipelines and views as files in the project, and updates them when things change.",
		Unasked: "I want this project documented, its tables, pipelines and views, as files in the project, and kept up to date as things change.",
		Plain:   "What tables does this project have?",
		Need:    []string{"read_data", "files"}, Forbid: []string{"change_data"},
		KeyTitle: "Docs writer",
		KeyRole: "You keep this project's documentation as Markdown files in the bucket under docs/. Read the tables, pipelines and views before you write about them, and describe what is there, not what might be. " +
			"Keep one file per topic: docs/schema.md for the tables, docs/pipelines.md and docs/views.md. Reply with the files you wrote or changed, with a one-line summary of each.",
		KeyGroups: []string{"read_data", "files"},
		Task:      "Write docs/schema.md: every table with its columns and what they hold.",
		Check: func(s *state) (bool, string) {
			f, ok := s.Files["docs/schema.md"]
			if !ok {
				return false, fmt.Sprintf("no docs/schema.md; files %v", keys(s.Files))
			}
			n := words(f, "btc_prices", "btc_news", "watch_keywords")
			return n == 3, fmt.Sprintf("docs/schema.md names %d of 3 tables", n)
		},
		JobWords: []string{"doc"},
	},
	{
		ID: "research", Name: "Researcher", Lang: "en",
		Ask:     "Make a research agent for this project: it looks things up on the web, compares options and keeps notes I can come back to.",
		Unasked: "I often need things looked up on the web for this project, like comparing data sources, with notes I can come back to later.",
		Plain:   "What does the daily_btc pipeline do?",
		Need:    []string{"web", "files", "memory"}, Forbid: []string{"change_data"},
		KeyTitle: "Researcher",
		KeyRole: "You research questions for this project on the web: search, read the sources and compare the options side by side, citing each fact's page. " +
			"Save your notes as Markdown files under notes/ in the bucket, and put a one-line conclusion the project should remember in project memory. " +
			"Reply with the comparison as a short table, your recommendation, and where you saved the notes.",
		KeyGroups: []string{"web", "files", "memory"},
		Task:      "Compare three Bitcoin price APIs: CoinGecko, CoinCap and Binance. Their free limits, how far back the history goes, and whether they need a key. Save your notes.",
		Check: func(s *state) (bool, string) {
			web := slices.ContainsFunc(s.Calls, func(c call) bool { return !c.Error && (c.Tool == "web_search" || c.Tool == "fetch_page") })
			saved := ""
			for k, f := range s.Files {
				if words(f, "coingecko", "coincap", "binance") == 3 {
					saved = k
				}
			}
			if saved == "" && words(s.Memory, "coingecko", "coincap", "binance") == 3 {
				saved = "memory"
			}
			return web && saved != "", fmt.Sprintf("read the web %v; notes on all three in %q", web, saved)
		},
		JobWords: []string{"research", "web", "compar"},
	},
	{
		ID: "persian", Name: "Persian news digest", Lang: "fa",
		Ask:     "یک عامل بساز که هر هفته اخبار بیت‌کوینی را که در پروژه ذخیره شده خلاصه کند و خلاصه را به صورت فایل در پروژه نگه دارد.",
		Unasked: "می‌خواهم از این به بعد هر هفته خلاصه‌ای از اخبار بیت‌کوینی که در پروژه داریم داشته باشم، به صورت فایل در خود پروژه.",
		Plain:   "چند خبر در جدول اخبار داریم؟",
		Need:    []string{"read_data", "files"}, Forbid: []string{"change_data", "web"},
		KeyTitle: "خلاصه اخبار",
		KeyRole: "تو خلاصه‌نویس اخبار بیت‌کوین این پروژه هستی. اخبار هفته را از جدول btc_news بخوان و خلاصه‌ای کوتاه به فارسی بنویس: مهم‌ترین خبرها، هر کدام با دلیل اهمیتش برای قیمت. " +
			"خلاصه را در فایل digest/<تاریخ>.md در پروژه ذخیره کن. در پایان بگو کدام فایل را ذخیره کردی و سه خبر مهم هفته چه بودند.",
		KeyGroups: []string{"read_data", "files"},
		Task:      "خلاصه اخبار این هفته را بنویس و در فایل digest/2026-11-12.md ذخیره کن.",
		Check: func(s *state) (bool, string) {
			read := slices.ContainsFunc(s.Calls, func(c call) bool { return !c.Error && c.Tool == "query" && strings.Contains(c.Args, "btc_news") })
			for k, f := range s.Files {
				if strings.HasPrefix(k, "digest/") {
					fa := persianShare(f)
					return read && fa > 0.5 && len([]rune(f)) >= 150, fmt.Sprintf("read the news %v; %s is %.0f%% Persian, %d characters", read, k, 100*fa, len([]rune(f)))
				}
			}
			return false, fmt.Sprintf("read the news %v; no digest file; files %v", read, keys(s.Files))
		},
		JobWords: []string{"خلاصه", "اخبار", "خبر"},
	},
	{
		ID: "write", Name: "Write with no agent yet", Lang: "en", Write: true,
		Ask:     "Add 'stablecoin' and 'MiCA' to the news keywords.",
		Unasked: "Add 'stablecoin' and 'MiCA' to the news keywords.",
		Plain:   "Which keywords do we watch for news?",
		Need:    []string{"change_data"}, Forbid: []string{"web"},
		KeyTitle: "Keywords",
		KeyRole: "You keep the news keywords in watch_keywords: add, rename or remove them when asked. Check for duplicates first, regardless of case. " +
			"Reply with the keywords you changed and the full list after the change.",
		KeyGroups: []string{"read_data", "change_data"},
		Task:      "Add 'halving' to the news keywords too.",
		Check: func(s *state) (bool, string) {
			ok := hasKeyword(s.Keywords, "halving")
			return ok, fmt.Sprintf("halving in watch_keywords %v", ok)
		},
		JobWords: []string{"keyword"},
	},
}

// delegated is the write scenario's request to Mother: did the two
// keywords land, through a chat she created or sent to?
func delegated(s *state) bool {
	return hasKeyword(s.Keywords, "stablecoin") && hasKeyword(s.Keywords, "mica")
}

func hasKeyword(ks []string, k string) bool {
	return slices.ContainsFunc(ks, func(x string) bool { return strings.EqualFold(strings.TrimSpace(x), k) })
}

func scenarioByID(id string) (scenarioDef, bool) {
	i := slices.IndexFunc(scenarios, func(s scenarioDef) bool { return s.ID == id })
	if i < 0 {
		return scenarioDef{}, false
	}
	return scenarios[i], true
}

// words counts how many of ws are in s, without case.
func words(s string, ws ...string) int {
	low := strings.ToLower(s)
	n := 0
	for _, w := range ws {
		if strings.Contains(low, strings.ToLower(w)) {
			n++
		}
	}
	return n
}

// persianShare is the share of letters in s that are Arabic script.
func persianShare(s string) float64 {
	var fa, all int
	for _, r := range s {
		switch {
		case r >= 0x0600 && r <= 0x06FF || r >= 0xFB50 && r <= 0xFDFF || r >= 0xFE70 && r <= 0xFEFF:
			fa++
			all++
		case r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z':
			all++
		}
	}
	if all == 0 {
		return 0
	}
	return float64(fa) / float64(all)
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
