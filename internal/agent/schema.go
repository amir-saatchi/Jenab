package agent

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// The schema agent (SPEC 8.2). request_schema_change hands a goal to a
// subagent (8.6) with the database-design and migrations skills; only it
// changes the schema, through apply_migration, and it can't edit memory.
// apply_migration tries the steps, asks for the user's approval when
// they delete data or the level asks (8.8), and applies them. A failed
// try goes back to the schema agent with its problems, up to 5 tries.
//
// The main agent gets applied, rejected or the user's denial: the card
// is answered inside the call, so needs_approval never reaches it. Its
// result is short, within tool_preview_tokens; the transcript has the
// rest, such as the SQL and every table described in full.

//go:embed prompts/schema.md
var schemaPrompt string

const (
	schemaAgent = "schema agent"
	schemaTries = 5 // the first try and 4 more (8.2)
)

// How a schema change ended.
const (
	schemaApplied  = "applied"
	schemaRejected = "rejected"
	schemaDenied   = "denied"
)

type schemaArgs struct {
	Goal string `json:"goal"`
}

// requestSchemaChange is request_schema_change(goal) (8.2).
func requestSchemaChange() tool.Tool {
	return tool.Func(tool.Spec{
		Name: "request_schema_change",
		Description: "Change the project's tables: create a table, add, rename or drop columns, add an index, or restructure or drop a table. " +
			"Only this tool changes the schema. Give the goal in plain words: what the table holds, its columns and key, and any data to keep or move. " +
			"The schema agent writes the migration and tries it; the user approves it when it deletes data or the approval level asks. " +
			"Returns applied with the migration and the tables as they are now, rejected with the reason, or the user's denial.",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {"goal": {"type": "string", "minLength": 1, "description": "What the schema should become and why, in plain words."}},
			"required": ["goal"],
			"additionalProperties": false
		}`),
		Effects: tool.ReadsDB | tool.WritesDB | tool.Schema | tool.AsksUser,
		Areas:   tool.Tables,
	}, runSchemaChange)
}

// schemaChange is one request_schema_change call.
type schemaChange struct {
	env    *tool.Env
	before map[string]bool // the tables at the start, by lower-case name
	deps   []store.DependentInfo
	tries  int
	state  string // "" while the schema agent works
	plan   string
	last   string // the last try's problems
	reason string // why it was rejected
	report store.MigrationReport
	tables []store.Table // after an applied migration: the tables it touched or made
	note   string        // what of them couldn't be read
	answer chat.Approval // the denied card
}

func runSchemaChange(ctx context.Context, env *tool.Env, a schemaArgs) (tool.Result, error) {
	if env.Subagent == nil {
		return tool.Result{}, &tool.Error{Msg: "request_schema_change can't run here: a subagent can't start the schema agent"}
	}
	sc := &schemaChange{env: env, before: map[string]bool{}}
	task, err := sc.task(ctx, a.Goal)
	if err != nil {
		return tool.Result{}, err
	}
	tools := []tool.Tool{sc.applyTool()}
	for _, t := range tool.DataTools() {
		if n := t.Spec().Name; n == "describe_table" || n == "query" {
			tools = append(tools, t)
		}
	}
	res, err := env.Subagent(ctx, tool.Subagent{Name: schemaAgent, System: schemaPrompt, Skills: []string{"database-design", "migrations"},
		Task: task, Tools: tools, Done: func() bool { return sc.state != "" }})
	switch {
	case sc.state == schemaApplied:
		// Applied stays applied, also when Stop came after it.
		return sc.applied(res.Ref), nil
	case err == nil:
	case ctx.Err() != nil:
		return tool.Result{}, err
	default:
		// A provider error, or one of the app's, such as a missing skill.
		msg := err.Error()
		var te *tool.Error
		if errors.As(err, &te) {
			msg = te.Msg
		}
		return tool.Result{}, &tool.Error{Msg: sc.fit(header("failed", res.Ref)+"\nNothing changed: "+msg, res.Ref), Err: err}
	}
	switch {
	case sc.state == schemaDenied:
		text := header(schemaDenied, res.Ref) + "\nNothing changed: " + strings.TrimPrefix(denied(&sc.answer), "not run: ") + "."
		return tool.Result{Text: sc.fit(sc.withPlan(text, "The plan was: "), res.Ref), Ref: res.Ref}, nil
	case sc.state == "" && res.Capped:
		sc.reason = "the schema agent reached its request limit before it applied a migration"
	case sc.state == "" && res.Cut:
		sc.reason = "the schema agent's answer was cut off by the output limit before it applied a migration"
		if res.Text != "" {
			sc.reason += "; it said: " + res.Text
		}
	case sc.state == "":
		sc.reason = res.Text
		if sc.reason == "" {
			sc.reason = "the schema agent stopped without a migration"
		}
	}
	text := header(schemaRejected, res.Ref) + "\nNothing changed. Reason: " + clip(sc.reason, 600)
	if sc.last != "" {
		text += "\nThe last try's problems:\n" + sc.last
	}
	return tool.Result{Text: sc.fit(text, res.Ref), Ref: res.Ref}, nil
}

// fit cuts a result to tool_preview_tokens at a line, pointing to the
// transcript for the rest.
func (sc *schemaChange) fit(text, ref string) string {
	max := sc.env.PreviewTokens * 4 // as tool.Output counts tokens
	if max <= 0 || len(text) <= max {
		return text
	}
	end := "\n… cut to fit"
	if ref != "" {
		end += "; the transcript has the rest: " + ref
	}
	cut := text[:max-len(end)]
	if i := strings.LastIndexByte(cut, '\n'); i > 0 {
		cut = cut[:i]
	}
	return strings.ToValidUTF8(cut, "") + end
}

// withPlan adds the plan as text's last line, cut to the room the result
// has left, so a long plan can't push out the lines before it.
func (sc *schemaChange) withPlan(text, label string) string {
	plan := clip(sc.plan, 600)
	if max := sc.env.PreviewTokens * 4; max > 0 && len(text)+1+len(label)+len(plan) > max {
		room := max - len(text) - 1 - len(label) - len("…")
		if room < 40 {
			return text
		}
		plan = strings.ToValidUTF8(plan[:room], "") + "…"
	}
	return text + "\n" + label + plan
}

// clip cuts s to n characters.
func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// header is a result's first line, which stays after a cut (tool.Stub).
func header(state, ref string) string {
	if ref == "" {
		return "[request_schema_change: " + state + "]"
	}
	return "[request_schema_change: " + state + " — ref: " + ref + "]"
}

// task is the schema agent's first message: the goal, every table as
// describe_table shows it, with its annotations and rows, and the
// dependents with the columns they use.
func (sc *schemaChange) task(ctx context.Context, goal string) (string, error) {
	db := sc.env.Project.DB
	names, err := db.Tables(ctx)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("Goal: " + strings.TrimSpace(goal) + "\n\n## Tables\n\n")
	if len(names) == 0 {
		b.WriteString("There are no tables yet.\n\n")
	}
	for _, n := range names {
		t, err := db.DescribeTable(ctx, n)
		if err != nil {
			return "", err
		}
		sc.before[strings.ToLower(n)] = true
		b.WriteString(tool.Describe(t) + "\n\n")
	}
	if sc.deps, err = db.Dependents(ctx); err != nil {
		return "", err
	}
	b.WriteString("## Dependents\n\n")
	if len(sc.deps) == 0 {
		b.WriteString("No saved views, pipelines or forms use the tables.\n")
	}
	for _, d := range sc.deps {
		b.WriteString("- " + depName(d) + ": uses " + strings.Join(d.Uses, ", ") + "\n")
	}
	return b.String(), nil
}

func depName(d store.DependentInfo) string {
	s := d.Kind + ":" + d.ID
	if d.Title != "" {
		s += " (" + d.Title + ")"
	}
	return s
}

type applyArgs struct {
	Plan    string                  `json:"plan"`
	Steps   []json.RawMessage       `json:"steps"`
	Updates []store.DependentUpdate `json:"updates"`
}

// applyTool is the schema agent's apply_migration.
func (sc *schemaChange) applyTool() tool.Tool {
	return tool.Func(tool.Spec{
		Name: "apply_migration",
		Description: "Try a migration, ask the user when it needs their approval, and apply it, all or nothing. " +
			"Steps are JSON objects with an op, as the migrations skill shows. " +
			"Returns the applied steps with their SQL, the user's denial, or the problems by path with the tries left; after a failed try nothing changed.",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"plan": {"type": "string", "minLength": 1, "description": "One or two sentences for the user: what changes and why."},
				"steps": {
					"type": "array", "minItems": 1,
					"items": {
						"type": "object",
						"properties": {"op": {"type": "string", "enum": ["create_table", "add_column", "rename_table", "rename_column", "create_index", "drop_index", "annotate_column", "insert_rows", "copy_data", "rebuild_table", "drop_column", "drop_table"]}},
						"required": ["op"]
					}
				},
				"updates": {
					"type": "array",
					"description": "The views, pipelines and forms that use what the steps rename or drop, each with its full new config.",
					"items": {
						"type": "object",
						"properties": {"kind": {"type": "string"}, "id": {"type": "string"}, "config": {}},
						"required": ["kind", "id", "config"],
						"additionalProperties": false
					}
				}
			},
			"required": ["plan", "steps"],
			"additionalProperties": false
		}`),
		Effects: tool.ReadsDB | tool.WritesDB | tool.Schema | tool.AsksUser,
		Areas:   tool.Tables,
	}, sc.apply)
}

func (sc *schemaChange) apply(ctx context.Context, env *tool.Env, a applyArgs) (tool.Result, error) {
	if sc.state != "" {
		return tool.Result{}, &tool.Error{Msg: "not run: the schema change is " + sc.state + " already"}
	}
	m := store.Migration{Steps: a.Steps, Updates: a.Updates}
	sc.tries++
	sc.plan = strings.TrimSpace(a.Plan)
	try, err := migrate(ctx, env, m, store.MigrateOptions{Try: true}, "trying")
	if err != nil {
		return tool.Result{}, sc.failed(ctx, err)
	}
	got, err := env.Approve(ctx, sc.card(a.Steps, try))
	if err != nil {
		return tool.Result{}, err
	}
	if got.Answer == chat.GrantDeny {
		sc.state, sc.answer = schemaDenied, got
		return tool.Result{Text: denied(&got)}, nil
	}
	// The card approved what the try drops and whose key it changes, and
	// no more: when a migration that ran meanwhile makes the steps drop
	// more, this one is refused.
	rep, err := migrate(ctx, env, m, store.MigrateOptions{Planned: &try}, "applying")
	if err != nil {
		return tool.Result{}, sc.failed(ctx, err)
	}
	sc.state, sc.report = schemaApplied, rep
	sc.describe(context.WithoutCancel(ctx), env)
	// The run ends here, so the schema agent doesn't read this; it is the
	// transcript's record, with the SQL and the tables in full.
	var b strings.Builder
	fmt.Fprintf(&b, "applied: schema version %d\n%s", rep.SchemaVersion, stepList(rep, true))
	if sc.note != "" {
		b.WriteString("\n" + sc.note + "\n")
	}
	for _, t := range sc.tables {
		b.WriteString("\n" + tool.Describe(t) + "\n")
	}
	return tool.Result{Text: strings.TrimRight(b.String(), "\n"), Whole: true}, nil
}

// migrate runs a try or the migration. A long one shows in the chat's
// status line while it runs (7.5).
func migrate(ctx context.Context, env *tool.Env, m store.Migration, o store.MigrateOptions, doing string) (store.MigrationReport, error) {
	if env.Progress != nil {
		shown := false
		o.Long = func(e store.Estimate) {
			shown = true
			env.Progress(doing + " a schema change, about " + aboutTime(e.Time))
		}
		defer func() {
			if shown {
				env.Progress("")
			}
		}()
	}
	return env.Project.DB.Migrate(ctx, env.Source, m, o)
}

// describe reads the tables an applied migration touched or made. One it
// can't read is noted: the migration is applied either way.
func (sc *schemaChange) describe(ctx context.Context, env *tool.Env) {
	db := env.Project.DB
	names, err := db.Tables(ctx)
	if err != nil {
		sc.note = "The tables couldn't be read after the migration (" + short(err.Error()) + "); describe_table shows them."
		return
	}
	now := touched(sc.report)
	var missed []string
	for _, n := range names {
		l := strings.ToLower(n)
		if sc.before[l] && !slices.Contains(now, l) {
			continue
		}
		t, terr := db.DescribeTable(ctx, n)
		if terr != nil {
			missed, err = append(missed, n), terr
			continue
		}
		sc.tables = append(sc.tables, t)
	}
	if len(missed) > 0 {
		sc.note = fmt.Sprintf("%s couldn't be read after the migration (%s); describe_table shows them.", strings.Join(missed, ", "), short(err.Error()))
	}
}

// failed turns a failed try or apply into a result the schema agent can
// act on. Nothing changed either way.
func (sc *schemaChange) failed(ctx context.Context, err error) error {
	var me *store.MigrationError
	switch {
	case ctx.Err() != nil:
		return err
	case errors.As(err, &me):
		var lines []string
		for i, p := range me.Problems {
			if i == maxProblems {
				lines = append(lines, fmt.Sprintf("- and %d more", len(me.Problems)-i))
				break
			}
			lines = append(lines, "- "+clip(p.String(), 300))
		}
		sc.last = strings.Join(lines, "\n")
	case errors.Is(err, store.ErrNeedsApproval):
		// The schema changed after the try, so the card no longer covers
		// what the steps drop.
		why := strings.TrimPrefix(err.Error(), store.ErrNeedsApproval.Error()+": ")
		sc.last = "- " + why + "; nothing was applied, so try the migration again for a new card"
	case errors.Is(err, store.ErrNoSpace):
		sc.state, sc.reason, sc.last = schemaRejected, "there isn't enough free disk space for the migration", ""
		return &tool.Error{Msg: "not applied: there isn't enough free disk space for the migration; nothing changed, and you are done", Err: err}
	default:
		// The app failed, such as a snapshot or the disk: another try
		// wouldn't help, so the change ends here.
		msg := short(err.Error())
		sc.state, sc.reason, sc.last = schemaRejected, "the app failed to run the migration: "+msg, ""
		return &tool.Error{Msg: "not applied: the app failed to run the migration (" + msg + "); nothing changed, and you are done", Err: err}
	}
	left := schemaTries - sc.tries
	if left <= 0 {
		sc.state, sc.reason = schemaRejected, fmt.Sprintf("the migration failed %d tries", schemaTries)
		return &tool.Error{Msg: "the migration failed; nothing changed, and there are no tries left:\n" + sc.last, Err: err}
	}
	return &tool.Error{Msg: fmt.Sprintf("the migration failed; nothing changed. Fix these and call apply_migration again (%s left):\n%s",
		count(left, "try", "tries"), sc.last), Err: err}
}

// maxProblems is how many of a failed try's problems the schema agent and
// the result get.
const maxProblems = 10

// card is the approval card for a tried migration (8.8): "migration" when
// it deletes data or changes a primary key, which always asks (8.2), and
// "schema" otherwise, which asks only at Strict. Its target is the steps'
// hash, so no answer covers another migration.
func (sc *schemaChange) card(steps []json.RawMessage, rep store.MigrationReport) chat.Approval {
	h := sha256.New()
	for _, s := range steps {
		h.Write(s)
		h.Write([]byte{0})
	}
	a := chat.Approval{ID: id.Approval(id.New()), Kind: "schema", Target: hex.EncodeToString(h.Sum(nil))[:32],
		Ask: "Apply this schema change?", Why: sc.plan, Risk: "Nothing is deleted.",
		Options: []chat.ApprovalOption{{Label: "Apply", Grant: chat.GrantOnce}, {Label: "Deny", Grant: chat.GrantDeny}}}
	var risk []string
	if len(rep.Lost) > 0 {
		a.Kind, a.Ask = "migration", "Apply a schema change that deletes data?"
		lost := make([]string, len(rep.Lost))
		for i, l := range rep.Lost {
			lost[i] = lostText(l, rep.LostRows)
		}
		risk = append(risk, "It deletes "+strings.Join(lost, ", ")+". A snapshot of the project is taken first.")
	}
	if len(rep.KeysChanged) > 0 {
		if a.Kind != "migration" {
			a.Kind, a.Ask = "migration", "Apply a schema change that changes a primary key?"
		}
		keys := make([]string, len(rep.KeysChanged))
		for i, t := range rep.KeysChanged {
			keys[i] = fmt.Sprintf("%s (%s)", t, count(int(rep.LostRows[t]), "row", "rows"))
		}
		risk = append(risk, "It changes the primary key of "+strings.Join(keys, ", ")+". If two rows get the same key, it fails and nothing changes.")
	}
	if len(risk) > 0 {
		a.Risk = strings.Join(risk, " ")
	}
	if rep.Estimate.Long {
		a.Risk += " It takes about " + aboutTime(rep.Estimate.Time) + ", and project writes wait until it ends."
	}
	a.Details = sc.details(rep)
	return a
}

// lostText is what a destructive step drops, with the rows its table
// held then, as the try planned it.
func lostText(l string, rows map[string]int64) string {
	table, col, ok := strings.Cut(l, ".")
	n := count(int(rows[table]), "row", "rows")
	if ok {
		return fmt.Sprintf("the column %s.%s (%s)", table, col, n)
	}
	return fmt.Sprintf("the table %s (%s)", table, n)
}

// details are the card's Details: the steps, the dependents of the tables
// they touch and the estimated time.
func (sc *schemaChange) details(rep store.MigrationReport) string {
	var b strings.Builder
	b.WriteString("Steps:\n" + stepList(rep, false))
	tables := touched(rep)
	var deps []string
	for _, d := range sc.deps {
		if slices.ContainsFunc(d.Uses, func(u string) bool {
			t, _, _ := strings.Cut(strings.ToLower(u), ".")
			return slices.Contains(tables, t)
		}) {
			deps = append(deps, depName(d))
		}
	}
	if len(deps) > 0 {
		b.WriteString("\nDependents: " + strings.Join(deps, ", ") + "\n")
	}
	if rep.Estimate.Rows > 0 {
		fmt.Fprintf(&b, "\nEstimated time: about %s, for %s rewritten or scanned\n", aboutTime(rep.Estimate.Time), count(int(rep.Estimate.Rows), "row", "rows"))
	}
	return strings.TrimRight(b.String(), "\n")
}

// touched are the tables a migration's steps name, in lower case.
func touched(rep store.MigrationReport) []string {
	var out []string
	for _, s := range rep.Steps {
		if t := strings.ToLower(s.Table); t != "" && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

// stepList numbers a migration's steps, with their SQL when sql is set.
func stepList(rep store.MigrationReport, sql bool) string {
	var b strings.Builder
	for i, s := range rep.Steps {
		fmt.Fprintf(&b, "%d. %s", i+1, s.Op)
		if s.Table != "" {
			b.WriteString(" " + s.Table)
		}
		if s.Destructive {
			b.WriteString(" (deletes data)")
		}
		if s.Rows > 0 {
			b.WriteString(", " + count(int(s.Rows), "row", "rows") + " written")
		}
		b.WriteString("\n")
		if sql {
			for _, q := range s.SQL {
				b.WriteString("   " + strings.ReplaceAll(q, "\n", "\n   ") + "\n")
			}
		}
		for _, n := range s.Notes {
			b.WriteString("   " + n + "\n")
		}
	}
	return b.String()
}

// applied is the main agent's result for an applied migration, and the
// chip with its steps (8.2). The tables are as they are now, in short, so
// the main agent can use them at once; the plan comes last, as it is the
// part the main agent already knows.
func (sc *schemaChange) applied(ref string) tool.Result {
	rep := sc.report
	var b strings.Builder
	b.WriteString(header(schemaApplied, ref) + "\n")
	fmt.Fprintf(&b, "Migration %s applied; the schema version is %d.\nSteps:\n%s", rep.Change, rep.SchemaVersion, stepList(rep, false))
	if len(rep.Updated) > 0 {
		b.WriteString("Dependents updated: " + strings.Join(rep.Updated, ", ") + "\n")
	}
	if rep.Snapshot != "" {
		b.WriteString("A snapshot was taken first: " + rep.Snapshot + "\n")
	}
	if sc.note != "" {
		b.WriteString(sc.note + "\n")
	}
	if len(sc.tables) > 0 {
		b.WriteString("Tables now:\n")
		for _, t := range sc.tables {
			b.WriteString("- " + compact(t) + "\n")
		}
	}
	chip := chat.Notice{Kind: chat.NoticeMigration, Text: fmt.Sprintf("Schema changed: %s · version %d", summary(rep), rep.SchemaVersion),
		Details: strings.TrimRight(stepList(rep, true), "\n")}
	text := sc.withPlan(strings.TrimRight(b.String(), "\n"), "Plan: ")
	return tool.Result{Text: sc.fit(text, ref), Ref: ref, Notices: []chat.Notice{chip}}
}

// compact is a table on one line: its rows, columns and keys.
// describe_table and the transcript have the rest, such as indexes.
func compact(t store.Table) string {
	cols := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		s := c.Name
		if c.Type != "" {
			s += " " + c.Type
		}
		if c.NotNull {
			s += " NOT NULL"
		}
		if c.Kind != "" {
			s += " " + c.Kind
		}
		cols[i] = s
	}
	s := fmt.Sprintf("%s (%s): %s", t.Name, count(t.Rows, "row", "rows"), strings.Join(cols, ", "))
	if len(t.Key) > 0 {
		s += "; key (" + strings.Join(t.Key, ", ") + ")"
	}
	for _, fk := range t.ForeignKeys {
		s += "; (" + strings.Join(fk.Columns, ", ") + ") references " + fk.Table
		if len(fk.To) > 0 {
			s += " (" + strings.Join(fk.To, ", ") + ")"
		}
	}
	return s
}

// summary names a migration's tables for its chip.
func summary(rep store.MigrationReport) string {
	ts := touched(rep)
	switch {
	case len(ts) == 0:
		return count(len(rep.Steps), "step", "steps")
	case len(ts) > 3:
		return strings.Join(ts[:3], ", ") + fmt.Sprintf(" and %d more", len(ts)-3)
	}
	return strings.Join(ts, ", ")
}

func count(n int, one, many string) string {
	s := fmt.Sprint(n)
	if n >= 10_000 {
		var b []byte
		for i, c := range []byte(s) {
			if i > 0 && (len(s)-i)%3 == 0 {
				b = append(b, ',')
			}
			b = append(b, c)
		}
		s = string(b)
	}
	if n == 1 {
		return s + " " + one
	}
	return s + " " + many
}

// aboutTime is an estimated time as the card shows it.
func aboutTime(d time.Duration) string {
	switch {
	case d < time.Second:
		return "a second"
	case d < 90*time.Second:
		return fmt.Sprintf("%d seconds", int(d.Round(time.Second)/time.Second))
	}
	return fmt.Sprintf("%d minutes", int(d.Round(time.Minute)/time.Minute))
}
