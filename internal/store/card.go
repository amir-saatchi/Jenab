package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The project card (SPEC 3.2), block 4 of the context: built by the app
// from the project's state, never written by the agent. Phase 2 has the
// tables and the bucket; views, pages, pipelines, connections, MCP servers
// and the workspace join in their phases, each as its own section.

// CardTokens is the card's size limit. A larger card lists table names
// only, and the agent reads columns with describe_table.
const CardTokens = 2000

// cardScanRows is the largest table whose date columns the card reads
// without an index: min and max of other columns scan the table.
const cardScanRows = 100_000

// cardScanCols is how many date columns one scan reads.
const cardScanCols = 500

// cardFolders is how many of the bucket's top-level prefixes the card
// names, and cardBucketBytes about the most its bucket line takes, so the
// line fits in the card even with long prefixes.
const (
	cardFolders     = 20
	cardBucketBytes = 1200
)

// Card builds the project card as of at. Counts and date ranges are read
// in one transaction, so they agree with each other.
func (p *ProjectDB) Card(ctx context.Context, at time.Time) (string, error) {
	return readTx(ctx, p.DB, func(tx *sql.Tx) (string, error) {
		var name string
		err := tx.QueryRowContext(ctx, `SELECT value FROM _jenab_meta WHERE key = 'name'`).Scan(&name)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		names, err := listAgentTables(ctx, tx)
		if err != nil {
			return "", err
		}
		slices.SortFunc(names, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
		lines := make([]string, 0, len(names))
		for _, n := range names {
			t, err := describe(ctx, tx, n, true)
			if err != nil {
				return "", err
			}
			ranges, err := dateRanges(ctx, tx, &t)
			if err != nil {
				return "", err
			}
			lines = append(lines, cardTable(t, ranges))
		}
		bucket, err := cardBucket(ctx, tx)
		if err != nil {
			return "", err
		}
		head := "Project card"
		if name != "" {
			head += " of " + quoteName(name)
		}
		head += ", as of " + at.Format("2006-01-02 15:04") + " (UTC" + at.Format("-07:00") + "). " +
			"Row counts and date ranges are from then; query for numbers now."
		full := head + "\n" + cardTables(lines) + "\n" + bucket
		if len(full)/4 < CardTokens {
			return full, nil
		}
		return head + "\n" + cardNames(names, max(CardTokens*4-len(head)-len(bucket)-2, 0)) + "\n" + bucket, nil
	})
}

// quoteName is the project's name in double quotes, as it is: only quotes
// and line breaks are escaped, so marks such as ZWNJ in a Persian name
// stay. A name past MaxTitle characters is cut, so the card's size
// doesn't hang on it.
func quoteName(s string) string {
	if t := truncateRunes(s, MaxTitle); t != s {
		s = t + "…"
	}
	return `"` + strings.NewReplacer(`"`, `\"`, "\r", `\r`, "\n", `\n`).Replace(s) + `"`
}

func cardTables(lines []string) string {
	if len(lines) == 0 {
		return "Tables: none yet."
	}
	return "Tables:\n" + strings.Join(lines, "\n")
}

// cardNames is the short form of the tables: their names, as many as fit
// in room bytes.
func cardNames(names []string, room int) string {
	head := fmt.Sprintf("Tables (%d; the card has no room for their columns, so use describe_table for a table's columns, key and rows): ", len(names))
	room -= len(head) + 40 // the "and N more" tail
	var b strings.Builder
	for i, n := range names {
		if b.Len()+len(n)+2 > room {
			if i == 0 {
				b.WriteString("no room to list them")
			} else {
				fmt.Fprintf(&b, ", and %d more", len(names)-i)
			}
			break
		}
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(n)
	}
	return head + b.String() + "."
}

// dateRange is the least and the greatest value of a date column.
type dateRange struct{ col, min, max string }

// cardTable is a table on one line: rows, columns with types, the key,
// unique columns, references, and the range of each date column.
func cardTable(t Table, ranges []dateRange) string {
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
	s := fmt.Sprintf("- %s (%s): %s", t.Name, countText(t.Rows, "row", "rows"), strings.Join(cols, ", "))
	if len(t.Key) > 0 {
		s += "; key (" + strings.Join(t.Key, ", ") + ")"
	}
	for _, ix := range t.Indexes {
		if ix.Unique && !ix.Partial && !slices.Contains(ix.Columns, "") {
			s += "; unique (" + strings.Join(ix.Columns, ", ") + ")"
		}
	}
	for _, fk := range t.ForeignKeys {
		s += "; (" + strings.Join(fk.Columns, ", ") + ") references " + fk.Table
		if len(fk.To) > 0 {
			s += " (" + strings.Join(fk.To, ", ") + ")"
		}
	}
	for _, r := range ranges {
		if r.min == r.max {
			s += "; " + r.col + " " + r.min
		} else {
			s += "; " + r.col + " " + r.min + " to " + r.max
		}
	}
	return s
}

// dateRanges finds t's date columns and their ranges. A date column is
// TEXT, has no type or a DATE or TIME type, and its least and greatest
// values are dates. In a table of up to cardScanRows rows one scan reads
// them all; in a larger one only columns that lead an index are read,
// from the index's ends.
func dateRanges(ctx context.Context, tx *sql.Tx, t *Table) ([]dateRange, error) {
	if t.Rows == 0 {
		return nil, nil
	}
	var cands []string
	for _, c := range t.Columns {
		a, d := t.affinityOf(&c), strings.ToUpper(c.Type)
		dated := a == "TEXT" || a == "" && !strings.Contains(d, "BLOB") ||
			a == "NUMERIC" && (strings.Contains(d, "DATE") || strings.Contains(d, "TIME"))
		if dated && (t.Rows <= cardScanRows || t.leads(c.Name)) {
			cands = append(cands, c.Name)
		}
	}
	if len(cands) == 0 {
		return nil, nil
	}
	vals := make([]sql.NullString, 2*len(cands)) // min and max of each
	if t.Rows <= cardScanRows {
		// One scan reads at most cardScanCols columns: a result has at most 2,000
		// columns (SQLITE_MAX_COLUMN).
		for lo := 0; lo < len(cands); lo += cardScanCols {
			hi := min(lo+cardScanCols, len(cands))
			aggs := make([]string, 0, 2*(hi-lo))
			dest := make([]any, 0, 2*(hi-lo))
			for i := lo; i < hi; i++ {
				aggs = append(aggs, "min("+quote(cands[i])+")", "max("+quote(cands[i])+")")
				dest = append(dest, &vals[2*i], &vals[2*i+1])
			}
			q := fmt.Sprintf("SELECT %s FROM %s", strings.Join(aggs, ", "), quote(t.Name))
			if err := tx.QueryRowContext(ctx, q).Scan(dest...); err != nil {
				return nil, err
			}
		}
	} else {
		// One aggregate per query, so SQLite reads the index's ends
		// instead of the table.
		for i, c := range cands {
			for k, f := range []string{"min", "max"} {
				q := fmt.Sprintf("SELECT %s(%s) FROM %s", f, quote(c), quote(t.Name))
				if err := tx.QueryRowContext(ctx, q).Scan(&vals[2*i+k]); err != nil {
					return nil, err
				}
			}
		}
	}
	var out []dateRange
	for i, c := range cands {
		lo, hi := vals[2*i], vals[2*i+1]
		if lo.Valid && hi.Valid && isDate(lo.String) && isDate(hi.String) {
			out = append(out, dateRange{col: c, min: lo.String, max: hi.String})
		}
	}
	return out, nil
}

// leads reports whether column c comes first in t's key or in an index,
// so min and max of it read the index.
func (t *Table) leads(c string) bool {
	if len(t.Key) > 0 && strings.EqualFold(t.Key[0], c) {
		return true
	}
	return slices.ContainsFunc(t.Indexes, func(ix Index) bool {
		return !ix.Partial && len(ix.Columns) > 0 && strings.EqualFold(ix.Columns[0], c)
	})
}

// isDate reports whether s is a date, or a date and a time, as SQLite
// writes them. A time alone is not a date.
func isDate(s string) bool {
	return len(s) >= len("2006-01-02") && s[4] == '-' && isDateTime(s)
}

// cardBucket is the bucket's line: its top-level prefixes with their
// object counts, the objects at the top, and the saved links once the
// project has them (SPEC 4.6).
func cardBucket(ctx context.Context, tx *sql.Tx) (string, error) {
	type folder struct {
		name string
		n    int
	}
	fs, err := queryTxContext(ctx, tx, `SELECT CASE WHEN instr(key, '/') > 0 THEN substr(key, 1, instr(key, '/')) ELSE '' END AS top, count(*)
		FROM _jenab_objects GROUP BY top ORDER BY top`, nil, func(r *sql.Rows) (folder, error) {
		var f folder
		return f, r.Scan(&f.name, &f.n)
	})
	if err != nil {
		return "", err
	}
	loose := 0
	if len(fs) > 0 && fs[0].name == "" { // '' sorts first
		loose, fs = fs[0].n, fs[1:]
	}
	// Prefixes past cardFolders, or past the line's budget, are counted
	// instead of named.
	var parts []string
	size, left := 0, 0
	for _, f := range fs {
		part := f.name + " " + countText(f.n, "object", "objects")
		if len(parts) == cardFolders || size+len(part)+2 > cardBucketBytes-100 { // 100 for the rest of the line
			left++
			continue
		}
		parts = append(parts, part)
		size += len(part) + 2
	}
	if left > 0 {
		parts = append(parts, "and "+countText(left, "more prefix", "more prefixes"))
	}
	if loose > 0 {
		parts = append(parts, countText(loose, "object", "objects")+" at the top")
	}
	s := "Bucket: empty."
	if len(parts) > 0 {
		s = "Bucket: " + strings.Join(parts, ", ") + "."
	}
	var links int
	err = tx.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = '_jenab_links'`).Scan(&links)
	if err != nil || links == 0 {
		return s, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM _jenab_links`).Scan(&links); err != nil {
		return "", err
	}
	return s + " Saved links: " + strconv.Itoa(links) + ".", nil
}

// countText is n with its noun, with thousands separators: "1 row",
// "12,345 rows".
func countText(n int, one, many string) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if n == 1 {
		return s + " " + one
	}
	return s + " " + many
}
