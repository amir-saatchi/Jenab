package main

// T-load: short requests on the SPEC 9 Bitcoin project (state after T3). 8 need a skill, 4 need
// none. Scored from the run's events: was every needed skill loaded before the first action that
// needs it, and how many unneeded skills were loaded.

import (
	"encoding/json"
	"regexp"
	"strings"
)

type LoadReq struct {
	ID         string
	Prompt     string
	Need       []string // skills that must be loaded before the gating action
	Acceptable []string // extra skills that do not count as unneeded
	Action     string   // write, search or answer: the action the needed skills must come before
}

var loadReqs = []LoadReq{
	{ID: "L01_news_view", Action: "write", Need: []string{"config-guide"}, Acceptable: []string{"sql-queries"},
		Prompt: "Add a table view news_last_week that shows the saved news of the last 7 days (date, title, link), newest first."},
	{ID: "L02_schedule", Action: "write", Need: []string{"pipelines"},
		Prompt: "Change the daily_btc pipeline so it runs at 07:30 UTC instead of 08:00."},
	{ID: "L03_add_column", Action: "write", Need: []string{"migrations"}, Acceptable: []string{"database-design"},
		Prompt: "Add an optional column volume (a number) to btc_prices for the daily trading volume."},
	{ID: "L04_sql_fails", Action: "answer", Need: []string{"sql-queries"},
		Prompt: "This query fails in the query tool. Why?\nSELECT status, count(*) FROM _burrow_runs GROUP BY status"},
	{ID: "L05_design_table", Action: "answer", Need: []string{"database-design"}, Acceptable: []string{"migrations"},
		Prompt: "Design a table for the crypto exchanges I want to compare: name, country, website and trading fee in percent. Describe the design first; don't create it yet."},
	{ID: "L06_etf_news", Action: "search", Need: []string{"web-research"},
		Prompt: "Search the web for this week's news about Bitcoin ETFs and tell me the three most important items."},
	{ID: "L07_chart", Action: "write", Need: []string{"config-guide"}, Acceptable: []string{"sql-queries"},
		Prompt: "Build a bar chart of the number of saved news items per day."},
	{ID: "L08_form", Action: "write", Need: []string{"config-guide"},
		Prompt: "Add a form for adding a news item by hand: url, date, title and reason."},
	{ID: "L09_moving_average", Action: "answer",
		Prompt: "What is a moving average? Answer in two sentences."},
	{ID: "L10_price_yesterday", Action: "answer",
		Prompt: "What was the BTC price yesterday?"},
	{ID: "L11_what_tracked", Action: "answer",
		Prompt: "In two or three sentences, what does this project track?"},
	{ID: "L12_two_plus_two", Action: "answer",
		Prompt: "What is 2+2?"},
}

func loadReqByID(id string) *LoadReq {
	for i := range loadReqs {
		if loadReqs[i].ID == id || strings.HasPrefix(loadReqs[i].ID, id+"_") {
			return &loadReqs[i]
		}
	}
	return nil
}

// LoadScore is the T-load verdict for one run.
type LoadScore struct {
	Gate         string   // the gating action that happened: write, search, answer, none
	RightBefore  bool     // all needed skills loaded before the gating action
	LoadedEver   bool     // all needed skills loaded at some point (incl. load_with after the action)
	Unneeded     []string // load_skill calls outside need + acceptable
	SelfLoads    int      // load_skill calls
	NeedSkill    bool
	WithCovers   bool // C: a needed skill came with the gating tool's result (load_with)
	MissingNames []string
}

func scoreLoad(r RunResult, lr *LoadReq) LoadScore {
	s := LoadScore{Gate: "none", NeedSkill: len(lr.Need) > 0}
	gate := -1
	for i, e := range r.Events {
		if e.Kind == "write" || e.Kind == "answer" || (e.Kind == "search" && lr.Action == "search") {
			gate = i
			s.Gate = e.Kind
			break
		}
	}
	before := map[string]bool{}
	ever := map[string]bool{}
	ok := map[string]bool{}
	for _, n := range append(append([]string{}, lr.Need...), lr.Acceptable...) {
		ok[n] = true
	}
	seen := map[string]bool{}
	for i, e := range r.Events {
		if e.Kind != "load_skill" && e.Kind != "load_with" {
			continue
		}
		if skills[e.Name] == nil {
			continue
		}
		ever[e.Name] = true
		if gate < 0 || i < gate {
			before[e.Name] = true
		}
		if e.Kind == "load_skill" {
			s.SelfLoads++
			if !ok[e.Name] && !seen[e.Name] {
				s.Unneeded = append(s.Unneeded, e.Name)
			}
			seen[e.Name] = true
		}
		if e.Kind == "load_with" && gate >= 0 && i > gate && r.Events[gate].Req == e.Req {
			for _, n := range lr.Need {
				if n == e.Name {
					s.WithCovers = true
				}
			}
		}
	}
	s.RightBefore, s.LoadedEver = true, true
	for _, n := range lr.Need {
		if !before[n] {
			s.RightBefore = false
			s.MissingNames = append(s.MissingNames, n)
		}
		if !ever[n] {
			s.LoadedEver = false
		}
	}
	if gate < 0 && len(lr.Need) > 0 {
		s.RightBefore = false
	}
	return s
}

// fakeSearch is the synthetic web_search result (no network).
var reGold = regexp.MustCompile(`(?i)gold|xau`)

func fakeSearch(args string) string {
	var in struct {
		Query string `json:"query"`
	}
	json.Unmarshal([]byte(args), &in)
	if reGold.MatchString(in.Query) {
		return `1. "Gold holds above 2,600 USD as the dollar weakens" - markets.example.com, 2026-09-29: spot gold at 2,641 USD/oz, +0.6% today.
2. "Central banks keep buying gold" - reuters.example, 2026-09-28.
3. "Gold ETF holdings rise for a third week" - ft.example, 2026-09-27.
`
	}
	return `1. "Spot bitcoin ETFs add 410M USD in a day" - coindesk.example, 2026-09-29: fourth day of inflows.
2. "SEC delays decision on staking ETFs" - reuters.example, 2026-09-28.
3. "Bitcoin hashrate sets a new record" - theblock.example, 2026-09-28.
4. "ETF issuers cut fees as competition grows" - ft.example, 2026-09-27.
5. "Miner reserves fall to a three-year low" - cryptoslate.example, 2026-09-25.
`
}
