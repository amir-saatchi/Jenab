package main

// Hand-labelled messages and queries. Each query lists the messages a user
// would want to find; "noise" messages exist to catch false hits.

var messages = map[string]string{
	// German
	"d1":  "Der Bitcoinpreis ist heute um 3,5 % gestiegen.",
	"d2":  "Kannst du mir eine Übersicht der Preise für Ethereum geben?",
	"d3":  "Die Ubersicht ist falsch formatiert.",
	"d4":  "Wir wohnen in der Hauptstraße 5.",
	"d5":  "Die Strasse war gesperrt.",
	"d6":  "Die Häuser in Berlin sind teuer geworden.",
	"d7":  "Das Haus hat drei Zimmer.",
	"d8":  "Preisvergleich zwischen Kraken und Binance",
	"d9":  "Der Preis für eine Tasse Kaffee.",
	"d10": "Ich habe die Tabelle 'coin_prices' umbenannt.",
	"d11": "Die Präsentation ist fertig.", // noise for "Preis" (trigram would need "preis")
	// English
	"e1": "The pipeline is running every morning at 8.",
	"e2": "I ran the import yesterday and it failed.",
	"e3": "Runs are listed in the run log.",
	"e4": "Use search_history to find older messages.",
	"e5": "See https://api.coindata.example/v1/price?coin=btc for the API.",
	"e6": "The function getUserName returns the display name.",
	"e7": "Error: SQLITE_BUSY (database is locked)",
	"e8": "We had brunch, then I truncated the log.", // noise for "run"
	// Persian
	"p1":  "می‌خواهم قیمت بیت‌کوین را ببینم.",   // ZWNJ in می‌خواهم and بیت‌کوین
	"p2":  "میخواهم گزارش هفتگی را دریافت کنم.", // same word without ZWNJ
	"p3":  "کتاب‌ها روی میز هستند.",             // plural with ZWNJ
	"p4":  "این كتاب را خواندی؟",                // Arabic kaf ك
	"p5":  "قيمت دلار امروز چقدر است؟",          // Arabic yeh ي
	"p6":  "گزارش سال ۲۰۲۶ آماده است.",          // Persian digits
	"p7":  "کـــتاب جدید رسید.",                 // tatweel
	"p8":  "قیمتِ طلا بالا رفت.",                // kasra on قیمتِ
	"p9":  "برای جستجو از search_history استفاده کن.",
	"p10": "بیت کوین دوباره ارزان شد.", // bitcoin written with a space
	"p11": "مستقیم به خانه رفتم.",      // noise: contains "قیم" inside another word
	// Mixed
	"m1": "Der API-Key für coindata.example ist abgelaufen, bitte erneuern.",
	"m2": "قیمت BTC امروز 64000 دلار است.",
	"m3": "SELECT avg(price) FROM prices WHERE coin = 'btc'",
}

type query struct {
	group, q string
	want     []string
}

var queries = []query{
	{"German", "Preis", []string{"d1", "d2", "d8", "d9"}},
	{"German", "Übersicht", []string{"d2", "d3"}},
	{"German", "Ubersicht", []string{"d2", "d3"}},
	{"German", "Straße", []string{"d4", "d5"}},
	{"German", "Strasse", []string{"d4", "d5"}},
	{"German", "Haus", []string{"d6", "d7"}},
	{"German", "Bitcoin", []string{"d1"}},
	{"German", "coin_prices", []string{"d10"}},
	{"English", "run", []string{"e1", "e2", "e3"}},
	{"English", "search_history", []string{"e4", "p9"}},
	{"English", "coindata", []string{"e5", "m1"}},
	{"English", "UserName", []string{"e6"}},
	{"English", "SQLITE_BUSY", []string{"e7"}},
	{"English", "locked", []string{"e7"}},
	{"Persian", "میخواهم", []string{"p1", "p2"}},
	{"Persian", "می‌خواهم", []string{"p1", "p2"}},
	{"Persian", "قیمت", []string{"p1", "p5", "p8", "m2"}},
	{"Persian", "كتاب", []string{"p3", "p4", "p7"}},
	{"Persian", "کتاب", []string{"p3", "p4", "p7"}},
	{"Persian", "بیت‌کوین", []string{"p1", "p10"}},
	{"Persian", "۲۰۲۶", []string{"p6"}},
	{"Persian", "2026", []string{"p6"}},
	{"Persian", "طلا", []string{"p8"}},
	{"Mixed", "BTC", []string{"e5", "m2", "m3"}},
	{"Mixed", "avg", []string{"m3"}},
	{"Mixed", "API-Key", []string{"m1"}},
}

// hostile queries must never cause an FTS5 syntax error
var hostile = []string{`"`, `AND`, `NOT`, `x OR`, `a*b`, `(`, `NEAR(a b`, `-btc`, `^preis`, `col:btc`, ``, `   `, `'`, `\`, `{}`}
