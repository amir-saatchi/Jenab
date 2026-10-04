package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
)

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"Straße STRAẞE":    "Strasse STRASSE",
		"ﬁle Ｆｕｌｌ":         "file Full",      // NFKC
		"قيمت كتاب ى":      "قیمت کتاب ی",    // Arabic yeh, kaf, alef maksura
		"کـــتاب قیمتِ":    "کتاب قیمت",      // tatweel, kasra
		"۲۰۲۶ ٣":           "2026 3",         // Persian and Arabic-Indic digits
		"می‌خواهم":         "میخواهم",        // ZWNJ
		"Übersicht café":   "Übersicht café", // left to the tokenizer
		"getUserName run!": "getUserName run!",
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFTSQuery(t *testing.T) {
	for in, want := range map[string]string{
		`bitcoin`:         `"bitcoin"*`,
		` Preis  Haus `:   `"Preis"* AND "Haus"*`,
		`"`:               `""""*`,
		`x OR`:            `"x"* AND "OR"*`,
		`NEAR(a b`:        `"NEAR(a"* AND "b"*`,
		`col:btc`:         `"col:btc"*`,
		`بیت‌کوین`:        `("بیتکوین"* OR "بیت کوین"*)`,
		`قيمت بیت‌کوین`:   `"قیمت"* AND ("بیتکوین"* OR "بیت کوین"*)`,
		"a\u200c\u200cb":  `("ab"* OR "a b"*)`,
		"\u200c x \u200c": `"x"*`, // a term of ZWNJs only is dropped
	} {
		terms, err := queryTerms(in)
		if err != nil {
			t.Fatal(err)
		}
		if got := ftsQuery(terms, ""); got != "text : ("+want+")" {
			t.Errorf("ftsQuery(%q) = %s, want text : (%s)", in, got, want)
		}
	}
	terms, _ := queryTerms("btc")
	if got, want := ftsQuery(terms, `01J"X`), `chat : "01J""X" AND text : ("btc"*)`; got != want {
		t.Errorf("in a chat: %s, want %s", got, want)
	}
	if _, err := queryTerms(strings.Repeat("w ", MaxSearchTerms)); err != nil {
		t.Errorf("%d words: %v", MaxSearchTerms, err)
	}
	if _, err := queryTerms(strings.Repeat("w ", MaxSearchTerms+1)); !errors.Is(err, ErrBadQuery) {
		t.Errorf("%d words: %v, want ErrBadQuery", MaxSearchTerms+1, err)
	}
}

// spikeMessages and spikeQueries are SPIKE-011's labelled set. Messages
// marked "noise" are there to catch false hits.
var spikeMessages = map[string]string{
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
	"d11": "Die Präsentation ist fertig.", // noise for "Preis"
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
	"p2":  "میخواهم گزارش هفتگی را دریافت کنم.", // the same word without ZWNJ
	"p3":  "کتاب‌ها روی میز هستند.",             // a plural with ZWNJ
	"p4":  "این كتاب را خواندی؟",                // Arabic kaf
	"p5":  "قيمت دلار امروز چقدر است؟",          // Arabic yeh
	"p6":  "گزارش سال ۲۰۲۶ آماده است.",          // Persian digits
	"p7":  "کـــتاب جدید رسید.",                 // tatweel
	"p8":  "قیمتِ طلا بالا رفت.",                // kasra
	"p9":  "برای جستجو از search_history استفاده کن.",
	"p10": "بیت کوین دوباره ارزان شد.", // bitcoin written with a space
	"p11": "مستقیم به خانه رفتم.",      // noise: "قیم" inside another word
	// Mixed
	"m1": "Der API-Key für coindata.example ist abgelaufen, bitte erneuern.",
	"m2": "قیمت BTC امروز 64000 دلار است.",
	"m3": "SELECT avg(price) FROM prices WHERE coin = 'btc'",
}

// Each query lists the messages a user would want, then what a word search
// and a substring search find. "" means the same as want.
var spikeQueries = []struct {
	q, want, words, inside string
}{
	{"Preis", "d1 d2 d8 d9", "d2 d8 d9", ""}, // Bitcoinpreis: inside a word
	{"Übersicht", "d2 d3", "", ""},
	{"Ubersicht", "d2 d3", "", ""},
	{"Straße", "d4 d5", "d5", ""}, // Hauptstraße: inside a word
	{"Strasse", "d4 d5", "d5", ""},
	{"Haus", "d6 d7", "", ""},
	{"Bitcoin", "d1", "", ""},
	{"coin_prices", "d10", "", ""},
	{"run", "e1 e2 e3", "e1 e3", "e1 e3 e8"}, // "ran" needs stemming; brunch and truncated contain "run"
	{"search_history", "e4 p9", "", ""},
	{"coindata", "e5 m1", "", ""},
	{"UserName", "e6", "-", ""}, // getUserName: inside a word
	{"SQLITE_BUSY", "e7", "", ""},
	{"locked", "e7", "", ""},
	{"میخواهم", "p1 p2", "", ""},
	{"می‌خواهم", "p1 p2", "", ""},
	{"قیمت", "p1 p5 p8 m2", "", ""},
	{"كتاب", "p3 p4 p7", "", ""},
	{"کتاب", "p3 p4 p7", "", ""},
	{"بیت‌کوین", "p1 p10", "", ""},
	{"۲۰۲۶", "p6", "", ""},
	{"2026", "p6", "", ""},
	{"طلا", "p8", "", ""},
	{"BTC", "e5 m2 m3", "", ""},
	{"avg", "m3", "", ""},
	{"API-Key", "m1", "", ""},
	// More than one word: every word must be there.
	{"Preis Kaffee", "d9", "", ""},
	{"قیمت طلا", "p8", "", ""},
}

// addSpikeCorpus stores the labelled messages in two chats and returns the
// label of each message ID.
func addSpikeCorpus(t testing.TB, c *ChatsDB) map[id.Message]string {
	t.Helper()
	ctx := context.Background()
	a, err := c.CreateChat(ctx, id.SourceUser, chat.Chat{Title: "German and English"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.CreateChat(ctx, id.SourceUser, chat.Chat{Title: "Persian"})
	if err != nil {
		t.Fatal(err)
	}
	labels := slices.Sorted(func(yield func(string) bool) {
		for k := range spikeMessages {
			if !yield(k) {
				return
			}
		}
	})
	byID := map[id.Message]string{}
	for i, l := range labels {
		ch := a.ID
		if l[0] == 'p' || l[0] == 'm' {
			ch = b.ID
		}
		m, _, err := c.AppendMessage(ctx, chat.Message{Chat: ch, Turn: i + 1, Role: chat.RoleUser,
			Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: spikeMessages[l]}}}})
		if err != nil {
			t.Fatal(err)
		}
		byID[m.ID] = l
	}
	return byID
}

func hitLabels(hits []Hit, byID map[id.Message]string) string {
	var ls []string
	for _, h := range hits {
		ls = append(ls, byID[h.Message])
	}
	return sortLabels(ls)
}

func sortLabels(ls []string) string {
	sort.Slice(ls, func(i, j int) bool { // d2 before d10
		if ls[i][0] != ls[j][0] {
			return ls[i][0] < ls[j][0]
		}
		return len(ls[i]) < len(ls[j]) || len(ls[i]) == len(ls[j]) && ls[i] < ls[j]
	})
	return strings.Join(ls, " ")
}

// TestSearchSpikeCorpus is SPIKE-011's labelled set, kept as a test.
func TestSearchSpikeCorpus(t *testing.T) {
	c := openTestChats(t)
	byID := addSpikeCorpus(t, c)
	ctx := context.Background()
	for _, q := range spikeQueries {
		for _, inside := range []bool{false, true} {
			want := q.words
			if inside {
				want = q.inside
			}
			switch want {
			case "":
				want = q.want
			case "-":
				want = ""
			}
			want = sortLabels(strings.Fields(want))
			res, err := c.Search(ctx, SearchReq{Query: q.q, Substring: inside})
			if err != nil {
				t.Fatalf("%q: %v", q.q, err)
			}
			if got := hitLabels(res.Hits, byID); got != want || res.Stopped {
				t.Errorf("%q (substring %v) = %q, stopped %v; want %q", q.q, inside, got, res.Stopped, want)
			}
		}
	}
}

func TestSearchHostile(t *testing.T) {
	c := openTestChats(t)
	addSpikeCorpus(t, c)
	ctx := context.Background()
	for _, q := range []string{`"`, `AND`, `NOT`, `x OR`, `a*b`, `(`, `NEAR(a b`, `-btc`, `^preis`, `col:btc`, `text:btc`, `'`, `\`, `{}`,
		`*`, `"btc" OR "eth"`, "\x00", "\xff\xfe", `می‌خواهم "`, `بیت‌کوین OR`, "\u200c"} {
		for _, inside := range []bool{false, true} {
			if _, err := c.Search(ctx, SearchReq{Query: q, Substring: inside}); err != nil {
				t.Errorf("Search(%q, substring %v): %v", q, inside, err)
			}
		}
	}
	// Operators are words: "OR" doesn't widen the search.
	res, err := c.Search(ctx, SearchReq{Query: "btc OR eth"})
	if err != nil || len(res.Hits) != 0 {
		t.Errorf("btc OR eth = %d hits, %v; want none (no message has all three words)", len(res.Hits), err)
	}
	for _, q := range []string{"", "   ", "\u200c \u200c"} {
		res, err := c.Search(ctx, SearchReq{Query: q})
		if err != nil || res.Hits == nil || len(res.Hits) != 0 {
			t.Errorf("Search(%q) = %#v, %v; want no hits", q, res.Hits, err)
		}
	}
}

// FuzzFTSQuery checks that no query reaches MATCH as anything but quoted
// terms: every query runs without an FTS5 error (Q34).
func FuzzFTSQuery(f *testing.F) {
	for _, q := range []string{`"`, `AND`, `x OR`, `a*b`, `NEAR(a b`, `col:btc`, `^preis`, `{}`, "\x00", `بیت‌کوین "`, "a\u200c\"b", `"" ""*`} {
		f.Add(q)
	}
	c, err := OpenChats(context.Background(), f.TempDir())
	if err != nil {
		f.Fatal(err)
	}
	defer c.Close(context.Background())
	addSpikeCorpus(f, c)
	f.Fuzz(func(t *testing.T, q string) {
		terms, err := queryTerms(q)
		if errors.Is(err, ErrBadQuery) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(terms) == 0 {
			return
		}
		fts := ftsQuery(terms, "")
		if _, err := Query(context.Background(), c.DB, `SELECT rowid FROM messages_fts WHERE messages_fts MATCH ?`, []any{fts},
			func(r *sql.Rows) (int64, error) {
				var n int64
				return n, r.Scan(&n)
			}); err != nil {
			t.Fatalf("query %q: MATCH %s: %v", q, fts, err)
		}
	})
}

func TestSearchScope(t *testing.T) {
	c := openTestChats(t)
	ctx := context.Background()
	a, b := newChat(t, c, chat.Chat{Title: "a"}), newChat(t, c, chat.Chat{Title: "b"})
	at := time.Date(2026, 10, 4, 9, 30, 0, 0, time.UTC)
	ma, _, err := c.AppendMessage(ctx, chat.Message{Chat: a.ID, Turn: 3, Role: chat.RoleAssistant, CreatedAt: at, Parts: []chat.Part{
		{Kind: chat.PartText, Text: &chat.Text{Text: "the bitcoin price"}},
		{Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{ID: "c1", Name: "fetch", Args: []byte(`{"q":"bitcoin"}`)}},
		{Kind: chat.PartText, Text: &chat.Text{Text: "bitcoin again, in a second part"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	mb := addTurn(t, c, b.ID, 1, "bitcoin\x00in chat\x01b")
	// Only text parts are searched.
	if _, _, err := c.AppendMessage(ctx, chat.Message{Chat: b.ID, Turn: 2, Role: chat.RoleAssistant, Parts: []chat.Part{
		{Kind: chat.PartThinking, Thinking: &chat.Thinking{Text: "thinking about bitcoin"}},
		{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: "c1", Text: "bitcoin 64000"}},
	}}); err != nil {
		t.Fatal(err)
	}

	res, err := c.Search(ctx, SearchReq{Query: "bitcoin"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 2 {
		t.Fatalf("project scope: %+v, want one hit for each message", res.Hits)
	}
	res, err = c.Search(ctx, SearchReq{Query: "bitcoin", Chat: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	want := []Hit{{Chat: a.ID, Message: ma.ID, Turn: 3, Role: chat.RoleAssistant, CreatedAt: at, Snippet: "the bitcoin price"}}
	if diff := cmp.Diff(want, res.Hits); diff != "" {
		t.Errorf("chat scope (-want +got):\n%s", diff)
	}
	// The scan: one hit for a message with two matching parts, its newest
	// part's snippet, and only the chat asked for.
	res, err = c.Search(ctx, SearchReq{Query: "bitcoin", Substring: true})
	if err != nil || len(res.Hits) != 2 {
		t.Errorf("substring, project scope: %+v, %v; want one hit for each message", res.Hits, err)
	}
	res, err = c.Search(ctx, SearchReq{Query: "bitcoin", Chat: a.ID, Substring: true})
	want[0].Snippet = "bitcoin again, in a second part"
	if diff := cmp.Diff(want, res.Hits); err != nil || diff != "" {
		t.Errorf("substring, chat scope: %v (-want +got):\n%s", err, diff)
	}
	for _, inside := range []bool{false, true} {
		res, err = c.Search(ctx, SearchReq{Query: "bitcoin chat\x00b", Chat: b.ID, Substring: inside})
		if err != nil || len(res.Hits) != 1 || res.Hits[0].Message != mb.ID {
			t.Errorf("substring %v in chat b = %+v, %v; want %s", inside, res.Hits, err, mb.ID)
		}
	}
	if _, err := c.Search(ctx, SearchReq{Query: "bitcoin", Chat: "missing"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("a missing chat: %v, want ErrNotFound", err)
	}
	// The chat column is only for scope: a chat's ID is not text.
	res, err = c.Search(ctx, SearchReq{Query: string(a.ID)})
	if err != nil || len(res.Hits) != 0 {
		t.Errorf("searching for a chat ID: %+v, %v; want no hits", res.Hits, err)
	}

	// Limits, and newest first in a substring search.
	for i := range 110 {
		addTurn(t, c, b.ID, 10+i, fmt.Sprintf("limit test %d", i))
	}
	for _, x := range []struct{ limit, want int }{{0, 20}, {5, 5}, {200, 100}} {
		for _, inside := range []bool{false, true} {
			res, err := c.Search(ctx, SearchReq{Query: "limit test", Limit: x.limit, Substring: inside})
			if err != nil || len(res.Hits) != x.want {
				t.Errorf("Limit %d, substring %v: %d hits, %v; want %d", x.limit, inside, len(res.Hits), err, x.want)
			}
		}
	}
	res, _ = c.Search(ctx, SearchReq{Query: "limit test", Substring: true, Limit: 2})
	if len(res.Hits) != 2 || res.Hits[0].Turn != 119 || res.Hits[1].Turn != 118 {
		t.Errorf("substring order: %+v, want turns 119 and 118", res.Hits)
	}
}

func TestSearchRanking(t *testing.T) {
	c := openTestChats(t)
	ctx := context.Background()
	ch := newChat(t, c, chat.Chat{Title: "r"}).ID
	// The best match is the oldest, so newest first would be wrong.
	best := addTurn(t, c, ch, 1, "bitcoin bitcoin bitcoin")
	for i := range 4 {
		addTurn(t, c, ch, 2+i, "ethereum, and once bitcoin, among many other words about markets and prices today")
	}
	addTurn(t, c, ch, 6, "nothing here")
	for _, limit := range []int{1, 20} {
		res, err := c.Search(ctx, SearchReq{Query: "bitcoin", Limit: limit})
		if err != nil || len(res.Hits) != min(limit, 5) || res.Hits[0].Message != best.ID {
			t.Errorf("Limit %d: hits = %+v, %v; want %s first", limit, res.Hits, err, best.ID)
		}
	}
	// Two strong parts of one message count once; the next message still
	// fills the limit.
	m, _, err := c.AppendMessage(ctx, chat.Message{Chat: ch, Turn: 7, Role: chat.RoleAssistant, Parts: []chat.Part{
		{Kind: chat.PartText, Text: &chat.Text{Text: "zebra zebra zebra"}},
		{Kind: chat.PartText, Text: &chat.Text{Text: "zebra zebra zebra"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	other := addTurn(t, c, ch, 8, "a zebra, among many other words about animals and places today")
	res, err := c.Search(ctx, SearchReq{Query: "zebra", Limit: 2})
	if err != nil || len(res.Hits) != 2 || res.Hits[0].Message != m.ID || res.Hits[1].Message != other.ID {
		t.Errorf("zebra = %+v, %v; want %s then %s", res.Hits, err, m.ID, other.ID)
	}
}

// TestSearchRankedMatches checks the bound on ranking: only the newest
// matches are ranked.
func TestSearchRankedMatches(t *testing.T) {
	c := openTestChats(t)
	ch := newChat(t, c, chat.Chat{Title: "r"}).ID
	best := addTurn(t, c, ch, 1, "bitcoin bitcoin bitcoin")
	addTurn(t, c, ch, 2, "ethereum, and once bitcoin, among many other words about markets and prices today")
	addTurn(t, c, ch, 3, "bitcoin, and some more words about markets and prices today")
	defer func(n int) { rankedMatches = n }(rankedMatches)
	rankedMatches = 2
	res, err := c.Search(context.Background(), SearchReq{Query: "bitcoin"})
	if err != nil || len(res.Hits) != 2 || res.Hits[0].Turn != 3 || res.Hits[1].Turn != 2 {
		t.Errorf("hits = %+v, %v; want turns 3 and 2", res.Hits, err)
	}
	rankedMatches = 3
	res, err = c.Search(context.Background(), SearchReq{Query: "bitcoin"})
	if err != nil || len(res.Hits) != 3 || res.Hits[0].Message != best.ID {
		t.Errorf("hits = %+v, %v; want %s first", res.Hits, err, best.ID)
	}
}

func TestSearchSubstringStops(t *testing.T) {
	c := openTestChats(t)
	ch := newChat(t, c, chat.Chat{Title: "s"}).ID
	addTurn(t, c, ch, 1, "bitcoin")
	defer func(b time.Duration) { substringBudget = b }(substringBudget)
	substringBudget = 0
	res, err := c.Search(context.Background(), SearchReq{Query: "bitcoin", Substring: true})
	if err != nil || !res.Stopped || res.Hits == nil {
		t.Errorf("with no time: %+v, %v; want Stopped and no error", res, err)
	}
	// A cancelled search is an error, not a stop.
	substringBudget = time.Minute
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Search(ctx, SearchReq{Query: "bitcoin", Substring: true}); err == nil {
		t.Error("a cancelled search returned no error")
	}
}

func TestSearchSnippet(t *testing.T) {
	// The cut lands on a word start here, so no word is dropped; and in the
	// middle of a word, which is then dropped.
	zebra, _ := queryTerms("zebra")
	if got := snippet(strings.Repeat("abcd ", 13)+"zebra "+strings.Repeat("wxyz ", 50), zebra); !strings.HasPrefix(got, "…abcd abcd") {
		t.Errorf("snippet at a word start = %q", got)
	}
	if got := snippet(strings.Repeat("abcdefg ", 20)+"zebra "+strings.Repeat("wxyz ", 50), zebra); !strings.HasPrefix(got, "…abcdefg abcdefg") {
		t.Errorf("snippet in a word = %q", got)
	}
	// A word search shows the word, not the same letters inside another
	// word before it.
	preis, _ := queryTerms("preis")
	if got := snippet("Der Bitcoinpreis "+strings.Repeat("Wort ", 60)+"und der Preis ist hoch", preis); !strings.Contains(got, "der Preis ist hoch") {
		t.Errorf("snippet for a word = %q", got)
	}
	long := strings.Repeat("alpha ", 40) + "the  Bitcoin\n\tprice " + strings.Repeat("omega ", 60)
	terms, _ := queryTerms("bitcoin")
	got := snippet(long, terms)
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") || !strings.Contains(got, "the Bitcoin price") {
		t.Errorf("snippet = %q", got)
	}
	if n := utf8.RuneCountInString(got); n > snippetLen+2 || n < snippetLen/2 {
		t.Errorf("snippet has %d characters", n)
	}
	for _, w := range strings.Fields(strings.Trim(got, "…")) {
		if w != "alpha" && w != "omega" && w != "the" && w != "Bitcoin" && w != "price" {
			t.Errorf("snippet cuts a word: %q in %q", w, got)
		}
	}
	for _, x := range []struct{ text, q, want string }{
		{"short text with bitcoin", "bitcoin", "short text with bitcoin"},
		{"no match here", "bitcoin", "no match here"},
		{"Die Häuser in Berlin", "haus", "Die Häuser in Berlin"},
		{strings.Repeat("کلمه ", 40) + "قیمتِ طلا بالا رفت", "قيمت", "…"},
		{strings.Repeat("کلمه ", 40) + "بیت کوین ارزان شد", "بیت‌کوین", "…"},
	} {
		terms, _ := queryTerms(x.q)
		got := snippet(x.text, terms)
		if x.want == "…" {
			// The match is in the snippet, past the cut.
			if !strings.HasPrefix(got, "…") || !strings.Contains(got, strings.Fields(x.text)[40]) {
				t.Errorf("snippet(%q) = %q", x.q, got)
			}
		} else if got != x.want {
			t.Errorf("snippet(%q) = %q, want %q", x.q, got, x.want)
		}
	}
	// A substring search shows the match inside a word.
	terms, _ = queryTerms("preis")
	if got := snippetAt(strings.Repeat("Wort ", 40)+"Der Bitcoinpreis ist hoch", terms, true); !strings.Contains(got, "Bitcoinpreis") {
		t.Errorf("substring snippet = %q", got)
	}
}

func TestTextOf(t *testing.T) {
	for _, x := range []chat.Text{
		{Text: "plain"},
		{Text: "plain", Stopped: true},
		{Text: ""},
		{Text: `a "quote" and a \ backslash`},
		{Text: "line\nnext", Stopped: true},
		{Text: "<b>&</b> سلام"},
		{Text: "bad \xff byte"},
	} {
		content, err := json.Marshal(x)
		if err != nil {
			t.Fatal(err)
		}
		want := strings.ToValidUTF8(x.Text, "\uFFFD")
		if got, err := textOf(string(content)); got != want || err != nil {
			t.Errorf("textOf(%s) = %q, %v; want %q", content, got, err, want)
		}
	}
	if _, err := textOf(`{"text":`); err == nil {
		t.Error("broken JSON: no error")
	}
}

func TestFoldOffsets(t *testing.T) {
	s := "Aß é می\u200cﬁ Việt ДОМ"
	var offs []int
	f := fold(s, &offs)
	if string(f) != "ass e میfi viet дом" {
		t.Errorf("fold = %q", string(f))
	}
	if len(offs) != len(f) {
		t.Fatalf("%d offsets for %d runes", len(offs), len(f))
	}
	for i, o := range offs {
		if !utf8.RuneStart(s[o]) || i > 0 && o < offs[i-1] {
			t.Errorf("offset %d = %d", i, o)
		}
	}
}

// ftsCount is how many index rows match q, read without the join to
// message_parts, so rows left behind are seen.
func ftsCount(t *testing.T, c *ChatsDB, q string) int {
	t.Helper()
	n, err := Query(context.Background(), c.DB, `SELECT count(*) FROM messages_fts WHERE messages_fts MATCH ?`, []any{q},
		func(r *sql.Rows) (int, error) {
			var n int
			return n, r.Scan(&n)
		})
	if err != nil {
		t.Fatal(err)
	}
	return n[0]
}

func TestSearchClearAndDelete(t *testing.T) {
	c := openTestChats(t)
	ctx := context.Background()
	a, b := newChat(t, c, chat.Chat{Title: "a"}), newChat(t, c, chat.Chat{Title: "b"})
	addTurn(t, c, a.ID, 1, "zebra in a")
	m := addTurn(t, c, a.ID, 2, "first part")
	if _, _, err := c.AppendPart(ctx, m.ID, chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: "zebra in a streamed part"}}); err != nil {
		t.Fatal(err)
	}
	addTurn(t, c, b.ID, 1, "zebra in b")
	// Other parts are not indexed, so nothing of theirs is left behind.
	if _, _, err := c.AppendMessage(ctx, chat.Message{Chat: b.ID, Turn: 2, Role: chat.RoleAssistant, Parts: []chat.Part{
		{Kind: chat.PartNotice, Notice: &chat.Notice{Kind: chat.NoticeTaskFinished, Text: "zebra task finished"}},
	}}); err != nil {
		t.Fatal(err)
	}
	if n := ftsCount(t, c, "zebra"); n != 3 {
		t.Fatalf("%d index rows, want 3", n)
	}
	if _, err := c.Clear(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if n := ftsCount(t, c, "zebra"); n != 1 {
		t.Errorf("after Clear: %d index rows, want 1", n)
	}
	if err := c.DeleteChat(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if n := ftsCount(t, c, "zebra"); n != 0 {
		t.Errorf("after DeleteChat: %d index rows, want 0", n)
	}
	all, err := Query(ctx, c.DB, `SELECT count(*) FROM messages_fts`, nil, func(r *sql.Rows) (int, error) {
		var n int
		return n, r.Scan(&n)
	})
	if err != nil || all[0] != 0 {
		t.Errorf("after DeleteChat: %v rows in the index, %v; want none", all, err)
	}
}

// TestSearchFormatStep opens a chats.db made before P1-07: its text parts
// are indexed when the format step runs.
func TestSearchFormatStep(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	old := Format{Name: ChatsFormat.Name, Steps: ChatsFormat.Steps[:1]}
	db, err := Open(ctx, dir+"/chats.db", Options{Format: old, Snapshots: dir + "/snapshots"})
	if err != nil {
		t.Fatal(err)
	}
	c := &ChatsDB{DB: db}
	ch, err := c.EnsureMother(ctx)
	if err != nil {
		t.Fatal(err)
	}
	m := chat.Message{ID: id.Message(id.New()), Chat: ch.ID, Turn: 1, Role: chat.RoleUser, Parts: []chat.Part{
		{Kind: chat.PartText, Text: &chat.Text{Text: "قيمت بیت‌کوین"}},
		{Kind: chat.PartNotice, Notice: &chat.Notice{Kind: chat.NoticeTaskFinished, Text: "bitcoin task finished"}},
	}}
	// The writes of P1-06, without the index.
	if _, err := Do(ctx, c.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		if _, err := tx.Exec(`INSERT INTO messages (id, chat_id, turn, role, created_at) VALUES (?, ?, 1, 'user', ?)`, m.ID, m.Chat, formatTime(now())); err != nil {
			return struct{}{}, err
		}
		rows, err := encodeParts(m.Parts)
		if err != nil {
			return struct{}{}, err
		}
		for i, r := range rows {
			if _, err := tx.Exec(`INSERT INTO message_parts (message_id, seq, type, content, ref, preview) VALUES (?, ?, ?, ?, ?, ?)`,
				m.ID, i, r.kind, r.content, r.ref, r.preview); err != nil {
				return struct{}{}, err
			}
		}
		return struct{}{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(ctx); err != nil {
		t.Fatal(err)
	}

	c, err = OpenChats(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(ctx)
	res, err := c.Search(ctx, SearchReq{Query: "قیمت بیتکوین"})
	if err != nil || len(res.Hits) != 1 || res.Hits[0].Message != m.ID {
		t.Errorf("after the format step: %+v, %v", res.Hits, err)
	}
	if n := ftsCount(t, c, "bitcoin"); n != 0 {
		t.Errorf("a notice part was indexed")
	}
	if entries, _ := os.ReadDir(dir + "/snapshots"); len(entries) == 0 {
		t.Error("no snapshot before the format step")
	}
}

// TestSearchSpeed is N-09: p95 under 50 ms at 100,000 messages. It takes
// about a minute, so it runs only with JENAB_PERF_TEST=1.
func TestSearchSpeed(t *testing.T) {
	if os.Getenv("JENAB_PERF_TEST") != "1" {
		t.Skip("set JENAB_PERF_TEST=1")
	}
	const n = 100_000
	c := openTestChats(t)
	ctx := context.Background()
	rng := rand.New(rand.NewSource(11))
	g := newTextGenerator(rng)
	var chats []id.Chat
	for i := range 20 {
		chats = append(chats, newChat(t, c, chat.Chat{Title: fmt.Sprint("chat ", i)}).ID)
	}
	start := time.Now()
	var size int
	for done := 0; done < n; {
		batch := min(1000, n-done)
		if _, err := Do(ctx, c.DB, limit.Background, func(tx *sql.Tx) (struct{}, error) {
			for i := range batch {
				text := g.message()
				size += len(text)
				m := id.Message(id.New())
				if _, err := tx.Exec(`INSERT INTO messages (id, chat_id, turn, role, created_at) VALUES (?, ?, ?, 'user', ?)`,
					m, chats[(done+i)%len(chats)], (done+i)/len(chats)+1, formatTime(now())); err != nil {
					return struct{}{}, err
				}
				rows, err := encodeParts([]chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: text}}})
				if err != nil {
					return struct{}{}, err
				}
				if err := insertPart(tx, m, 0, rows[0]); err != nil {
					return struct{}{}, err
				}
			}
			return struct{}{}, nil
		}); err != nil {
			t.Fatal(err)
		}
		done += batch
	}
	t.Logf("%d messages, %.1f MB of text, written in %v", n, float64(size)/1e6, time.Since(start).Round(time.Millisecond))

	queries := g.queries(rng, 60)
	measure := func(name string, scope id.Chat, inside bool) {
		var times []time.Duration
		hits, stopped := 0, 0
		for range 2 { // the first round warms the cache
			times = times[:0]
			for _, q := range queries {
				t0 := time.Now()
				res, err := c.Search(ctx, SearchReq{Query: q, Chat: scope, Substring: inside})
				times = append(times, time.Since(t0))
				if err != nil {
					t.Fatalf("%q: %v", q, err)
				}
				hits += len(res.Hits)
				if res.Stopped {
					stopped++
				}
			}
		}
		slices.Sort(times)
		p50, p95 := times[len(times)/2], times[len(times)*95/100]
		t.Logf("%s: p50 %v, p95 %v, max %v; %d hits, %d stopped", name, p50.Round(10*time.Microsecond), p95.Round(10*time.Microsecond),
			times[len(times)-1].Round(10*time.Microsecond), hits/2, stopped/2)
		if !inside && p95 > 50*time.Millisecond {
			t.Errorf("%s: p95 %v, want under 50 ms", name, p95)
		}
	}
	measure("project", "", false)
	measure("one chat", chats[0], false)
	measure("substring, project", "", true)
}

// textGenerator makes synthetic chat messages from three made-up
// vocabularies (English-, German- and Persian-like) with Zipf word
// frequencies, as in SPIKE-011.
type textGenerator struct {
	rng           *rand.Rand
	en, de, fa    []string
	zen, zde, zfa *rand.Zipf
}

func randomWord(rng *rand.Rand, letters []rune, lo, hi int) string {
	r := make([]rune, lo+rng.Intn(hi-lo+1))
	for i := range r {
		r[i] = letters[rng.Intn(len(letters))]
	}
	return string(r)
}

func newTextGenerator(rng *rand.Rand) *textGenerator {
	enL := []rune("abcdefghijklmnopqrstuvwxyzeeeaaoitns")
	deL := []rune("abcdefghijklmnopqrstuvwxyzäöüßeeennrrstt")
	faL := []rune("ابپتثجچحخدذرزژسشصضطظعغفقکگلمنوهیی")
	g := &textGenerator{rng: rng}
	for i := range 20000 {
		g.en = append(g.en, randomWord(rng, enL, 2, 10))
		w := randomWord(rng, deL, 3, 11)
		if i > 500 && rng.Intn(5) == 0 { // compounds of two earlier words
			w = g.de[rng.Intn(500)] + g.de[rng.Intn(i)]
		}
		g.de = append(g.de, w)
		f := randomWord(rng, faL, 2, 8)
		switch rng.Intn(8) {
		case 0:
			f = "می\u200c" + f
		case 1:
			f = f + "\u200cها"
		}
		g.fa = append(g.fa, f)
	}
	g.zen = rand.NewZipf(rng, 1.1, 2, uint64(len(g.en)-1))
	g.zde = rand.NewZipf(rng, 1.1, 2, uint64(len(g.de)-1))
	g.zfa = rand.NewZipf(rng, 1.1, 2, uint64(len(g.fa)-1))
	return g
}

func (g *textGenerator) message() string {
	vocab, z := g.en, g.zen
	switch g.rng.Intn(4) {
	case 2:
		vocab, z = g.de, g.zde
	case 3:
		vocab, z = g.fa, g.zfa
	}
	n := 5 + g.rng.Intn(30) // mostly short turns, some long replies
	if g.rng.Intn(4) == 0 {
		n = 40 + g.rng.Intn(260)
	}
	var b strings.Builder
	for i := range n {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(vocab[z.Uint64()])
		if g.rng.Intn(12) == 0 {
			b.WriteString(".")
		}
	}
	if g.rng.Intn(20) == 0 {
		b.WriteString(" https://api." + g.en[z.Uint64()%2000] + ".example/v1/" + g.en[g.rng.Intn(2000)] + "?coin=btc")
	}
	return b.String()
}

// queries are frequent, middle and rare words of each vocabulary and a few
// two-word queries.
func (g *textGenerator) queries(rng *rand.Rand, n int) []string {
	var qs []string
	pick := func(v []string, lo, hi int) string { return v[lo+rng.Intn(hi-lo)] }
	for len(qs) < n {
		for _, v := range [][]string{g.en, g.de, g.fa} {
			qs = append(qs, pick(v, 0, 20), pick(v, 100, 1000), pick(v, 5000, 20000))
		}
		qs = append(qs, pick(g.en, 0, 200)+" "+pick(g.en, 0, 200), pick(g.de, 0, 200)+" "+pick(g.de, 0, 200))
	}
	return qs[:n]
}

func TestSearchText(t *testing.T) {
	zwnj := string(rune(0x200C))
	lines := []string{
		"Bitcoin price today\n",
		"The Bitcoinpreis is high\n",
		"قیمت بیت" + zwnj + "کوین امروز\n",
		"PRICE of bitcoin, again\n",
		"Straße and ۱۲۳ coins",
	}
	text := strings.Join(lines, "")
	offset := func(i int) int { return len(strings.Join(lines[:i], "")) }
	for _, tc := range []struct {
		query string
		lines []int // from 0
	}{
		{"bitcoin price", []int{0, 3}},
		{"preis", nil},              // inside a word
		{"Bitcoin", []int{0, 1, 3}}, // the start of Bitcoinpreis
		{"بيت", []int{2}},           // Arabic yeh
		{"بیت" + zwnj + "کوین", []int{2}},
		{"strasse 123", []int{4}},
		{"again today", nil},
	} {
		hits, total, err := SearchText(text, tc.query, 10)
		if err != nil {
			t.Fatalf("%q: %v", tc.query, err)
		}
		var got []int
		for _, h := range hits {
			got = append(got, h.Line-1)
			if h.Offset != offset(h.Line-1) || h.Snippet == "" {
				t.Errorf("%q: hit %+v, want offset %d", tc.query, h, offset(h.Line-1))
			}
		}
		if !slices.Equal(got, tc.lines) || total != len(tc.lines) {
			t.Errorf("%q: lines %v (total %d), want %v", tc.query, got, total, tc.lines)
		}
	}
	// max bounds the hits, not the total.
	hits, total, _ := SearchText(text, "bitcoin", 1)
	if len(hits) != 1 || hits[0].Line != 1 || total != 3 {
		t.Errorf("max 1: %+v, total %d", hits, total)
	}
	if hits, total, err := SearchText(text, "  ", 10); hits != nil || total != 0 || err != nil {
		t.Errorf("blank query: %v %d %v", hits, total, err)
	}
	if _, _, err := SearchText(text, strings.Repeat("w ", 33), 10); !errors.Is(err, ErrBadQuery) {
		t.Errorf("33 words: %v", err)
	}
}
