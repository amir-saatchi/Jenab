package view

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/skill"
	"github.com/amir-saatchi/jenab/internal/store"
)

// guideTables are the tables the config-guide skill's examples use.
var guideTables = []string{
	`CREATE TABLE cities(id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE)`,
	`CREATE TABLE weather(date TEXT NOT NULL, city TEXT NOT NULL, temp REAL, PRIMARY KEY (date, city)) WITHOUT ROWID`,
	`CREATE TABLE tasks(id INTEGER PRIMARY KEY, title TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'open', due TEXT)`,
}

var yamlBlock = regexp.MustCompile("(?s)```yaml\n(.*?)```")

// TestConfigGuideExamples validates every YAML example of the
// config-guide skill (P3-03) in order, on a project with the tables they
// use, and saves each, so a later example can use an earlier one. An
// example that fails would teach the model a config the app refuses.
func TestConfigGuideExamples(t *testing.T) {
	ctx := context.Background()
	s, err := skill.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	g, ok := s.Get(chat.KindChat, "config-guide")
	if !ok {
		t.Fatal("no config-guide skill")
	}
	examples := yamlBlock.FindAllStringSubmatch(g.Body, -1)
	if len(examples) < 5 {
		t.Fatalf("%d examples, want 5 or more", len(examples))
	}
	p, err := store.OpenProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close(ctx)
	_, err = store.Do(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		for _, q := range guideTables {
			if _, err := tx.Exec(q); err != nil {
				return struct{}{}, err
			}
		}
		return struct{}{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	v := &Validator{Env: ProjectEnv{DB: p}}
	for i, m := range examples {
		src := m[1]
		kind, parsed, res, err := v.Config(ctx, []byte(src))
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Issues) > 0 {
			t.Errorf("example %d:\n%s\n%s", i+1, src, res)
			continue
		}
		save := store.ConfigSave{Kind: kind, YAML: src, Source: id.SourceUser}
		switch c := parsed.(type) {
		case *View:
			save.ID, save.Type, save.Title = c.ID, c.Type, c.Title
		case *Page:
			save.ID, save.Title = c.ID, c.Title
		}
		if _, err := p.SaveConfig(ctx, save); err != nil {
			t.Fatalf("example %d: %v", i+1, err)
		}
	}
}
