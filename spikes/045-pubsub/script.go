package main

import (
	"fmt"
	"strings"
	"time"
)

// The prices the script turns on: the day before yesterday, yesterday
// (+1.8%) and today (−6.4%, which daily_btc's 12:00 run adds).
const (
	priceBefore    = 70000.00
	priceYesterday = 71260.00
	priceToday     = 66700.00
)

// runDef is a pipeline run the app reports. Those with an At play in the
// day as pipeline_run events; r_100 ran yesterday.
type runDef struct {
	ID, Pipeline, Outcome, At, Status string
}

var runs = []runDef{
	{"r_100", "daily_btc", "success", "", "success on 2026-11-11 at 08:00: all 7 steps ok; saved the price for 2026-11-11 (71,260.00) and 5 news items."},
	{"r_101", "daily_btc", "failed", "10:00", "failed on 2026-11-12 at 10:00 at step price (http.get): HTTP 503 Service Unavailable from https://api.example.com/btc/daily, after 3 retries. " +
		"The steps after it were skipped, so btc_prices has no row for 2026-11-12 and no news was saved."},
	{"r_102", "weekly_summary", "success", "10:15", "success on 2026-11-12 at 10:15 (started by the user from the page): all 6 steps ok; saved summaries/2026-11-12.md."},
	{"r_103", "daily_btc", "success", "12:00", "success on 2026-11-12 at 12:00: all 7 steps ok; saved the price for 2026-11-12 (66,700.00) and 5 news items."},
}

func runStatus(id string) string {
	id = strings.TrimSpace(id)
	for _, r := range runs {
		if r.ID == id {
			return fmt.Sprintf("Run %s of %s: %s", r.ID, r.Pipeline, r.Status)
		}
	}
	if strings.HasPrefix(id, "r_2") {
		return "Run " + id + ": success, all steps ok."
	}
	return fmt.Sprintf("There is no run %q. Run IDs come from run_pipeline and from run notices.", id)
}

// runNotice is a run's pipeline_run notice.
func runNotice(r runDef) string {
	what := r.Outcome
	if r.Outcome == "failed" {
		what = "failed at step price: HTTP 503 from api.example.com"
	}
	return fmt.Sprintf("[pipeline_run by app: %s %s, run %s at %s]", r.Pipeline, what, r.ID, r.At)
}

// What a message's turn is to the publish measures.
const (
	pubRoutine = "routine" // nothing another chat depends on: a publish is noise
	pubExtra   = "extra"   // changed or found something, but no subscriber needs a report (app events cover schema changes)
	pubNeed    = "need"    // a subscriber needs it: the price watcher's big move, on alerts
)

// msgDef is one scripted message.
type msgDef struct {
	At    string // HH:MM on the day
	To    string // the agent's key
	Text  string
	Pub   string
	First bool // the agent's first message: the turn it was created for
}

// script is the scripted user's day: 14 messages to the six agents. App
// events (pipeline runs) play before the first message after their time;
// schema changes happen in the schema agent's turns.
var script = []msgDef{
	{At: "09:00", To: "docs", First: true, Pub: pubRoutine, Text: "Please write docs/schema.md for the project's current tables."},
	{At: "09:10", To: "schema", First: true, Pub: pubExtra, Text: "Add a column volume to btc_prices: the day's trading volume in USD, as a REAL."},
	{At: "09:20", To: "reviewer", First: true, Pub: pubRoutine, Text: "Have a look at yesterday's daily_btc run, r_100, and tell me if anything looks off."},
	{At: "09:30", To: "watcher", First: true, Pub: pubRoutine, Text: "How did BTC move in the newest prices?"},
	{At: "09:40", To: "reporter", First: true, Pub: pubRoutine, Text: "Start this week's draft in summaries/draft.md with the prices so far."},
	{At: "09:50", To: "entry", First: true, Pub: pubRoutine, Text: "Add two keywords to watch_keywords: halving and MiCA."},
	{At: "10:30", To: "docs", Pub: pubRoutine, Text: "Add a short intro at the top of docs/schema.md that says what this project tracks."},
	{At: "10:40", To: "reviewer", Pub: pubExtra, Text: "Does weekly_summary still look fine after this morning's manual run?"},
	{At: "12:30", To: "watcher", Pub: pubNeed, Text: "Today's price is in. How did BTC move?"},
	{At: "13:00", To: "schema", Pub: pubExtra, Text: "Add a table eth_prices for daily Ethereum prices: date and price."},
	{At: "14:00", To: "reporter", Pub: pubRoutine, Text: "Bring the draft up to date with today."},
	{At: "14:30", To: "entry", Pub: pubRoutine, Text: "این خبر امروز را به btc_news اضافه کن: تاریخ 2026-11-12، عنوان «Bitcoin falls 6% as ETF outflows grow»، نشانی https://news.example.com/crypto/1010، دلیل: خروج پول از صندوق‌ها فشار فروش می‌آورد."},
	{At: "15:00", To: "docs", Pub: pubRoutine, Text: "Please add a line \"Last reviewed: 2026-11-12\" at the end of docs/schema.md."},
	{At: "16:00", To: "watcher", Pub: pubRoutine, Text: "Any change since your last check?"},
}

// The messages the use measures read (1-based): the subscriber's next
// turn after a notice.
const (
	useDocsVolume = 7  // the docs show volume (schema change at 2)
	useReviewer   = 8  // the reviewer looks at r_101 (failed at 10:00)
	useReporter   = 11 // the draft has today's drop (published at 9 in Part 2)
	useDocsEth    = 13 // the docs show eth_prices (schema change at 10)
	needMsg       = 9  // the price watcher's turn whose move crosses 5%
)

func clock(hm string) time.Time {
	t, err := time.Parse("15:04", hm)
	if err != nil {
		panic(err)
	}
	return time.Date(today.Year(), today.Month(), today.Day(), t.Hour(), t.Minute(), 0, 0, time.UTC)
}
