//go:build corpuscheck

// Checks for the message corpus, run by hand after an edit:
//
//	go test -tags corpuscheck ./corpus
//
// The build tag keeps it out of the harness's own go test.
package corpus

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"go.yaml.in/yaml/v3"
	_ "modernc.org/sqlite"

	"github.com/amir-saatchi/jenab/internal/store"
)

type message struct {
	ID   string `yaml:"id"`
	Role string `yaml:"role"`
	Lang string `yaml:"lang"`
	Text string `yaml:"text"`
}

type chatFile struct {
	Chats []struct {
		ID       string    `yaml:"id"`
		Title    string    `yaml:"title"`
		Messages []message `yaml:"messages"`
	} `yaml:"chats"`
}

type query struct {
	ID      string `yaml:"id"`
	Kind    string `yaml:"kind"`
	Lang    string `yaml:"lang"`
	Query   string `yaml:"query"`
	Targets []struct {
		Message string `yaml:"message"`
	} `yaml:"targets"`
}

func load(t *testing.T, name string, v any) {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(b, v); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

var langs = map[string]bool{"en": true, "de": true, "fa": true}

// TestMessages checks the corpus and the labels: the format, that every
// target exists, and what each query kind promises.
func TestMessages(t *testing.T) {
	var cf chatFile
	load(t, "messages.yaml", &cf)
	var qs []query
	load(t, "queries-messages.yaml", &qs)

	msgs := map[string]message{}
	var order []string
	chats := map[string]bool{}
	langCount := map[string]int{}
	for _, c := range cf.Chats {
		if chats[c.ID] || c.Title == "" {
			t.Errorf("chat %q: duplicate ID or no title", c.ID)
		}
		chats[c.ID] = true
		for _, m := range c.Messages {
			if _, dup := msgs[m.ID]; dup {
				t.Errorf("message %s twice", m.ID)
			}
			if m.Role != "user" && m.Role != "assistant" || !langs[m.Lang] || strings.TrimSpace(m.Text) == "" {
				t.Errorf("message %s: bad role, lang or text", m.ID)
			}
			if len(order) > 0 && m.ID <= order[len(order)-1] {
				t.Errorf("message %s is out of order", m.ID)
			}
			msgs[m.ID] = m
			order = append(order, m.ID)
			langCount[m.Lang]++
		}
	}

	db := ftsIndex(t, order, msgs)
	qid := regexp.MustCompile(`^qm\d{3}$`)
	seen := map[string]bool{}
	count := map[string]int{}
	for _, q := range qs {
		if !qid.MatchString(q.ID) || seen[q.ID] {
			t.Errorf("query %q: bad or duplicate ID", q.ID)
		}
		seen[q.ID] = true
		count[q.Lang+" "+q.Kind]++
		if !langs[q.Lang] {
			t.Errorf("%s: bad lang %q", q.ID, q.Lang)
		}
		for _, tg := range q.Targets {
			m, ok := msgs[tg.Message]
			if !ok {
				t.Errorf("%s: target %s doesn't exist", q.ID, tg.Message)
				continue
			}
			if cross := m.Lang != q.Lang; cross != (q.Kind == "cross_language") {
				t.Errorf("%s: target %s is in %s, query in %s, kind %s", q.ID, m.ID, m.Lang, q.Lang, q.Kind)
			}
		}
		hits := ftsHits(t, db, q.Query)
		switch q.Kind {
		case "same_words":
			// FTS must already find at least one target.
			if !anyTarget(q, hits) {
				t.Errorf("%s: FTS finds no target for %q (hits %v)", q.ID, q.Query, hits)
			}
		case "paraphrase":
			for _, tg := range q.Targets {
				if w := shared(q.Query, msgs[tg.Message].Text); w != "" {
					t.Errorf("%s: shares %q with %s", q.ID, w, tg.Message)
				}
			}
		case "cross_language":
			if anyTarget(q, hits) {
				t.Errorf("%s: FTS finds a target", q.ID)
			}
		case "no_answer":
			if len(q.Targets) != 0 || len(hits) != 0 {
				t.Errorf("%s: has targets, or FTS finds %v", q.ID, hits)
			}
		default:
			t.Errorf("%s: bad kind %q", q.ID, q.Kind)
		}
		if q.Kind != "no_answer" && len(q.Targets) == 0 {
			t.Errorf("%s: no targets", q.ID)
		}
	}
	t.Logf("%d chats, %d messages, by language %v", len(chats), len(order), langCount)
	t.Logf("%d queries, by language and kind %v", len(qs), count)
}

func anyTarget(q query, hits []string) bool {
	for _, tg := range q.Targets {
		for _, h := range hits {
			if h == tg.Message {
				return true
			}
		}
	}
	return false
}

// ftsIndex builds an FTS5 table like chats.db's messages_fts, over the
// normalized text.
func ftsIndex(t *testing.T, order []string, msgs map[string]message) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE VIRTUAL TABLE fts USING fts5 (id UNINDEXED, text, tokenize = 'unicode61 remove_diacritics 2')`); err != nil {
		t.Fatal(err)
	}
	for _, id := range order {
		if _, err := db.Exec(`INSERT INTO fts (id, text) VALUES (?, ?)`, id, store.Normalize(msgs[id].Text)); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

// ftsHits runs the query the way the store's ftsQuery builds it: every word
// quoted with a prefix star, joined with AND; a word with a ZWNJ also
// matches its split form.
func ftsHits(t *testing.T, db *sql.DB, q string) []string {
	t.Helper()
	quote := func(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
	var parts []string
	for _, w := range strings.Fields(store.Normalize(strings.ReplaceAll(q, "‌", "\x00"))) {
		joined := strings.ReplaceAll(w, "\x00", "")
		p := quote(joined) + "*"
		if strings.Contains(w, "\x00") {
			p = "(" + p + " OR " + quote(strings.ReplaceAll(w, "\x00", " ")) + "*)"
		}
		parts = append(parts, p)
	}
	rows, err := db.Query(`SELECT id FROM fts WHERE fts MATCH ? ORDER BY rank`, "text : ("+strings.Join(parts, " AND ")+")")
	if err != nil {
		t.Fatalf("%q: %v", q, err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

// Stop words, left out when a paraphrase is checked for shared words.
var stop = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`
		a an the of in on at to for from by with and or is are was be do did does
		i me my you your we our it this that what which who when where how can
		der die das den dem des ein eine einen und oder ist sind ich du er sie es
		wir mit für von zu im in an am auf bei nach wann wie was wo kann
		و در به از که این اون آن را رو با برای تا یا هم من تو ما یه یک چی چه کی`) {
		for _, f := range words(w) {
			stop[f] = true
		}
	}
}

// words is the normalized, folded words of s.
func words(s string) []string {
	s = strings.ToLower(store.Normalize(s))
	s = strings.NewReplacer("ä", "a", "ö", "o", "ü", "u", "é", "e", "á", "a", "ó", "o", "í", "i", "ç", "c", "ã", "a").Replace(s)
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
}

// shared returns a query word that the text shares, or "". Two words are
// shared when one starts with the other (3 letters at least, or any length
// for FTS's prefix match of a query word), or when they start with the same
// 5 letters (one less for words of 4 or 5), which catches most stems.
func shared(q, text string) string {
	tw := words(text)
	for _, a := range words(q) {
		if stop[a] {
			continue
		}
		for _, b := range tw {
			if strings.HasPrefix(b, a) || strings.HasPrefix(a, b) && len([]rune(b)) >= 3 || stem(a, b) {
				return fmt.Sprintf("%s~%s", a, b)
			}
		}
	}
	return ""
}

func stem(a, b string) bool {
	ra, rb := []rune(a), []rune(b)
	short := min(len(ra), len(rb))
	if short < 4 {
		return false
	}
	need := min(5, short-1)
	for i := range need {
		if ra[i] != rb[i] {
			return false
		}
	}
	return true
}
