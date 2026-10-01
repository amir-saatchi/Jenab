package main

// Query is one tracker-style search. Match is used only by sources that
// have no server-side search (RSS feeds): every group must match at least
// one of its alternatives (case-insensitive substring of title+summary).
type Query struct {
	ID    string
	Text  string
	Lang  string // en, de, fa
	Topic string
	Match [][]string
}

var queries = []Query{
	{"q01", "bitcoin price news this week", "en", "crypto news", [][]string{{"bitcoin", "btc"}, {"price", "rall", "drop", "fall", "surge", "slump", "$"}}},
	{"q02", "Ethereum Fusaka upgrade", "en", "crypto release", [][]string{{"fusaka"}}},
	{"q03", "SEC bitcoin ETF decision", "en", "regulation", [][]string{{"sec"}, {"etf"}}},
	{"q04", "Bundesbank Zinsen", "de", "rates (German)", [][]string{{"bundesbank", "ezb", "ecb"}, {"zins", "rate"}}},
	{"q05", "قیمت بیت کوین", "fa", "crypto (Persian)", [][]string{{"بیت کوین", "بیت‌کوین", "بیتکوین"}}},
	{"q06", "Hacker News Rust release", "en", "releases", [][]string{{"rust"}, {"release", "1."}}},
	{"q07", "Go 1.26 release", "en", "releases", [][]string{{"go 1.26", "go1.26"}}},
	{"q08", "Notion vs Obsidian", "en", "competitors", [][]string{{"obsidian"}, {"notion"}}},
	{"q09", "how to build a daily meditation habit", "en", "habits", [][]string{{"meditat"}, {"habit", "daily", "routine"}}},
	{"q10", "ECB interest rate decision", "en", "rates", [][]string{{"ecb", "european central bank"}, {"rate"}}},
}
