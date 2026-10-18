package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// maxCascade is the most row changes the foreign-key actions of one
// write, a chunk in one transaction, may make: each time an action of one
// foreign key reaches a row counts, so a row that 30 foreign keys reach
// counts 30 times, as SQLite does the work 30 times. Each one is read
// before the write and logged after it. At the cap, a delete that cascades
// took 244 ms, one that sets NULL 325 ms and an update that cascades 407
// ms, growing in step with the rows, on a machine at 94% load; 30 foreign
// keys reaching 333 rows each took 135 ms at 23% load (TestCascadeSpeed).
// A write past it is refused before anything changes.
const maxCascade = 10000

// foreignKey is a foreign key of table child that references parent.
type foreignKey struct {
	child              string
	from, to           []string // child columns; the parent columns they match
	onUpdate, onDelete string
}

// changes reports whether action, one of a foreign key's ON UPDATE and ON
// DELETE actions, changes child rows. RESTRICT and NO ACTION only refuse.
func changes(action string) bool {
	return action == "CASCADE" || action == "SET NULL" || action == "SET DEFAULT"
}

// cascade is what foreign-key actions may do to other rows when a write
// deletes rows or changes their columns: every row they can reach, with its
// before-image, found before the write. After the write, log keeps the
// ones that were deleted or changed (SPEC 2.5).
type cascade struct {
	tables map[string]*agentTable // by lower-case name
	fks    map[string][]foreignKey
	found  []reached
	at     map[string]int      // table and key: the index in found
	walked map[string]bool     // table, key and what changed: followed once
	edges  map[string][]string // table and key: the rows an action reaches from it
	edge   map[string]bool
}

// reached is a row an action can reach. level is the length of the
// longest chain of actions from a written row to it; gone is set when an
// ON DELETE CASCADE reaches it, so it is surely deleted.
type reached struct {
	tab         *agentTable
	key, before string
	level       int
	gone        bool
}

// root is rows a write deletes, with changed nil, or whose columns changed
// it gives new values: keys is a JSON array of their row keys.
type root struct {
	keys    string
	changed []string
}

// reach finds the rows that foreign-key actions may change when a write
// deletes or changes the rows of tab in roots. It follows chains: a
// cascaded delete deletes further, and SET NULL, SET DEFAULT and ON UPDATE
// CASCADE change the child's columns, which other keys may reference. It
// returns nil if no action can reach any row, and fails if the actions
// would make more than maxCascade row changes, or change a child's key.
func reach(tx *sql.Tx, tab *agentTable, roots ...root) (*cascade, error) {
	if len(roots) == 0 {
		return nil, nil
	}
	c := &cascade{tables: map[string]*agentTable{strings.ToLower(tab.name): tab}, fks: map[string][]foreignKey{},
		at: map[string]int{}, walked: map[string]bool{}, edges: map[string][]string{}, edge: map[string]bool{}}
	writing := "deleting"
	if roots[0].changed != nil {
		writing = "changing"
	}
	type step struct {
		tab     *agentTable
		keys    string
		changed []string
	}
	var steps []step
	for _, r := range roots {
		steps = append(steps, step{tab, r.keys, r.changed})
	}
	reads := 0
	for level := 1; len(steps) > 0; level++ {
		var next []step
		for _, s := range steps {
			fks, err := c.foreignKeysTo(tx, s.tab)
			if err != nil {
				return nil, err
			}
			for _, fk := range fks {
				on, action := "DELETE", fk.onDelete
				if s.changed != nil {
					on, action = "UPDATE", fk.onUpdate
					if !slices.ContainsFunc(fk.to, func(col string) bool { return slices.Contains(s.changed, col) }) {
						continue
					}
				}
				if !changes(action) {
					continue
				}
				child, err := c.table(tx, fk.child)
				if err != nil {
					return nil, fmt.Errorf("store: rows of %s change with those of %s (ON %s %s): %w", fk.child, s.tab.name, on, action, err)
				}
				from := make([]string, len(fk.from))
				for i, col := range fk.from {
					if from[i], err = child.column(col); err != nil {
						return nil, err
					}
				}
				gone := s.changed == nil && action == "CASCADE"
				childChanged := from // SET NULL, SET DEFAULT and ON UPDATE CASCADE
				if gone {
					childChanged = nil
				}
				// Rows keyed by the columns the action sets would get a new
				// key, or a NULL one.
				keyed := !gone && slices.ContainsFunc(from, child.isKey)
				// The parent's column is on the left of each match, so its
				// collation and affinity decide, as they do for SQLite's
				// own actions.
				match := make([]string, len(from))
				for i := range from {
					match[i] = "_jenab_p." + quote(fk.to[i]) + " = _jenab_c." + quote(from[i])
				}
				body := fmt.Sprintf("FROM %s AS _jenab_p JOIN %s AS _jenab_c ON %s WHERE %s", quote(s.tab.name), quote(child.name),
					strings.Join(match, " AND "), s.tab.keyMatchAs("_jenab_p"))
				var follow []string
				err = func() error {
					rows, err := tx.Query(fmt.Sprintf("SELECT %s, %s, %s, %s %s", s.tab.keyExprAs("_jenab_p"), child.keyExprAs("_jenab_c"),
						child.beforeExprAs("_jenab_c"), child.blobKeyAs("_jenab_c"), body), s.keys)
					if err != nil {
						return err
					}
					defer rows.Close()
					for rows.Next() {
						var parent, key, before string
						var blob bool
						if err := rows.Scan(&parent, &key, &before, &blob); err != nil {
							return err
						}
						if blob {
							return fault("store: rows of %s change with those of %s (ON %s %s), and one has a BLOB in its key, which the change log can't hold",
								child.name, s.tab.name, on, action)
						}
						if keyed {
							return fault("store: rows of %s change with those of %s (ON %s %s), and the action changes their primary key, by which the change log knows them",
								child.name, s.tab.name, on, action)
						}
						if reads++; reads > maxCascade {
							rows.Close()
							var n int
							if err := tx.QueryRow(fmt.Sprintf("SELECT count(*) FROM (SELECT DISTINCT %s %s)", child.keyExprAs("_jenab_c"), body),
								s.keys).Scan(&n); err != nil {
								return err
							}
							what := "change"
							if gone {
								what = "delete"
							}
							return fault("store: %s these rows would also %s %d rows in %s (ON %s %s), past the %d row changes that foreign keys may make in one write; %s them first in smaller batches",
								writing, what, n, child.name, on, action, maxCascade, what)
						}
						node := child.name + "\x00" + key
						c.link(s.tab.name+"\x00"+parent, node)
						if i, ok := c.at[node]; ok {
							c.found[i].level = level // found deeper: levels sets it after the walk
							c.found[i].gone = c.found[i].gone || gone
						} else {
							c.at[node] = len(c.found)
							c.found = append(c.found, reached{tab: child, key: key, before: before, level: level, gone: gone})
						}
						w := node + "\x00" + strings.Join(childChanged, "\x00")
						if gone {
							w += "\x00delete"
						}
						if !c.walked[w] {
							c.walked[w] = true
							follow = append(follow, key)
						}
					}
					return rows.Err()
				}()
				if err != nil {
					return nil, err
				}
				if len(follow) > 0 {
					next = append(next, step{child, "[" + strings.Join(follow, ",") + "]", childChanged})
				}
			}
		}
		steps = next
	}
	if len(c.found) == 0 {
		return nil, nil
	}
	c.levels()
	return c, nil
}

// link notes that an action on the row from reaches the row to.
func (c *cascade) link(from, to string) {
	if from == to || c.edge[from+"\x00\x00"+to] {
		return
	}
	c.edge[from+"\x00\x00"+to] = true
	c.edges[from] = append(c.edges[from], to)
}

// levels sets each found row's level to the longest chain of actions that
// reaches it, so every row is deeper than each row it was reached from,
// however many ways lead to it. A row on a cycle of foreign keys keeps the
// deepest level the walk found it at; no order puts a cycle back row by
// row, so undo runs with PRAGMA defer_foreign_keys=ON (P2-03).
func (c *cascade) levels() {
	in := map[string]int{}
	for from, tos := range c.edges {
		in[from] += 0
		for _, to := range tos {
			in[to]++
		}
	}
	var ready []string
	for n, d := range in {
		if d == 0 {
			ready = append(ready, n)
		}
	}
	level := map[string]int{}
	for len(ready) > 0 {
		n := ready[len(ready)-1]
		ready = ready[:len(ready)-1]
		for _, to := range c.edges[n] {
			level[to] = max(level[to], level[n]+1)
			if in[to]--; in[to] == 0 {
				ready = append(ready, to)
			}
		}
	}
	for i, r := range c.found {
		if n := r.tab.name + "\x00" + r.key; in[n] == 0 {
			c.found[i].level = level[n]
		}
	}
}

// table loads an agent table once per cascade. A table the change log
// can't key, such as one without a primary key, is refused.
func (c *cascade) table(tx *sql.Tx, name string) (*agentTable, error) {
	if t, ok := c.tables[strings.ToLower(name)]; ok {
		return t, nil
	}
	t, err := loadTable(tx, name)
	if err != nil {
		return nil, err
	}
	c.tables[strings.ToLower(name)] = t
	return t, nil
}

// foreignKeysTo returns the foreign keys of any table that reference
// parent, with the parent columns as the schema names them.
func (c *cascade) foreignKeysTo(tx *sql.Tx, parent *agentTable) ([]foreignKey, error) {
	if fks, ok := c.fks[parent.name]; ok {
		return fks, nil
	}
	type part struct {
		child              string
		id                 int
		from               string
		to                 sql.NullString
		onUpdate, onDelete string
	}
	parts, err := queryTx(tx, `SELECT m.name, f.id, f."from", f."to", f.on_update, f.on_delete
		FROM pragma_table_list m, pragma_foreign_key_list(m.name) f
		WHERE m.schema = 'main' AND m.type = 'table' AND f."table" = ? COLLATE NOCASE ORDER BY m.name, f.id, f.seq`,
		[]any{parent.name}, func(r *sql.Rows) (part, error) {
			var p part
			return p, r.Scan(&p.child, &p.id, &p.from, &p.to, &p.onUpdate, &p.onDelete)
		})
	if err != nil {
		return nil, err
	}
	var fks []foreignKey
	for i, p := range parts {
		if i == 0 || p.child != parts[i-1].child || p.id != parts[i-1].id {
			fks = append(fks, foreignKey{child: p.child, onUpdate: p.onUpdate, onDelete: p.onDelete})
		}
		fk := &fks[len(fks)-1]
		fk.from = append(fk.from, p.from)
		if p.to.Valid && p.to.String != "" {
			to, err := parent.column(p.to.String)
			if err != nil {
				return nil, err
			}
			fk.to = append(fk.to, to)
		}
	}
	for i := range fks {
		if len(fks[i].to) == 0 { // REFERENCES parent: its primary key
			fks[i].to = parent.pk
		}
	}
	c.fks[parent.name] = fks
	return fks, nil
}

// cascadeOf is reach for the rows of tab with the given keys, by row key;
// nil if there are none.
func cascadeOf(tx *sql.Tx, tab *agentTable, keys []string, changed []string) (*cascade, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	return reach(tx, tab, root{"[" + strings.Join(keys, ",") + "]", changed})
}

// given is a row a write sets values of: its key, and its values in the
// columns the write names, with has false for one it leaves alone.
type given struct {
	Key  json.RawMessage `json:"k"` // a JSON array
	Vals []any           `json:"v"`
	Has  []bool          `json:"h"`
	Blob []bool          `json:"b"` // set by changing
}

// changing returns the rows of t that a write really changes in cols, as
// roots, one for each set of columns that changes. A value is compared
// with the stored one after the column's affinity, byte for byte, so a
// change that only a collation hides still counts: walking too far is
// safe, missing a row is not.
func (t *agentTable) changing(tx *sql.Tx, cols []string, rows []given) ([]root, error) {
	if len(rows) == 0 || len(cols) == 0 {
		return nil, nil
	}
	// Only columns an ON UPDATE action follows matter; most tables have none.
	fks, err := (&cascade{fks: map[string][]foreignKey{}}).foreignKeysTo(tx, t)
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(cols, func(c string) bool {
		return slices.ContainsFunc(fks, func(fk foreignKey) bool { return changes(fk.onUpdate) && slices.Contains(fk.to, c) })
	}) {
		return nil, nil
	}
	// JSON holds a BLOB as base64 text, which can't be compared with the
	// stored value, so a BLOB counts as a change.
	for i, r := range rows {
		rows[i].Blob = make([]bool, len(r.Vals))
		for j, v := range r.Vals {
			_, rows[i].Blob[j] = v.([]byte)
		}
	}
	b, err := json.Marshal(rows)
	if err != nil {
		return nil, err
	}
	keyOf := make([]string, len(t.pk))
	for i := range t.pk {
		keyOf[i] = fmt.Sprintf("_jenab_r.value->'k'->>%d", i)
	}
	diff := make([]string, len(cols))
	for i, c := range cols {
		diff[i] = fmt.Sprintf("coalesce(_jenab_r.value->'h'->>%d AND (_jenab_r.value->'b'->>%d OR %s IS NOT _jenab_r.value->'v'->>%d COLLATE BINARY), 0)",
			i, i, "_jenab_t."+quote(c), i)
	}
	type row struct {
		key  string
		diff []bool
	}
	found, err := queryTx(tx, fmt.Sprintf("SELECT %s, %s FROM json_each(?) _jenab_r JOIN %s AS _jenab_t ON (%s) = (%s)",
		t.keyExprAs("_jenab_t"), strings.Join(diff, ", "), quote(t.name), strings.Join(qualified("_jenab_t", t.pk), ", "), strings.Join(keyOf, ", ")), []any{string(b)}, func(r *sql.Rows) (row, error) {
		x := row{diff: make([]bool, len(cols))}
		dst := []any{&x.key}
		for i := range x.diff {
			dst = append(dst, &x.diff[i])
		}
		return x, r.Scan(dst...)
	})
	if err != nil {
		return nil, err
	}
	groups := map[string]*root{}
	var keys []string // of groups, in the order found
	for _, f := range found {
		var changed []string
		for i, d := range f.diff {
			if d {
				changed = append(changed, cols[i])
			}
		}
		if len(changed) == 0 {
			continue
		}
		sig := strings.Join(changed, "\x00")
		g := groups[sig]
		if g == nil {
			g = &root{changed: changed}
			groups[sig] = g
			keys = append(keys, sig)
		}
		if g.keys == "" {
			g.keys = "[" + f.key
		} else {
			g.keys += "," + f.key
		}
	}
	out := make([]root, len(keys))
	for i, k := range keys {
		out[i] = *groups[k]
		out[i].keys += "]"
	}
	return out, nil
}

// log adds the rows the actions deleted or changed to l, after the write.
// They go before the write's own rows, deepest first, so that undo, going
// backwards, puts a parent back before its children. A row of the write
// that an action also reaches, in a table that references itself, moves to
// its level too. Only rows an action may have changed are read again; one
// an ON DELETE CASCADE reached is gone.
func (c *cascade) log(tx *sql.Tx, l *chunkLog) error {
	if c == nil {
		return nil
	}
	own := map[string]int{}
	for i, e := range l.rows {
		own[e.table+"\x00"+e.key] = i
	}
	byTable := map[*agentTable][]string{}
	for _, r := range c.found {
		if _, ok := own[r.tab.name+"\x00"+r.key]; !ok && !r.gone {
			byTable[r.tab] = append(byTable[r.tab], r.key)
		}
	}
	now := map[string]string{}
	for t, keys := range byTable {
		cur, err := t.befores(tx, "["+strings.Join(keys, ",")+"]")
		if err != nil {
			return err
		}
		for k, b := range cur {
			now[t.name+"\x00"+k] = b
		}
	}
	type leveled struct {
		e     rowEntry
		level int
	}
	var out []leveled
	moved := map[int]bool{}
	for _, r := range c.found {
		k := r.tab.name + "\x00" + r.key
		if i, ok := own[k]; ok {
			out = append(out, leveled{l.rows[i], r.level})
			moved[i] = true
			continue
		}
		e := rowEntry{table: r.tab.name, key: r.key, before: sql.NullString{String: r.before, Valid: true}}
		b, ok := now[k]
		switch {
		case r.gone, !ok:
			e.kind = rowDeleted
		case b != r.before:
			e.kind = rowUpdated
		default:
			continue // reachable, but the action left it as it was
		}
		l.seen[k] = true
		l.res.Cascaded++
		out = append(out, leveled{e, r.level})
	}
	slices.SortStableFunc(out, func(a, b leveled) int { return b.level - a.level })
	for _, o := range out {
		l.cascade = append(l.cascade, o.e)
	}
	rest := l.rows[:0]
	for i, e := range l.rows {
		if !moved[i] {
			rest = append(rest, e)
		}
	}
	l.rows = rest
	return nil
}
