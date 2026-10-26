package main

// Part B's chats: 15 scripted messages each, with tool use in most turns
// and two background tasks, so 17 turns. The follow-ups need a fact from
// an earlier answer, a detail only in a tool result, the source, a name
// the agent made (a table, a ref, a task ID), and more of the same work.
// The jobs chat runs with today's window sizes, the trip chat with a low
// history_max_turns, so cuts happen in both.

// check is how a follow-up is scored.
type check struct {
	Kind   string   // fact, detail, source, table, task, ref, more, claims
	Answer []string // the answer matches every regex (case-insensitive)
	Any    []string // ...and at least one of these
	Not    []string // ...and none of these: wrong claims
	// Names: for fact, the names whose mention in turn 1's answer the
	// follow-up must repeat (at least two of those turn 1 named).
	Names []string
	Rule  string // task: the rule of the task whose ID task_status needs
	Pages int    // more: new pages the turn must fetch
}

var jobsWeb = &webWorld{
	Searches: []search{
		{Match: `indeed`, Results: `1. "Junior Data Analyst, Risk — N26" — jobs.example.com/n26-risk — Berlin, full time.
2. "Analyst, Data & Reporting — Trade Republic" — jobs.example.com/trade-republic-reporting — Berlin, posted 1 day ago.
3. "Werkstudent Data Analytics — Siemens Energy" — jobs.example.com/siemens-werkstudent — Berlin, 20 h a week.`},
		{Match: `glassdoor`, Results: `1. "Junior Data Analyst (Fashion Store) — Zalando" — jobs.example.com/zalando-jda — rated 4.1.
2. "Junior Data Analyst, Operations — Delivery Hero" — jobs.example.com/delivery-hero-ops — rated 3.9, posted 3 days ago.`},
		{Match: `data|analyst|job|berlin`, Results: `1. "Junior Data Analyst (Fashion Store) — Zalando" — jobs.example.com/zalando-jda — Berlin, posted 2026-10-20: "Join our pricing analytics team…"
2. "Junior Data Analyst, Risk — N26" — jobs.example.com/n26-risk — Berlin, posted 2026-10-22: "Help us spot fraud patterns…"
3. "Junior Data Analyst, Network Planning — Flix" — jobs.example.com/flix-network — Berlin, posted 2026-10-18.
4. "Data Analyst Trainee — HelloFresh" — jobs.example.com/hellofresh-trainee — Berlin, 12-month programme.
5. "Junior BI Analyst — Siemens Energy" — jobs.example.com/siemens-bi — Berlin, posted 2026-10-12.`},
	},
	Pages: []page{
		{URL: "jobs.example.com/zalando-jda", Title: "Junior Data Analyst (Fashion Store) — Zalando", Text: `Zalando SE — Junior Data Analyst (Fashion Store), Berlin (Friedrichshain)

About the team: the pricing analytics team of 9 people decides discounts for the fashion store in 25 markets.

What you'll do:
- Build dashboards in Looker on sales and discount data.
- Write SQL against our data warehouse (BigQuery).
- Run A/B test analyses with the experimentation team.

What we look for:
- A degree in economics, statistics, computer science or similar.
- Good SQL; some Python (pandas) is a plus.
- English at C1; German is not required.

What we offer: a salary of €52,000 a year, 28 days of holiday, up to 2 days a week working from home, a Deutschlandticket.

Apply by 15 November 2026. Contact: Lena Hoffmann, talent acquisition.`},
		{URL: "jobs.example.com/n26-risk", Title: "Junior Data Analyst, Risk — N26", Text: `N26 — Junior Data Analyst, Risk, Berlin (Mitte)

About the team: you join the fraud analytics team, 7 analysts who look for fraud patterns in card payments across 24 countries.

What you'll do:
- Analyse transaction data in Snowflake with SQL and Python.
- Build alerting rules with the fraud operations team.
- Report weekly fraud figures to the Head of Risk.

What we look for:
- 0–2 years of experience in analytics, ideally in finance.
- Strong SQL, basic statistics, Python.
- English; German is a plus.

What we offer: €54,000 a year, an office in Mitte with 3 office days a week, a learning budget of €1,000.

Apply by 30 November 2026. Contact: Jonas Becker, recruiting.`},
		{URL: "jobs.example.com/flix-network", Title: "Junior Data Analyst, Network Planning — Flix", Text: `Flix SE — Junior Data Analyst, Network Planning, Berlin

You help plan bus and train routes: demand forecasts, seat utilisation, new connections. Team of 12. Tools: SQL, Tableau, Python.
We offer €50,000 a year, hybrid work (2 days at home) and free FlixBus rides. Apply by 10 December 2026. Contact: Mara Schulz.`},
		{URL: "jobs.example.com/hellofresh-trainee", Title: "Data Analyst Trainee — HelloFresh", Text: `HelloFresh — Data Analyst Trainee (12 months), Berlin

Rotations through supply chain, marketing and finance analytics. €46,000 a year. Starts 1 February 2027. Contact: Aylin Demir.`},
		{URL: "jobs.example.com/siemens-bi", Title: "Junior BI Analyst — Siemens Energy", Text: `Siemens Energy — Junior BI Analyst, Berlin (Siemensstadt)

Power BI reports for the gas turbine service business. €55,000 a year, 30 days of holiday. German at B2 required. Contact: Thomas Wagner.`},
		{URL: "jobs.example.com/trade-republic-reporting", Title: "Analyst, Data & Reporting — Trade Republic", Text: `Trade Republic — Analyst, Data & Reporting, Berlin (Mitte)

You build the regulatory and management reporting for our brokerage: SQL, dbt and Python. Team of 5. Salary on request; the range is €55,000–€62,000. Four office days a week. Apply by 20 November 2026. Contact: Sofia Rossi.`},
		{URL: "jobs.example.com/delivery-hero-ops", Title: "Junior Data Analyst, Operations — Delivery Hero", Text: `Delivery Hero — Junior Data Analyst, Operations, Berlin (Kreuzberg)

Analyse delivery times and rider supply for 3 markets. Team of 6. SQL and Looker. €49,000 a year, hybrid (3 days at home). Apply by 5 December 2026. Contact: Kemal Yilmaz.`},
		{URL: "jobs.example.com/siemens-werkstudent", Title: "Werkstudent Data Analytics — Siemens Energy", Text: `Siemens Energy — Werkstudent Data Analytics, Berlin. 20 hours a week, €17 an hour, for enrolled students only.`},
	},
}

var tripWeb = &webWorld{
	Searches: []search{
		{Match: `cais|sodr`, Results: `1. "Cais do Sodré Loft" — travel.example.com/hotels/cais-sodre-loft — rated 9.0, from €139 a night.
2. "Baixa House Apartments" — travel.example.com/hotels/baixa-house — rated 8.9, from €128 a night, 8 minutes' walk from Cais do Sodré.
3. "River Lodge Lisboa" — travel.example.com/hotels/river-lodge — rated 8.4, from €118 a night.`},
		{Match: `fado`, Results: `1. "Fado in Alfama tonight" — travel.example.com/fado — shows at Clube de Fado and Tasca do Chico.`},
		{Match: `hotel|alfama|baixa|lisbon|lisboa|stay`, Results: `1. "Alfama Patio Hotel" — travel.example.com/hotels/alfama-patio — rated 8.8, from €135 a night.
2. "Baixa House Apartments" — travel.example.com/hotels/baixa-house — rated 8.9, from €128 a night.
3. "Memmo Alfama" — travel.example.com/hotels/memmo-alfama — rated 9.2, from €240 a night.
4. "Lisbon Story Guesthouse" — travel.example.com/hotels/lisbon-story — rated 8.6, from €95 a night, shared bathrooms.
5. "Casa Amora" — travel.example.com/hotels/casa-amora — rated 9.1, from €160 a night.`},
	},
	Pages: []page{
		{URL: "travel.example.com/hotels/alfama-patio", Title: "Alfama Patio Hotel", Text: `Alfama Patio Hotel — Escadinhas das Portas do Sol 1, Alfama, Lisbon. Rated 8.8 from 2,340 reviews.

Rooms: double room with patio view, €135 a night for 12–16 March; breakfast buffet included.
Location: 2 minutes' walk to the tram 28 stop Largo das Portas do Sol; 15 minutes' walk to Baixa.
Building: a renovated 18th-century house with no elevator; rooms are on floors 1 to 3, and the staff carry luggage up.
Check-in from 14:00, check-out until 11:00. Free cancellation until 7 days before arrival; after that, the first night is charged.
Rooftop bar open daily from 17:00.`},
		{URL: "travel.example.com/hotels/baixa-house", Title: "Baixa House Apartments", Text: `Baixa House Apartments — Rua dos Fanqueiros 81, Baixa, Lisbon. Rated 8.9 from 1,120 reviews.

Apartments: one-bedroom apartment with kitchenette, €128 a night for 12–16 March; no breakfast, but a café on the ground floor.
Location: 3 minutes' walk to Praça do Comércio, 8 minutes to Cais do Sodré station.
Building: elevator to all floors; washing machine in every apartment.
Check-in from 15:00 with a door code sent by text message; check-out until 10:00.
Non-refundable rate; a flexible rate is €146 a night with free cancellation until 48 hours before arrival.`},
		{URL: "travel.example.com/hotels/memmo-alfama", Title: "Memmo Alfama", Text: `Memmo Alfama — boutique hotel with a pool terrace over the river. €240 a night, breakfast included, elevator, check-in from 15:00. Free cancellation until 3 days before arrival.`},
		{URL: "travel.example.com/hotels/lisbon-story", Title: "Lisbon Story Guesthouse", Text: `Lisbon Story Guesthouse — Largo de São Domingos, Baixa. €95 a night, shared bathrooms, no elevator, check-in from 13:00.`},
		{URL: "travel.example.com/hotels/casa-amora", Title: "Casa Amora", Text: `Casa Amora — guesthouse in Amoreiras, 25 minutes by metro from Baixa. €160 a night, homemade breakfast, garden, check-in from 15:00.`},
		{URL: "travel.example.com/hotels/cais-sodre-loft", Title: "Cais do Sodré Loft", Text: `Cais do Sodré Loft — Rua Nova do Carvalho 34. Studio lofts at €139 a night for 12–16 March, breakfast €12 extra. Elevator. Check-in from 16:00. Free cancellation until 5 days before arrival. Note: the street is a nightlife street; rooms at the back are quieter.`},
		{URL: "travel.example.com/hotels/river-lodge", Title: "River Lodge Lisboa", Text: `River Lodge Lisboa — Avenida 24 de Julho 52. €118 a night, breakfast included, no elevator, tram stop outside. Check-in from 14:00.`},
	},
}

var partB = []testDef{
	{ID: "b_jobs", Part: "B", Window: "today", World: jobsWeb,
		Messages: []scripted{
			{Text: "I'm looking for a junior data analyst job in Berlin. Search the web for current openings and tell me the three most promising."},
			{Text: "Open the Zalando and the N26 listings and tell me what each requires."},
			{Text: "I like both. Make a table to keep a shortlist of jobs I like, with company, role, salary and link, and add these two."},
			{Text: "Start a background search on LinkedIn for similar junior data roles in Berlin, so I don't have to wait. Meanwhile, how should I prepare for a SQL test?"},
			{Text: "In two sentences, what's the difference between a data analyst and a data engineer?"},
			{Text: "Add the LinkedIn roles that pay at least €50,000 to my shortlist."},
			{Text: "How big is the team for the N26 role?"},
			{Text: "Where did you read that?"},
			{Text: "Start another background search, on StepStone this time."},
			{Text: "Is the StepStone search done yet?"},
			{Text: "Look at the Zalando listing again: does it allow working from home?"},
			{Text: "Search two more sources for openings, Indeed and Glassdoor, and open the most promising new listing from each."},
			{Text: "Which three openings did you recommend at the very start?"},
			{Text: "List every source we've used in this chat so far, and what each gave us."},
			{Text: "Show me what's on my shortlist now."},
		},
		Tasks: []*taskRule{
			{Name: "linkedin", Match: `linkedin`, After: 3,
				Result: "LinkedIn, 3 roles in Berlin:\n1. Trade Republic — Junior Data Analyst, Payments — €56,000 a year.\n2. Delivery Hero — Data Analyst (Junior) — €49,000 a year.\n3. GetYourGuide — Junior Product Analyst — €51,000 a year."},
			{Name: "stepstone", Match: `step ?stone`, After: 9,
				Result: "StepStone, 2 roles in Berlin:\n1. Siemens Energy — Junior BI Analyst — €55,000 a year (also found on the web search).\n2. Axel Springer — Junior Data Analyst, Audience — €48,000 a year."},
		},
		Checks: map[int]check{
			5:  {Kind: "table", Answer: []string{`trade ?republic`, `getyourguide`}},
			6:  {Kind: "detail", Answer: []string{`\b7\b|seven`}},
			7:  {Kind: "source", Any: []string{`n26-risk`, `jobs\.example\.com`, `listing|posting|job ad|page`}},
			9:  {Kind: "task", Rule: "stepstone", Any: []string{`running|not (yet )?(done|finished)|still|in progress`}},
			10: {Kind: "ref", Answer: []string{`2 days|two days`}},
			11: {Kind: "more", Pages: 2},
			12: {Kind: "fact", Names: []string{"zalando", "n26", "flix", "hellofresh", "siemens"}},
			13: {Kind: "claims", Answer: []string{`linkedin`, `stepstone`, `indeed`, `glassdoor`}, Not: []string{`xing`, `monster`, `arbeitsagentur`, `kununu`}},
			14: {Kind: "table", Answer: []string{`zalando`, `n26`}},
		}},
	{ID: "b_trip", Part: "B", Window: "low", World: tripWeb,
		Messages: []scripted{
			{Text: "I'm planning 4 nights in Lisbon from 12 to 16 March. Search for well-rated mid-range hotels in Alfama or Baixa and suggest three."},
			{Text: "Open the pages for the Alfama Patio Hotel and the Baixa House and compare them."},
			{Text: "Make a table for my trip plan with day, plan and notes, and put the hotel check-in on day 1."},
			{Text: "Start a background search for flights from Frankfurt to Lisbon on 12 March, returning on 16 March. While it runs, what are the must-see sights in Belém?"},
			{Text: "Do I need to tip in Lisbon restaurants?"},
			{Text: "Add the cheapest direct flight from Frankfurt Airport to day 1 of my trip plan."},
			{Text: "What was the check-in time at the Baixa House?"},
			{Text: "Where did you find that?"},
			{Text: "Start a background search for a fado show on 14 March."},
			{Text: "Has the fado search finished?"},
			{Text: "Check the Alfama Patio Hotel page again: does it have an elevator?"},
			{Text: "Search for two more hotels near Cais do Sodré and open the best one."},
			{Text: "Which three hotels did you suggest at the start?"},
			{Text: "List every source we've used so far, and what each told us."},
			{Text: "Show me my trip plan as it stands."},
		},
		Tasks: []*taskRule{
			{Name: "flights", Match: `flight|frankfurt|fra\b`, After: 3,
				Result: "Flights Frankfurt → Lisbon, 12–16 March, return, one adult:\n1. TAP TP579 FRA 06:40 → LIS 08:55, back TP578 LIS 17:30 → FRA 21:35 — €212.\n2. Lufthansa LH1172 FRA 09:55 → LIS 12:10, back LH1173 LIS 13:05 → FRA 17:10 — €248.\n3. Ryanair FR3412 from Frankfurt-Hahn (HHN, 120 km from Frankfurt) 07:15 → LIS 09:30 — €96."},
			{Name: "fado", Match: `fado`, After: 9,
				Result: "Fado on 14 March: Clube de Fado, Rua de São João da Praça 94, dinner and show from 20:00, €65 a person, booking needed. Tasca do Chico, Bairro Alto, from 21:00, no booking, free entry with drinks."},
		},
		Checks: map[int]check{
			5:  {Kind: "table", Answer: []string{`tp ?579|tap`}},
			6:  {Kind: "detail", Answer: []string{`15[:.h]00|15 ?h|3 ?pm|15 o.clock`}},
			7:  {Kind: "source", Any: []string{`baixa-house`, `travel\.example\.com`, `page|listing|website|site`}},
			9:  {Kind: "task", Rule: "fado", Any: []string{`running|not (yet )?(done|finished)|still|in progress`}},
			10: {Kind: "ref", Answer: []string{`no (elevator|lift)|not have an? (elevator|lift)|doesn.t have an? (elevator|lift)|without an? (elevator|lift)`}},
			11: {Kind: "more", Pages: 1},
			12: {Kind: "fact", Names: []string{"alfama patio", "baixa house", "memmo", "lisbon story", "casa amora"}},
			13: {Kind: "claims", Answer: []string{`flight`, `fado`}, Not: []string{`booking\.com`, `tripadvisor`, `expedia`, `airbnb`, `skyscanner`, `google flights`}},
			14: {Kind: "table", Answer: []string{`tp ?579|tap`, `check.?in`}},
		}},
}

func testByID(id string) (testDef, bool) {
	for _, t := range append(append([]testDef{}, partA...), partB...) {
		if t.ID == id {
			return t, true
		}
	}
	return testDef{}, false
}
