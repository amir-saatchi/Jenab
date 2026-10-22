package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"unicode/utf8"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/skill"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// schemaHarness is a harness with request_schema_change and the built-in
// skills, which the schema agent loads.
func schemaHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, t.TempDir(), "", testSettings(), Tools()...)
	set, err := skill.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	h.o.d.Skills = set
	return h
}

// pricesTable makes prices(date, close, volume) with two rows.
func (h *harness) pricesTable() {
	h.t.Helper()
	h.open(func(p *project.Project) {
		_, err := p.DB.Migrate(context.Background(), id.SourceUser, store.Migration{Steps: []json.RawMessage{
			json.RawMessage(`{"op": "create_table", "table": "prices", "columns": [{"name": "date", "type": "TEXT"}, {"name": "close", "type": "REAL"}, {"name": "volume", "type": "REAL"}], "primary_key": ["date"]}`),
			json.RawMessage(`{"op": "insert_rows", "table": "prices", "rows": [{"date": "2026-01-01", "close": 1, "volume": 10}, {"date": "2026-01-02", "close": 2, "volume": 20}]}`),
		}}, store.MigrateOptions{})
		if err != nil {
			h.t.Fatal(err)
		}
	})
}

// columns lists a table's columns, or nil when it isn't there.
func (h *harness) columns(table string) []string {
	h.t.Helper()
	var out []string
	h.open(func(p *project.Project) {
		t, err := p.DB.DescribeTable(context.Background(), table)
		if err != nil {
			return
		}
		for _, c := range t.Columns {
			out = append(out, c.Name)
		}
	})
	return out
}

// transcripts are the schema agent's transcripts of chat c.
func (h *harness) transcripts(c id.Chat) []store.Object {
	h.t.Helper()
	var out []store.Object
	h.open(func(p *project.Project) {
		l, err := p.DB.ListObjects(context.Background(), "subagents/"+string(c)+"/", "", 100)
		if err != nil {
			h.t.Fatal(err)
		}
		out = l.Objects
	})
	return out
}

// The schema agent's requests and the main agent's after a result are
// large, so their replies report enough input tokens for the truncation
// check.
func schemaGoal(goal string) fake.Reply {
	return fake.ToolCall("c1", "request_schema_change", map[string]string{"goal": goal})
}

func applyCall(callID, plan string, steps ...string) fake.Reply {
	raw := make([]json.RawMessage, len(steps))
	for i, s := range steps {
		raw[i] = json.RawMessage(s)
	}
	return read(fake.ToolCall(callID, "apply_migration", map[string]any{"plan": plan, "steps": raw}), 5000)
}

const createPrices = `{"op": "create_table", "table": "prices", "columns": [{"name": "date", "type": "TEXT"}, {"name": "close", "type": "REAL"}], "primary_key": ["date"]}`

// schemaResult is the result of the request_schema_change call.
func schemaResult(t *testing.T, ms []chat.Message) chat.ToolResult {
	t.Helper()
	for _, m := range ms {
		for _, p := range m.Parts {
			if p.ToolResult != nil && p.ToolResult.CallID == "c1" {
				return *p.ToolResult
			}
		}
	}
	t.Fatal("no result for c1")
	return chat.ToolResult{}
}

// notices are the chat's notices of kind k.
func notices(ms []chat.Message, k chat.NoticeKind) []chat.Notice {
	var out []chat.Notice
	for _, m := range ms {
		for _, p := range m.Parts {
			if p.Notice != nil && p.Notice.Kind == k {
				out = append(out, *p.Notice)
			}
		}
	}
	return out
}

func toolNames(r provider.Request) []string {
	var out []string
	for _, d := range r.Tools {
		out = append(out, d.Name)
	}
	return out
}

// At Standard, a migration that deletes nothing applies without a card.
// The schema agent runs in memory with its skills and its own tools, its
// writes carry the parent call's source, the transcript goes to the
// bucket as the chat's, and the chat gets a short result and the chip.
func TestSchemaAgentApplies(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := schemaHarness(t)
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(schemaGoal("a prices table with date and close"),
			applyCall("s1", "Add a prices table with one close price a day.", createPrices),
			read(fake.Text("done"), 5000))
		h.turn(c.ID, "make a prices table")

		if got := h.columns("prices"); !slices.Equal(got, []string{"date", "close"}) {
			t.Errorf("columns %v", got)
		}
		calls := h.fp.Calls()
		if len(calls) != 3 {
			t.Fatalf("%d requests, want 3", len(calls))
		}
		sub := calls[1]
		sys := sub.System[0].Text
		if !strings.HasPrefix(sys, "You are the schema agent") || !strings.Contains(sys, "Skill loaded: database-design") || !strings.Contains(sys, "Skill loaded: migrations") {
			t.Errorf("schema agent's system prompt %q", sys)
		}
		if got := toolNames(sub); !slices.Equal(got, []string{"apply_migration", "query", "describe_table"}) {
			t.Errorf("schema agent's tools %v", got)
		}
		if got := texts(sub.Messages); len(got) != 1 || !strings.HasPrefix(got[0], "user: Goal: a prices table with date and close\n\n## Tables\n\nThere are no tables yet.") ||
			!strings.Contains(got[0], "## Dependents\n\nNo saved views") {
			t.Errorf("schema agent's task %q", got)
		}
		if !slices.Contains(toolNames(calls[0]), "request_schema_change") || slices.Contains(toolNames(calls[0]), "apply_migration") {
			t.Errorf("main tools %v", toolNames(calls[0]))
		}

		ms := h.messages(c.ID)
		validHistory(t, ms)
		res := schemaResult(t, ms)
		if res.IsError || !strings.HasPrefix(res.Text, "[request_schema_change: applied — ref: subagents/"+string(c.ID)+"/") || res.Ref == "" ||
			!strings.Contains(res.Text, "the schema version is ") || !strings.Contains(res.Text, "1. create_table prices") ||
			!strings.Contains(res.Text, "\nTables now:\n- prices (0 rows): date TEXT NOT NULL, close REAL; key (date)\nPlan: ") {
			t.Errorf("result %+v", res)
		}
		if got := tool.Stub(res); got != "[request_schema_change: applied — ref: "+res.Ref+"]" {
			t.Errorf("stub %q", got)
		}
		chips := notices(ms, chat.NoticeMigration)
		if len(chips) != 1 || !strings.HasPrefix(chips[0].Text, "Schema changed: prices · version ") || !strings.Contains(chips[0].Details, "CREATE TABLE") {
			t.Errorf("chips %+v", chips)
		}
		if cs := cards(ms); len(cs) != 0 {
			t.Errorf("cards %+v", cs)
		}
		if len(notices(ms, chat.NoticeSkillLoaded)) != 1 {
			t.Errorf("migrations wasn't loaded with the tool: %q", texts(ms))
		}

		// Nothing of the schema agent's run is in the chat; its
		// transcript is in the bucket, written as the chat's, with the
		// SQL and the tables in full.
		var src id.Source
		for _, m := range ms {
			if m.Role == chat.RoleAssistant && slices.ContainsFunc(m.Parts, func(p chat.Part) bool { return p.ToolCall != nil && p.ToolCall.ID == "c1" }) {
				src = id.SourceOf("message", string(m.ID))
			}
			if slices.ContainsFunc(m.Parts, func(p chat.Part) bool { return p.ToolCall != nil && p.ToolCall.Name == "apply_migration" }) {
				t.Error("the schema agent's call is in the chat")
			}
		}
		trs := h.transcripts(c.ID)
		if len(trs) != 1 || trs[0].Key != res.Ref || trs[0].CreatedBy != id.SourceOf("chat", string(c.ID)) || trs[0].Source != "agent" {
			t.Fatalf("transcripts %+v", trs)
		}
		h.open(func(p *project.Project) {
			_, r, err := p.DB.OpenObject(context.Background(), res.Ref)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			b, _ := io.ReadAll(r)
			var tr transcript
			if err := json.Unmarshal(b, &tr); err != nil {
				t.Fatal(err)
			}
			if tr.Name != schemaAgent || tr.Parent != src || tr.Requests != 1 || len(tr.Messages) != 3 || tr.Error != "" {
				t.Fatalf("transcript %+v", tr)
			}
			if got := texts(tr.Messages[2:]); len(got) != 1 || !strings.Contains(got[0], "CREATE TABLE") || !strings.Contains(got[0], "Table prices: 0 rows\nColumns:\n- date TEXT NOT NULL") {
				t.Errorf("the transcript's applied result %q", got)
			}
			got, err := store.Query(context.Background(), p.DB.DB, `SELECT source FROM _jenab_changes WHERE op = 'migrate'`, nil,
				func(r *sql.Rows) (id.Source, error) {
					var s id.Source
					return s, r.Scan(&s)
				})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, []id.Source{src}) {
				t.Errorf("the migrations' sources are %v, want %s", got, src)
			}
		})
	})
}

// A migration that fails its tries goes back to the schema agent with its
// problems; after the first try and 4 more, the change is rejected with
// the last problems, and nothing changed.
func TestSchemaAgentRejected(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := schemaHarness(t)
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		bad := `{"op": "create_table", "table": "prices", "columns": [{"name": "date", "type": "DATE"}], "primary_key": ["date"]}`
		h.fp.Push(schemaGoal("a prices table"))
		for i := range schemaTries {
			h.fp.Push(applyCall("s"+string(rune('1'+i)), "Add prices.", bad))
		}
		h.fp.Push(read(fake.Text("it didn't work"), 5000))
		h.turn(c.ID, "make a prices table")

		calls := h.fp.Calls()
		if len(calls) != 2+schemaTries {
			t.Fatalf("%d requests, want %d", len(calls), 2+schemaTries)
		}
		second := texts(calls[2].Messages)
		if got := second[len(second)-1]; !strings.Contains(got, "4 tries left") || !strings.Contains(got, `steps[0].columns[0].type: unknown type "DATE"`) {
			t.Errorf("the first failed try %q", got)
		}
		res := schemaResult(t, h.messages(c.ID))
		if res.IsError || !strings.HasPrefix(res.Text, "[request_schema_change: rejected — ref: ") ||
			!strings.Contains(res.Text, "Reason: the migration failed 5 tries") || !strings.Contains(res.Text, `unknown type "DATE"`) {
			t.Errorf("result %+v", res)
		}
		if got := h.columns("prices"); got != nil {
			t.Errorf("prices has %v", got)
		}
		if len(h.transcripts(c.ID)) != 1 {
			t.Error("no transcript")
		}
	})
}

// A schema agent that answers instead of migrating rejects the change
// with its answer as the reason.
func TestSchemaAgentAnswers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := schemaHarness(t)
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(schemaGoal("something"), read(fake.Text("The goal doesn't say which table."), 5000), read(fake.Text("ok"), 5000))
		h.turn(c.ID, "change it")
		res := schemaResult(t, h.messages(c.ID))
		if !strings.HasSuffix(res.Text, "Nothing changed. Reason: The goal doesn't say which table.") {
			t.Errorf("result %q", res.Text)
		}
	})
}

// A migration that deletes data asks at every level, with what it deletes
// and the row counts; Apply takes a snapshot first and applies it.
func TestSchemaAgentDestructiveApproved(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := schemaHarness(t)
		defer h.stop()
		h.setLevel(project.Auto)
		h.pricesTable()
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(schemaGoal("drop the volume column of prices"),
			applyCall("s1", "Drop the volume column, which isn't used.", `{"op": "drop_column", "table": "prices", "column": "volume"}`),
			read(fake.Text("done"), 5000))
		h.send(c.ID, "we don't need volume")
		w := h.waiting(c.ID)
		if w.Kind != chat.PartApproval || w.Text != "Apply a schema change that deletes data?" {
			t.Errorf("waiting %+v", w)
		}
		if got := h.columns("prices"); !slices.Contains(got, "volume") {
			t.Fatal("applied before the answer")
		}
		h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantOnce})
		h.wait()

		if got := h.columns("prices"); !slices.Equal(got, []string{"date", "close"}) {
			t.Errorf("columns %v", got)
		}
		ms := h.messages(c.ID)
		validHistory(t, ms)
		cs := cards(ms)
		if len(cs) != 1 {
			t.Fatalf("cards %+v", cs)
		}
		card := cs[0]
		if card.Kind != "migration" || card.Why != "Drop the volume column, which isn't used." ||
			card.Risk != "It deletes the column prices.volume (2 rows). A snapshot of the project is taken first." ||
			!strings.Contains(card.Details, "1. drop_column prices (deletes data)") ||
			card.Answer != chat.GrantOnce || card.By != id.SourceUser {
			t.Errorf("card %+v", card)
		}
		if _, ok := card.Option(chat.GrantAlways); ok {
			t.Error("a migration card can't be approved for the project")
		}
		res := schemaResult(t, ms)
		if !strings.HasPrefix(res.Text, "[request_schema_change: applied") || !strings.Contains(res.Text, "A snapshot was taken first: ") {
			t.Errorf("result %q", res.Text)
		}
		if len(notices(ms, chat.NoticeMigration)) != 1 {
			t.Error("no chip")
		}
	})
}

// Deny ends the schema agent without another request; the main agent
// gets the denial with the note and the plan, and nothing changed.
func TestSchemaAgentDestructiveDenied(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := schemaHarness(t)
		defer h.stop()
		h.pricesTable()
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(schemaGoal("drop prices"),
			applyCall("s1", "Drop the prices table.", `{"op": "drop_table", "table": "prices"}`),
			read(fake.Text("ok, kept"), 5000))
		h.send(c.ID, "drop prices")
		w := h.waiting(c.ID)
		h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantDeny, Note: "keep it"})
		h.wait()

		if got := h.columns("prices"); len(got) != 3 {
			t.Errorf("columns %v", got)
		}
		if n := len(h.fp.Calls()); n != 3 {
			t.Errorf("%d requests, want 3", n)
		}
		ms := h.messages(c.ID)
		if cs := cards(ms); len(cs) != 1 || cs[0].Risk != "It deletes the table prices (2 rows). A snapshot of the project is taken first." || cs[0].Answer != chat.GrantDeny {
			t.Errorf("cards %+v", cs)
		}
		res := schemaResult(t, ms)
		want := "Nothing changed: the user denied it (Apply a schema change that deletes data?); their note: keep it.\nThe plan was: Drop the prices table."
		if res.IsError || !strings.HasPrefix(res.Text, "[request_schema_change: denied — ref: ") || !strings.HasSuffix(res.Text, want) {
			t.Errorf("result %q", res.Text)
		}
		if len(notices(ms, chat.NoticeMigration)) != 0 {
			t.Error("a chip for a denied change")
		}
	})
}

// At Strict, a migration that deletes nothing asks too.
func TestSchemaAgentStrict(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := schemaHarness(t)
		defer h.stop()
		h.setLevel(project.Strict)
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(schemaGoal("a prices table"), applyCall("s1", "Add a prices table.", createPrices), read(fake.Text("done"), 5000))
		h.send(c.ID, "make a prices table")
		w := h.waiting(c.ID)
		if w.Text != "Apply this schema change?" {
			t.Errorf("waiting %+v", w)
		}
		h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantOnce})
		h.wait()
		ms := h.messages(c.ID)
		if cs := cards(ms); len(cs) != 1 || cs[0].Kind != "schema" || cs[0].Risk != "Nothing is deleted." || !strings.Contains(cs[0].Details, "1. create_table prices") {
			t.Errorf("cards %+v", cs)
		}
		if got := h.columns("prices"); len(got) != 2 {
			t.Errorf("columns %v", got)
		}
		if as := h.approvals(); len(as) != 1 || as[0].Kind != "schema" || as[0].Answer != chat.GrantOnce {
			t.Errorf("records %+v", as)
		}
	})
}

// Stop while the card waits closes it as stopped; nothing is applied and
// no transcript is stored.
func TestSchemaAgentStopDuringCard(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := schemaHarness(t)
		defer h.stop()
		h.pricesTable()
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(schemaGoal("drop prices"), applyCall("s1", "Drop the prices table.", `{"op": "drop_table", "table": "prices"}`))
		h.send(c.ID, "drop prices")
		h.waiting(c.ID)
		h.o.Stop(h.pid, c.ID)
		h.wait()

		ms := h.messages(c.ID)
		validHistory(t, ms)
		if cs := cards(ms); len(cs) != 1 || !cs[0].Stopped || cs[0].Answer != "" {
			t.Errorf("cards %+v", cs)
		}
		if res := schemaResult(t, ms); res.Text != "cancelled by user" {
			t.Errorf("result %q", res.Text)
		}
		if got := h.columns("prices"); len(got) != 3 {
			t.Errorf("columns %v", got)
		}
		if trs := h.transcripts(c.ID); len(trs) != 0 {
			t.Errorf("transcripts %+v", trs)
		}
		if n := len(h.fp.Calls()); n != 2 {
			t.Errorf("%d requests, want 2", n)
		}
	})
}

// The card approves what the try drops, and no more: a column another
// migration adds while the card waits isn't dropped with the rest. The
// schema agent gets why, and nothing changed.
func TestSchemaAgentPlannedLoss(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := schemaHarness(t)
		defer h.stop()
		h.pricesTable()
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(schemaGoal("keep only date and close in prices"),
			applyCall("s1", "Rebuild prices with date and close.", `{"op": "rebuild_table", "table": "prices", "columns": [{"name": "date", "type": "TEXT"}, {"name": "close", "type": "REAL"}], "primary_key": ["date"]}`),
			read(fake.Text("The table changed meanwhile, so I stopped."), 5000), read(fake.Text("ok"), 5000))
		h.send(c.ID, "drop volume")
		w := h.waiting(c.ID)
		h.open(func(p *project.Project) {
			_, err := p.DB.Migrate(context.Background(), id.SourceUser, store.Migration{Steps: []json.RawMessage{
				json.RawMessage(`{"op": "add_column", "table": "prices", "column": {"name": "note", "type": "TEXT"}, "value": "NULL"}`)}}, store.MigrateOptions{})
			if err != nil {
				t.Fatal(err)
			}
		})
		h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantOnce})
		h.wait()

		if got := h.columns("prices"); !slices.Equal(got, []string{"date", "close", "volume", "note"}) {
			t.Errorf("columns %v", got)
		}
		calls := h.fp.Calls()
		got := texts(calls[2].Messages)
		if last := got[len(got)-1]; !strings.Contains(last, "and now it drops or changes prices.note too") || !strings.Contains(last, "4 tries left") {
			t.Errorf("the schema agent got %q", last)
		}
		res := schemaResult(t, h.messages(c.ID))
		if !strings.HasPrefix(res.Text, "[request_schema_change: rejected") || !strings.Contains(res.Text, "prices.note") {
			t.Errorf("result %q", res.Text)
		}
	})
}

// A rebuild that changes the primary key keeps every column, but asks at
// every level (8.2), with the rows as the try counts them under the new
// name of a renamed table.
func TestSchemaAgentKeyChange(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := schemaHarness(t)
		defer h.stop()
		h.setLevel(project.Auto)
		h.pricesTable()
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(schemaGoal("rename prices to quotes, keyed by date and volume"),
			applyCall("s1", "Rename prices to quotes, with date and volume as the key.",
				`{"op": "rename_table", "from": "prices", "to": "quotes"}`,
				`{"op": "rebuild_table", "table": "quotes", "columns": [{"name": "date", "type": "TEXT"}, {"name": "close", "type": "REAL"}, {"name": "volume", "type": "REAL"}], "primary_key": ["date", "volume"]}`),
			read(fake.Text("done"), 5000))
		h.send(c.ID, "key quotes by date and volume")
		w := h.waiting(c.ID)
		if w.Text != "Apply a schema change that changes a primary key?" {
			t.Errorf("waiting %+v", w)
		}
		h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantOnce})
		h.wait()
		ms := h.messages(c.ID)
		cs := cards(ms)
		if len(cs) != 1 || cs[0].Kind != "migration" ||
			cs[0].Risk != "It changes the primary key of quotes (2 rows). If two rows get the same key, it fails and nothing changes." {
			t.Fatalf("cards %+v", cs)
		}
		res := schemaResult(t, ms)
		if !strings.Contains(res.Text, "- quotes (2 rows): date TEXT NOT NULL, close REAL, volume REAL NOT NULL; key (date, volume)") {
			t.Errorf("result %q", res.Text)
		}

		// Renamed, then a column dropped: the card counts the rows of
		// the table it drops from, by its new name.
		h.fp.Push(schemaGoal("rename quotes to prices and drop volume"),
			applyCall("s2", "Rename quotes back to prices without volume.",
				`{"op": "rename_table", "from": "quotes", "to": "prices"}`,
				`{"op": "rebuild_table", "table": "prices", "columns": [{"name": "date", "type": "TEXT"}, {"name": "close", "type": "REAL"}], "primary_key": ["date"]}`),
			read(fake.Text("done"), 5000))
		c2 := h.newChat(chat.Chat{Title: "Quotes"})
		h.send(c2.ID, "back to prices")
		w = h.waiting(c2.ID)
		h.answer(c2.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantDeny})
		h.wait()
		cs = cards(h.messages(c2.ID))
		if len(cs) != 1 || cs[0].Risk != "It deletes the column prices.volume (2 rows). A snapshot of the project is taken first. "+
			"It changes the primary key of prices (2 rows). If two rows get the same key, it fails and nothing changes." {
			t.Errorf("cards %+v", cs)
		}
	})
}

// A widened key keeps the rows apart, so its card doesn't warn of equal
// keys, and the steps show the new name.
func TestSchemaAgentWidenCard(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := schemaHarness(t)
		defer h.stop()
		h.setLevel(project.Auto)
		h.pricesTable()
		c := h.newChat(chat.Chat{Title: "Coins"})
		h.fp.Push(schemaGoal("track every coin"),
			applyCall("s1", "Widen prices for every coin.",
				`{"op": "widen_table", "table": "prices", "column": {"name": "coin", "type": "TEXT"}, "value": "'BTC'", "to": "coin_prices"}`),
			read(fake.Text("done"), 5000))
		h.send(c.ID, "track every coin")
		w := h.waiting(c.ID)
		h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantOnce})
		h.wait()
		cs := cards(h.messages(c.ID))
		if len(cs) != 1 || cs[0].Kind != "migration" ||
			cs[0].Risk != "It puts a new column in front of the primary key of prices (2 rows); the rows keep their data." ||
			!strings.Contains(cs[0].Details, "1. widen_table prices → coin_prices, 2 rows written") {
			t.Fatalf("cards %+v", cs)
		}
	})
}

// A table that a widen and another step both rekey is listed once, with
// the warning about equal keys.
func TestSchemaCardMixedKeys(t *testing.T) {
	sc := &schemaChange{plan: "Rekey and widen prices."}
	for _, tc := range []struct {
		rep  store.MigrationReport
		risk string
	}{
		{store.MigrationReport{KeysChanged: []string{"prices", "prices"}, LostRows: map[string]int64{"prices": 2}},
			"It changes the primary key of prices (2 rows). If two rows get the same key, it fails and nothing changes."},
		{store.MigrationReport{KeysChanged: []string{"prices", "news"}, KeysWidened: []string{"news"}, LostRows: map[string]int64{"prices": 2, "news": 1}},
			"It puts a new column in front of the primary key of news (1 row); the rows keep their data. " +
				"It changes the primary key of prices (2 rows). If two rows get the same key, it fails and nothing changes."},
	} {
		if a := sc.card(nil, tc.rep); a.Risk != tc.risk {
			t.Errorf("risk %q, want %q", a.Risk, tc.risk)
		}
	}
}

// Undo turn leaves the transcript, which is the chat's record, and the
// migration, which is never undone (2.6); Clear removes the transcripts
// with the messages.
func TestSchemaAgentUndoKeepsTranscript(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := schemaHarness(t)
		defer h.stop()
		ctx := context.Background()
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(schemaGoal("a prices table"), applyCall("s1", "Add prices.", createPrices), read(fake.Text("done"), 5000))
		h.turn(c.ID, "make a prices table")
		if st, err := h.o.UndoState(ctx, h.pid, c.ID, 1); err != nil || st.Undo.Changes != 0 {
			t.Errorf("undo state %+v, %v", st, err)
		}
		h.o.UndoTurn(ctx, h.pid, c.ID, UndoRequest{Turn: 1})
		if trs := h.transcripts(c.ID); len(trs) != 1 {
			t.Errorf("transcripts after the undo %+v", trs)
		}
		if got := h.columns("prices"); len(got) != 2 {
			t.Errorf("columns %v", got)
		}
		if err := h.o.Clear(ctx, h.pid, c.ID); err != nil {
			t.Fatal(err)
		}
		if trs := h.transcripts(c.ID); len(trs) != 0 {
			t.Errorf("transcripts after Clear %+v", trs)
		}
	})
}

// A long try and a long migration show in the chat's status line while
// they run, and the line clears after.
func TestSchemaAgentProgress(t *testing.T) {
	old := store.LongRows
	store.LongRows = 1
	defer func() { store.LongRows = old }()
	synctest.Test(t, func(t *testing.T) {
		h := schemaHarness(t)
		defer h.stop()
		h.pricesTable()
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(schemaGoal("a note on every price"),
			applyCall("s1", "Add a note column.", `{"op": "add_column", "table": "prices", "column": {"name": "note", "type": "TEXT", "not_null": true, "default": "''"}}`),
			read(fake.Text("done"), 5000))
		h.turn(c.ID, "add a note")
		if got := h.columns("prices"); !slices.Contains(got, "note") {
			t.Fatalf("columns %v", got)
		}
		var doing []string
		for _, st := range h.pub.allStatuses() {
			if len(doing) == 0 || doing[len(doing)-1] != st.Doing {
				doing = append(doing, st.Doing)
			}
		}
		want := []string{"", "trying a schema change, about a second", "", "applying a schema change, about a second", ""}
		if !slices.Equal(doing, want) {
			t.Errorf("status lines %q, want %q", doing, want)
		}
	})
}

// A long result is cut to tool_preview_tokens at a line, with the
// transcript's ref for the rest.
func TestSchemaAgentResultFits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		set := testSettings()
		set.Context.ToolPreviewTokens = 100
		h := newHarness(t, t.TempDir(), "", set, Tools()...)
		skills, err := skill.Builtin()
		if err != nil {
			t.Fatal(err)
		}
		h.o.d.Skills = skills
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		var cols []string
		for i := range 40 {
			cols = append(cols, fmt.Sprintf(`{"name": "c%d", "type": "REAL"}`, i))
		}
		wide := `{"op": "create_table", "table": "wide", "columns": [{"name": "k", "type": "TEXT"}, ` + strings.Join(cols, ", ") + `], "primary_key": ["k"]}`
		h.fp.Push(schemaGoal("a wide table"), applyCall("s1", "Add a wide table.", wide), read(fake.Text("done"), 5000))
		h.turn(c.ID, "make a wide table")
		res := schemaResult(t, h.messages(c.ID))
		if len(res.Text) > 400 || !strings.HasPrefix(res.Text, "[request_schema_change: applied") ||
			!strings.HasSuffix(res.Text, "\n… cut to fit; the transcript has the rest: "+res.Ref) {
			t.Errorf("result %d bytes: %q", len(res.Text), res.Text)
		}
	})
}

// At the least tool_preview_tokens, a long plan is cut to the room left
// after the steps and the tables, which stay whole (P2-06).
func TestSchemaAgentLongPlanFits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		set := testSettings()
		set.Context.ToolPreviewTokens = 100 // the least config allows
		h := newHarness(t, t.TempDir(), "", set, Tools()...)
		skills, err := skill.Builtin()
		if err != nil {
			t.Fatal(err)
		}
		h.o.d.Skills = skills
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		plan := strings.Repeat("یک جدول برای قیمت‌ها. ", 30) // about 600 characters
		h.fp.Push(schemaGoal("a prices table"), applyCall("s1", plan, createPrices), read(fake.Text("done"), 5000))
		h.turn(c.ID, "make a prices table")
		res := schemaResult(t, h.messages(c.ID))
		if len(res.Text) > 400 || !strings.Contains(res.Text, "\nSteps:\n1. create_table prices") ||
			!strings.Contains(res.Text, "\nTables now:\n- prices (0 rows): date TEXT NOT NULL, close REAL; key (date)\nPlan: یک جدول") ||
			!strings.HasSuffix(res.Text, "…") || strings.Contains(res.Text, "cut to fit") || !utf8.ValidString(res.Text) {
			t.Errorf("result %d bytes: %q", len(res.Text), res.Text)
		}
	})
}

// An answer cut off by the output limit rejects the change and says so.
func TestSchemaAgentCutOff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := schemaHarness(t)
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		cut := fake.Reply{Events: []provider.Event{
			{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: "I will create"}}},
			fake.Done(provider.StopMaxTokens, chat.Usage{Input: 5000, Output: 100}),
		}}
		h.fp.Push(schemaGoal("a prices table"), cut, read(fake.Text("sorry"), 5000))
		h.turn(c.ID, "make a prices table")
		res := schemaResult(t, h.messages(c.ID))
		if !strings.HasSuffix(res.Text, "Reason: the schema agent's answer was cut off by the output limit before it applied a migration; it said: I will create") {
			t.Errorf("result %q", res.Text)
		}
	})
}

// An error of the app's, such as a missing skill, reaches the main agent
// with the header.
func TestSchemaAgentMissingSkill(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings(), Tools()...)
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(schemaGoal("a prices table"), read(fake.Text("sorry"), 5000))
		h.turn(c.ID, "make a prices table")
		res := schemaResult(t, h.messages(c.ID))
		if !res.IsError || !strings.HasPrefix(res.Text, "[request_schema_change: failed]\nNothing changed: ") || !strings.Contains(res.Text, "database-design") {
			t.Errorf("result %+v", res)
		}
	})
}

// A provider error the schema agent can't wait out ends the call with an
// error the main agent can read, with the kind.
func TestSchemaAgentProviderError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := schemaHarness(t)
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(schemaGoal("a prices table"), fake.Fail(&provider.Error{Kind: provider.BadRequest, Provider: "p", Status: 400, Message: "bad"}), read(fake.Text("sorry"), 5000))
		h.turn(c.ID, "make a prices table")
		res := schemaResult(t, h.messages(c.ID))
		if !res.IsError || !strings.HasPrefix(res.Text, "[request_schema_change: failed — ref: ") || !strings.Contains(res.Text, "the schema agent stopped on an error from p (request): bad") {
			t.Errorf("result %+v", res)
		}
	})
}
