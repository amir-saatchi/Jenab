// Command gen writes SPIKE-046's SQL task sets: database requests over
// seven fixtures, each with a reference answer worked out here by running
// the task's reference SQL on the fixture.
//
//	go run ./spikes/046-sql-lora/gen            # writes the fixtures and the sets
//	go run ./spikes/046-sql-lora/gen -check     # fails if a written file is out of date
//
// It writes:
//
//   - scenarios/fixtures/{shop,clinic,bills,reading}/data.sql: the new
//     fixtures, made from a fixed seed, so every run writes the same rows;
//   - scenarios/sql-train-<fixture>[-short] and sql-heldout-<fixture>[-short]:
//     one set per fixture and card. A plain set has the card the app builds
//     (project_card: true). A -short set has the card the app writes for a
//     project too big for columns: table names only, so the chat needs
//     describe_table first;
//   - an _offline.yaml per set: scripted replies that pass, so a scripted
//     run checks every reference answer on Jenab's own database, with no
//     keys.
//
// Training fixtures: bitcoin, shop, clinic, bills. Held out: the
// data-tools ledger, crypto-watch, reading. The checks are the runner's
// asserts only (tool_called with args_match and result_match,
// answer_contains, db_query); no judge model.
//
// More variants: add parameters to a template's list in fixtures.go.
// Every task is written in English; one with an FA text also gets a
// Persian twin, a task of its own.
package main

import (
	"bytes"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v4"
	_ "modernc.org/sqlite"
)

func main() {
	root := flag.String("root", ".", "the repository's root")
	check := flag.Bool("check", false, "write nothing; fail if a file would change")
	flag.Parse()
	if err := run(*root, *check); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}

// fixture is a project database and what the sets over it share.
type fixture struct {
	name    string // folder and ID prefix
	title   string // the project's title
	today   string // the fixture's day
	data    string // the set's data: path, from the set's folder
	file    string // the data file, from the root
	sql     string // the data, when this program writes it
	split   string // train or heldout
	short   bool   // also a -short set
	tasks   func(f *fixture) []*task
	db      *sql.DB
	written []string
}

func run(root string, check bool) error {
	var stale []string
	write := func(path string, b []byte) error {
		full := filepath.Join(root, path)
		old, err := os.ReadFile(full)
		// A Windows checkout has CRLF (text=auto); the content is what counts.
		if err == nil && bytes.Equal(bytes.ReplaceAll(old, []byte("\r\n"), []byte("\n")), b) {
			return nil
		}
		if check {
			stale = append(stale, path)
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		return os.WriteFile(full, b, 0o644)
	}
	total := map[string]int{}
	for _, f := range fixtures() {
		if f.sql != "" {
			if err := write(f.file, []byte(f.sql)); err != nil {
				return err
			}
		} else {
			b, err := os.ReadFile(filepath.Join(root, f.file))
			if err != nil {
				return err
			}
			f.sql = string(b)
		}
		db, err := open(f.sql)
		if err != nil {
			return fmt.Errorf("%s: %w", f.name, err)
		}
		f.db = db
		defer db.Close()
		tasks := f.tasks(f)
		var plain, short []*task
		ids := map[string]bool{}
		for _, t := range tasks {
			if t.err != nil {
				return fmt.Errorf("%s: %w", t.id, t.err)
			}
			if t.skip {
				continue
			}
			if ids[t.id] {
				return fmt.Errorf("%s: two tasks with this ID", t.id)
			}
			ids[t.id] = true
			if t.short {
				short = append(short, t)
			} else {
				plain = append(plain, t)
			}
		}
		for _, s := range []struct {
			suffix string
			tasks  []*task
		}{{"", plain}, {"-short", short}} {
			if len(s.tasks) == 0 {
				continue
			}
			dir := filepath.Join("scenarios", "sql-"+f.split+"-"+f.name+s.suffix)
			files, err := setFiles(f, s.suffix != "", s.tasks)
			if err != nil {
				return err
			}
			for name, b := range files {
				if err := write(filepath.Join(dir, name), b); err != nil {
					return err
				}
			}
			total[f.split] += len(s.tasks)
			fmt.Printf("%-34s %3d tasks\n", dir, len(s.tasks))
		}
	}
	fmt.Printf("train %d, heldout %d\n", total["train"], total["heldout"])
	if len(stale) > 0 {
		return errors.New("out of date: " + strings.Join(stale, ", "))
	}
	return nil
}

// open loads a fixture into an in-memory database, with the internal
// table its data may fill.
func open(data string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE _jenab_columns (table_name TEXT, column_name TEXT, kind TEXT)`); err != nil {
		return nil, err
	}
	if _, err := db.Exec(data); err != nil {
		return nil, err
	}
	return db, nil
}

// The files of a set.

type setYAML struct {
	Title       string `yaml:"title"`
	Split       string `yaml:"split"`
	Card        string `yaml:"card,omitempty"`
	ProjectCard bool   `yaml:"project_card,omitempty"`
	Today       string `yaml:"today"`
	Data        string `yaml:"data"`
}

type scenarioYAML struct {
	ID       string       `yaml:"id"`
	Title    string       `yaml:"title"`
	Tests    []string     `yaml:"tests"`
	Context  string       `yaml:"context"`
	Why      string       `yaml:"why"`
	Messages []message    `yaml:"messages"`
	Asserts  []assertYAML `yaml:"asserts"`
}

type message struct {
	Text string `yaml:"text"`
}

type assertYAML struct {
	Type        string   `yaml:"type"`
	Test        string   `yaml:"test"`
	Tool        string   `yaml:"tool,omitempty"`
	ArgsMatch   string   `yaml:"args_match,omitempty"`
	ResultMatch string   `yaml:"result_match,omitempty"`
	Match       string   `yaml:"match,omitempty"`
	Query       string   `yaml:"query,omitempty"`
	Final       bool     `yaml:"final,omitempty"`
	N           int      `yaml:"n,omitempty"`
	Tools       []string `yaml:"tools,omitempty"`
}

type reply struct {
	Text  string `yaml:"text,omitempty"`
	Calls []call `yaml:"calls,omitempty"`
}

type call struct {
	Name string         `yaml:"name"`
	Args map[string]any `yaml:"args"`
}

func setFiles(f *fixture, short bool, tasks []*task) (map[string][]byte, error) {
	out := map[string][]byte{}
	head := "# SPIKE-046: written by spikes/046-sql-lora/gen; edit that, not this file.\n"
	s := setYAML{Title: f.title, Split: f.split, Today: f.today, Data: "../" + f.data}
	if short {
		s.Card = "card.md"
		out["card.md"] = []byte(shortCard(f))
	} else {
		s.ProjectCard = true
	}
	b, err := marshal(s)
	if err != nil {
		return nil, err
	}
	what := "the card the app builds from the project"
	if short {
		what = "a card with table names only, as the app writes it for a big project, so the chat reads columns with describe_table"
	}
	out["_set.yaml"] = append([]byte(head+"# "+f.title+" ("+f.split+"), with "+what+".\n"), b...)
	script := map[string][]reply{}
	for _, t := range tasks {
		sc := scenarioYAML{ID: t.id, Title: t.title, Context: "main", Why: t.why, Messages: []message{{t.text}}, Asserts: t.asserts}
		seen := map[string]bool{}
		for _, a := range t.asserts {
			if a.Test != "info" && !seen[a.Test] {
				seen[a.Test] = true
				sc.Tests = append(sc.Tests, a.Test)
			}
		}
		b, err := marshal(sc)
		if err != nil {
			return nil, err
		}
		out[strings.ToLower(t.id)+".yaml"] = append([]byte(head), b...)
		script[t.id] = t.script
	}
	b, err = marshal(map[string]any{"scenarios": script})
	if err != nil {
		return nil, err
	}
	out["_offline.yaml"] = append([]byte(head+"# Scripted replies that pass: jenab-scenarios run -model scripted:<this file> <set>\n"), b...)
	return out, nil
}

func marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	e := yaml.NewEncoder(&b)
	e.SetIndent(2)
	if err := e.Encode(v); err != nil {
		return nil, err
	}
	return b.Bytes(), e.Close()
}

// shortCard is the card the app writes when the columns don't fit
// (store.CardWith): the head, then the table names.
func shortCard(f *fixture) string {
	rows, err := f.db.Query(`SELECT name FROM sqlite_schema WHERE type = 'table' AND name NOT LIKE '\_jenab\_%' ESCAPE '\' ORDER BY lower(name)`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		rows.Scan(&n)
		names = append(names, n)
	}
	slices.Sort(names)
	return fmt.Sprintf("Project card of %q, as of %s 08:00 (UTC+02:00). Row counts and date ranges are from then; query for numbers now.\nTables (%d; the card has no room for their columns, so use describe_table for a table's columns, key and rows): %s.\nBucket: empty. Saved links: 0.\n",
		f.title, f.today, len(names), strings.Join(names, ", "))
}
