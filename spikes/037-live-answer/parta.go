package main

// Part A's tests: requests that start 3 or 4 background subagents whose
// results arrive one after another (SPIKE-023's multi-part pattern). The
// harness delivers the finish notices in each test's order, each once the
// chat is idle, so every model gets the same sequence however fast it is.

// scripted is a user message. After: Part A sends it once this many
// notice turns have been delivered.
type scripted struct {
	Text  string
	After int
}

type testDef struct {
	ID       string
	Part     string // A or B
	Kind     string // A: text, table, mix, failed, empty, together, late, persian
	Messages []scripted
	Tasks    []*taskRule
	Persian  bool
	// Part B
	Window string        // "today" (the default sizes) or "low"
	Checks map[int]check // by message index
	World  *webWorld
}

var jobRules = []*taskRule{
	{Name: "linkedin", Match: `linkedin|لینکدین|لینکداین`, Order: 1,
		Result: "LinkedIn, 2 junior data analyst roles in Berlin:\n1. Zalando — Junior Data Analyst (Fashion Store) — €52,000 a year — posted 2 days ago.\n2. Delivery Hero — Data Analyst (Junior) — €49,000 a year — posted 5 days ago.",
		Facts:  []string{`zalando|زالاندو`, `delivery ?hero|دلیوری ?هیرو`, `52`, `49`}},
	{Name: "stepstone", Match: `step ?stone|استپ ?استون`, Order: 2,
		Result: "StepStone, 2 roles:\n1. Siemens Energy — Junior BI Analyst — €55,000 a year.\n2. HelloFresh — Data Analyst Trainee — €46,000 a year.",
		Facts:  []string{`siemens|زیمنس`, `hello ?fresh|هلو ?فرش`, `55`, `46`}},
	{Name: "indeed", Match: `indeed|ایندید`, Order: 3,
		Result: "Indeed, 2 roles:\n1. N26 — Junior Data Analyst, Risk — €54,000 a year.\n2. Trade Republic — Analyst, Data & Reporting — salary not listed.",
		Facts:  []string{`n ?26`, `trade ?republic|ترید ?ریپابلیک`, `54`}},
	{Name: "xing", Match: `xing|زینگ|شینگ`, Order: 4,
		Result: "Xing, 2 roles:\n1. Flix — Junior Data Analyst, Network Planning — €50,000 a year.\n2. Zalando — Junior Data Analyst (Fashion Store) — €52,000 a year (the same posting as on LinkedIn).",
		Facts:  []string{`flix|فلیکس`, `50`}},
}

var partA = []testDef{
	{ID: "a_text", Part: "A", Kind: "text",
		Messages: []scripted{{Text: "I want to go hiking near Garmisch-Partenkirchen this Saturday. Start three background subagents: one for the weather forecast, one for the trail conditions on the Partnachklamm–Eckbauer route, and one for train connections from Munich. Meanwhile tell me what to pack, and give me your recommendation for the day as the results come in."}},
		Tasks: []*taskRule{
			{Name: "weather", Match: `weather|forecast|wetter`, Order: 1,
				Result: "Forecast for Garmisch-Partenkirchen, Saturday: sunny in the morning, thunderstorms likely from 15:00, high 21 °C, freezing level at 3,400 m.",
				Facts:  []string{`thunder|storm`, `15[:.]00|3 ?pm|15 ?h`, `21`}},
			{Name: "trail", Match: `trail|partnach|eckbauer|gorge|path`, Order: 2,
				Result: "Partnachklamm gorge: open, entry €8, opens at 08:00. The Eckbauer path is muddy after Thursday's rain. The Eckbauerbahn cable car is closed for maintenance until 3 November, so the descent is on foot (about 1 h 15 min).",
				Facts:  []string{`€ ?8\b|\b8 ?(€|eur)`, `cable ?car|eckbauerbahn|gondola|lift`, `mud`}},
			{Name: "trains", Match: `train|connection|rail|\bbahn|\bdb\b|munich|münchen`, Order: 3,
				Result: "Munich Hbf → Garmisch-Partenkirchen: RB 6 every hour from 06:32, 1 h 22 min. The Bayern-Ticket costs €29 for one person and is valid all day on weekends.",
				Facts:  []string{`0?6[:.]32`, `29`, `bayern`}},
		}},
	{ID: "a_table", Part: "A", Kind: "table",
		Messages: []scripted{{Text: "Find junior data analyst jobs in Berlin. Start one background subagent per source: LinkedIn, StepStone, Indeed and Xing. Keep one table of all the jobs with company, role, salary and source, and add to it as the results come in."}},
		Tasks:    jobRules},
	{ID: "a_mix", Part: "A", Kind: "mix",
		Messages: []scripted{{Text: "I'm choosing a laptop for university: Lenovo ThinkPad X13 Gen 6, Apple MacBook Air 13 M5 or Dell XPS 13 9360. Start one background subagent per laptop to find its price in Germany, its battery life and its weight. Give me a comparison table and your recommendation, and update them as the results come in."}},
		Tasks: []*taskRule{
			{Name: "thinkpad", Match: `thinkpad|lenovo|x13`, Order: 1,
				Result: "Lenovo ThinkPad X13 Gen 6: €1,149 at Cyberport; 10% student discount through Lenovo Education; battery 14 h in a video test; weight 1.10 kg.",
				Facts:  []string{`1[.,]?149`, `14 ?h|14 hours|14 ?std`, `1[.,]10? ?kg`}},
			{Name: "macbook", Match: `macbook|apple|air`, Order: 2,
				Result: "Apple MacBook Air 13 M5: €1,199 with Apple Education pricing (€1,299 regular); battery 18 h in a video test; weight 1.24 kg.",
				Facts:  []string{`1[.,]?199`, `18 ?h|18 hours|18 ?std`, `1[.,]24`}},
			{Name: "dell", Match: `dell|xps|9360`, Order: 3,
				Result: "Dell XPS 13 9360: €1,349 at dell.de, no student discount found; battery 11 h in a video test; weight 1.19 kg.",
				Facts:  []string{`1[.,]?349`, `11 ?h|11 hours|11 ?std`, `1[.,]19`}},
		}},
	{ID: "a_failed", Part: "A", Kind: "failed",
		Messages: []scripted{{Text: "Find one-bedroom flats in Leipzig for under €700 a month, heating included. Start one background subagent per site: ImmoScout24, WG-Gesucht and Kleinanzeigen. Keep one table of all the flats with address, size, rent and site, and add to it as the results come in."}},
		Tasks: []*taskRule{
			{Name: "immoscout", Match: `immo ?scout`, Order: 1,
				Result: "ImmoScout24, 2 flats:\n1. Karl-Liebknecht-Str. 41 — 42 m² — €640 warm.\n2. Eisenbahnstr. 112 — 38 m² — €560 warm.",
				Facts:  []string{`liebknecht`, `640`, `eisenbahn`, `560`}},
			{Name: "wggesucht", Match: `wg[- ]?gesucht`, Order: 2,
				Result: "WG-Gesucht, 2 flats:\n1. Josephinenstr. 9 — 45 m² — €690 warm, from 1 December.\n2. Holbeinstr. 28 — 35 m² — €610 warm.",
				Facts:  []string{`josephinen`, `690`, `holbein`, `610`}},
			{Name: "kleinanzeigen", Match: `kleinanzeigen`, Order: 3, Fail: true,
				Result: "Kleinanzeigen blocked automated access (HTTP 403); no listings were read."},
		}},
	{ID: "a_empty", Part: "A", Kind: "empty",
		Messages: []scripted{{Text: "Check three news sources for anything about the planned Deutsche Bahn strike next week: one background subagent each for Tagesschau, Spiegel and the GDL union's website. Summarize what's known so far, and update the summary as results come in."}},
		Tasks: []*taskRule{
			{Name: "tagesschau", Match: `tagesschau`, Order: 1,
				Result: "Tagesschau: the GDL announced a strike in regional and long-distance passenger traffic from Wednesday 02:00 to Thursday 18:00. DB will publish an emergency timetable on Monday.",
				Facts:  []string{`wednesday|mittwoch`, `thursday|donnerstag`, `emergency timetable|notfahrplan`}},
			{Name: "spiegel", Match: `spiegel`, Order: 2, Empty: true,
				Result: "Spiegel: no new information; nothing beyond what other outlets already reported."},
			{Name: "gdl", Match: `gdl|union|gewerkschaft`, Order: 3,
				Result: "GDL press release: the strike also covers freight traffic, which starts on Tuesday at 18:00. Talks failed over the 35-hour week.",
				Facts:  []string{`freight|güter`, `tuesday|dienstag`, `35`}},
		}},
	{ID: "a_together", Part: "A", Kind: "together",
		Messages: []scripted{{Text: "Compare ticket prices for the Ed Sheeran concert in Berlin's Olympiastadion on 14 March. Start one background subagent each for Eventim, Ticketmaster and viagogo, keep one table with category, price and site, and add to it as the results come in."}},
		Tasks: []*taskRule{
			{Name: "eventim", Match: `eventim`, Order: 1,
				Result: "Eventim: standing €89.90, seated category 1 €149.90, category 3 €99.90; up to 4 tickets per order.",
				Facts:  []string{`89[.,]90`, `149[.,]90`, `99[.,]90`}},
			{Name: "ticketmaster", Match: `ticket ?master`, Order: 2,
				Result: "Ticketmaster: sold out; only the official resale has standing tickets at €104.50.",
				Facts:  []string{`sold out|ausverkauft`, `104[.,]50`}},
			{Name: "viagogo", Match: `viagogo`, Order: 2,
				Result: "viagogo (resale): standing from €178, category 1 from €265; prices include fees.",
				Facts:  []string{`178`, `265`}},
		}},
	{ID: "a_late", Part: "A", Kind: "late",
		Messages: []scripted{
			{Text: "Find bike shops in Cologne that repair e-bikes. Start one background subagent each for Google Maps, Yelp and the ADAC's workshop finder, keep one table with shop, address and opening hours, and add to it as the results come in."},
			{Text: "By the way, I can only go on Saturdays.", After: 1}},
		Tasks: []*taskRule{
			{Name: "maps", Match: `google|maps`, Order: 1,
				Result: "Google Maps, 2 shops:\n1. Rad & Tat — Venloer Str. 210 — Mon–Fri 10:00–19:00, Sat 10:00–14:00.\n2. E-Motion Köln — Aachener Str. 75 — Mon–Fri 09:00–18:00, closed on Saturdays.",
				Facts:  []string{`rad ?& ?tat`, `venloer`, `e-?motion`, `aachener`}},
			{Name: "yelp", Match: `yelp`, Order: 2,
				Result: "Yelp, 1 shop: Veloverde — Zülpicher Str. 18 — Tue–Sat 10:00–18:00.",
				Facts:  []string{`veloverde`, `zülpicher|zuelpicher`}},
			{Name: "adac", Match: `adac|workshop`, Order: 3,
				Result: "ADAC workshop finder, 1 shop: Pedelec-Werkstatt Ehrenfeld — Subbelrather Str. 300 — Mon–Fri 08:00–17:00, closed on Saturdays.",
				Facts:  []string{`pedelec`, `subbelrather`}},
		}},
	{ID: "a_persian", Part: "A", Kind: "persian", Persian: true,
		Messages: []scripted{{Text: "برای من آگهی‌های شغلی «تحلیلگر داده جونیور» در برلین پیدا کن. برای هر منبع یک ساب‌ایجنت پس‌زمینه راه بنداز: LinkedIn، StepStone و Indeed. یک جدول از همه‌ی شغل‌ها با شرکت، عنوان شغل، حقوق و منبع نگه دار و هر وقت نتیجه‌ای رسید به آن اضافه کن."}},
		Tasks:    jobRules[:3]},
}
