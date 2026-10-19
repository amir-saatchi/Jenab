package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// The corpus format is the shared contract of SPIKE-033's pieces: see
// corpus/README-messages.md and corpus/README-docs.md.

type message struct {
	ID   string `yaml:"id"`
	Role string `yaml:"role"`
	Lang string `yaml:"lang"`
	Text string `yaml:"text"`
	Chat string `yaml:"-"`
}

type chatYAML struct {
	ID       string    `yaml:"id"`
	Title    string    `yaml:"title"`
	Messages []message `yaml:"messages"`
}

type doc struct {
	File       string `yaml:"file"`
	Lang       string `yaml:"lang"`
	Title      string `yaml:"title"`
	SourceName string `yaml:"source_name"`
	Text       string `yaml:"-"`
}

type target struct {
	Message string `yaml:"message,omitempty"`
	File    string `yaml:"file,omitempty"`
	Quote   string `yaml:"quote,omitempty"`
	// Start and End are the quote's byte range in the file.
	Start, End int `yaml:"-"`
}

type query struct {
	ID      string   `yaml:"id"`
	Kind    string   `yaml:"kind"`
	Lang    string   `yaml:"lang"`
	Query   string   `yaml:"query"`
	Targets []target `yaml:"targets"`
}

var (
	queryKinds = []string{"same_words", "paraphrase", "cross_language", "no_answer"}
	langs      = []string{"en", "de", "fa"}
)

type corpus struct {
	Chats       []chatYAML
	Messages    []message // every chat's, in order
	Docs        []doc
	MsgQueries  []query
	FileQueries []query
}

// loadCorpus reads what the folder has: the messages with their queries,
// the documents with theirs, or both.
func loadCorpus(dir string) (*corpus, error) {
	c := &corpus{}
	var err error
	if have(dir, "messages.yaml") {
		if err = c.loadMessages(dir); err != nil {
			return nil, err
		}
	}
	if have(dir, "docs.yaml") {
		if err = c.loadDocs(dir); err != nil {
			return nil, err
		}
	}
	if c.Messages == nil && c.Docs == nil {
		return nil, fmt.Errorf("%s: neither messages.yaml nor docs.yaml", dir)
	}
	return c, nil
}

func have(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

func readYAML(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func (c *corpus) loadMessages(dir string) error {
	var f struct {
		Chats []chatYAML `yaml:"chats"`
	}
	path := filepath.Join(dir, "messages.yaml")
	if err := readYAML(path, &f); err != nil {
		return err
	}
	ids := map[string]bool{}
	chats := map[string]bool{}
	for _, ch := range f.Chats {
		if ch.ID == "" || chats[ch.ID] {
			return fmt.Errorf("%s: chat %q: missing or repeated ID", path, ch.ID)
		}
		chats[ch.ID] = true
		for _, m := range ch.Messages {
			switch {
			case m.ID == "" || ids[m.ID]:
				return fmt.Errorf("%s: chat %s: message %q: missing or repeated ID", path, ch.ID, m.ID)
			case m.Role != "user" && m.Role != "assistant":
				return fmt.Errorf("%s: message %s: role %q is not user or assistant", path, m.ID, m.Role)
			case !slices.Contains(langs, m.Lang):
				return fmt.Errorf("%s: message %s: lang %q is not en, de or fa", path, m.ID, m.Lang)
			case strings.TrimSpace(m.Text) == "":
				return fmt.Errorf("%s: message %s has no text", path, m.ID)
			}
			ids[m.ID] = true
			m.Chat = ch.ID
			c.Messages = append(c.Messages, m)
		}
	}
	c.Chats = f.Chats
	qpath := filepath.Join(dir, "queries-messages.yaml")
	if err := readYAML(qpath, &c.MsgQueries); err != nil {
		return err
	}
	return checkQueries(qpath, c.MsgQueries, func(t *target) error {
		if t.Message == "" || t.File != "" || !ids[t.Message] {
			return fmt.Errorf("target %+v is not a message of the corpus", *t)
		}
		return nil
	})
}

func (c *corpus) loadDocs(dir string) error {
	path := filepath.Join(dir, "docs.yaml")
	if err := readYAML(path, &c.Docs); err != nil {
		return err
	}
	byFile := map[string]*doc{}
	for i := range c.Docs {
		d := &c.Docs[i]
		switch {
		case d.File == "" || byFile[d.File] != nil:
			return fmt.Errorf("%s: document %q: missing or repeated file", path, d.File)
		case !slices.Contains(langs, d.Lang):
			return fmt.Errorf("%s: %s: lang %q is not en, de or fa", path, d.File, d.Lang)
		case d.SourceName == "":
			return fmt.Errorf("%s: %s has no source_name", path, d.File)
		}
		b, err := os.ReadFile(filepath.Join(dir, "docs", d.File))
		if err != nil {
			return err
		}
		if !utf8.Valid(b) {
			return fmt.Errorf("%s: not UTF-8", d.File)
		}
		// Byte ranges count from the file as written, with CRLF made LF
		// so a checkout with autocrlf gives the same ranges.
		d.Text = strings.ReplaceAll(string(b), "\r\n", "\n")
		byFile[d.File] = d
	}
	qpath := filepath.Join(dir, "queries-files.yaml")
	if err := readYAML(qpath, &c.FileQueries); err != nil {
		return err
	}
	return checkQueries(qpath, c.FileQueries, func(t *target) error {
		d := byFile[t.File]
		switch {
		case t.Message != "" || d == nil:
			return fmt.Errorf("target %q is not a file of the corpus", t.File)
		case t.Quote == "":
			return fmt.Errorf("target in %s has no quote", t.File)
		}
		var err error
		t.Start, err = uniqueIndex(d.Text, t.Quote)
		if err != nil {
			return fmt.Errorf("%s: quote %q: %w", t.File, clip(t.Quote, 60), err)
		}
		t.End = t.Start + len(t.Quote)
		return nil
	})
}

var (
	errNoQuote   = errors.New("not found in the file")
	errTwoQuotes = errors.New("found more than once in the file")
)

func uniqueIndex(text, quote string) (int, error) {
	i := strings.Index(text, quote)
	if i < 0 {
		return 0, errNoQuote
	}
	if strings.Contains(text[i+1:], quote) {
		return 0, errTwoQuotes
	}
	return i, nil
}

func checkQueries(path string, qs []query, check func(*target) error) error {
	ids := map[string]bool{}
	for _, q := range qs {
		switch {
		case q.ID == "" || ids[q.ID]:
			return fmt.Errorf("%s: query %q: missing or repeated ID", path, q.ID)
		case !slices.Contains(queryKinds, q.Kind):
			return fmt.Errorf("%s: %s: kind %q is not one of %s", path, q.ID, q.Kind, strings.Join(queryKinds, ", "))
		case !slices.Contains(langs, q.Lang):
			return fmt.Errorf("%s: %s: lang %q is not en, de or fa", path, q.ID, q.Lang)
		case strings.TrimSpace(q.Query) == "":
			return fmt.Errorf("%s: %s has no query", path, q.ID)
		case q.Kind == "no_answer" && len(q.Targets) > 0:
			return fmt.Errorf("%s: %s: a no_answer query has no targets", path, q.ID)
		case q.Kind != "no_answer" && len(q.Targets) == 0:
			return fmt.Errorf("%s: %s has no targets", path, q.ID)
		}
		ids[q.ID] = true
		for i := range q.Targets {
			if err := check(&q.Targets[i]); err != nil {
				return fmt.Errorf("%s: %s: %w", path, q.ID, err)
			}
		}
	}
	return nil
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}
