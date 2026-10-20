package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

const fixture = "testdata/corpus"

func loadFixture(t *testing.T) *corpus {
	t.Helper()
	c, err := loadCorpus(fixture)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLoadCorpus(t *testing.T) {
	c := loadFixture(t)
	if len(c.Messages) != 10 || len(c.Chats) != 2 || len(c.Docs) != 2 {
		t.Fatalf("%d messages, %d chats, %d docs", len(c.Messages), len(c.Chats), len(c.Docs))
	}
	if c.Messages[6].Chat != "c2" {
		t.Errorf("m007 is in %q", c.Messages[6].Chat)
	}
	for _, q := range c.FileQueries {
		for _, tg := range q.Targets {
			text := docText(t, c, tg.File)
			if text[tg.Start:tg.End] != tg.Quote {
				t.Errorf("%s: range [%d, %d) is %q", q.ID, tg.Start, tg.End, text[tg.Start:tg.End])
			}
		}
	}
}

func docText(t *testing.T, c *corpus, file string) string {
	for _, d := range c.Docs {
		if d.File == file {
			return d.Text
		}
	}
	t.Fatalf("no doc %s", file)
	return ""
}

// copyFixture copies the fixture to a temporary folder and overwrites the
// named files.
func copyFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	err := filepath.Walk(fixture, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(fixture, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestBadCorpus(t *testing.T) {
	tests := []struct {
		name, file, text, want string
		is                     error
	}{
		{"missing quote", "queries-files.yaml",
			`- {id: qf1, kind: same_words, lang: en, query: x, targets: [{file: rental-contract-en.md, quote: "no such words here"}]}`, "", errNoQuote},
		{"quote twice", "queries-files.yaml",
			`- {id: qf1, kind: same_words, lang: en, query: x, targets: [{file: rental-contract-en.md, quote: "the landlord"}]}`, "", errTwoQuotes},
		{"unknown file", "queries-files.yaml",
			`- {id: qf1, kind: same_words, lang: en, query: x, targets: [{file: nope.md, quote: "x"}]}`, "not a file of the corpus", nil},
		{"unknown message", "queries-messages.yaml",
			`- {id: qm1, kind: same_words, lang: en, query: x, targets: [{message: m999}]}`, "not a message", nil},
		{"no_answer with a target", "queries-messages.yaml",
			`- {id: qm1, kind: no_answer, lang: en, query: x, targets: [{message: m001}]}`, "has no targets", nil},
		{"bad kind", "queries-messages.yaml",
			`- {id: qm1, kind: fuzzy, lang: en, query: x, targets: [{message: m001}]}`, "kind", nil},
		{"repeated message ID", "messages.yaml",
			"chats:\n  - id: c1\n    messages:\n      - {id: m001, role: user, lang: en, text: a}\n      - {id: m001, role: user, lang: en, text: b}\n", "repeated", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadCorpus(copyFixture(t, map[string]string{tt.file: tt.text}))
			switch {
			case err == nil:
				t.Fatal("no error")
			case tt.is != nil && !errors.Is(err, tt.is):
				t.Fatalf("%v, want %v", err, tt.is)
			case !strings.Contains(err.Error(), tt.want):
				t.Fatalf("%v, want %q", err, tt.want)
			}
		})
	}
}

func checkChunks(t *testing.T, d doc, cs []chunk) {
	t.Helper()
	for _, c := range cs {
		if tokens(c.Text) > maxChunkTokens {
			t.Errorf("%s: %d tokens", c.ID, tokens(c.Text))
		}
		if c.Start >= c.End || c.End > len(d.Text) {
			t.Errorf("%s: range [%d, %d)", c.ID, c.Start, c.End)
		}
	}
}

func TestFixedChunker(t *testing.T) {
	c := loadFixture(t)
	for _, d := range c.Docs {
		cs, _ := fixedChunker{}.Chunk(context.Background(), d)
		checkChunks(t, d, cs)
		covered := make([]bool, len(d.Text))
		for i, ch := range cs {
			if ch.Text != d.Text[ch.Start:ch.End] {
				t.Errorf("%s: the text isn't its range", ch.ID)
			}
			for j := ch.Start; j < ch.End; j++ {
				covered[j] = true
			}
			if i == len(cs)-1 {
				continue
			}
			if !sentenceEnd(d.Text, ch.End) {
				t.Errorf("%s ends mid-sentence: %q", ch.ID, d.Text[max(0, ch.End-20):ch.End+5])
			}
			if over := ch.End - cs[i+1].Start; over <= 0 || over > overlapBytes {
				t.Errorf("%s: %d bytes of overlap", ch.ID, over)
			}
		}
		for j, ok := range covered {
			if !ok && !strings.ContainsRune(" \t\n", rune(d.Text[j])) {
				t.Fatalf("%s: byte %d is in no chunk", d.File, j)
			}
		}
	}
}

func TestHeadingChunker(t *testing.T) {
	c := loadFixture(t)
	d := c.Docs[0]
	cs, _ := headingChunker{}.Chunk(context.Background(), d)
	checkChunks(t, d, cs)
	var tableParts, noticeParts int
	for _, ch := range cs {
		lead, body, _ := strings.Cut(ch.Text, "\n\n")
		if !strings.HasPrefix(lead, "Rental contract.docx › Rental contract") {
			t.Errorf("%s: lead %q", ch.ID, lead)
		}
		if strings.HasSuffix(lead, "› 3 Utilities") && strings.HasPrefix(body, "|") {
			tableParts++
			if !strings.HasPrefix(body, "| Item | Cost | Shared | Billed |\n|---|---|---|---|\n") {
				t.Errorf("%s: no header: %q", ch.ID, clip(body, 80))
			}
			for _, row := range strings.Split(body, "\n") {
				if !strings.HasPrefix(row, "|") || !strings.HasSuffix(row, "|") {
					t.Errorf("%s: a cut row %q", ch.ID, row)
				}
			}
		}
		if strings.HasSuffix(lead, "› 4.1 Notice") {
			noticeParts++
			if rest := d.Text[ch.End:]; !strings.HasPrefix(rest, "\n\n") {
				t.Errorf("%s doesn't end at a paragraph end: %q", ch.ID, clip(rest, 30))
			}
		}
	}
	if tableParts < 2 || noticeParts < 2 {
		t.Errorf("%d table parts, %d notice parts; the fixture should split both", tableParts, noticeParts)
	}
	// The first piece of a section covers its heading.
	h := strings.Index(d.Text, "### 4.1 Notice")
	if !overlapsAny(cs, h, h+len("### 4.1 Notice"), "› 4.1 Notice") {
		t.Error("no 4.1 chunk covers its heading")
	}
}

func overlapsAny(cs []chunk, start, end int, lead string) bool {
	for _, c := range cs {
		if c.Start < end && start < c.End && strings.Contains(strings.SplitN(c.Text, "\n", 2)[0], lead) {
			return true
		}
	}
	return false
}

func TestHeadingChunkerSmall(t *testing.T) {
	text := "Intro without a heading.\n\n# A\n\n## A.1\n\n| x | y |\n|---|---|\n| 1 | 2 |\n\n```\n# not a heading\n\nstill code\n```\n\n## A.2\n\nLast words.\n"
	d := doc{File: "s.md", SourceName: "s.docx", Text: text}
	cs, _ := headingChunker{}.Chunk(context.Background(), d)
	var got []string
	for _, c := range cs {
		got = append(got, strings.SplitN(c.Text, "\n", 2)[0])
	}
	want := []string{"s.docx", "s.docx › A › A.1", "s.docx › A › A.2"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("leads %q, want %q", got, want)
	}
	if !strings.Contains(cs[1].Text, "| 1 | 2 |") || !strings.Contains(cs[1].Text, "still code") {
		t.Errorf("A.1: %q", cs[1].Text)
	}
}

// Persian text has bytes such as 0x85 (in م) that are white space when
// read as runes. No cut may fall inside a character.
func TestChunkersPersian(t *testing.T) {
	sentences := strings.Repeat("مهلت فسخ قرارداد سه ماه است و مستأجر باید نامه را امضا کند. ", 60)
	noSpaces := strings.Repeat("مممممممممم", 300)
	text := "# قرارداد\n\n## ۴ فسخ\n\n" + sentences + "\n\n" + noSpaces + "\n"
	d := doc{File: "fa.md", SourceName: "قرارداد.docx", Text: text}
	for _, ck := range chunkers {
		cs, _ := ck.Chunk(context.Background(), d)
		checkChunks(t, d, cs)
		for _, c := range cs {
			if !utf8.ValidString(c.Text) {
				t.Errorf("%s %s: a character is cut", ck.Name(), c.ID)
			}
		}
		if len(cs) < 4 {
			t.Errorf("%s: %d chunks", ck.Name(), len(cs))
		}
	}
}

func TestQueryBuilder(t *testing.T) {
	terms, err := queryTerms(`قیمت‌ها "x`)
	if err != nil {
		t.Fatal(err)
	}
	got := ftsQuery(terms)
	want := `text : (("قیمتها"* OR "قیمت ها"*) AND """x"*)`
	if got != want {
		t.Fatalf("%s\nwant %s", got, want)
	}
	if _, err := queryTerms(strings.Repeat("a ", 33)); err == nil {
		t.Error("33 words: no error")
	}
}

func TestFTS(t *testing.T) {
	ctx := context.Background()
	c := loadFixture(t)
	sets, closeSets, err := buildSets(ctx, c, t.TempDir(), chunkers)
	if err != nil {
		t.Fatal(err)
	}
	defer closeSets()
	firstHit := func(s *set, q, want string) {
		t.Helper()
		l, err := s.fts(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		if len(l) == 0 || l[0].ID != want {
			t.Errorf("%s %q: %v, want %s first", s.Name, q, l, want)
		}
	}
	firstHit(sets[0], "prices in euros", "m001")
	firstHit(sets[0], "قیمت‌ها تومان", "m005")
	firstHit(sets[0], "Strasse", "m009") // ß → ss
	l, _ := sets[0].fts(ctx, "flight to Lisbon")
	if len(l) != 0 {
		t.Errorf("no-answer query found %v", l)
	}
	for _, s := range sets[1:] {
		l, err := s.fts(ctx, "Kündigungsfrist")
		if err != nil || len(l) == 0 || !strings.HasPrefix(l[0].ID, "arbeitsvertrag-de.md#") {
			t.Errorf("%s: %v %v", s.Name, l, err)
		}
	}
}

func TestRRF(t *testing.T) {
	a := []scored{{ID: "x"}, {ID: "y"}, {ID: "z"}}
	b := []scored{{ID: "y"}, {ID: "w"}}
	got := rrf(10, a, b)
	ids := []string{}
	for _, s := range got {
		ids = append(ids, s.ID)
	}
	if strings.Join(ids, ",") != "y,x,w,z" {
		t.Fatalf("order %v", ids)
	}
	if want := 1.0/62 + 1.0/61; math.Abs(got[0].Score-want) > 1e-12 {
		t.Errorf("y: %v, want %v", got[0].Score, want)
	}
	if want := 1.0 / 61; math.Abs(got[1].Score-want) > 1e-12 {
		t.Errorf("x: %v, want %v", got[1].Score, want)
	}
	if len(rrf(2, a, b)) != 2 {
		t.Error("not cut to n")
	}
}

func TestForms(t *testing.T) {
	f := newFake("f", 512)
	vs, _ := f.Embed(context.Background(), []string{"prices in euros please", "euros prices now", "Wochenbericht am Montag"})
	for _, fm := range []form{formFull, form256, formInt8} {
		ix := newIndex(fm, []string{"a", "b"}, vs[1:])
		got := ix.search(vs[0], 2)
		if got[0].ID != "a" || got[0].Score <= got[1].Score {
			t.Errorf("%s: %v", fm, got)
		}
		if got[0].Score > 1+1e-6 {
			t.Errorf("%s: cosine %v over 1", fm, got[0].Score)
		}
	}
	full := newIndex(formFull, []string{"a"}, vs[1:2]).search(vs[0], 1)[0].Score
	q := newIndex(formInt8, []string{"a"}, vs[1:2]).search(vs[0], 1)[0].Score
	if math.Abs(full-q) > 0.02 {
		t.Errorf("int8 cosine %v, full %v", q, full)
	}
	if v, _ := prepare(form256, vs[0]); len(v) != 256 || math.Abs(norm(v)-1) > 1e-5 {
		t.Errorf("256: width %d, norm %v", len(v), norm(v))
	}
	if bytesPerVector(formInt8, 768) != 772 || bytesPerVector(form256, 768) != 1024 {
		t.Error("bytes per vector")
	}
	if len(formsFor(modelSpec{Matryoshka: false}, 768)) != 2 {
		t.Error("a model without Matryoshka gets the 256 run")
	}
}

func norm(v []float32) float64 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	return math.Sqrt(s)
}

func TestHTTPEmbedders(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&gotBody)
		switch r.URL.Path {
		case "/api/embed":
			w.Write([]byte(`{"embeddings": [[1, 0, 0], [0, 1, 0]]}`))
		case "/v1/embeddings":
			// Out of order, as an endpoint may send them.
			w.Write([]byte(`{"data": [{"index": 1, "embedding": [0, 1, 0]}, {"index": 0, "embedding": [1, 0, 0]}]}`))
		case "/v1/limited/embeddings":
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": "slow down, secret-key"}`))
		case "/v1/quota/embeddings":
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error": {"code": "insufficient_quota"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	k := keys{"TEST_KEY": "secret-key"}

	o, err := newEmbedder(modelSpec{Name: "o", Kind: "ollama", BaseURL: srv.URL + "/", Model: "m"}, k)
	if err != nil {
		t.Fatal(err)
	}
	vs, err := o.Embed(ctx, []string{"a", "b"})
	if err != nil || len(vs) != 2 || vs[1][1] != 1 || o.Dims() != 3 {
		t.Fatalf("ollama: %v %v dims %d", vs, err, o.Dims())
	}
	if gotPath != "/api/embed" || gotBody["model"] != "m" || gotAuth != "" {
		t.Errorf("ollama request: %s %v %q", gotPath, gotBody, gotAuth)
	}

	spec := modelSpec{Name: "c", Kind: "openai_compatible", BaseURL: srv.URL + "/v1", Model: "e", KeyEnv: "TEST_KEY"}
	c, _ := newEmbedder(spec, k)
	vs, err = c.Embed(ctx, []string{"a", "b"})
	if err != nil || vs[0][0] != 1 || vs[1][1] != 1 {
		t.Fatalf("openai: %v %v", vs, err)
	}
	if gotPath != "/v1/embeddings" || gotAuth != "Bearer secret-key" || gotBody["encoding_format"] != "float" {
		t.Errorf("openai request: %s %v %q", gotPath, gotBody, gotAuth)
	}

	for _, path := range []string{"/v1/limited", "/v1/quota"} {
		spec.BaseURL = srv.URL + path
		e, _ := newEmbedder(spec, k)
		_, err := e.Embed(ctx, []string{"a"})
		if !errors.Is(err, errStopped) {
			t.Errorf("%s: %v, want a stop", path, err)
		}
		if err != nil && strings.Contains(err.Error(), "secret-key") {
			t.Errorf("%s: the key is in the error: %v", path, err)
		}
	}

	if _, err := newEmbedder(modelSpec{Name: "c", Kind: "openai_compatible", KeyEnv: "UNSET_SPIKE033_KEY"}, keys{}); err == nil {
		t.Error("a missing key: no error")
	}
	if _, err := newEmbedder(modelSpec{Name: "b", Kind: "builtin"}, k); err == nil || !strings.Contains(err.Error(), "no model folder") {
		t.Errorf("builtin without dir: %v", err)
	}
	if _, err := newEmbedder(modelSpec{Name: "b", Kind: "builtin", Dir: t.TempDir()}, k); err == nil || !strings.Contains(err.Error(), "no model in") {
		t.Errorf("builtin without a model: %v", err)
	}
	ce, err := newEmbedder(modelSpec{Name: "r", Kind: "cached"}, k)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ce.Embed(ctx, []string{"a"}); !errors.Is(err, errNotCached) {
		t.Errorf("cached: %v", err)
	}
}

func TestModelsFile(t *testing.T) {
	ms, err := loadModels("models.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pickModels(ms, []string{"gemma-ollama", "gemini", "granite", "granite-reftok", "fake"}); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(t.TempDir(), "m.yaml")
	os.WriteFile(bad, []byte("embedders:\n  - {name: x, kind: guess, base_url: http://localhost/}\n"), 0o644)
	if _, err := loadModels(bad); err == nil {
		t.Error("kind guess: no error")
	}
}

func TestEvaluate(t *testing.T) {
	ctx := context.Background()
	c := loadFixture(t)
	all, err := loadModels("models.yaml")
	if err != nil {
		t.Fatal(err)
	}
	models, _ := pickModels(all, []string{"fake"})
	// A built-in model whose folder has no model stops; the others go on.
	models = append(models, modelSpec{Name: "builtin", Kind: "builtin", Dir: t.TempDir(), BatchSize: 32})
	dir := t.TempDir()
	cache, err := openCache(filepath.Join(dir, "vectors.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	var fakes []*fakeEmbedder
	o := options{Models: models, Cache: cache, Chunkings: chunkers, newEmbedder: func(m modelSpec, k keys) (embedder, error) {
		e, err := newEmbedder(m, k)
		if f, ok := e.(*fakeEmbedder); ok {
			fakes = append(fakes, f)
		}
		return e, err
	}}
	out, err := evaluate(ctx, c, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Stops) != 1 || out.Stops[0].Model != "builtin" {
		t.Errorf("stops %v, want builtin only", out.Stops)
	}
	// FTS once per set; for the fake: 2 prefix settings × 3 forms × 2
	// conditions, per set.
	nq := len(c.MsgQueries) + 2*len(c.FileQueries)
	if want := nq + 2*3*2*nq; len(out.Results) != want {
		t.Errorf("%d results, want %d", len(out.Results), want)
	}
	find := func(set, model string, prefix bool, f form, cond, q string) result {
		for _, r := range out.Results {
			if r.Set == set && r.Model == model && r.Prefix == prefix && r.Form == f && r.Condition == cond && r.Query == q {
				return r
			}
		}
		t.Fatalf("no result %s %s %v %s %s %s", set, model, prefix, f, cond, q)
		return result{}
	}
	if r := find("messages", "", false, "", "fts", "qm001"); r.First != 1 {
		t.Errorf("fts qm001: first %d", r.First)
	}
	if r := find("messages", "fake", false, formFull, "hybrid", "qm001"); r.First != 1 || len(r.Top5) == 0 {
		t.Errorf("hybrid qm001: %+v", r)
	}
	if r := find("files/heading", "", false, "", "fts", "qf003"); r.First != 1 {
		t.Errorf("fts qf003 (a table row): first %d", r.First)
	}
	if r := find("messages", "fake", false, formInt8, "vector", "qm007"); r.First != 0 || r.Returned != len(c.Messages) {
		t.Errorf("vector no-answer: %+v", r)
	}
	if r := find("messages", "", false, "", "fts", "qm007"); r.Returned != 0 {
		t.Errorf("fts no-answer returned %d", r.Returned)
	}

	if err := writeResults(filepath.Join(dir, "results"), out, time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	sum, _ := os.ReadFile(filepath.Join(dir, "results", "summary.md"))
	for _, want := range []string{"## Recall@5 and MRR", "| messages | — | — | fts |", "| files/heading | fake +prefix | int8 | hybrid |",
		"## No-answer queries", "## Embedding time", "## Storage", "| fake | int8 | 516 |", "builtin stopped"} {
		if !strings.Contains(string(sum), want) {
			t.Errorf("summary.md has no %q", want)
		}
	}
	lines, _ := os.ReadFile(filepath.Join(dir, "results", "queries.jsonl"))
	if n := strings.Count(string(lines), "\n"); n != len(out.Results) {
		t.Errorf("queries.jsonl has %d lines", n)
	}

	// A rerun reads every vector from the cache.
	before := fakes[0].texts.Load()
	out2, err := evaluate(ctx, c, o)
	if err != nil {
		t.Fatal(err)
	}
	if n := fakes[1].texts.Load(); n != 0 || before == 0 {
		t.Errorf("first run embedded %d texts, the rerun %d", before, n)
	}
	for _, s := range out2.Speeds {
		if s.Fresh != 0 {
			t.Errorf("rerun speed %+v", s)
		}
	}
}

// loadingFake loads like the built-in embedder: before a job with texts to
// embed, and it must be released after each one.
type loadingFake struct {
	*fakeEmbedder
	loaded          bool
	loads, releases int
}

func (l *loadingFake) load(context.Context) error {
	if !l.loaded {
		l.loaded = true
		l.loads++
	}
	return nil
}

func (l *loadingFake) release() error {
	if l.loaded {
		l.loaded = false
		l.releases++
	}
	return nil
}

func (l *loadingFake) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if !l.loaded {
		return nil, errors.New("embed before load")
	}
	return l.fakeEmbedder.Embed(ctx, texts)
}

func TestLoaderReleased(t *testing.T) {
	ctx := context.Background()
	c := loadFixture(t)
	cache, err := openCache(filepath.Join(t.TempDir(), "vectors.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	var ls []*loadingFake
	o := options{Models: []modelSpec{{Name: "l", Kind: "fake", Model: "m", BatchSize: 4}}, Cache: cache, Chunkings: chunkers,
		newEmbedder: func(m modelSpec, _ keys) (embedder, error) {
			l := &loadingFake{fakeEmbedder: newFake(m.Name, 64)}
			ls = append(ls, l)
			return l, nil
		}}
	out, err := evaluate(ctx, c, o)
	if err != nil || len(out.Stops) != 0 {
		t.Fatalf("%v %v", err, out.Stops)
	}
	// Per set: the items, then the queries. The file queries repeat across
	// chunkings, so the second chunking finds them in the cache.
	if l := ls[0]; l.loaded || l.loads != 5 || l.releases != 5 {
		t.Errorf("loaded %v, %d loads, %d releases; want 5 each", l.loaded, l.loads, l.releases)
	}
	if _, err := evaluate(ctx, c, o); err != nil {
		t.Fatal(err)
	}
	if ls[1].loads != 0 {
		t.Errorf("a cached rerun loaded the model %d times", ls[1].loads)
	}
}
