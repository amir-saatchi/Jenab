package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"modernc.org/sqlite"

	"github.com/amir-saatchi/jenab/internal/disk"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/sqlguard"
)

// Migrations (SPEC 7.5, 8.2): the schema agent sends structured steps, Go
// checks them, chooses the SQL and runs them in one transaction with the
// dependents, the change-log entry and the schema version, or not at all.

var (
	// ErrNeedsApproval is a migration that drops data or changes a primary
	// key, which the user hasn't approved (SPEC 8.8). Nothing ran; the
	// report lists the steps and keys.
	ErrNeedsApproval = errors.New("store: the migration drops data or changes a primary key, which needs the user's approval")
	// ErrNoSpace is a long or destructive migration without enough free
	// disk space for it (SPEC 7.5). Nothing ran.
	ErrNoSpace = errors.New("store: not enough free disk space for the migration")
)

// Problem is one thing wrong with a migration, at its path in the request,
// e.g. steps[2].column.type, so the model can fix it.
type Problem struct {
	Path string `json:"path"`
	Msg  string `json:"msg"`
}

func (p Problem) String() string {
	if p.Path == "" {
		return p.Msg
	}
	return p.Path + ": " + p.Msg
}

// MigrationError is a migration that failed its checks, a step, a
// dependent or the schema guard. Nothing was changed. It matches
// ErrInvalid.
type MigrationError struct{ Problems []Problem }

func (e *MigrationError) Error() string {
	parts := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		parts[i] = p.String()
	}
	return "store: migration: " + strings.Join(parts, "; ")
}

func (e *MigrationError) Is(target error) bool { return target == ErrInvalid }

func problem(path, format string, a ...any) error {
	return &MigrationError{Problems: []Problem{{Path: path, Msg: fmt.Sprintf(format, a...)}}}
}

// Migration is the schema agent's request: steps, as JSON objects with an
// op (SPEC 8.2), and the dependent configs it updates (7.5).
type Migration struct {
	Steps   []json.RawMessage `json:"steps"`
	Updates []DependentUpdate `json:"updates,omitempty"`
}

// DependentUpdate is a full config sent with a migration, e.g. a view
// that now reads from the new table.
type DependentUpdate struct {
	Kind   string          `json:"kind"` // view, pipeline, form
	ID     string          `json:"id"`
	Config json.RawMessage `json:"config"`
}

// Dependent is one kind of config that depends on the schema: views,
// pipelines and forms, which Phases 3 and 4 add. A migration saves the
// updates it carries, then checks every config of each kind against the
// new schema, all inside its transaction (SPEC 7.5).
type Dependent interface {
	Kind() string
	// Update saves u inside the migration's transaction.
	Update(tx *sql.Tx, src id.Source, u DependentUpdate) error
	// Check validates every config of the kind against the schema as tx
	// has it. Each problem's Path names a config, e.g. view:prices.
	Check(tx *sql.Tx) ([]Problem, error)
}

// DependentInfo is one saved config that depends on the schema, with the
// columns it uses, for the schema agent and the approval card (SPEC 8.2).
type DependentInfo struct {
	Kind  string   `json:"kind"`
	ID    string   `json:"id"`
	Title string   `json:"title,omitempty"`
	Uses  []string `json:"uses"` // table.column, or a table alone
}

// DependentLister is optional for a Dependent: it lists the configs of
// its kind, as tx has them.
type DependentLister interface {
	List(ctx context.Context, tx *sql.Tx) ([]DependentInfo, error)
}

// Dependents lists the saved configs of every kind that can list them, by
// kind and ID, in one read.
func (p *ProjectDB) Dependents(ctx context.Context) ([]DependentInfo, error) {
	kinds := p.dependentKinds()
	if len(kinds) == 0 {
		return nil, nil
	}
	return readTx(ctx, p.DB, func(tx *sql.Tx) ([]DependentInfo, error) {
		var out []DependentInfo
		for _, k := range sortedKeys(kinds) {
			l, ok := kinds[k].(DependentLister)
			if !ok {
				continue
			}
			ds, err := l.List(ctx, tx)
			if err != nil {
				return nil, err
			}
			out = append(out, ds...)
		}
		slices.SortStableFunc(out, func(a, b DependentInfo) int {
			if c := strings.Compare(a.Kind, b.Kind); c != 0 {
				return c
			}
			return strings.Compare(a.ID, b.ID)
		})
		return out, nil
	})
}

// dependents are a project's Dependent kinds.
type dependents struct {
	mu sync.Mutex
	m  map[string]Dependent
}

// AddDependent adds a kind of config that migrations update and check.
func (p *ProjectDB) AddDependent(d Dependent) {
	p.deps.mu.Lock()
	defer p.deps.mu.Unlock()
	if p.deps.m == nil {
		p.deps.m = map[string]Dependent{}
	}
	p.deps.m[d.Kind()] = d
}

func (p *ProjectDB) dependentKinds() map[string]Dependent {
	p.deps.mu.Lock()
	defer p.deps.mu.Unlock()
	out := make(map[string]Dependent, len(p.deps.m))
	for k, d := range p.deps.m {
		out[k] = d
	}
	return out
}

// MigrateOptions adjust a migration.
type MigrateOptions struct {
	// Try runs it and rolls it back, for the report (SPEC 7.5). A long
	// try runs on a copy, so it doesn't hold the writer.
	Try bool
	// Approved: the user approved its destructive steps and key changes
	// (SPEC 8.2, 8.8). Without it, a migration with any is refused with
	// ErrNeedsApproval.
	Approved bool
	// Planned, when set, is the try the user approved on a card. It
	// approves what its Lost and KeysChanged list, and nothing more: a
	// migration that drops data or changes a key beyond them, e.g. after
	// another migration ran while the card waited, is refused with
	// ErrNeedsApproval.
	Planned *MigrationReport
	// Long, if set, is called before a long migration or try runs, with
	// its estimate, such as to show it in the chat's status (SPEC 7.5).
	Long func(Estimate)
}

// MigrationReport says what a migration did, or would do.
type MigrationReport struct {
	Change        id.Change        `json:"change,omitempty"` // its change-log entry; "" for a try
	SchemaVersion int              `json:"schema_version"`   // after it
	Tried         bool             `json:"tried,omitempty"`
	Steps         []StepReport     `json:"steps"`
	Destructive   []int            `json:"destructive,omitempty"`  // the steps that drop data
	Lost          []string         `json:"lost,omitempty"`         // what they drop: "table" or "table.column", in lower case
	KeysChanged   []string         `json:"keys_changed,omitempty"` // the tables whose primary key changes, in lower case
	LostRows      map[string]int64 `json:"lost_rows,omitempty"`    // the rows of those tables at the step, by lower-case name
	Estimate      Estimate         `json:"estimate"`
	Snapshot      string           `json:"snapshot,omitempty"` // taken before it, in snapshots/
	Updated       []string         `json:"updated,omitempty"`  // dependents saved, as kind:id
	Problems      []Problem        `json:"problems,omitempty"`
}

// StepReport is what one step did.
type StepReport struct {
	Op          string   `json:"op"`
	Table       string   `json:"table,omitempty"`
	Destructive bool     `json:"destructive,omitempty"`
	SQL         []string `json:"sql,omitempty"`   // the statements Go chose
	Rows        int64    `json:"rows,omitempty"`  // rows written
	Notes       []string `json:"notes,omitempty"` // e.g. an index dropped with its column
}

// Estimate is a migration's cost, from row counts (SPEC 7.5).
type Estimate struct {
	Rows  int64         `json:"rows"`  // rows rewritten or scanned
	Time  time.Duration `json:"time"`  // for the approval card
	Bytes int64         `json:"bytes"` // size of the tables rewritten
	Long  bool          `json:"long"`
}

// LongRows is where a migration becomes long (SPEC 7.5). Tests lower it.
var LongRows int64 = 500_000

// Time per row, from SPIKE-010's 1M-row runs, at their slow end.
const (
	rateRebuild    = 13 * time.Microsecond
	rateDropColumn = 1300 * time.Nanosecond
	rateIndex      = 1400 * time.Nanosecond
	rateCopy       = 6 * time.Microsecond
	rateScan       = 500 * time.Nanosecond
)

// Step ops (SPEC 8.2) and the fields each takes.
var opFields = map[string][]string{
	"create_table":    {"table", "columns", "primary_key", "unique", "foreign_keys"},
	"add_column":      {"table", "column"},
	"rename_table":    {"from", "to"},
	"rename_column":   {"table", "from", "to"},
	"create_index":    {"table", "columns", "name", "unique"},
	"drop_index":      {"name"},
	"annotate_column": {"table", "column", "kind"},
	"insert_rows":     {"table", "rows"},
	"copy_data":       {"into", "columns", "from"},
	"rebuild_table":   {"table", "columns", "primary_key", "unique", "foreign_keys", "mapping"},
	"drop_column":     {"table", "column"},
	"drop_table":      {"table"},
}

var opNames = []string{"create_table", "add_column", "rename_table", "rename_column", "create_index", "drop_index",
	"annotate_column", "insert_rows", "copy_data", "rebuild_table", "drop_column", "drop_table"}

// Limits of a step.
const (
	maxSeedRows = 50   // insert_rows (SPEC 8.2)
	maxName     = 64   // characters in a table, column or index name
	maxExpr     = 2000 // bytes in a check or mapping expression
	limitLength = 0    // SQLITE_LIMIT_LENGTH
)

// columnTypes are the types a migration declares, with SQLite's affinity
// rules (SQLite 3.1) as plain as they get.
var columnTypes = []string{"TEXT", "INTEGER", "REAL", "BLOB"}

// columnKinds are the annotations of annotate_column (SPEC 4.5); "" removes
// one.
var columnKinds = []string{"object_ref", ""}

var fkActions = map[string]string{"no action": "NO ACTION", "restrict": "RESTRICT", "set null": "SET NULL",
	"set default": "SET DEFAULT", "cascade": "CASCADE"}

type columnDef struct {
	name, typ string
	notNull   bool
	dflt      string // the default as SQL; "" for none
	check     string // an expression; "" for none
}

type foreignKeyDef struct {
	cols, to           []string
	table              string
	onDelete, onUpdate string // SQL; "" for the default
}

type tableDef struct {
	cols         []columnDef
	key          []string
	unique       [][]string
	fks          []foreignKeyDef
	withoutRowid bool // kept from the schema when Go rebuilds for drop_column
	strict       bool
}

func (d *tableDef) has(c string) bool {
	return slices.ContainsFunc(d.cols, func(x columnDef) bool { return strings.EqualFold(x.name, c) })
}

// step is a checked step.
type step struct {
	op, path string
	table    string // the table it works on: into for copy_data, from for rename_table
	def      *tableDef
	col      *columnDef // add_column
	from, to string     // renames
	column   string     // annotate_column, drop_column
	cols     []string   // create_index, copy_data
	name     string     // create_index, drop_index
	unique   bool       // create_index
	kind     string     // annotate_column
	rows     []map[string]any
	query    string            // copy_data
	mapping  map[string]string // rebuild_table: new column → expression over the old columns
	raw      json.RawMessage
	est      int64 // rows the planner expects a copy or rebuild to read, for its deadline
}

// ---- parsing: every problem, with its path ----

type parser struct{ probs []Problem }

func (p *parser) add(path, format string, a ...any) {
	p.probs = append(p.probs, Problem{Path: path, Msg: fmt.Sprintf(format, a...)})
}

// obj is a JSON object being read.
type obj struct {
	p    *parser
	path string
	m    map[string]json.RawMessage
}

func (p *parser) object(path string, raw json.RawMessage) (obj, bool) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		p.add(path, "must be an object")
		return obj{}, false
	}
	return obj{p, path, m}, true
}

func (o obj) at(key string) string { return o.path + "." + key }

// only reports the fields that are not in allowed.
func (o obj) only(allowed ...string) {
	var extra []string
	for k := range o.m {
		if !slices.Contains(allowed, k) {
			extra = append(extra, k)
		}
	}
	slices.Sort(extra)
	for _, k := range extra {
		o.p.add(o.at(k), "unknown field (this object takes %s)", strings.Join(allowed, ", "))
	}
}

func (o obj) has(key string) bool {
	v, ok := o.m[key]
	return ok && string(v) != "null"
}

func (o obj) str(key string, required bool) string {
	if !o.has(key) {
		if required {
			o.p.add(o.at(key), "is required")
		}
		return ""
	}
	var s string
	if err := json.Unmarshal(o.m[key], &s); err != nil {
		o.p.add(o.at(key), "must be a string")
		return ""
	}
	if required && strings.TrimSpace(s) == "" {
		o.p.add(o.at(key), "must not be empty")
	}
	return s
}

func (o obj) flag(key string) bool {
	if !o.has(key) {
		return false
	}
	var b bool
	if err := json.Unmarshal(o.m[key], &b); err != nil {
		o.p.add(o.at(key), "must be true or false")
	}
	return b
}

// names reads a list of names; required means at least one.
func (o obj) names(key string, required bool) []string {
	if !o.has(key) {
		if required {
			o.p.add(o.at(key), "is required")
		}
		return nil
	}
	return o.p.nameList(o.at(key), o.m[key], required)
}

func (p *parser) nameList(path string, raw json.RawMessage, required bool) []string {
	var l []string
	if err := json.Unmarshal(raw, &l); err != nil {
		p.add(path, "must be a list of names")
		return nil
	}
	if required && len(l) == 0 {
		p.add(path, "must name at least one column")
	}
	seen := map[string]bool{}
	for i, n := range l {
		k := strings.ToLower(n)
		if seen[k] {
			p.add(fmt.Sprintf("%s[%d]", path, i), "%q is listed twice", n)
		}
		seen[k] = true
	}
	return l
}

func (o obj) list(key string, required bool) []json.RawMessage {
	if !o.has(key) {
		if required {
			o.p.add(o.at(key), "is required")
		}
		return nil
	}
	var l []json.RawMessage
	if err := json.Unmarshal(o.m[key], &l); err != nil {
		o.p.add(o.at(key), "must be a list")
		return nil
	}
	if required && len(l) == 0 {
		o.p.add(o.at(key), "must not be empty")
	}
	return l
}

// checkName reports a name a migration can't give a table, column or
// index: agents write queries with them, so they are plain words.
func (p *parser) checkName(path, n string) {
	switch {
	case n == "":
		return // reported as required
	case utf8.RuneCountInString(n) > maxName:
		p.add(path, "%q is longer than %d characters", n, maxName)
	case reservedName(n):
		p.add(path, "%q is reserved: names can't start with _jenab_, sqlite_ or pragma_", n)
	default:
		for i, r := range n {
			if r != '_' && !unicode.IsLetter(r) && (i == 0 || !unicode.IsDigit(r)) {
				p.add(path, "%q is not a plain name: use letters, digits and _, starting with a letter or _", n)
				return
			}
		}
	}
}

func reservedName(n string) bool {
	return internalName(n) || strings.HasPrefix(strings.ToLower(n), "pragma_")
}

// parseSteps checks the shape of every step and returns all problems.
func parseSteps(raws []json.RawMessage) ([]*step, []Problem) {
	p := &parser{}
	if len(raws) == 0 {
		p.add("steps", "a migration needs at least one step")
	}
	var out []*step
	for i, raw := range raws {
		path := fmt.Sprintf("steps[%d]", i)
		o, ok := p.object(path, raw)
		if !ok {
			continue
		}
		s := &step{path: path, raw: raw}
		s.op = o.str("op", true)
		fields, known := opFields[s.op]
		if !known {
			if s.op != "" {
				p.add(o.at("op"), "unknown op %q (use %s)", s.op, strings.Join(opNames, ", "))
			}
			continue
		}
		o.only(append([]string{"op"}, fields...)...)
		switch s.op {
		case "create_table", "rebuild_table":
			s.table = o.str("table", true)
			if s.op == "create_table" {
				p.checkName(o.at("table"), s.table)
			}
			s.def = p.tableDef(o)
			if s.op == "rebuild_table" && o.has("mapping") {
				var raw map[string]string
				if err := json.Unmarshal(o.m["mapping"], &raw); err != nil {
					p.add(o.at("mapping"), "must be an object of column names and SQL expressions, like {\"coin\": \"'BTC'\"}")
				}
				// Keyed by the new table's names, as SQLite compares them.
				s.mapping = map[string]string{}
				for _, c := range sortedKeys(raw) {
					at := o.at("mapping") + "." + c
					p.checkExpr(at, raw[c])
					i := slices.IndexFunc(s.def.cols, func(x columnDef) bool { return strings.EqualFold(x.name, c) })
					switch {
					case i < 0:
						p.add(at, "the new table has no column %q", c)
					case s.mapping[s.def.cols[i].name] != "":
						p.add(at, "the column %q has a mapping already", s.def.cols[i].name)
					default:
						s.mapping[s.def.cols[i].name] = raw[c]
					}
				}
			}
		case "add_column":
			s.table = o.str("table", true)
			if o.has("column") {
				s.col = p.columnDef(o.at("column"), o.m["column"])
			} else {
				p.add(o.at("column"), "is required")
			}
		case "rename_table":
			s.from, s.to = o.str("from", true), o.str("to", true)
			s.table = s.from
			p.checkName(o.at("to"), s.to)
		case "rename_column":
			s.table, s.from, s.to = o.str("table", true), o.str("from", true), o.str("to", true)
			p.checkName(o.at("to"), s.to)
		case "create_index":
			s.table = o.str("table", true)
			s.cols = o.names("columns", true)
			s.name = o.str("name", false)
			if s.name != "" {
				p.checkName(o.at("name"), s.name)
			}
			s.unique = o.flag("unique")
		case "drop_index":
			s.name = o.str("name", true)
		case "annotate_column":
			s.table, s.column = o.str("table", true), o.str("column", true)
			if !o.has("kind") {
				p.add(o.at("kind"), "is required (use object_ref, or \"\" to remove the annotation)")
			}
			s.kind = o.str("kind", false)
			if !slices.Contains(columnKinds, s.kind) {
				p.add(o.at("kind"), "unknown kind %q (use object_ref, or \"\" to remove the annotation)", s.kind)
			}
		case "insert_rows":
			s.table = o.str("table", true)
			rows := o.list("rows", true)
			if len(rows) > maxSeedRows {
				p.add(o.at("rows"), "has %d rows; insert_rows takes at most %d (use copy_data or the insert tool for more)", len(rows), maxSeedRows)
			}
			for j, r := range rows {
				d := json.NewDecoder(bytes.NewReader(r))
				d.UseNumber()
				var m map[string]any
				if err := d.Decode(&m); err != nil || m == nil {
					p.add(fmt.Sprintf("%s.rows[%d]", path, j), "must be an object of column names and values")
					continue
				}
				s.rows = append(s.rows, m)
			}
		case "copy_data":
			s.table = o.str("into", true)
			s.cols = o.names("columns", true)
			s.query = o.str("from", true)
			if s.query != "" {
				if err := sqlguard.Check(s.query); err != nil {
					p.add(o.at("from"), "the query was refused: %s", err.Error())
				} else if ps, err := sqlguard.Params(s.query); err == nil && len(ps) > 0 {
					p.add(o.at("from"), "the query can't have parameters such as %s; write the values into it", ps[0].Name)
				}
			}
		case "drop_column":
			s.table, s.column = o.str("table", true), o.str("column", true)
		case "drop_table":
			s.table = o.str("table", true)
		}
		out = append(out, s)
	}
	return out, p.probs
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	slices.Sort(ks)
	return ks
}

// checkExpr is layer 1 of the SQL guard for an expression a step puts in
// DDL or a copy (sqlguard.CheckExpr): one expression, which can't close
// the brackets around it. Layer 2 runs on it in the transaction
// (explainExprs, guarded).
func (p *parser) checkExpr(path, e string) {
	switch {
	case strings.TrimSpace(e) == "":
		p.add(path, "must be an SQL expression")
	case len(e) > maxExpr:
		p.add(path, "the expression is %d bytes; at most %d are allowed", len(e), maxExpr)
	default:
		if err := sqlguard.CheckExpr(e); err != nil {
			p.add(path, "the expression was refused: %s", err.Error())
		}
	}
}

func (p *parser) tableDef(o obj) *tableDef {
	d := &tableDef{}
	for i, raw := range o.list("columns", true) {
		if c := p.columnDef(fmt.Sprintf("%s[%d]", o.at("columns"), i), raw); c != nil {
			if d.has(c.name) {
				p.add(fmt.Sprintf("%s[%d].name", o.at("columns"), i), "the column %q is listed twice", c.name)
			}
			d.cols = append(d.cols, *c)
		}
	}
	if len(d.cols) > maxColumns {
		p.add(o.at("columns"), "has %d columns; a table can have at most %d", len(d.cols), maxColumns)
	}
	inTable := func(path string, names []string) {
		for i, n := range names {
			if !d.has(n) {
				p.add(fmt.Sprintf("%s[%d]", path, i), "the table has no column %q", n)
			}
		}
	}
	if !o.has("primary_key") {
		p.add(o.at("primary_key"), "is required: every table has an explicit primary key")
	}
	d.key = o.names("primary_key", o.has("primary_key"))
	inTable(o.at("primary_key"), d.key)
	for i, k := range d.key {
		for _, c := range d.cols {
			if strings.EqualFold(c.name, k) && c.typ == "BLOB" {
				p.add(fmt.Sprintf("%s[%d]", o.at("primary_key"), i), "the key column %q is a BLOB; keys are TEXT, INTEGER or REAL", k)
			}
		}
	}
	for i, raw := range o.list("unique", false) {
		at := fmt.Sprintf("%s[%d]", o.at("unique"), i)
		u := p.nameList(at, raw, true)
		inTable(at, u)
		d.unique = append(d.unique, u)
	}
	for i, raw := range o.list("foreign_keys", false) {
		at := fmt.Sprintf("%s[%d]", o.at("foreign_keys"), i)
		fo, ok := p.object(at, raw)
		if !ok {
			continue
		}
		fo.only("columns", "table", "to", "on_delete", "on_update")
		f := foreignKeyDef{cols: fo.names("columns", true), table: fo.str("table", true), to: fo.names("to", false)}
		if reservedName(f.table) {
			// A cascade from Jenab's own tables would change rows without
			// a change-log entry.
			p.add(fo.at("table"), "%q is one of Jenab's own tables; a foreign key can only point to an agent table", f.table)
		}
		inTable(fo.at("columns"), f.cols)
		if len(f.to) > 0 && len(f.to) != len(f.cols) {
			p.add(fo.at("to"), "names %d columns for %d in columns", len(f.to), len(f.cols))
		}
		for _, k := range []string{"on_delete", "on_update"} {
			a := strings.ToLower(strings.ReplaceAll(fo.str(k, false), "_", " "))
			if a == "" {
				continue
			}
			sqlA, ok := fkActions[a]
			if !ok {
				p.add(fo.at(k), "unknown action %q (use cascade, set null, set default, restrict or no action)", a)
			}
			if k == "on_delete" {
				f.onDelete = sqlA
			} else {
				f.onUpdate = sqlA
			}
		}
		d.fks = append(d.fks, f)
	}
	return d
}

func (p *parser) columnDef(path string, raw json.RawMessage) *columnDef {
	o, ok := p.object(path, raw)
	if !ok {
		return nil
	}
	o.only("name", "type", "not_null", "default", "check")
	c := &columnDef{name: o.str("name", true), notNull: o.flag("not_null")}
	p.checkName(o.at("name"), c.name)
	typ := o.str("type", true)
	c.typ = strings.ToUpper(strings.TrimSpace(typ))
	if typ != "" && !slices.Contains(columnTypes, c.typ) {
		p.add(o.at("type"), "unknown type %q (use TEXT, INTEGER, REAL or BLOB)", typ)
	}
	if o.has("default") {
		d := json.NewDecoder(bytes.NewReader(o.m["default"]))
		d.UseNumber()
		var v any
		d.Decode(&v)
		switch x := v.(type) {
		case string:
			c.dflt = literal(x)
		case json.Number:
			c.dflt = x.String()
		case bool:
			c.dflt = fmt.Sprint(boolInt(x))
		default:
			p.add(o.at("default"), "must be a string, a number or true or false")
		}
	}
	if o.has("check") {
		c.check = o.str("check", false)
		p.checkExpr(o.at("check"), c.check)
	}
	return c
}

// ---- SQL ----

func (c *columnDef) sql() string {
	s := quote(c.name) + " " + c.typ
	if c.notNull {
		s += " NOT NULL"
	}
	if c.dflt != "" {
		s += " DEFAULT " + c.dflt
	}
	if c.check != "" {
		s += " CHECK (" + c.check + "\n)" // on its own line: a -- comment can't hide it
	}
	return s
}

// sql is CREATE TABLE name for d. Key columns are NOT NULL, as SQLite
// otherwise lets them hold NULL, except an INTEGER key, which is the rowid.
func (d *tableDef) sql(name string) string {
	rowid := len(d.key) == 1 && !d.withoutRowid
	parts := make([]string, 0, len(d.cols)+2)
	for _, c := range d.cols {
		key := slices.ContainsFunc(d.key, func(k string) bool { return strings.EqualFold(k, c.name) })
		if key && !c.notNull && !(rowid && c.typ == "INTEGER") {
			c.notNull = true
		}
		parts = append(parts, c.sql())
	}
	parts = append(parts, "PRIMARY KEY ("+strings.Join(quoteAll(d.key), ", ")+")")
	for _, u := range d.unique {
		parts = append(parts, "UNIQUE ("+strings.Join(quoteAll(u), ", ")+")")
	}
	for _, f := range d.fks {
		s := "FOREIGN KEY (" + strings.Join(quoteAll(f.cols), ", ") + ") REFERENCES " + quote(f.table)
		if len(f.to) > 0 {
			s += " (" + strings.Join(quoteAll(f.to), ", ") + ")"
		}
		if f.onDelete != "" {
			s += " ON DELETE " + f.onDelete
		}
		if f.onUpdate != "" {
			s += " ON UPDATE " + f.onUpdate
		}
		parts = append(parts, s)
	}
	s := "CREATE TABLE " + quote(name) + " (\n  " + strings.Join(parts, ",\n  ") + "\n)"
	var opts []string
	if d.withoutRowid {
		opts = append(opts, "WITHOUT ROWID")
	}
	if d.strict {
		opts = append(opts, "STRICT")
	}
	if len(opts) > 0 {
		s += " " + strings.Join(opts, ", ")
	}
	return s
}

// ---- the plan: destructive steps and the estimate, before the writer ----

type simTable struct {
	name  string
	cols  []string
	key   []string // the primary key's columns
	rows  int64
	bytes int64
}

func (t *simTable) col(c string) (string, bool) {
	for _, x := range t.cols {
		if strings.EqualFold(x, c) {
			return x, true
		}
	}
	return "", false
}

// planner replays the steps on the table names and columns, as a reader
// sees them now, for what the approval card needs (SPEC 8.2): which steps
// drop data, and how many rows the migration rewrites or scans. The steps
// are checked again in the transaction.
type planner struct {
	ctx      context.Context
	tx       *sql.Tx
	tables   map[string]*simTable // by lower-case name; nil once dropped
	all      []string             // the agent tables, as the reader sees them
	probs    []Problem
	est      Estimate
	destruct []int
	lost     []string // what the destructive steps drop: "table" or "table.column"
	keys     []string // the tables whose primary key changes
	rows     map[string]int64
}

// drops marks step i, on table t, destructive; what is what it drops.
func (pl *planner) drops(i int, t *simTable, what ...string) {
	pl.destruct = append(pl.destruct, i)
	for _, w := range what {
		pl.lost = append(pl.lost, strings.ToLower(w))
	}
	pl.counts(t)
}

// counts keeps the rows of t at this step, for the card.
func (pl *planner) counts(t *simTable) {
	if pl.rows == nil {
		pl.rows = map[string]int64{}
	}
	k := strings.ToLower(t.name)
	pl.rows[k] = max(pl.rows[k], t.rows)
}

func (pl *planner) add(path, format string, a ...any) {
	pl.probs = append(pl.probs, Problem{Path: path, Msg: fmt.Sprintf(format, a...)})
}

// get returns the table name as the migration has it at this step.
func (pl *planner) get(name string) (*simTable, error) {
	k := strings.ToLower(name)
	if t, ok := pl.tables[k]; ok {
		return t, nil
	}
	if internalName(name) || !slices.ContainsFunc(pl.all, func(n string) bool { return strings.EqualFold(n, name) }) {
		pl.tables[k] = nil
		return nil, nil
	}
	d, err := describe(pl.ctx, pl.tx, name, true)
	if err != nil {
		return nil, err
	}
	t := &simTable{name: d.Name, key: d.Key, rows: int64(d.Rows)}
	for _, c := range d.Columns {
		t.cols = append(t.cols, c.Name)
	}
	if t.rows > 0 {
		// The table and its indexes, as dbstat counts their pages.
		err := pl.tx.QueryRowContext(pl.ctx, `SELECT coalesce(sum(pgsize), 0) FROM dbstat
			WHERE name IN (SELECT name FROM sqlite_schema WHERE tbl_name = ? COLLATE NOCASE) AND aggregate = TRUE`, d.Name).Scan(&t.bytes)
		if err != nil {
			return nil, err
		}
	}
	pl.tables[k] = t
	return t, nil
}

// need returns the table, or reports at path that there is none.
func (pl *planner) need(path, name string) (*simTable, error) {
	if name == "" {
		return nil, nil
	}
	t, err := pl.get(name)
	if err == nil && t == nil {
		pl.add(path, "there is no table %q%s", name, pl.tableHint())
	}
	return t, err
}

func (pl *planner) tableHint() string {
	var names []string
	for _, n := range pl.all {
		if t, ok := pl.tables[strings.ToLower(n)]; !ok || t != nil {
			names = append(names, n)
		}
	}
	for _, t := range pl.tables {
		if t != nil && !slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, t.name) }) {
			names = append(names, t.name)
		}
	}
	if len(names) == 0 {
		return " (the project has no tables yet)"
	}
	slices.Sort(names)
	return " (tables: " + strings.Join(names, ", ") + ")"
}

func (pl *planner) cost(rows int64, rate time.Duration, bytes int64) {
	pl.est.Rows += rows
	pl.est.Time += time.Duration(rows) * rate
	pl.est.Bytes += bytes
}

// refs are the tables a query names, as the lexer of layer 1 would read
// its words, for the estimate of copy_data.
func (pl *planner) refs(q string) []*simTable {
	words := map[string]bool{}
	var w strings.Builder
	flush := func() {
		if w.Len() > 0 {
			words[strings.ToLower(w.String())] = true
			w.Reset()
		}
	}
	for _, r := range q {
		if r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			w.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	var out []*simTable
	seen := map[string]bool{}
	names := append(slices.Clone(pl.all), sortedKeys(pl.tables)...)
	for _, n := range names {
		k := strings.ToLower(n)
		if !words[k] || seen[k] {
			continue
		}
		seen[k] = true
		if t, err := pl.get(n); err == nil && t != nil {
			out = append(out, t)
		}
	}
	return out
}

func (pl *planner) run(steps []*step) error {
	for i, s := range steps {
		var err error
		if err = pl.step(i, s); err != nil {
			return err
		}
	}
	pl.est.Long = pl.est.Rows > LongRows
	return nil
}

func (pl *planner) step(i int, s *step) error {
	switch s.op {
	case "create_table":
		if t, err := pl.get(s.table); err != nil {
			return err
		} else if t != nil {
			pl.add(s.path+".table", "the table %q already exists", t.name)
			return nil
		}
		t := &simTable{name: s.table, key: s.def.key}
		for _, c := range s.def.cols {
			t.cols = append(t.cols, c.name)
		}
		pl.tables[strings.ToLower(s.table)] = t
	case "add_column":
		t, err := pl.need(s.path+".table", s.table)
		if t == nil || s.col == nil {
			return err
		}
		if _, ok := t.col(s.col.name); ok {
			pl.add(s.path+".column.name", "the table %q already has a column %q", t.name, s.col.name)
		}
		t.cols = append(t.cols, s.col.name)
		if s.col.check != "" || s.col.notNull {
			pl.cost(t.rows, rateScan, 0)
		}
	case "rename_table":
		t, err := pl.need(s.path+".from", s.from)
		if t == nil {
			return err
		}
		if u, err := pl.get(s.to); err != nil {
			return err
		} else if u != nil && !strings.EqualFold(s.from, s.to) {
			pl.add(s.path+".to", "the table %q already exists", u.name)
			return nil
		}
		pl.tables[strings.ToLower(s.from)] = nil
		t.name = s.to
		pl.tables[strings.ToLower(s.to)] = t
	case "rename_column":
		t, err := pl.need(s.path+".table", s.table)
		if t == nil {
			return err
		}
		c, ok := t.col(s.from)
		if !ok {
			pl.add(s.path+".from", "the table %q has no column %q (columns: %s)", t.name, s.from, strings.Join(t.cols, ", "))
			return nil
		}
		if d, ok := t.col(s.to); ok && !strings.EqualFold(c, s.to) {
			pl.add(s.path+".to", "the table %q already has a column %q", t.name, d)
			return nil
		}
		t.cols[slices.Index(t.cols, c)] = s.to
		if k := slices.IndexFunc(t.key, func(x string) bool { return strings.EqualFold(x, c) }); k >= 0 {
			t.key = slices.Clone(t.key)
			t.key[k] = s.to
		}
	case "create_index":
		t, err := pl.need(s.path+".table", s.table)
		if t == nil {
			return err
		}
		pl.cols(s.path+".columns", t, s.cols)
		pl.cost(t.rows, rateIndex, t.bytes/2)
	case "annotate_column":
		t, err := pl.need(s.path+".table", s.table)
		if t == nil {
			return err
		}
		pl.cols(s.path+".column", t, []string{s.column})
		if s.kind != "" {
			pl.cost(t.rows, rateScan, 0)
		}
	case "insert_rows":
		t, err := pl.need(s.path+".table", s.table)
		if t != nil {
			t.rows += int64(len(s.rows))
		}
		return err
	case "copy_data":
		t, err := pl.need(s.path+".into", s.table)
		if t == nil {
			return err
		}
		pl.cols(s.path+".columns", t, s.cols)
		var rows, size int64
		for _, src := range pl.refs(s.query) {
			rows += src.rows
			size += src.bytes
		}
		t.rows += rows
		t.bytes += size
		s.est = rows
		pl.cost(rows, rateCopy, size)
	case "rebuild_table":
		t, err := pl.need(s.path+".table", s.table)
		if t == nil {
			return err
		}
		var lost []string
		for _, c := range t.cols {
			// The mapping's keys are the definition's names, which may
			// differ in case from the old column's.
			n := slices.IndexFunc(s.def.cols, func(x columnDef) bool { return strings.EqualFold(x.name, c) })
			if n >= 0 && s.mapping[s.def.cols[n].name] == "" || mapsColumn(s.mapping, c) {
				continue
			}
			lost = append(lost, t.name+"."+c)
		}
		if len(lost) > 0 {
			pl.drops(i, t, lost...)
		}
		if !slices.EqualFunc(t.key, s.def.key, strings.EqualFold) {
			pl.keys = append(pl.keys, strings.ToLower(t.name))
			pl.counts(t)
		}
		t.key = s.def.key
		t.cols = nil
		for _, c := range s.def.cols {
			t.cols = append(t.cols, c.name)
		}
		s.est = t.rows
		pl.cost(t.rows, rateRebuild, t.bytes)
	case "drop_column":
		t, err := pl.need(s.path+".table", s.table)
		if t == nil {
			return err
		}
		c, ok := t.col(s.column)
		if !ok {
			pl.add(s.path+".column", "the table %q has no column %q (columns: %s)", t.name, s.column, strings.Join(t.cols, ", "))
			return nil
		}
		t.cols = slices.DeleteFunc(t.cols, func(x string) bool { return x == c })
		pl.drops(i, t, t.name+"."+c)
		pl.cost(t.rows, rateDropColumn, t.bytes)
	case "drop_table":
		t, err := pl.need(s.path+".table", s.table)
		if t == nil {
			return err
		}
		pl.tables[strings.ToLower(s.table)] = nil
		pl.drops(i, t, t.name)
	}
	return nil
}

func (pl *planner) cols(path string, t *simTable, cols []string) {
	for j, c := range cols {
		if _, ok := t.col(c); !ok {
			at := path
			if len(cols) > 1 || strings.HasSuffix(path, "s") {
				at = fmt.Sprintf("%s[%d]", path, j)
			}
			pl.add(at, "the table %q has no column %q (columns: %s)", t.name, c, strings.Join(t.cols, ", "))
		}
	}
}

// mapsColumn reports whether a mapping copies the old column c as it is,
// to a column of another name.
func mapsColumn(m map[string]string, c string) bool {
	for _, e := range m {
		if isColumn(e, c) {
			return true
		}
	}
	return false
}

// isColumn reports whether the expression e is just the column c.
func isColumn(e, c string) bool {
	e = strings.TrimSpace(e)
	return strings.EqualFold(e, c) || strings.EqualFold(e, quote(c))
}

// mappedFrom returns the old column that the expression e is, if it is
// just one.
func mappedFrom(e string, old []Column) (string, bool) {
	for _, c := range old {
		if isColumn(e, c.Name) {
			return c.Name, true
		}
	}
	return "", false
}

// ---- Migrate ----

// Migrate runs m (SPEC 7.5): every step, the dependents' updates and
// checks, the schema guard, the change-log entry (op migrate) and the
// schema version, in one transaction on the writer, as one interactive
// request. If anything fails, nothing changes and the error is a
// *MigrationError with every problem it found, each at its path, e.g.
// steps[2].column.type. src is migration:<id>.
//
// Before it runs, Go replays the steps on the schema to find the
// destructive ones, the primary keys it changes and the rows it rewrites
// or scans. Destructive steps and key changes need o.Approved (or
// o.Planned), and destructive steps get a snapshot first. A long
// migration needs free disk space for itself (2× the rewritten tables on
// the project's drive, 0.5× on the temp drive), and is checkpointed after. With o.Try it runs
// and rolls back; a long try runs on a copy, not on the writer.
func (p *ProjectDB) Migrate(ctx context.Context, src id.Source, m Migration, o MigrateOptions) (MigrationReport, error) {
	rep := MigrationReport{Tried: o.Try}
	if err := checkSource(src); err != nil {
		return rep, err
	}
	steps, probs := parseSteps(m.Steps)
	kinds := p.dependentKinds()
	for i, u := range m.Updates {
		if _, ok := kinds[u.Kind]; !ok {
			probs = append(probs, Problem{Path: fmt.Sprintf("updates[%d].kind", i), Msg: fmt.Sprintf("unknown kind %q%s", u.Kind, kindHint(kinds))})
		}
	}
	for _, s := range steps {
		rep.Steps = append(rep.Steps, StepReport{Op: s.op, Table: s.table})
	}
	if len(probs) > 0 {
		rep.Problems = probs
		return rep, &MigrationError{Problems: probs}
	}
	var basis planBasis
	pl, err := readTx(ctx, p.DB, func(tx *sql.Tx) (*planner, error) {
		if err := tx.QueryRowContext(ctx, "PRAGMA schema_version").Scan(&basis.version); err != nil {
			return nil, err
		}
		return plan(ctx, tx, steps)
	})
	if err != nil {
		return rep, err
	}
	pl.report(&rep)
	basis.lost, basis.keys = pl.lost, pl.keys
	if len(pl.probs) > 0 {
		rep.Problems = pl.probs
		return rep, &MigrationError{Problems: pl.probs}
	}
	if o.Planned != nil && !o.Try {
		// The plan is checked against what the user approved, here and
		// again in the transaction.
		basis.lost, basis.keys = o.Planned.Lost, o.Planned.KeysChanged
		if err := basis.covers(pl, "the schema changed after the try"); err != nil {
			return rep, err
		}
	} else if (len(rep.Destructive) > 0 || len(rep.KeysChanged) > 0) && !o.Try && !o.Approved {
		return rep, ErrNeedsApproval
	}
	dir := filepath.Dir(p.path)
	long := rep.Estimate.Long
	if long && o.Long != nil {
		o.Long(rep.Estimate)
	}
	if o.Try && long {
		return p.tryOnCopy(ctx, src, steps, m, rep)
	}
	if !o.Try {
		var snap int64
		if len(rep.Destructive) > 0 {
			snap = fileSize(p.path)
		}
		if err := checkSpace(dir, rep.Estimate, long, snap); err != nil {
			return rep, err
		}
	}
	out, err := doConn(ctx, p.DB, limit.Interactive, func(ctx context.Context, c *sql.Conn) (MigrationReport, error) {
		// The snapshot is taken while the migration holds the writer, so
		// no write lands between it and the migration.
		if len(rep.Destructive) > 0 && !o.Try {
			var err error
			if rep.Snapshot, err = p.takeSnapshot(ctx); err != nil {
				return rep, err
			}
		}
		r, err := p.migrateOn(ctx, c, src, steps, m, rep, !o.Try, &basis)
		if err == nil && long && !o.Try {
			checkpoint(ctx, c) // the WAL holds about 1.2× the rewritten data (SPEC 7.5)
		}
		return r, err
	})
	switch {
	case rep.Snapshot == "":
	case err == nil:
		p.pruneSnapshots()
	default:
		// Nothing was committed, so the snapshot isn't needed.
		os.Remove(filepath.Join(p.snapshotDir(), rep.Snapshot))
		out.Snapshot = ""
	}
	return out, err
}

// plan runs the planner on the schema as tx sees it.
func plan(ctx context.Context, tx *sql.Tx, steps []*step) (*planner, error) {
	pl := &planner{ctx: ctx, tx: tx, tables: map[string]*simTable{}}
	var err error
	if pl.all, err = listAgentTables(ctx, tx); err != nil {
		return nil, err
	}
	return pl, pl.run(steps)
}

// report puts the plan's estimate and destructive steps in rep.
func (pl *planner) report(rep *MigrationReport) {
	rep.Estimate, rep.Destructive, rep.Lost = pl.est, pl.destruct, pl.lost
	rep.KeysChanged, rep.LostRows = pl.keys, pl.rows
	for i := range rep.Steps {
		rep.Steps[i].Destructive = slices.Contains(pl.destruct, i)
	}
}

// planBasis is what a plan was made on: the schema cookie, and what its
// steps drop and whose key they change, or what the user approved of
// that.
type planBasis struct {
	version int64
	lost    []string
	keys    []string
}

// covers refuses a plan that drops data or changes keys beyond the basis.
func (b *planBasis) covers(pl *planner, why string) error {
	var more []string
	for _, w := range pl.lost {
		if !slices.Contains(b.lost, w) {
			more = append(more, w)
		}
	}
	for _, t := range pl.keys {
		if !slices.Contains(b.keys, t) {
			more = append(more, "the primary key of "+t)
		}
	}
	if len(more) > 0 {
		return fmt.Errorf("%w: %s, and now it drops or changes %s too", ErrNeedsApproval, why, strings.Join(more, ", "))
	}
	return nil
}

// replan plans the steps again in the migration's transaction when the
// schema changed after the first plan, e.g. by a migration that ran while
// this one waited for the writer. Data the steps drop now but didn't then
// had no approval and maybe no snapshot, so the migration is refused.
func replan(ctx context.Context, tx *sql.Tx, steps []*step, rep *MigrationReport, planned *planBasis, commit bool) error {
	var v int64
	if err := tx.QueryRowContext(ctx, "PRAGMA schema_version").Scan(&v); err != nil {
		return err
	}
	if v == planned.version {
		return nil
	}
	pl, err := plan(ctx, tx, steps)
	if err != nil {
		return err
	}
	if len(pl.probs) > 0 {
		return &MigrationError{Problems: pl.probs}
	}
	pl.report(rep)
	if commit {
		return planned.covers(pl, "the schema changed while the migration waited")
	}
	return nil
}

func kindHint(kinds map[string]Dependent) string {
	if len(kinds) == 0 {
		return " (this project has no dependents to update yet)"
	}
	return " (use " + strings.Join(sortedKeys(kinds), ", ") + ")"
}

func fileSize(name string) int64 {
	st, err := os.Stat(name)
	if err != nil {
		return 0
	}
	return st.Size()
}

// freeSpace is disk.Free; tests swap it.
var freeSpace = disk.Free

// checkSpace checks free space before a migration (SPEC 7.5): 2× the
// rewritten tables on the project's drive and 0.5× on the temp drive
// SQLite sorts on, for a long one, and the snapshot's size for a
// destructive one. A drive whose space can't be told is not checked.
func checkSpace(dir string, est Estimate, long bool, snapshot int64) error {
	need := map[string]int64{}
	where := map[string]string{}
	add := func(path string, n int64) {
		if n <= 0 {
			return
		}
		_, vol, ok := freeSpace(path)
		if !ok {
			return
		}
		need[vol] += n
		if where[vol] == "" {
			where[vol] = path
		}
	}
	add(dir, snapshot)
	if long {
		add(dir, 2*est.Bytes)
		add(os.TempDir(), est.Bytes/2)
	}
	for _, vol := range sortedKeys(need) {
		free, _, ok := freeSpace(where[vol])
		if ok && free < uint64(need[vol]) {
			return fmt.Errorf("%w: it needs %s free on the drive of %s, which has %s", ErrNoSpace,
				byteText(need[vol]), where[vol], byteText(int64(free)))
		}
	}
	return nil
}

// byteText writes n for people, e.g. "12.3 MB".
func byteText(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	f, i := float64(n), 0
	for f >= unit && i < 4 {
		f /= unit
		i++
	}
	return fmt.Sprintf("%.1f %cB", f, "kMGTP"[i-1])
}

// listAgentTables lists the agent tables as tx sees them.
func listAgentTables(ctx context.Context, tx *sql.Tx) ([]string, error) {
	names, err := queryTxContext(ctx, tx, `SELECT name FROM pragma_table_list WHERE schema = 'main' AND type = 'table' ORDER BY name`, nil,
		func(r *sql.Rows) (string, error) {
			var n string
			return n, r.Scan(&n)
		})
	return slices.DeleteFunc(names, internalName), err
}

// tryOnCopy runs a long try on a VACUUM INTO copy in tmp/, on its own
// connection, so it doesn't hold the writer (SPEC 7.5). The copy is
// deleted afterwards.
func (p *ProjectDB) tryOnCopy(ctx context.Context, src id.Source, steps []*step, m Migration, rep MigrationReport) (MigrationReport, error) {
	tmp := filepath.Join(filepath.Dir(p.path), "tmp")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return rep, fmt.Errorf("store: %w", err)
	}
	if err := checkSpace(tmp, rep.Estimate, true, fileSize(p.path)); err != nil {
		return rep, err
	}
	copyPath := filepath.Join(tmp, "try-"+id.New()+".db")
	defer removePartial(copyPath)
	if err := vacuumInto(ctx, p.path, copyPath, nil); err != nil {
		return rep, err
	}
	db, err := sql.Open("sqlite", dsn(copyPath, false, common...))
	if err != nil {
		return rep, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	c, err := db.Conn(ctx)
	if err != nil {
		return rep, err
	}
	defer c.Close()
	return p.migrateOn(ctx, c, src, steps, m, rep, false, nil)
}

// errTried rolls back a try.
var errTried = errors.New("store: migration tried")

// migrateOn runs the migration on c in one transaction, with foreign keys
// off around it, as SQLite's table rebuild needs (SPEC 7.5); commit false
// rolls it back after the checks. planned is what the plan was made on;
// nil for a copy, which doesn't change under it.
func (p *ProjectDB) migrateOn(ctx context.Context, c *sql.Conn, src id.Source, steps []*step, m Migration, rep MigrationReport,
	commit bool, planned *planBasis) (MigrationReport, error) {
	if _, err := c.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return rep, err
	}
	defer c.ExecContext(context.WithoutCancel(ctx), "PRAGMA foreign_keys = ON")
	out := rep
	_, err := inTx(ctx, c, func(tx *sql.Tx) (struct{}, error) {
		out = rep
		out.Steps = slices.Clone(rep.Steps)
		if planned != nil {
			if err := replan(ctx, tx, steps, &out, planned, commit); err != nil {
				return struct{}{}, err
			}
		}
		r := &run{ctx: ctx, c: c, tx: tx, rep: &out, marks: map[string]bool{}, written: map[string]bool{}, made: map[string]bool{}}
		if err := r.all(src, steps, m, p.dependentKinds()); err != nil {
			return struct{}{}, err
		}
		if !commit {
			return struct{}{}, errTried
		}
		return struct{}{}, nil
	})
	if errors.Is(err, errTried) {
		err = nil
	}
	var me *MigrationError
	if errors.As(err, &me) {
		out.Problems = me.Problems
		out.Change = ""
	}
	if !commit {
		out.Change = ""
	}
	return out, err
}

// run is one migration inside its transaction.
type run struct {
	ctx     context.Context
	c       *sql.Conn
	tx      *sql.Tx
	rep     *MigrationReport
	cur     *StepReport
	marks   map[string]bool // tables changed, by name: true if it stands in the way of undo
	written map[string]bool // tables data steps wrote, by name
	made    map[string]bool // tables create_table and rebuilds made, by name
}

// freeName is base, or base with a number, whichever no table, index,
// view or trigger has.
func (r *run) freeName(base string) (string, error) {
	name := base
	for n := 2; ; n++ {
		var taken bool
		if err := r.tx.QueryRowContext(r.ctx, `SELECT count(*) > 0 FROM sqlite_schema WHERE name = ? COLLATE NOCASE`, name).Scan(&taken); err != nil || !taken {
			return name, err
		}
		name = fmt.Sprintf("%s%d", base, n)
	}
}

func (r *run) exec(q string, args ...any) (sql.Result, error) {
	r.cur.SQL = append(r.cur.SQL, q)
	return r.tx.ExecContext(r.ctx, q, args...)
}

func (r *run) note(format string, a ...any) {
	r.cur.Notes = append(r.cur.Notes, fmt.Sprintf(format, a...))
}

// mark records a table the step changed; block says whether an undo of
// earlier changes to it must stop (see logMigration).
func (r *run) mark(table string, block bool) { r.marks[table] = r.marks[table] || block }

func (r *run) all(src id.Source, steps []*step, m Migration, kinds map[string]Dependent) error {
	before, err := internalSchema(r.ctx, r.tx)
	if err != nil {
		return err
	}
	for i, s := range steps {
		r.cur = &r.rep.Steps[i]
		r.cur.SQL, r.cur.Notes, r.cur.Rows = nil, nil, 0
		if err := r.step(s); err != nil {
			return asProblem(s.path, err)
		}
	}
	for i, u := range m.Updates {
		if err := kinds[u.Kind].Update(r.tx, src, u); err != nil {
			return asProblem(fmt.Sprintf("updates[%d]", i), err)
		}
		r.rep.Updated = append(r.rep.Updated, u.Kind+":"+u.ID)
	}
	var probs []Problem
	for _, k := range sortedKeys(kinds) {
		ps, err := kinds[k].Check(r.tx)
		if err != nil {
			return err
		}
		probs = append(probs, ps...)
	}
	gs, err := r.guard(before)
	if err != nil {
		return err
	}
	if probs = append(probs, gs...); len(probs) > 0 {
		return &MigrationError{Problems: probs}
	}
	if r.rep.SchemaVersion, err = bumpSchemaVersion(r.tx); err != nil {
		return err
	}
	after, err := json.Marshal(map[string]any{"steps": m.Steps, "schema_version": r.rep.SchemaVersion, "updated": r.rep.Updated})
	if err != nil {
		return err
	}
	var block, keep []string
	for _, t := range sortedKeys(r.marks) {
		if r.marks[t] {
			block = append(block, t)
		} else {
			keep = append(keep, t)
		}
	}
	r.rep.Change, err = logMigration(r.tx, src, block, keep, string(after))
	return err
}

// asProblem pins an error of SQLite or of a check on the step or update at
// path; other errors stay as they are.
func asProblem(path string, err error) error {
	var me *MigrationError
	if errors.As(err, &me) {
		for i := range me.Problems {
			if me.Problems[i].Path == "" {
				me.Problems[i].Path = path
			} else if !strings.HasPrefix(me.Problems[i].Path, path) {
				me.Problems[i].Path = path + "." + me.Problems[i].Path
			}
		}
		return me
	}
	var se *sqlite.Error
	switch {
	case errors.As(err, &se):
		return problem(path, "%s", sqliteMsg(err))
	case errors.Is(err, ErrInvalid):
		return problem(path, "%s", strings.TrimPrefix(err.Error(), "store: "))
	}
	return err
}

// sqliteMsg is SQLite's message without the driver's class and code,
// "SQL logic error: no such column: x (1)" as "no such column: x".
func sqliteMsg(err error) string {
	s := err.Error()
	if i := strings.LastIndex(s, " ("); i >= 0 && strings.HasSuffix(s, ")") {
		s = s[:i]
	}
	for _, p := range []string{"SQL logic error: ", "constraint failed: "} {
		if i := strings.Index(s, p); i >= 0 {
			s = s[i+len(p):]
		}
	}
	return s
}

// ---- the steps ----

// table returns the agent table name as the transaction has it, or a
// problem at field.
func (r *run) table(field, name string) (string, error) {
	var n, typ string
	err := r.tx.QueryRowContext(r.ctx, `SELECT name, type FROM pragma_table_list WHERE schema = 'main' AND name = ? COLLATE NOCASE`, name).Scan(&n, &typ)
	switch {
	case errors.Is(err, sql.ErrNoRows) || err == nil && internalName(n):
		return "", problem(field, "there is no table %q", name)
	case err != nil:
		return "", err
	case typ != "table":
		return "", problem(field, "%q is a %s, not a table", n, typ)
	}
	return n, nil
}

func (r *run) columnOf(field, table, col string) (string, error) {
	var n string
	err := r.tx.QueryRowContext(r.ctx, `SELECT name FROM pragma_table_xinfo(?) WHERE name = ? COLLATE NOCASE`, table, col).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return "", problem(field, "the table %q has no column %q", table, col)
	}
	return n, err
}

func (r *run) step(s *step) error {
	switch s.op {
	case "create_table":
		if t, err := r.table("", s.table); err == nil {
			return problem("table", "the table %q already exists", t)
		}
		if _, err := r.exec(s.def.sql(s.table)); err != nil {
			return err
		}
		if err := r.explainChecks("columns", quote(s.table), s.def.cols); err != nil {
			return err
		}
		r.mark(s.table, true)
		r.made[s.table] = true
	case "add_column":
		t, err := r.table("table", s.table)
		if err != nil {
			return err
		}
		if s.col.notNull && s.col.dflt == "" {
			return problem("column.default", "SQLite needs a default to add a NOT NULL column to a table; "+
				"add it without not_null, or use rebuild_table with a mapping that gives the rows their values")
		}
		// SQLite runs the check on the rows as it adds the column, so
		// layer 2 sees it first, over the rows with the new column, under
		// the table's name and with its rowid.
		old, err := describe(r.ctx, r.tx, t, false)
		if err != nil {
			return err
		}
		cols := "*, "
		if _, ok := old.Column("rowid"); !ok && !old.WithoutRowid {
			cols = "rowid AS rowid, *, "
		}
		from := "(SELECT " + cols + "NULL AS " + quote(s.col.name) + " FROM " + quote(t) + ") AS " + quote(t)
		if err := r.explainChecks("column", from, []columnDef{*s.col}); err != nil {
			return err
		}
		if _, err := r.exec("ALTER TABLE " + quote(t) + " ADD COLUMN " + s.col.sql()); err != nil {
			return err
		}
		r.mark(t, false)
	case "rename_table":
		t, err := r.table("from", s.from)
		if err != nil {
			return err
		}
		if u, err := r.table("", s.to); err == nil && !strings.EqualFold(u, t) {
			return problem("to", "the table %q already exists", u)
		}
		if t == s.to {
			return problem("to", "the table is named %q already", t)
		}
		if strings.EqualFold(t, s.to) {
			// SQLite takes a new name that differs only in case for one
			// that is taken, so the rename goes through another name.
			mid, err := r.freeName(t + "_rename")
			if err != nil {
				return err
			}
			if _, err := r.exec("ALTER TABLE " + quote(t) + " RENAME TO " + quote(mid)); err != nil {
				return err
			}
			if _, err := r.exec("ALTER TABLE " + quote(mid) + " RENAME TO " + quote(s.to)); err != nil {
				return err
			}
		} else if _, err := r.exec("ALTER TABLE " + quote(t) + " RENAME TO " + quote(s.to)); err != nil {
			return err
		}
		if _, err := r.tx.ExecContext(r.ctx, `UPDATE _jenab_columns SET table_name = ? WHERE table_name = ? COLLATE NOCASE`, s.to, t); err != nil {
			return err
		}
		r.mark(t, true)
		r.mark(s.to, true)
		for _, m := range []map[string]bool{r.written, r.made} {
			if m[t] {
				delete(m, t)
				m[s.to] = true
			}
		}
	case "rename_column":
		t, err := r.table("table", s.table)
		if err != nil {
			return err
		}
		c, err := r.columnOf("from", t, s.from)
		if err != nil {
			return err
		}
		if _, err := r.exec("ALTER TABLE " + quote(t) + " RENAME COLUMN " + quote(c) + " TO " + quote(s.to)); err != nil {
			return err
		}
		if _, err := r.tx.ExecContext(r.ctx, `UPDATE _jenab_columns SET column_name = ? WHERE table_name = ? COLLATE NOCASE AND column_name = ? COLLATE NOCASE`,
			s.to, t, c); err != nil {
			return err
		}
		r.mark(t, true)
	case "create_index":
		return r.createIndex(s)
	case "drop_index":
		var name, t, origin string
		err := r.tx.QueryRowContext(r.ctx, `SELECT s.name, s.tbl_name, l.origin FROM sqlite_schema s, pragma_index_list(s.tbl_name) l
			WHERE s.type = 'index' AND s.name = ? COLLATE NOCASE AND l.name = s.name`, s.name).Scan(&name, &t, &origin)
		switch {
		case errors.Is(err, sql.ErrNoRows) || err == nil && internalName(t):
			return problem("name", "there is no index %q", s.name)
		case err != nil:
			return err
		case origin != "c":
			return problem("name", "the index %q belongs to a key or UNIQUE constraint of %q; change those with rebuild_table", name, t)
		}
		r.cur.Table = t
		if _, err := r.exec("DROP INDEX " + quote(name)); err != nil {
			return err
		}
		r.mark(t, false)
	case "annotate_column":
		return r.annotate(s)
	case "insert_rows":
		return r.insertRows(s)
	case "copy_data":
		return r.copyData(s)
	case "rebuild_table":
		t, err := r.table("table", s.table)
		if err != nil {
			return err
		}
		return r.rebuild(t, s.def, s.mapping)
	case "drop_column":
		return r.dropColumn(s)
	case "drop_table":
		t, err := r.table("table", s.table)
		if err != nil {
			return err
		}
		if _, err := r.exec("DROP TABLE " + quote(t)); err != nil {
			return err
		}
		if _, err := r.tx.ExecContext(r.ctx, `DELETE FROM _jenab_columns WHERE table_name = ? COLLATE NOCASE`, t); err != nil {
			return err
		}
		r.mark(t, true)
		delete(r.written, t)
	}
	return nil
}

func (r *run) createIndex(s *step) error {
	t, err := r.table("table", s.table)
	if err != nil {
		return err
	}
	cols := make([]string, len(s.cols))
	for i, c := range s.cols {
		if cols[i], err = r.columnOf(fmt.Sprintf("columns[%d]", i), t, c); err != nil {
			return err
		}
	}
	name := s.name
	if name == "" {
		base := t + "_" + strings.Join(cols, "_")
		name = base
		for n := 2; ; n++ {
			var taken bool
			if err := r.tx.QueryRowContext(r.ctx, `SELECT count(*) > 0 FROM sqlite_schema WHERE name = ? COLLATE NOCASE`, name).Scan(&taken); err != nil {
				return err
			}
			if !taken {
				break
			}
			name = fmt.Sprintf("%s_%d", base, n)
		}
		r.note("the index is named %s", name)
	}
	u := ""
	if s.unique {
		u = "UNIQUE "
	}
	if _, err := r.exec("CREATE " + u + "INDEX " + quote(name) + " ON " + quote(t) + " (" + strings.Join(quoteAll(cols), ", ") + ")"); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return problem("name", "the name %q is taken", name)
		}
		return err
	}
	r.mark(t, false)
	return nil
}

// annotate sets or removes a column's annotation (SPEC 4.5). Every value
// of an object_ref column must be a stored file's key.
func (r *run) annotate(s *step) error {
	t, err := r.table("table", s.table)
	if err != nil {
		return err
	}
	c, err := r.columnOf("column", t, s.column)
	if err != nil {
		return err
	}
	if s.kind == "" {
		_, err := r.exec(`DELETE FROM _jenab_columns WHERE table_name = ? COLLATE NOCASE AND column_name = ? COLLATE NOCASE`, t, c)
		r.mark(t, true)
		return err
	}
	var bad int
	var first sql.NullString
	q := fmt.Sprintf(`SELECT count(*), min(%[1]s) FROM %[2]s WHERE %[1]s IS NOT NULL AND (typeof(%[1]s) <> 'text' OR %[1]s NOT IN (SELECT key FROM _jenab_objects))`,
		quote(c), quote(t))
	if err := r.tx.QueryRowContext(r.ctx, q).Scan(&bad, &first); err != nil {
		return err
	}
	if bad > 0 {
		return problem("column", "%d values of %s.%s are not keys of stored files, such as %s; an object_ref column holds only file keys or null",
			bad, t, c, show(first.String))
	}
	if _, err := r.exec(`INSERT INTO _jenab_columns (table_name, column_name, kind) VALUES (?, ?, ?)
		ON CONFLICT (table_name, column_name) DO UPDATE SET kind = excluded.kind`, t, c, s.kind); err != nil {
		return err
	}
	r.mark(t, true)
	return nil
}

// insertRows adds seed rows, checked as the insert tool checks them
// (SPEC 8.1).
func (r *run) insertRows(s *step) error {
	t, err := r.table("table", s.table)
	if err != nil {
		return err
	}
	desc, err := describe(r.ctx, r.tx, t, false)
	if err != nil {
		return err
	}
	rows, errs := desc.CheckRows(s.rows)
	if len(errs) > 0 {
		var probs []Problem
		for _, e := range errs {
			at := fmt.Sprintf("rows[%d]", e.Row)
			if e.Column != "" {
				at += "." + e.Column
			}
			probs = append(probs, Problem{Path: at, Msg: e.Msg})
		}
		return &MigrationError{Problems: probs}
	}
	tab, err := loadTable(r.tx, t)
	if err != nil {
		return err
	}
	named := make([]map[string]any, len(rows))
	for i, row := range rows {
		named[i] = row
	}
	if err := tab.checkRefs(r.tx, named); err != nil {
		return problem("rows", "%s", strings.TrimPrefix(err.Error(), "store: "))
	}
	for i, row := range rows {
		cols := make([]string, 0, len(row))
		for _, c := range tab.cols {
			if _, ok := row[c]; ok {
				cols = append(cols, c)
			}
		}
		args := make([]any, len(cols))
		for j, c := range cols {
			args[j] = row[c]
		}
		q := fmt.Sprintf("INSERT OR ABORT INTO %s (%s) VALUES (%s)", quote(t), strings.Join(quoteAll(cols), ", "),
			strings.TrimSuffix(strings.Repeat("?, ", len(cols)), ", "))
		if i == 0 {
			r.cur.SQL = append(r.cur.SQL, q)
		}
		if _, err := r.tx.ExecContext(r.ctx, q, args...); err != nil {
			return asProblem(fmt.Sprintf("rows[%d]", i), err)
		}
	}
	r.cur.Rows = int64(len(rows))
	r.mark(t, true)
	r.written[t] = true
	return nil
}

// copyDeadline bounds a copy that the planner expects to read rows rows:
// the agent's query or mapping runs on the writer, where only a deadline
// stops it (SPEC 2.2). SPIKE-010 measured 6 s for 1M rows with GROUP BY
// and 54 s for 5M. Tests swap it.
var copyDeadline = func(rows int64) time.Duration {
	return time.Minute + time.Duration(rows)*20*time.Microsecond
}

// explainChecks is layer 2 of the SQL guard on the CHECK expressions of
// cols, over from (a quoted table or a subquery) as the transaction has
// it: the same functions and tables as an agent query. field is where
// cols are in the step: "columns" for a list, "column" for one.
func (r *run) explainChecks(field, from string, cols []columnDef) error {
	for i, c := range cols {
		if c.check == "" {
			continue
		}
		at := field + ".check"
		if field == "columns" {
			at = fmt.Sprintf("columns[%d].check", i)
		}
		if err := sqlguard.New().Explain(r.ctx, r.c, "SELECT ("+c.check+"\n) FROM "+from); err != nil {
			var re *sqlguard.RejectError
			if errors.As(err, &re) {
				return problem(at, "the expression was refused: %s", re.Reason)
			}
			return asProblem(at, err)
		}
	}
	return nil
}

// guarded runs an INSERT whose rows come from agent SQL on the writer,
// with the SQL guard's layer 2 on the query first. Values and records are
// capped at sqlguard.MaxLength, as on a reader, and the statement at a
// deadline (copyDeadline).
func (r *run) guarded(field, query, insert string, rows int64) (int64, error) {
	if err := sqlguard.New().Explain(r.ctx, r.c, query); err != nil {
		var re *sqlguard.RejectError
		if errors.As(err, &re) {
			return 0, problem(field, "the query was refused: %s", re.Error())
		}
		return 0, err
	}
	old, err := sqlite.Limit(r.c, limitLength, sqlguard.MaxLength)
	if err != nil {
		return 0, err
	}
	defer sqlite.Limit(r.c, limitLength, old)
	d := copyDeadline(rows)
	ctx, cancel := context.WithTimeout(r.ctx, d)
	defer cancel()
	r.cur.SQL = append(r.cur.SQL, insert)
	res, err := r.tx.ExecContext(ctx, insert)
	if err != nil {
		if ctx.Err() != nil && r.ctx.Err() == nil {
			return 0, problem(field, "the copy ran longer than %v and was stopped", d.Round(time.Second))
		}
		return 0, err
	}
	return res.RowsAffected()
}

// copyData copies the rows of a guarded query into a table (SPEC 8.2). It
// runs on the writer, inside the migration, so it sees the tables the
// steps before it made.
func (r *run) copyData(s *step) error {
	t, err := r.table("into", s.table)
	if err != nil {
		return err
	}
	cols := make([]string, len(s.cols))
	for i, c := range s.cols {
		if cols[i], err = r.columnOf(fmt.Sprintf("columns[%d]", i), t, c); err != nil {
			return err
		}
	}
	body, err := sqlguard.Body(s.query)
	if err != nil {
		return problem("from", "the query was refused: %s", err.Error())
	}
	n, err := r.guarded("from", s.query,
		fmt.Sprintf("INSERT OR ABORT INTO %s (%s) SELECT * FROM (%s\n)", quote(t), strings.Join(quoteAll(cols), ", "), body),
		s.est)
	if err != nil {
		if strings.Contains(err.Error(), "values for") {
			return problem("from", "%s: the query must give one column for each of columns (%d)", sqliteMsg(err), len(cols))
		}
		return err
	}
	r.cur.Rows = n
	if err := r.checkObjectRefs("into", t); err != nil {
		return err
	}
	r.mark(t, true)
	r.written[t] = true
	return nil
}

// rows is a table's row count, for a deadline; 0 if it can't be told.
func (r *run) rows(t string) int64 {
	var n int64
	r.tx.QueryRowContext(r.ctx, "SELECT count(*) FROM "+quote(t)).Scan(&n)
	return n
}

// checkObjectRefs fails if an object_ref column of t holds a value that
// is not a stored file's key.
func (r *run) checkObjectRefs(field, t string) error {
	refs, err := queryTxContext(r.ctx, r.tx, `SELECT column_name FROM _jenab_columns WHERE table_name = ? COLLATE NOCASE AND kind = 'object_ref'`,
		[]any{t}, func(rs *sql.Rows) (string, error) {
			var c string
			return c, rs.Scan(&c)
		})
	if err != nil {
		return err
	}
	for _, c := range refs {
		var bad int
		q := fmt.Sprintf(`SELECT count(*) FROM %[2]s WHERE %[1]s IS NOT NULL AND %[1]s NOT IN (SELECT key FROM _jenab_objects)`, quote(c), quote(t))
		if err := r.tx.QueryRowContext(r.ctx, q).Scan(&bad); err != nil {
			if strings.Contains(err.Error(), "no such column") {
				continue // the annotation of a dropped column
			}
			return err
		}
		if bad > 0 {
			return problem(field, "%d values of %s.%s are not keys of stored files; the column holds object references (object_ref)", bad, t, c)
		}
	}
	return nil
}

// rebuild replaces table t with one defined by d, following SQLite's
// documented procedure (SPEC 7.5): create the new table, copy the rows,
// drop the old one, rename the new one, and create the indexes again
// where their columns are still there. Foreign keys are off around the
// transaction, and the schema guard runs foreign_key_check before commit.
// mapping gives a new column's values as an expression over the old
// columns; a new column without one takes the old column of its name, or
// its default.
func (r *run) rebuild(t string, d *tableDef, mapping map[string]string) error {
	old, err := describe(r.ctx, r.tx, t, false)
	if err != nil {
		return err
	}
	type index struct{ name, sql string }
	indexes, err := queryTxContext(r.ctx, r.tx, `SELECT name, sql FROM sqlite_schema WHERE type = 'index' AND tbl_name = ? AND sql IS NOT NULL ORDER BY name`,
		[]any{t}, func(rs *sql.Rows) (index, error) {
			var x index
			return x, rs.Scan(&x.name, &x.sql)
		})
	if err != nil {
		return err
	}
	tmp, err := r.freeName(t + "_rebuild")
	if err != nil {
		return err
	}
	// The table keeps its options; the step's definition has none.
	kept := *d
	kept.strict, kept.withoutRowid = old.Strict, old.WithoutRowid
	if old.Strict {
		r.note("the table stays STRICT")
	}
	if old.WithoutRowid {
		r.note("the table stays WITHOUT ROWID")
	}
	if _, err := r.exec(kept.sql(tmp)); err != nil {
		return err
	}
	if err := r.explainChecks("columns", quote(tmp), d.cols); err != nil {
		return err
	}
	var into, from []string
	for _, c := range d.cols {
		switch e := mapping[c.name]; {
		case e != "":
			into, from = append(into, c.name), append(from, "("+e+"\n)")
		default:
			if oc, ok := old.Column(c.name); ok {
				into, from = append(into, c.name), append(from, quote(oc.Name))
			}
		}
	}
	for _, oc := range old.Columns {
		if !d.has(oc.Name) && !mapsColumn(mapping, oc.Name) {
			r.note("the column %s is dropped", oc.Name)
		}
	}
	if len(into) > 0 {
		query := "SELECT " + strings.Join(from, ", ") + " FROM " + quote(t)
		n, err := r.guarded("mapping", query,
			fmt.Sprintf("INSERT OR ABORT INTO %s (%s) %s", quote(tmp), strings.Join(quoteAll(into), ", "), query),
			r.rows(t))
		if err != nil {
			if strings.Contains(err.Error(), "NOT NULL") {
				return problem("mapping", "%s: give the column a value for the old rows in the mapping", sqliteMsg(err))
			}
			return err
		}
		r.cur.Rows = n
	}
	if _, err := r.exec("DROP TABLE " + quote(t)); err != nil {
		return err
	}
	if _, err := r.exec("ALTER TABLE " + quote(tmp) + " RENAME TO " + quote(t)); err != nil {
		return err
	}
	for _, ix := range indexes {
		if _, err := r.tx.ExecContext(r.ctx, "SAVEPOINT jenab_index"); err != nil {
			return err
		}
		_, err = r.exec(ix.sql)
		if err == nil {
			if _, err := r.tx.ExecContext(r.ctx, "RELEASE jenab_index"); err != nil {
				return err
			}
			continue
		}
		r.cur.SQL = r.cur.SQL[:len(r.cur.SQL)-1]
		if _, rerr := r.tx.ExecContext(r.ctx, "ROLLBACK TO jenab_index"); rerr != nil {
			return errors.Join(err, rerr)
		}
		if _, rerr := r.tx.ExecContext(r.ctx, "RELEASE jenab_index"); rerr != nil {
			return errors.Join(err, rerr)
		}
		var se *sqlite.Error
		if !errors.As(err, &se) || strings.Contains(err.Error(), "UNIQUE") {
			return err
		}
		r.note("the index %s is dropped: %s", ix.name, sqliteMsg(err))
	}
	// A column the mapping copies as it is, under a new name, keeps its
	// annotation.
	for _, c := range d.cols {
		oc, ok := mappedFrom(mapping[c.name], old.Columns)
		if !ok || strings.EqualFold(oc, c.name) {
			continue
		}
		if _, err := r.tx.ExecContext(r.ctx, `INSERT OR IGNORE INTO _jenab_columns (table_name, column_name, kind)
			SELECT table_name, ?, kind FROM _jenab_columns WHERE table_name = ? COLLATE NOCASE AND column_name = ? COLLATE NOCASE`,
			c.name, t, oc); err != nil {
			return err
		}
	}
	if _, err := r.tx.ExecContext(r.ctx, `DELETE FROM _jenab_columns WHERE table_name = ? COLLATE NOCASE
		AND column_name NOT IN (SELECT name FROM pragma_table_xinfo(?))`, t, t); err != nil {
		return err
	}
	if err := r.checkObjectRefs("mapping", t); err != nil {
		return err
	}
	r.mark(t, true)
	r.made[t] = true
	r.written[t] = true
	return nil
}

// dropColumn drops a column with ALTER TABLE where SQLite allows it, after
// the indexes that use it. A column in a UNIQUE constraint or a foreign
// key needs a rebuild, which Go does when it can read the whole table
// definition from the schema; a column of the key needs rebuild_table.
func (r *run) dropColumn(s *step) error {
	t, err := r.table("table", s.table)
	if err != nil {
		return err
	}
	c, err := r.columnOf("column", t, s.column)
	if err != nil {
		return err
	}
	old, err := describe(r.ctx, r.tx, t, false)
	if err != nil {
		return err
	}
	if slices.ContainsFunc(old.Key, func(k string) bool { return strings.EqualFold(k, c) }) {
		return problem("column", "%s is part of the primary key of %s; use rebuild_table with a new key", c, t)
	}
	for _, ix := range old.Indexes {
		if !slices.ContainsFunc(ix.Columns, func(x string) bool { return strings.EqualFold(x, c) }) {
			continue
		}
		var origin string
		if err := r.tx.QueryRowContext(r.ctx, `SELECT origin FROM pragma_index_list(?) WHERE name = ?`, t, ix.Name).Scan(&origin); err != nil {
			return err
		}
		if origin == "c" {
			if _, err := r.exec("DROP INDEX " + quote(ix.Name)); err != nil {
				return err
			}
			r.note("the index %s is dropped with the column", ix.Name)
		}
	}
	if _, err := r.tx.ExecContext(r.ctx, "SAVEPOINT jenab_drop"); err != nil {
		return err
	}
	_, alterErr := r.exec("ALTER TABLE " + quote(t) + " DROP COLUMN " + quote(c))
	if alterErr == nil {
		if _, err := r.tx.ExecContext(r.ctx, "RELEASE jenab_drop"); err != nil {
			return err
		}
	} else {
		r.cur.SQL = r.cur.SQL[:len(r.cur.SQL)-1]
		if _, err := r.tx.ExecContext(r.ctx, "ROLLBACK TO jenab_drop"); err != nil {
			return errors.Join(alterErr, err)
		}
		if _, err := r.tx.ExecContext(r.ctx, "RELEASE jenab_drop"); err != nil {
			return errors.Join(alterErr, err)
		}
		var se *sqlite.Error
		if !errors.As(alterErr, &se) {
			return alterErr
		}
		d, why, err := r.defFromSchema(old)
		if err != nil {
			return err
		}
		if d == nil {
			return problem("column", "SQLite can't drop %s.%s (%s), and Go can't rebuild the table: %s; use rebuild_table with the whole new definition",
				t, c, sqliteMsg(alterErr), why)
		}
		d.cols = slices.DeleteFunc(d.cols, func(x columnDef) bool { return strings.EqualFold(x.name, c) })
		d.unique = slices.DeleteFunc(d.unique, func(u []string) bool {
			if slices.ContainsFunc(u, func(x string) bool { return strings.EqualFold(x, c) }) {
				r.note("the UNIQUE constraint (%s) is dropped with the column", strings.Join(u, ", "))
				return true
			}
			return false
		})
		d.fks = slices.DeleteFunc(d.fks, func(f foreignKeyDef) bool {
			if slices.ContainsFunc(f.cols, func(x string) bool { return strings.EqualFold(x, c) }) {
				r.note("the foreign key (%s) to %s is dropped with the column", strings.Join(f.cols, ", "), f.table)
				return true
			}
			return false
		})
		r.note("SQLite can't drop the column in place (%s), so the table is rebuilt", sqliteMsg(alterErr))
		if err := r.rebuild(t, d, nil); err != nil {
			return err
		}
	}
	if _, err := r.tx.ExecContext(r.ctx, `DELETE FROM _jenab_columns WHERE table_name = ? COLLATE NOCASE AND column_name = ? COLLATE NOCASE`, t, c); err != nil {
		return err
	}
	r.mark(t, true)
	return nil
}

// sqlWords are the keywords and plain names in SQL text, in upper case,
// outside strings, quoted names and comments.
func sqlWords(s string) map[string]bool {
	out := map[string]bool{}
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '\'' || c == '"' || c == '`' || c == '[':
			end := c
			if c == '[' {
				end = ']'
			}
			j := strings.IndexByte(s[i+1:], end)
			if j < 0 {
				return out
			}
			i += j + 2 // a doubled quote is two quoted parts, which is fine here
		case strings.HasPrefix(s[i:], "--"):
			j := strings.IndexByte(s[i:], '\n')
			if j < 0 {
				return out
			}
			i += j
		case strings.HasPrefix(s[i:], "/*"):
			j := strings.Index(s[i+2:], "*/")
			if j < 0 {
				return out
			}
			i += j + 4
		case c == '_' || c >= 0x80 || unicode.IsLetter(rune(c)) || unicode.IsDigit(rune(c)):
			j := i
			for j < len(s) && (s[j] == '_' || s[j] >= 0x80 || unicode.IsLetter(rune(s[j])) || unicode.IsDigit(rune(s[j]))) {
				j++
			}
			out[strings.ToUpper(s[i:j])] = true
			i = j
		default:
			i++
		}
	}
	return out
}

// defFromSchema reads t's definition back from the schema for a rebuild.
// Parts the pragmas don't show, such as CHECK constraints, collations,
// generated and AUTOINCREMENT columns, deferred or MATCH foreign keys and
// ON CONFLICT clauses, would be lost, so a table with any gets no
// definition, and why.
func (r *run) defFromSchema(t Table) (*tableDef, string, error) {
	var text string
	if err := r.tx.QueryRowContext(r.ctx, `SELECT sql FROM sqlite_schema WHERE type = 'table' AND name = ?`, t.Name).Scan(&text); err != nil {
		return nil, "", err
	}
	words := sqlWords(text)
	for _, w := range []string{"CHECK", "COLLATE", "GENERATED", "AUTOINCREMENT", "DEFERRABLE", "MATCH", "CONFLICT", "DESC"} {
		if words[w] {
			return nil, "its definition has " + w + ", which Go can't carry over", nil
		}
	}
	d := &tableDef{key: t.Key, withoutRowid: t.WithoutRowid, strict: t.Strict}
	for _, c := range t.Columns {
		if c.Generated {
			return nil, "it has generated columns", nil
		}
		cd := columnDef{name: c.Name, typ: c.Type, notNull: c.NotNull}
		if c.Default != nil {
			cd.dflt = *c.Default
		}
		d.cols = append(d.cols, cd)
	}
	type part struct {
		index string
		col   sql.NullString
	}
	parts, err := queryTxContext(r.ctx, r.tx, `SELECT l.name, i.name FROM pragma_index_list(?1) l, pragma_index_info(l.name) i
		WHERE l.origin = 'u' ORDER BY l.name, i.seqno`, []any{t.Name}, func(rs *sql.Rows) (part, error) {
		var p part
		return p, rs.Scan(&p.index, &p.col)
	})
	if err != nil {
		return nil, "", err
	}
	for i, p := range parts {
		if i == 0 || p.index != parts[i-1].index {
			d.unique = append(d.unique, nil)
		}
		d.unique[len(d.unique)-1] = append(d.unique[len(d.unique)-1], p.col.String)
	}
	for _, f := range t.ForeignKeys {
		fk := foreignKeyDef{cols: f.Columns, table: f.Table, to: f.To}
		if f.OnDelete != "NO ACTION" {
			fk.onDelete = f.OnDelete
		}
		if f.OnUpdate != "NO ACTION" {
			fk.onUpdate = f.OnUpdate
		}
		d.fks = append(d.fks, fk)
	}
	return d, "", nil
}

// ---- the schema guard ----

type schemaEntry struct{ typ, name, table, sql string }

// internalSchema is every sqlite_schema entry of Jenab's own tables.
func internalSchema(ctx context.Context, tx *sql.Tx) ([]schemaEntry, error) {
	return queryTxContext(ctx, tx, `SELECT type, name, tbl_name, coalesce(sql, '') FROM sqlite_schema
		WHERE lower(name) LIKE '\_jenab\_%' ESCAPE '\' OR lower(tbl_name) LIKE '\_jenab\_%' ESCAPE '\' ORDER BY type, name`, nil,
		func(rs *sql.Rows) (schemaEntry, error) {
			var e schemaEntry
			return e, rs.Scan(&e.typ, &e.name, &e.table, &e.sql)
		})
}

// guard is the schema guard before commit (SPEC 7.5, 2.2): Jenab's own
// entries are as before; there are no triggers, SQL views or virtual
// tables; every agent table has an explicit primary key and at most
// maxColumns columns, as typed writes need (P2-02); every foreign key's
// table exists and foreign_key_check is clean; and the tables the steps
// wrote rows to hold no BLOB in a key.
func (r *run) guard(before []schemaEntry) ([]Problem, error) {
	var probs []Problem
	bad := func(format string, a ...any) {
		probs = append(probs, Problem{Path: "schema", Msg: fmt.Sprintf(format, a...)})
	}
	after, err := internalSchema(r.ctx, r.tx)
	if err != nil {
		return nil, err
	}
	if !slices.Equal(before, after) {
		bad("the migration changed Jenab's internal tables")
	}
	type entry struct{ typ, name string }
	others, err := queryTxContext(r.ctx, r.tx, `SELECT type, name FROM sqlite_schema WHERE type IN ('trigger', 'view')
		UNION ALL SELECT type, name FROM sqlite_temp_schema WHERE type = 'trigger'
		UNION ALL SELECT type, name FROM pragma_table_list WHERE schema = 'main' AND type NOT IN ('table', 'view')
		ORDER BY 2`, nil, func(rs *sql.Rows) (entry, error) {
		var e entry
		return e, rs.Scan(&e.typ, &e.name)
	})
	if err != nil {
		return nil, err
	}
	for _, e := range others {
		bad("%s is a %s; agent tables can't have triggers, SQL views or virtual tables", e.name, e.typ)
	}
	type shape struct {
		name      string
		cols, key int
	}
	shapes, err := queryTxContext(r.ctx, r.tx, `SELECT t.name, count(*), sum(c.pk > 0) FROM pragma_table_list t, pragma_table_xinfo(t.name) c
		WHERE t.schema = 'main' AND t.type = 'table' GROUP BY t.name ORDER BY t.name`, nil, func(rs *sql.Rows) (shape, error) {
		var s shape
		return s, rs.Scan(&s.name, &s.cols, &s.key)
	})
	if err != nil {
		return nil, err
	}
	for _, s := range shapes {
		switch {
		case internalName(s.name):
		case s.key == 0:
			bad("the table %s has no explicit primary key", s.name)
		case s.cols > maxColumns:
			bad("the table %s has %d columns; at most %d are allowed", s.name, s.cols, maxColumns)
		}
	}
	tables, err := listAgentTables(r.ctx, r.tx)
	if err != nil {
		return nil, err
	}
	exists := map[string]string{} // lower-case name → name
	for _, t := range tables {
		exists[strings.ToLower(t)] = t
	}
	type ref struct{ table, parent string }
	refs, err := queryTxContext(r.ctx, r.tx, `SELECT DISTINCT t.name, f."table" FROM pragma_table_list t, pragma_foreign_key_list(t.name) f
		WHERE t.schema = 'main' AND t.type = 'table' ORDER BY 1, 2`, nil, func(rs *sql.Rows) (ref, error) {
		var x ref
		return x, rs.Scan(&x.table, &x.parent)
	})
	if err != nil {
		return nil, err
	}
	// foreign_key_check runs on the tables the migration changed and the
	// tables that point to them.
	// A dependent's Update may write any agent table, so after one the
	// check runs on them all.
	check := map[string]bool{}
	for t := range r.marks {
		if n, ok := exists[strings.ToLower(t)]; ok {
			check[n] = true
		}
	}
	if len(r.rep.Updated) > 0 {
		for _, n := range exists {
			check[n] = true
		}
	}
	var missing []ref
	for _, x := range refs {
		if internalName(x.table) {
			continue
		}
		parent, ok := exists[strings.ToLower(x.parent)]
		switch {
		case internalName(x.parent):
			bad("the table %s has a foreign key to %s, one of Jenab's own tables; foreign keys can only point to agent tables", x.table, x.parent)
		case !ok:
			missing = append(missing, x)
			bad("the table %s has a foreign key to %s, which doesn't exist; drop or rebuild %s first", x.table, x.parent, x.table)
		case check[parent]:
			check[x.table] = true
		}
	}
	type broken struct {
		table, parent string
		n             int
	}
	for _, t := range sortedKeys(check) {
		fk, err := queryTxContext(r.ctx, r.tx, `SELECT "table", parent, count(*) FROM pragma_foreign_key_check(?) GROUP BY 1, 2 ORDER BY 1, 2`, []any{t},
			func(rs *sql.Rows) (broken, error) {
				var b broken
				return b, rs.Scan(&b.table, &b.parent, &b.n)
			})
		if err != nil {
			var se *sqlite.Error
			if !errors.As(err, &se) {
				return nil, err
			}
			bad("%s", sqliteMsg(err)) // e.g. a foreign key mismatch: its columns are not a key of the other table
		}
		for _, b := range fk {
			if slices.ContainsFunc(missing, func(m ref) bool {
				return strings.EqualFold(m.table, b.table) && strings.EqualFold(m.parent, b.parent)
			}) {
				continue
			}
			bad("%d rows of %s refer to rows of %s that don't exist (foreign key)", b.n, b.table, b.parent)
		}
	}
	// Steps make no generated or hidden columns.
	for _, t := range sortedKeys(r.made) {
		n, ok := exists[strings.ToLower(t)]
		if !ok {
			continue
		}
		var hidden int
		if err := r.tx.QueryRowContext(r.ctx, `SELECT count(*) FROM pragma_table_xinfo(?) WHERE hidden <> 0`, n).Scan(&hidden); err != nil {
			return nil, err
		}
		if hidden > 0 {
			bad("the table %s has generated or hidden columns", n)
		}
	}
	for _, t := range sortedKeys(r.written) {
		tab, err := loadTable(r.tx, t)
		if err != nil {
			continue // dropped or not an agent table, reported above
		}
		var n int
		if err := r.tx.QueryRowContext(r.ctx, fmt.Sprintf("SELECT count(*) FROM %s WHERE %s", quote(tab.name), tab.blobKeyAs(""))).Scan(&n); err != nil {
			return nil, err
		}
		if n > 0 {
			bad("%d rows of %s have a BLOB in their key; keys are text or numbers", n, tab.name)
		}
	}
	return probs, nil
}

// metaSchemaVersion is the _jenab_meta key of the schema version (SPEC
// 2.2), which every migration raises by one.
const metaSchemaVersion = "schema_version"

func bumpSchemaVersion(tx *sql.Tx) (int, error) {
	var v int
	err := tx.QueryRow(`INSERT INTO _jenab_meta (key, value) VALUES (?, '1')
		ON CONFLICT (key) DO UPDATE SET value = CAST(value AS INTEGER) + 1 RETURNING CAST(value AS INTEGER)`, metaSchemaVersion).Scan(&v)
	return v, err
}

// SchemaVersion is the project's schema version: how many migrations ran.
func (p *ProjectDB) SchemaVersion(ctx context.Context) (int, error) {
	m, err := p.Meta(ctx)
	if err != nil {
		return 0, err
	}
	var v int
	fmt.Sscan(m[metaSchemaVersion], &v)
	return v, nil
}
