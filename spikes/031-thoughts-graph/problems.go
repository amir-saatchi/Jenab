package main

// problem is one how-to task. Only synthetic text is sent.
type problem struct {
	ID   string
	Lang string
	Text string
}

var problems = []problem{
	{"report-page", "en", "Our internal sales report page takes about 12 seconds to load. It runs on PostgreSQL behind a Go API, the data grows by 50,000 rows a day, and the team is two developers. How should we make it load in under 1 second?"},
	{"db-move", "en", "We have to move a 200 GB PostgreSQL 14 database to a new server in another data center. The app may be down for at most 5 minutes. How should we do it?"},
	{"b1-exam", "en", "I work full time and want to pass the German B1 exam in 6 months, starting from A2. I can study about 1 hour on weekdays and 3 hours on weekend days. How should I plan it?"},
	{"umzug", "de", "Wir ziehen in drei Wochen mit zwei Kindern (4 und 9 Jahre) von Berlin nach München. Beide Eltern arbeiten weiter. Wie organisieren wir den Umzug, damit nichts Wichtiges vergessen wird?"},
	{"bakery", "fa", "می‌خواهم در محله‌ام در تهران یک کسب‌وکار کوچک فروش نان خانگی راه بیندازم. حدود ۵۰ میلیون تومان سرمایه دارم و فقط آشپزخانهٔ خانه‌ام را. از کجا و چطور شروع کنم؟"},
}

func findProblem(id string) (problem, bool) {
	for _, p := range problems {
		if p.ID == id {
			return p, true
		}
	}
	return problem{}, false
}
