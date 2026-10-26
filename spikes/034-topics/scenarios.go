package main

// Message kinds.
const (
	kindWork     = "work"     // creates or decides something
	kindChange   = "change"   // changes earlier work, or changes the user's mind on it
	kindQuestion = "question" // a plain question: no topic, no subject
	kindCross    = "cross"    // a question that spans subjects, asked in the main thread
	kindFollowup = "followup" // asks about an older turn of one subject
	kindMerge    = "merge"    // says two subjects are one thing
)

// mainLabel is the label of messages that belong in the main thread.
const mainLabel = "main"

// message is one scripted user message and how its turn is scored.
type message struct {
	Label string // its subject, or mainLabel
	Kind  string
	Text  string
	// Parallel: sent at the same time as the message before it, each in
	// its own topic (the ticket's parallel step).
	Parallel bool
	// Waiting: the message leaves work waiting, which should show as an
	// open item in the notes of the thread that answered it (conditions
	// 2–3) or in its subject (condition 4).
	Waiting bool
	// Decision: a question in the main thread about a decision made in a
	// topic (the main thread's reach).
	Decision bool
	// Answer: the turn's final text matches every pattern (ignoring case).
	Answer []string
	// Call: one call of CallTool in the message's turns has arguments that
	// match every pattern.
	CallTool string
	Call     []string
	// Done: an earlier write call whose arguments match every pattern did
	// the work already, so the call check passes without a call.
	Done []string
	// Merge: the labels that are one thing for the user (kindMerge).
	Merge []string
	// Memory: after the message (and its parallel partner), project
	// memory matches every pattern.
	Memory []string
}

type scenarioDef struct {
	ID    string
	Title string
	Role  string
	// Titles are the topic titles the scripted user gives in condition 2,
	// by label.
	Titles   map[string]string
	Messages []message
}

// The runs use history_max_turns 4 and history_min_turns 2, as SPIKE-029
// did, so a chat with one thread is cut every few turns and the
// follow-ups ask about turns that are out of its window by then.
var scenarios = []scenarioDef{
	{
		ID:    "sqlite",
		Title: "SQLite expert",
		Role:  "You are a SQLite expert. You design and change this project's tables and answer database questions.",
		Titles: map[string]string{
			"users": "Users table", "orders": "Orders table", "products": "Products table", "inventory": "Inventory",
		},
		Messages: []message{
			{Label: "users", Kind: kindWork, Text: "Create a table users with id, email, name and created_at. id is the key. Make email unique: people log in with their email."},
			{Label: "users", Kind: kindWork, Text: "Add three users: Ana (ana@example.org), Ben (ben@example.org) and Cem (cem@example.org)."},
			{Label: "orders", Kind: kindWork, Text: "Next, a table orders with id, user_id, total and created_at. Use a text key like ORD-0001, not a number: I read order numbers to customers on the phone."},
			{Label: mainLabel, Kind: kindQuestion, Text: "What's the difference between INTEGER PRIMARY KEY and the rowid in SQLite?"},
			{Label: "products", Kind: kindWork, Text: "Create a table products with sku, name and price. sku is the key."},
			{Label: "orders", Kind: kindWork, Text: "In orders, user_id must point to users.id.",
				Call: []string{`orders`, `users`}, Done: []string{`"orders"`, `user_id`, `references"\s*:\s*"users\.id`}},
			{Label: mainLabel, Kind: kindQuestion, Text: "Is SQLite fine for about 10,000 orders a year?"},
			{Label: "inventory", Kind: kindWork, Waiting: true, Text: "I need stock counts too: a table inventory with sku and quantity. I also want a pipeline restock_check every morning at 07:00 that lists the skus with fewer than 5 in stock, but wait with the pipeline: I haven't decided whether it should email me or post to Slack."},
			{Label: mainLabel, Kind: kindCross, Text: "How do users and orders relate?",
				Answer: []string{`user_id`, `users?\.id|id of (the )?users?|users? table'?s? id`}},
			{Label: "products", Kind: kindChange, Text: "Add a column discount to products, 0 by default.",
				CallTool: "alter_table", Call: []string{`products`, `discount`}},
			{Label: "orders", Kind: kindChange, Text: "Change of mind on orders: use a plain INTEGER key after all. The shop system prints its own order number, so I don't need ORD-0001.",
				Call: []string{`orders`, `integer`}},
			{Label: "users", Kind: kindWork, Text: "Remember this for the whole project: all timestamps are stored as UTC text in ISO 8601."},
			{Label: "orders", Kind: kindWork, Parallel: true, Text: "Remember this for the whole project: money is stored as whole cents in INTEGER columns, never as REAL.",
				Memory: []string{`utc|iso.?8601`, `cent`}},
			{Label: mainLabel, Kind: kindCross, Decision: true, Text: "Did we settle the key for orders?",
				Answer: []string{`integer`}},
			{Label: mainLabel, Kind: kindMerge, Text: "Products and inventory are one thing for me: my catalog. Keep them together from now on.",
				Merge: []string{"products", "inventory"}},
			{Label: mainLabel, Kind: kindQuestion, Text: "Should I turn on WAL mode for this database?"},
			{Label: "users", Kind: kindFollowup, Text: "Why did we make email unique in users?",
				Answer: []string{`log ?in|logs in|sign ?in|login`}},
			{Label: "products", Kind: kindFollowup, Text: "Is the restock check done?",
				Answer: []string{`email|slack`}},
			{Label: mainLabel, Kind: kindCross, Text: "What's still open everywhere?",
				Answer: []string{`restock`}},
		},
	},
	{
		ID:    "jobs",
		Title: "Job search",
		Role:  "You help me run my job search: applications, follow-ups and job alerts.",
		Titles: map[string]string{
			"apps": "Applications", "follow_up": "Follow-up pipeline", "zalando": "Zalando job watch", "board": "Board view",
		},
		Messages: []message{
			{Label: "apps", Kind: kindWork, Text: "Create a table applications with company, role, applied (a date), status and link. Company plus role is the key."},
			{Label: "apps", Kind: kindWork, Text: "Add these three: Zalando, Data Analyst, applied 2026-09-20, waiting. N26, Backend Engineer, applied 2026-09-25, interview. SAP, Data Engineer, applied 2026-10-01, waiting."},
			{Label: "apps", Kind: kindWork, Text: "When I say a company rejected me, set the status to closed, not rejected: my reports should only show waiting, interview and closed. Remember this for the project.",
				Memory: []string{`closed`}},
			{Label: mainLabel, Kind: kindQuestion, Text: "How long should I wait before following up on an application?"},
			{Label: "follow_up", Kind: kindWork, Text: "Add a pipeline follow_up that runs every Monday at 09:00 and lists the applications that have been waiting for more than 14 days."},
			{Label: "zalando", Kind: kindWork, Waiting: true, Text: "I also want a pipeline that checks Zalando's careers page every day for new data jobs. I'll send you the page link later, so wait with it until then."},
			{Label: mainLabel, Kind: kindQuestion, Text: "Any tips for a thank-you email after an interview?"},
			{Label: "board", Kind: kindWork, Text: "Make a view board that groups the applications by status."},
			{Label: "apps", Kind: kindChange, Text: "N26 rejected me.",
				CallTool: "update_rows", Call: []string{`n26`, `closed`}},
			{Label: mainLabel, Kind: kindCross, Text: "Which of my applications are still waiting, and is anything automated for them yet?",
				Answer: []string{`sap`, `follow.?up`}},
			{Label: "follow_up", Kind: kindChange, Text: "Make follow_up run on Fridays instead, and use 10 days instead of 14.",
				CallTool: "save_pipeline", Call: []string{`fri|\* \* 5`, `10 ?day|-10|> ?10|>= ?10`}},
			{Label: "apps", Kind: kindWork, Text: "Remember this for the whole project: I only apply to jobs in Berlin or remote."},
			{Label: "zalando", Kind: kindWork, Parallel: true, Text: "Remember this for the whole project: job alerts should only include roles with \"data\" in the title.",
				Memory: []string{`berlin`, `data`}},
			{Label: mainLabel, Kind: kindMerge, Text: "follow_up and the board are really one thing for me: my Friday review, where I go through both together. Keep them together from now on.",
				Merge: []string{"follow_up", "board"}},
			{Label: mainLabel, Kind: kindCross, Decision: true, Text: "What day does follow_up run now, and after how many days does it list an application?",
				Answer: []string{`friday`, `\b10\b|ten`}},
			{Label: "zalando", Kind: kindChange, Text: "Here's the Zalando link: https://jobs.zalando.com/en/jobs?search=data. Go ahead with it.",
				CallTool: "save_pipeline", Call: []string{`zalando`, `data`}},
			{Label: mainLabel, Kind: kindQuestion, Text: "What's a typical salary range for a data engineer in Berlin?"},
			{Label: "apps", Kind: kindFollowup, Text: "What happened with my N26 application, and why does it say closed?",
				Answer: []string{`reject`, `closed`}},
			{Label: mainLabel, Kind: kindCross, Text: "What have we set up so far, and is anything still waiting on me?",
				Answer: []string{`follow.?up`, `board`, `zalando`}},
		},
	},
}
