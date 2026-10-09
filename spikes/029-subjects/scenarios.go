package main

// Message kinds.
const (
	kindWork     = "work"     // creates or decides something: a subject is expected
	kindChange   = "change"   // changes earlier work: its subject is updated, not a new one created
	kindQuestion = "question" // a plain question: no subject
	kindFollowup = "followup" // asks about earlier turns: no subject, the answer is checked
)

// message is one scripted user message and how its turn is scored.
type message struct {
	Kind  string
	Topic string // the piece of work it belongs to; "" for questions and follow-ups
	Text  string
	// Status: after this message, the topic's subject has one of these
	// statuses (conditions 2–4).
	Status []string
	// Answer: the turn's final text matches every pattern. Patterns ignore
	// case.
	Answer []string
	// Call: one call of CallTool in the turn has arguments that match every
	// pattern; "" for CallTool means any write tool.
	CallTool string
	Call     []string
}

type scenarioDef struct {
	ID       string
	Title    string
	Messages []message
}

// The runs use history_max_turns 4 and history_min_turns 2, so the window
// is cut at turns 6, 9 and 12, and the follow-ups ask about turns that are
// out of the window by then.
var scenarios = []scenarioDef{
	{
		ID:    "crypto",
		Title: "Crypto prices",
		Messages: []message{
			{Kind: kindWork, Topic: "btc", Text: "Create a table btc_prices for daily Bitcoin prices with date, open, close and volume. The date is the key."},
			{Kind: kindWork, Topic: "btc", Text: "Add a pipeline daily_btc that gets yesterday's BTC price from CoinGecko every day at 08:00 and writes it to btc_prices. Run it once to test it."},
			{Kind: kindWork, Topic: "currency", Text: "Show all prices in EUR, not USD. I'm in Germany and compare them with my bank statements. Remember that.", Status: []string{"done"}},
			{Kind: kindWork, Topic: "alerts", Text: "I want an alert when BTC drops more than 5% in a day. Don't build it yet: I haven't decided whether alerts go to email or Telegram. Keep it open until I decide.", Status: []string{"open", "blocked"}},
			{Kind: kindQuestion, Text: "What's the difference between the open and the close price?"},
			{Kind: kindWork, Topic: "btc", Text: "Make a view btc_chart with the closing price of the last 30 days."},
			{Kind: kindChange, Topic: "btc", Text: "Change daily_btc to run at 07:00 instead, and skip weekends: I only check prices on workdays.",
				Status: []string{"done"}, CallTool: "save_pipeline", Call: []string{`0 7 |07:00|7:00`, `1-5|mon-fri|weekday|weekend|workday`}},
			{Kind: kindWork, Topic: "eth", Text: "Now do the same for Ethereum: a table, a pipeline and a chart, set up like Bitcoin's.",
				CallTool: "save_pipeline", Call: []string{`eth`, `0 7 |07:00|7:00`, `1-5|mon-fri|weekday|weekend|workday`}},
			{Kind: kindQuestion, Text: "Roughly how many rows will btc_prices have after a year?"},
			{Kind: kindChange, Topic: "alerts", Text: "About the alert I mentioned earlier: send it to Telegram. Set it up now.",
				Status: []string{"done", "in_progress"}, Call: []string{`5 ?%|0\.05|five percent|5 percent|-5|>= ?5|> ?5`, `telegram`}},
			{Kind: kindQuestion, Text: "Is CoinGecko's API free for this?"},
			{Kind: kindFollowup, Text: "Why did we stop collecting weekend prices?", Answer: []string{`work ?days?|weekdays?|only check`}},
			{Kind: kindChange, Topic: "alerts", Text: "Actually, drop the Telegram alert. I'll use my exchange's own price alerts instead.", Status: []string{"dropped"}},
			{Kind: kindFollowup, Text: "Give me a short list of what we set up in this chat, and what we dropped.", Answer: []string{`btc|bitcoin`, `eth`, `alert`, `drop|cancel|remov`}},
		},
	},
	{
		ID:    "jobs",
		Title: "Job applications",
		Messages: []message{
			{Kind: kindWork, Topic: "apps", Text: "Create a table applications with company, role, applied (a date), status and link. Company plus role is the key."},
			{Kind: kindWork, Topic: "apps", Text: "Add these three: Zalando, Data Analyst, applied 2026-09-20, waiting. N26, Backend Engineer, applied 2026-09-25, interview. SAP, Data Engineer, applied 2026-10-01, waiting."},
			{Kind: kindWork, Topic: "status_rule", Text: "From now on, when I say a company rejected me, set the status to closed, not rejected. I want only waiting, interview and closed in my reports. Remember this.", Status: []string{"done"}},
			{Kind: kindQuestion, Text: "How long should I wait before following up on an application?"},
			{Kind: kindWork, Topic: "follow_up", Text: "Add a pipeline follow_up that runs every Monday at 09:00 and lists the applications that have been waiting for more than 14 days."},
			{Kind: kindWork, Topic: "zalando_watch", Text: "I also want a pipeline that checks Zalando's careers page every day for new data jobs. I'll send you the page link later, so wait with it until then.", Status: []string{"open", "blocked"}},
			{Kind: kindWork, Topic: "board", Text: "Make a view board that groups the applications by status."},
			{Kind: kindQuestion, Text: "Any tips for a thank-you email after an interview?"},
			{Kind: kindChange, Topic: "apps", Text: "N26 rejected me.", CallTool: "update_rows", Call: []string{`n26`, `closed`}},
			{Kind: kindChange, Topic: "follow_up", Text: "Make follow_up run on Fridays instead, and use 10 days instead of 14.",
				Status: []string{"done"}, CallTool: "save_pipeline", Call: []string{`fri|\* \* 5`, `10 ?day|-10|> ?10|>= ?10`}},
			{Kind: kindChange, Topic: "zalando_watch", Text: "Here's the Zalando link: https://jobs.zalando.com/en/jobs?search=data. Go ahead with it.",
				Status: []string{"done", "in_progress"}, CallTool: "save_pipeline", Call: []string{`zalando`, `data`}},
			{Kind: kindQuestion, Text: "What's a typical salary range for a data engineer in Berlin?"},
			{Kind: kindFollowup, Text: "What happened with my N26 application, and why does it say closed?", Answer: []string{`reject`, `closed`}},
			{Kind: kindFollowup, Text: "What have we set up so far, and is anything still waiting on me?", Answer: []string{`follow.?up`, `board`, `zalando`}},
		},
	},
}
