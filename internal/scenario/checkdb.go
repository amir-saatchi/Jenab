package scenario

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
)

// What the runs left behind: the project's tables at the end (db_query),
// the approval cards (approval_shown) and the subagents' transcripts, so
// the schema agent's calls and requests count too.

// subagentDir is where the app keeps subagent transcripts in the bucket
// (agent.transcriptDir).
const subagentDir = "subagents/"

// storedSub is the part of a stored subagent transcript the runner reads.
type storedSub struct {
	Name     string         `json:"name"`
	Requests int            `json:"requests"`
	Usage    chat.Usage     `json:"usage"`
	Messages []chat.Message `json:"messages"`
}

// readSub adds the subagent run whose transcript is at ref, run by the
// chat's call callID in turn, to rec.sub.
func (rec *record) readSub(ctx context.Context, p *project.Project, turn int, callID, ref string) error {
	_, rd, err := p.DB.OpenObject(ctx, ref)
	if err != nil {
		return fmt.Errorf("subagent transcript %s: %w", ref, err)
	}
	defer rd.Close()
	b, err := io.ReadAll(rd)
	if err != nil {
		return fmt.Errorf("subagent transcript %s: %w", ref, err)
	}
	var tr storedSub
	if err := json.Unmarshal(b, &tr); err != nil {
		return fmt.Errorf("subagent transcript %s: %w", ref, err)
	}
	rec.subRequests += tr.Requests
	rec.subUsage.Input += tr.Usage.Input
	rec.subUsage.CacheRead += tr.Usage.CacheRead
	rec.subUsage.CacheWrite += tr.Usage.CacheWrite
	rec.subUsage.Output += tr.Usage.Output
	for _, m := range tr.Messages {
		for _, pt := range m.Parts {
			e := event{turn: turn, msg: -1, agent: tr.Name, parent: callID}
			switch {
			case pt.Text != nil && m.Role == chat.RoleUser:
				e.kind, e.text = evUser, pt.Text.Text
			case pt.Text != nil:
				e.kind, e.text = evText, pt.Text.Text
			case pt.ToolCall != nil:
				// The stored transcript is indented; the chat's calls aren't.
				var args bytes.Buffer
				if json.Compact(&args, pt.ToolCall.Args) != nil {
					args.Reset()
					args.Write(pt.ToolCall.Args)
				}
				e.kind, e.tool, e.args, e.callID = evCall, pt.ToolCall.Name, args.String(), pt.ToolCall.ID
			case pt.ToolResult != nil:
				e.kind, e.text, e.callID, e.err = evResult, pt.ToolResult.Text, pt.ToolResult.CallID, pt.ToolResult.IsError
				for _, c := range rec.sub {
					if c.kind == evCall && c.parent == callID && c.callID == e.callID {
						e.tool = c.tool
					}
				}
			default:
				continue
			}
			rec.sub = append(rec.sub, e)
		}
	}
	return nil
}

// subTranscript writes the subagent run of the chat's call callID.
func (rec *record) subTranscript(b *strings.Builder, callID string) {
	for _, e := range rec.sub {
		if e.parent != callID {
			continue
		}
		switch e.kind {
		case evUser:
			fmt.Fprintf(b, "\n>> %s task:\n>> %s\n", e.agent, strings.ReplaceAll(clip(e.text, 1500), "\n", "\n>> "))
		case evText:
			if t := strings.TrimSpace(e.text); t != "" {
				fmt.Fprintf(b, "\n>> %s: %s\n", e.agent, strings.ReplaceAll(clip(t, 600), "\n", "\n>> "))
			}
		case evCall:
			fmt.Fprintf(b, "\n>> call %s %s\n", e.tool, clip(e.args, 2500))
		case evResult:
			mark := ""
			if e.err {
				mark = " (error)"
			}
			fmt.Fprintf(b, "\n>> result %s%s:\n>> %s\n", e.tool, mark, strings.ReplaceAll(clip(e.text, 1200), "\n", "\n>> "))
		}
	}
}

// approvalText is a card as approval_shown reads it: the kind, the
// question, the agent's line, the risk, the details and the answer.
func approvalText(a *chat.Approval) string {
	s := a.Kind + ": " + a.Ask
	for _, x := range []string{a.Why, a.Risk, a.Details} {
		if x != "" {
			s += "\n" + x
		}
	}
	if a.Answer != "" {
		s += "\nanswer: " + string(a.Answer)
	}
	return s
}

// dbAnswer is one db_query run: its table, if it had one, and its rows as
// text, or its error.
type dbAnswer struct {
	table string
	text  string
	err   string
}

func (d dbAnswer) String() string {
	s := d.text
	if d.err != "" {
		s = "error: " + d.err
	}
	if d.table != "" {
		return d.table + ": " + s
	}
	return s
}

func dbKey(a Assert) string { return a.Table + "\x00" + a.Query }

// dbQueries runs the db_query checks' SQL on the project's database.
func (r *run) dbQueries(ctx context.Context, p *project.Project) (map[string][]dbAnswer, error) {
	out := map[string][]dbAnswer{}
	for _, a := range r.sc.Asserts {
		if _, done := out[dbKey(a)]; a.Type != "db_query" || done {
			continue
		}
		ans, err := store.Do(ctx, p.DB.DB, limit.Interactive, func(tx *sql.Tx) ([]dbAnswer, error) {
			if a.Table == "" {
				return []dbAnswer{dbRun(ctx, tx, "", a.Query)}, nil
			}
			tables, err := dbTables(ctx, tx)
			if err != nil {
				return nil, err
			}
			re := regexp.MustCompile(a.Table)
			var ans []dbAnswer
			for _, t := range tables {
				if re.MatchString(t) {
					q := strings.NewReplacer("{table}", `"`+strings.ReplaceAll(t, `"`, `""`)+`"`,
						"{name}", `'`+strings.ReplaceAll(t, `'`, `''`)+`'`).Replace(a.Query)
					ans = append(ans, dbRun(ctx, tx, t, q))
				}
			}
			return ans, nil
		})
		if err != nil {
			return nil, fmt.Errorf("db_query: %w", err)
		}
		out[dbKey(a)] = ans
	}
	return out, nil
}

// dbTables are the project's own tables.
func dbTables(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT name FROM sqlite_schema WHERE type = 'table' AND name NOT LIKE 'sqlite\_%' ESCAPE '\' AND name NOT LIKE '\_jenab\_%' ESCAPE '\' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// dbRun runs q and gives its rows as lines of cells joined by "|", NULL
// as NULL.
func dbRun(ctx context.Context, tx *sql.Tx, table, q string) dbAnswer {
	d := dbAnswer{table: table}
	rows, err := tx.QueryContext(ctx, q)
	if err != nil {
		d.err = err.Error()
		return d
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	var lines []string
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			d.err = err.Error()
			return d
		}
		cells := make([]string, len(cols))
		for i, v := range vals {
			switch x := v.(type) {
			case nil:
				cells[i] = "NULL"
			case []byte:
				cells[i] = string(x)
			default:
				cells[i] = fmt.Sprint(x)
			}
		}
		lines = append(lines, strings.Join(cells, "|"))
	}
	if err := rows.Err(); err != nil {
		d.err = err.Error()
	}
	d.text = strings.Join(lines, "\n")
	return d
}

// dbCheck passes when an answer of a's query matches a.Match.
func (rec *record) dbCheck(a Assert) (string, string, float64) {
	ans := rec.db[dbKey(a)]
	if len(ans) == 0 {
		if a.Table != "" {
			return Fail, "no table matches " + a.Table, 0
		}
		return Fail, "the query didn't run", 0
	}
	re := regexp.MustCompile(a.Match)
	var seen []string
	for _, d := range ans {
		if d.err == "" && re.MatchString(d.text) {
			return Pass, clip(d.String(), 160), 0
		}
		seen = append(seen, clip(d.String(), 120))
	}
	return Fail, strings.Join(seen, "; "), 0
}
