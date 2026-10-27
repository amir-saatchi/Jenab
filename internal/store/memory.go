package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
)

// MemoryScope is user memory or project memory (SPEC 3.3).
type MemoryScope string

const (
	ScopeUser    MemoryScope = "user"    // all projects, in registry.db
	ScopeProject MemoryScope = "project" // all chats of one project, in project.db
)

// Cap is the scope's size cap in tokens, counted as bytes/4 like
// provider.EstimateTokens (SPEC 3.3).
func (s MemoryScope) Cap() int {
	if s == ScopeUser {
		return 1000
	}
	return 2000
}

// DefaultSections are the sections every memory starts with; the agent may
// add more (SPEC 3.3).
var DefaultSections = []string{"goals", "preferences", "decisions", "conventions"}

// MemorySection is one section of a memory. A deleted section keeps its
// row with Content "", so its revision goes on counting (a tombstone).
type MemorySection struct {
	Scope     MemoryScope `json:"scope"`
	Section   string      `json:"section"`
	Content   string      `json:"content"`
	Revision  int         `json:"revision"`
	Source    id.Source   `json:"source"` // who wrote it last
	UpdatedAt time.Time   `json:"updated_at"`
}

// Target is the section's change-log target, memory:<scope>/<section>.
func (m MemorySection) Target() string { return MemoryTarget(m.Scope, m.Section) }

// MemoryTarget is a section's change-log target (SPEC 2.5).
func MemoryTarget(scope MemoryScope, section string) string {
	return "memory:" + string(scope) + "/" + section
}

// MemoryEdit is an edit of one section: Content replaces it, "" deletes
// it. Expected is the revision the editor read; 0 for a section that
// isn't there.
type MemoryEdit struct {
	Section  string
	Content  string
	Expected int
	Source   id.Source
}

// MemoryWrite says what an edit did.
type MemoryWrite struct {
	Section MemorySection `json:"section"` // the section after it
	Op      string        `json:"op"`      // save or delete; "" when it changed nothing
	// Change is the edit's change: a project change ID, or for user
	// memory the number of its entry in registry.db's log.
	Change string `json:"change"`
}

// MemoryConflictError is an edit whose expected revision is stale. Current
// is the section as it is now (Content "" if it isn't there), so the
// editor can merge (SPEC 3.3).
type MemoryConflictError struct{ Current MemorySection }

func (e *MemoryConflictError) Error() string {
	return fmt.Sprintf("store: %s is at revision %d", e.Current.Target(), e.Current.Revision)
}

func (e *MemoryConflictError) Is(target error) bool { return target == ErrConflict }

// MemoryCapError is an edit that would take a memory over its cap.
type MemoryCapError struct {
	Scope       MemoryScope
	Tokens, Cap int
}

func (e *MemoryCapError) Error() string {
	return fmt.Sprintf("store: %s memory would be %d tokens, over its cap of %d", e.Scope, e.Tokens, e.Cap)
}

func (e *MemoryCapError) Is(target error) bool { return target == ErrInvalid }

// MemoryTokens is the size of a memory's sections as the cap counts it:
// bytes/4 of each live section's name and content.
func MemoryTokens(ss []MemorySection) int {
	n := 0
	for _, s := range ss {
		if s.Content != "" {
			n += len(s.Section) + len(s.Content)
		}
	}
	return (n + 3) / 4
}

// sectionName is a section's name: letters, digits, _ and -, with the
// marks and ZWNJ of Persian words, starting with a letter or digit and
// ending with one or a mark.
var sectionName = regexp.MustCompile(`^[\p{L}\p{N}](?:[\p{L}\p{N}\p{Mn}_\x{200C}-]*[\p{L}\p{N}\p{Mn}])?$`)

// sectionLetters maps the Arabic yeh and kaf to the Persian ones, so names
// typed on either keyboard are one section.
var sectionLetters = strings.NewReplacer("ي", "ی", "ك", "ک")

// QuoteMemory marks each line of memory content that looks like a line of
// the memory blocks or a notice, "[…]", with a backslash, so content can't
// pass for a section's header or a notice. A line marked so already gets
// one more. UnquoteMemory takes one off, so text a model copies back from
// what it was shown is stored as it was.
func QuoteMemory(s string) string {
	return bracketLines(s, func(ws, rest string) string { return ws + `\` + rest })
}

// UnquoteMemory undoes QuoteMemory.
func UnquoteMemory(s string) string {
	return bracketLines(s, func(ws, rest string) string { return ws + strings.TrimPrefix(rest, `\`) })
}

// bracketLines maps each line that, past its leading space and
// backslashes, is "[…]", by f on its leading space and the rest.
func bracketLines(s string, f func(ws, rest string) string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		rest := strings.TrimLeftFunc(l, unicode.IsSpace)
		t := strings.TrimRightFunc(strings.TrimLeft(rest, `\`), unicode.IsSpace)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			lines[i] = f(l[:len(l)-len(rest)], rest)
		}
	}
	return strings.Join(lines, "\n")
}

// NormalizeSection checks a section name, up to 40 characters, and returns
// it in NFC and lower case, with Persian yeh and kaf.
func NormalizeSection(s string) (string, error) {
	s = sectionLetters.Replace(strings.ToLower(norm.NFC.String(strings.TrimSpace(s))))
	if utf8.RuneCountInString(s) > 40 || !sectionName.MatchString(s) {
		return "", fault("%w: a section name is 1 to 40 letters, digits, _ or -, starting and ending with a letter or digit", ErrInvalid)
	}
	return s, nil
}

// SortMemory orders sections: the default ones first, in their order,
// then the others by name.
func SortMemory(ss []MemorySection) {
	slices.SortFunc(ss, func(a, b MemorySection) int {
		ia, ib := slices.Index(DefaultSections, a.Section), slices.Index(DefaultSections, b.Section)
		switch {
		case ia >= 0 && ib >= 0:
			return ia - ib
		case ia >= 0:
			return -1
		case ib >= 0:
			return 1
		}
		return strings.Compare(a.Section, b.Section)
	})
}

// memorySQL is the shape both memory tables share.
const memoryCols = "section, content, revision, source, updated_at"

func scanMemory(scope MemoryScope) func(*sql.Rows) (MemorySection, error) {
	return func(r *sql.Rows) (MemorySection, error) {
		m := MemorySection{Scope: scope}
		var at string
		if err := r.Scan(&m.Section, &m.Content, &m.Revision, &m.Source, &at); err != nil {
			return m, err
		}
		var err error
		m.UpdatedAt, err = parseTime(at)
		return m, err
	}
}

// memoryIn reads every row of a memory table, tombstones too.
func memoryIn(tx *sql.Tx, table string, scope MemoryScope) ([]MemorySection, error) {
	ss, err := queryTx(tx, "SELECT "+memoryCols+" FROM "+table, nil, scanMemory(scope))
	SortMemory(ss)
	return ss, err
}

// planMemory checks an edit against the memory as it is (all, tombstones
// too) and returns the section after it, and the op: "" when it changes
// nothing.
func planMemory(scope MemoryScope, all []MemorySection, e MemoryEdit, now time.Time) (cur *MemorySection, next MemorySection, op string, err error) {
	if err := checkSource(e.Source); err != nil {
		return nil, next, "", err
	}
	if e.Section, err = NormalizeSection(e.Section); err != nil {
		return nil, next, "", err
	}
	if e.Expected < 0 {
		return nil, next, "", fault("%w: expected_revision is 0 or more", ErrInvalid)
	}
	content := strings.TrimSpace(e.Content)
	if i := slices.IndexFunc(all, func(s MemorySection) bool { return s.Section == e.Section }); i >= 0 {
		cur = &all[i]
	}
	rev, absent := 0, true
	if cur != nil {
		rev, absent = cur.Revision, cur.Content == ""
	}
	if e.Expected != rev && !(absent && e.Expected == 0) {
		c := MemorySection{Scope: scope, Section: e.Section, Revision: rev}
		if cur != nil {
			c = *cur
		}
		return cur, next, "", &MemoryConflictError{Current: c}
	}
	switch {
	case content == "" && absent, cur != nil && content == cur.Content:
		if cur != nil {
			return cur, *cur, "", nil
		}
		return nil, MemorySection{Scope: scope, Section: e.Section}, "", nil
	case content == "":
		op = "delete"
	default:
		op = "save"
		old := MemoryTokens(all)
		after := slices.Clone(all)
		if cur == nil {
			after = append(after, MemorySection{Section: e.Section})
		}
		for i := range after {
			if after[i].Section == e.Section {
				after[i].Content = content
			}
		}
		if n := MemoryTokens(after); n > scope.Cap() && n > old {
			return cur, next, "", &MemoryCapError{Scope: scope, Tokens: n, Cap: scope.Cap()}
		}
	}
	next = MemorySection{Scope: scope, Section: e.Section, Content: content, Revision: rev + 1, Source: e.Source,
		UpdatedAt: now.UTC().Truncate(time.Millisecond)}
	return cur, next, op, nil
}

// PlanMemoryEdit is what e would do to all, the sections of one scope,
// without writing it: the section before (nil if it never was), after it,
// and the op, "" when it changes nothing. Its errors are the write's.
func PlanMemoryEdit(scope MemoryScope, all []MemorySection, e MemoryEdit) (cur *MemorySection, next MemorySection, op string, err error) {
	return planMemory(scope, all, e, now())
}

// memoryRev is a section as the change log keeps it in before and after:
// NULL for a section that never was.
func memoryRev(m *MemorySection) sql.NullString {
	if m == nil {
		return sql.NullString{}
	}
	b, _ := json.Marshal(m)
	return sql.NullString{String: string(b), Valid: true}
}

func parseMemoryRev(s sql.NullString) (*MemorySection, error) {
	if !s.Valid {
		return nil, nil
	}
	var m MemorySection
	if err := json.Unmarshal([]byte(s.String), &m); err != nil {
		return nil, fmt.Errorf("store: a memory revision: %w", err)
	}
	return &m, nil
}

// sameMemory reports whether a section has the content a revision left
// it with; a section that isn't there has "". The revision isn't compared:
// an undo writes a new one, so a turn's second edit undone first, or a
// later edit since undone, would look like a change. Logged later edits
// are found apart (later, laterUser).
func sameMemory(cur, rev *MemorySection) bool {
	return memoryContent(cur) == memoryContent(rev)
}

func memoryContent(m *MemorySection) string {
	if m == nil {
		return ""
	}
	return m.Content
}

// rowKind is the row entry kind of a change from before to after.
func memoryKind(before, after *MemorySection) string {
	switch {
	case before == nil || before.Content == "":
		return rowInserted
	case after.Content == "":
		return rowDeleted
	}
	return rowUpdated
}

func putMemory(tx *sql.Tx, table string, m MemorySection) error {
	_, err := tx.Exec(`INSERT INTO `+table+` (`+memoryCols+`) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (section) DO UPDATE SET content = excluded.content, revision = excluded.revision,
			source = excluded.source, updated_at = excluded.updated_at`,
		m.Section, m.Content, m.Revision, string(m.Source), formatTime(m.UpdatedAt))
	return err
}

// ---- project memory ----

// memoryTable is project memory's table, and its UndoItem table.
const memoryTable = "_jenab_memory"

// memoryTables is project.db format step 4: project memory (SPEC 3.3).
var memoryTables = execAll(
	`CREATE TABLE _jenab_memory (
		section    TEXT PRIMARY KEY,
		content    TEXT NOT NULL,
		revision   INTEGER NOT NULL,
		source     TEXT NOT NULL,
		updated_at TEXT NOT NULL
	) WITHOUT ROWID`,
)

// Memory returns project memory's sections in SortMemory's order,
// deleted ones (Content "") too.
func (p *ProjectDB) Memory(ctx context.Context) ([]MemorySection, error) {
	return readTx(ctx, p.DB, func(tx *sql.Tx) ([]MemorySection, error) { return memoryIn(tx, memoryTable, ScopeProject) })
}

// UpdateMemory edits a section of project memory, as an interactive write
// with its change-log entry (SPEC 3.3): target memory:project/<section>,
// op save or delete, before and after the section's revisions. A stale
// e.Expected fails with a *MemoryConflictError, an edit over the cap with
// a *MemoryCapError. An edit that changes nothing writes nothing.
func (p *ProjectDB) UpdateMemory(ctx context.Context, e MemoryEdit) (MemoryWrite, error) {
	return Do(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (MemoryWrite, error) {
		all, err := memoryIn(tx, memoryTable, ScopeProject)
		if err != nil {
			return MemoryWrite{}, err
		}
		cur, next, op, err := planMemory(ScopeProject, all, e, time.Now())
		if err != nil || op == "" {
			return MemoryWrite{Section: next}, err
		}
		if err := putMemory(tx, memoryTable, next); err != nil {
			return MemoryWrite{}, err
		}
		c := newChange(next.Source, next.Target(), op)
		c.before, c.after = memoryRev(cur), memoryRev(&next)
		row := rowEntry{table: memoryTable, key: next.Target(), kind: memoryKind(cur, &next), before: c.before}
		if err := writeChange(tx, c, []rowEntry{row}); err != nil {
			return MemoryWrite{}, err
		}
		return MemoryWrite{Section: next, Op: op, Change: string(c.id)}, nil
	})
}

// userMemoryPointer is the after of a user-memory edit's entry in a
// project's log: the entry in registry.db's log it stands for.
type userMemoryPointer struct {
	UserChange int64 `json:"user_change"`
	Revision   int   `json:"revision"`
}

// LogUserMemory adds a user-memory edit made in this project to its
// change log: target memory:user/<section>, the op, and after pointing
// to the edit's entry in registry.db, where its before-revision is kept.
// It has no row entries, and project undo leaves it alone: user memory is
// undone in registry.db (Registry.UndoUserMemory).
func (p *ProjectDB) LogUserMemory(ctx context.Context, w MemoryWrite) error {
	if w.Op == "" {
		return nil
	}
	var n int64
	if _, err := fmt.Sscan(w.Change, &n); err != nil {
		return fault("store: user memory change %q is not a number", w.Change)
	}
	_, err := Do(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		c := newChange(w.Section.Source, w.Section.Target(), w.Op)
		b, _ := json.Marshal(userMemoryPointer{UserChange: n, Revision: w.Section.Revision})
		c.after = sql.NullString{String: string(b), Valid: true}
		return struct{}{}, writeChange(tx, c, nil)
	})
	return err
}

// revertMemory puts a project memory section back as it was before the
// change c. The section gets a new revision, so an editor holding the old
// one is refused, as for any edit. The cap isn't checked: an undo puts
// back what was.
func (u *undoRun) revertMemory(c *undoChange) error {
	now, err := parseMemoryRev(c.after)
	if err != nil {
		return err
	}
	want, err := parseMemoryRev(c.before)
	if err != nil {
		return err
	}
	section := strings.TrimPrefix(c.target, "memory:project/")
	all, err := memoryIn(u.tx, memoryTable, ScopeProject)
	if err != nil {
		return err
	}
	var cur *MemorySection
	if i := slices.IndexFunc(all, func(s MemorySection) bool { return s.Section == section }); i >= 0 {
		cur = &all[i]
	}
	item := UndoItem{memoryTable, c.target}
	if !sameMemory(cur, now) {
		if !u.has(item) { // later has it already when a logged change did it
			u.conflict(UndoConflict{Item: item, Reason: ConflictChanged, Detail: "the memory section changed since"})
		}
		return nil
	}
	w := u.entry(c.id)
	next := MemorySection{Scope: ScopeProject, Section: section, Revision: 1, Source: w.source, UpdatedAt: w.at}
	if cur != nil {
		next.Revision = cur.Revision + 1
	}
	if want != nil {
		next.Content = want.Content
	}
	if err := putMemory(u.tx, memoryTable, next); err != nil {
		return err
	}
	before := memoryRev(cur)
	if err := u.log(c.id, []rowEntry{{table: memoryTable, key: c.target, kind: memoryKind(cur, &next), before: before}}); err != nil {
		return err
	}
	w.before, w.after = before, memoryRev(&next)
	return nil
}

// ---- user memory ----

// userMemoryLog is registry.db format step 2: who wrote user memory, and
// its own change log (SPEC 2.4, 3.3). User memory serves every project,
// so its revisions and undo live in registry.db; a project's log keeps a
// pointer to the entry (LogUserMemory). An undo of entry n is an entry
// with the source undo:user_memory:<n>, as for chat roles in chats.db.
var userMemoryLog = execAll(
	`ALTER TABLE user_memory ADD COLUMN source TEXT NOT NULL DEFAULT ''`,
	`CREATE TABLE user_memory_changes (
		id      INTEGER PRIMARY KEY AUTOINCREMENT,
		at      TEXT NOT NULL,
		source  TEXT NOT NULL,
		project TEXT NOT NULL DEFAULT '',
		section TEXT NOT NULL,
		op      TEXT NOT NULL,
		before  TEXT,
		after   TEXT NOT NULL
	)`,
	`CREATE INDEX user_memory_changes_source ON user_memory_changes (source)`,
	`CREATE INDEX user_memory_changes_section ON user_memory_changes (section, id)`,
)

// UserMemoryItem is the UndoItem table of a user memory section; its key
// is the section's name.
const UserMemoryItem = "_jenab_user_memory"

// UserMemory returns user memory's sections in SortMemory's order,
// deleted ones (Content "") too.
func (r *Registry) UserMemory(ctx context.Context) ([]MemorySection, error) {
	var out []MemorySection
	err := r.tx(ctx, func(tx *sql.Tx) error {
		var err error
		out, err = memoryIn(tx, "user_memory", ScopeUser)
		return err
	})
	return out, err
}

// UpdateUserMemory edits a section of user memory, with the rules of
// ProjectDB.UpdateMemory. The entry in registry.db's log names the
// project the edit came from, "" for Settings. MemoryWrite.Change is the
// entry's number; the caller adds the pointer to the project's log.
func (r *Registry) UpdateUserMemory(ctx context.Context, e MemoryEdit, from id.Project) (MemoryWrite, error) {
	var w MemoryWrite
	err := r.tx(ctx, func(tx *sql.Tx) error {
		all, err := memoryIn(tx, "user_memory", ScopeUser)
		if err != nil {
			return err
		}
		cur, next, op, err := planMemory(ScopeUser, all, e, time.Now())
		w = MemoryWrite{Section: next, Op: op}
		if err != nil || op == "" {
			return err
		}
		if err := putMemory(tx, "user_memory", next); err != nil {
			return err
		}
		n, err := logUserMemory(tx, next.UpdatedAt, next.Source, from, next.Section, op, memoryRev(cur), next)
		w.Change = fmt.Sprint(n)
		return err
	})
	return w, err
}

func logUserMemory(tx *sql.Tx, at time.Time, src id.Source, from id.Project, section, op string, before sql.NullString, after MemorySection) (int64, error) {
	res, err := tx.Exec(`INSERT INTO user_memory_changes (at, source, project, section, op, before, after) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		formatTime(at), string(src), string(from), section, op, before, memoryRev(&after).String)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// tx runs fn in one transaction on the registry's connection.
func (r *Registry) tx(ctx context.Context, fn func(*sql.Tx) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	tx, err := r.c.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// userMemoryUndoSource is the source of the undo of entry n.
func userMemoryUndoSource(n int64) id.Source {
	return id.SourceOf("undo", fmt.Sprintf("user_memory:%d", n))
}

// userTip is the tip of a chain of user memory entries, as undoTips. An
// undo that skips a section writes an entry with the op "skip": it
// reverts nothing, so only the other entries (eff) say whether the base
// edit is undone (odd) or in effect (even). real is the chain's last
// entry that isn't a skip.
type userTip struct {
	id, real      int64
	section       string
	before, after sql.NullString // the base edit's
	eff           int
	at            string // when real was written
}

func (t userTip) undone() bool { return t.eff%2 == 1 }

// userTips returns the tip of each chain that starts at an entry from
// base: entries whose source is one of srcs, or whose id is one of ids.
func userTips(tx *sql.Tx, srcs string, ids string) ([]userTip, error) {
	return queryTx(tx, `WITH RECURSIVE chain(base, id, depth, eff, real) AS (
			SELECT id, id, 0, 0, id FROM user_memory_changes
				WHERE source IN (SELECT value FROM json_each(?1)) OR id IN (SELECT value FROM json_each(?2))
			UNION ALL
			SELECT chain.base, c.id, chain.depth + 1, chain.eff + (c.op <> 'skip'), CASE WHEN c.op = 'skip' THEN chain.real ELSE c.id END
			FROM chain JOIN user_memory_changes c ON c.source = 'undo:user_memory:' || chain.id)
		SELECT t.id, t.real, b.section, b.before, b.after, t.eff, x.at
		FROM (SELECT base, id, real, eff, max(depth) FROM chain GROUP BY base) t JOIN user_memory_changes b ON b.id = t.base
			JOIN user_memory_changes x ON x.id = t.real
		ORDER BY t.id`, []any{srcs, ids}, func(r *sql.Rows) (userTip, error) {
		var x userTip
		return x, r.Scan(&x.id, &x.real, &x.section, &x.before, &x.after, &x.eff, &x.at)
	})
}

// pickUser returns the tips an undo, or with redo a redo, reverts.
func pickUser(tips []userTip, redo bool) []userTip {
	var out []userTip
	for _, t := range tips {
		if t.undone() == redo {
			out = append(out, t)
		}
	}
	return out
}

func userPlan(ts []userTip, skip map[UndoItem]bool) UndoPlan {
	sections := map[string]bool{}
	for _, t := range ts {
		if !skip[UndoItem{UserMemoryItem, t.section}] {
			sections[t.section] = true
		}
	}
	p := UndoPlan{Changes: len(ts)}
	if len(sections) > 0 {
		p.Rows = []TableRows{{Table: UserMemoryItem, Rows: len(sections)}}
	}
	return p
}

// UserMemoryUndo says what an undo of user memory did.
type UserMemoryUndo struct {
	Plan      UndoPlan       `json:"plan"`
	Skipped   []UndoItem     `json:"skipped,omitempty"`
	Conflicts []UndoConflict `json:"conflicts,omitempty"`
}

// userLast is the newest undo or redo among the tips: the newest entry
// that is no skip and not the edit itself.
func userLast(ts []userTip) *UndoAction {
	var last *UndoAction
	for _, t := range ts {
		if t.eff > 0 {
			last = last.newer(&UndoAction{At: t.at, Redo: !t.undone()})
		}
	}
	return last
}

// UserMemoryUndoState is what UndoUserMemory and a redo would revert now
// for the entries from sources, as ProjectDB.UndoState.
func (r *Registry) UserMemoryUndoState(ctx context.Context, sources []id.Source) (UndoState, error) {
	var s UndoState
	if len(sources) == 0 {
		return s, nil
	}
	err := r.tx(ctx, func(tx *sql.Tx) error {
		tips, err := userTips(tx, sourcesJSON(sources), "[]")
		s = UndoState{Undo: userPlan(pickUser(tips, false), nil), Redo: userPlan(pickUser(tips, true), nil), Last: userLast(tips)}
		return err
	})
	return s, err
}

// UndoUserMemory reverts the user memory edits from sources, the
// message:<id> of a turn's messages, newest first, in one transaction:
// each section gets its content from before the edit, with a new
// revision, recorded as an entry with the source undo:user_memory:<n>,
// so o.Redo puts it back (SPEC 2.6). A later edit of the section outside
// the undo, from any project or Settings, is a conflict
// (ErrUndoConflict) unless o.Skip has the section's UndoItem, or the
// later edit is undone now and its last undo put the section back. No
// edit to revert is not an error: a turn rarely edits user memory. The
// undo's entries name from, the project it was made in, "" for none.
func (r *Registry) UndoUserMemory(ctx context.Context, sources []id.Source, from id.Project, o UndoOptions) (UserMemoryUndo, error) {
	if len(sources) == 0 {
		return UserMemoryUndo{}, nil
	}
	return r.undoUser(ctx, sourcesJSON(sources), "[]", from, o)
}

// UndoUserMemoryChange is UndoUserMemory for one edit, entry n of the log,
// for the chip's undo (SPEC 3.3). Nothing to revert is ErrNothingToUndo,
// and an entry the cleanup dropped ErrUndoWindow.
func (r *Registry) UndoUserMemoryChange(ctx context.Context, n int64, from id.Project, o UndoOptions) (UserMemoryUndo, error) {
	res, err := r.undoUser(ctx, "[]", fmt.Sprintf("[%d]", n), from, o)
	if err == nil && res.Plan.Changes == 0 {
		err = ErrNothingToUndo
	}
	if errors.Is(err, ErrNothingToUndo) {
		switch gone, gerr := r.userEntryGone(ctx, n); {
		case gerr != nil:
			err = gerr
		case gone:
			err = fault("%w", ErrUndoWindow)
		}
	}
	return res, err
}

func (r *Registry) undoUser(ctx context.Context, srcs, ids string, from id.Project, o UndoOptions) (UserMemoryUndo, error) {
	var res UserMemoryUndo
	err := r.tx(ctx, func(tx *sql.Tx) error {
		res = UserMemoryUndo{}
		tips, err := userTips(tx, srcs, ids)
		if err != nil {
			return err
		}
		skip := map[UndoItem]bool{}
		for _, it := range o.Skip {
			skip[it] = true
		}
		ts := pickUser(tips, o.Redo)
		res.Plan = userPlan(ts, skip)
		if len(ts) == 0 {
			return nil
		}
		set := map[int64]bool{}
		first := map[string]int64{}
		for _, t := range ts {
			set[t.real] = true
			if it := (UndoItem{UserMemoryItem, t.section}); skip[it] {
				if !slices.Contains(res.Skipped, it) {
					res.Skipped = append(res.Skipped, it)
				}
				continue
			}
			if f, ok := first[t.section]; !ok || t.real < f {
				first[t.section] = t.real
			}
		}
		if err := r.laterUser(tx, first, set, &res); err != nil {
			return err
		}
		if len(res.Conflicts) > 0 {
			return ErrUndoConflict
		}
		all, err := memoryIn(tx, "user_memory", ScopeUser)
		if err != nil {
			return err
		}
		now := time.Now().UTC().Truncate(time.Millisecond)
		reverted := 0
		for i := len(ts) - 1; i >= 0; i-- {
			t := ts[i]
			var cur *MemorySection
			if j := slices.IndexFunc(all, func(s MemorySection) bool { return s.Section == t.section }); j >= 0 {
				cur = &all[j]
			}
			src := userMemoryUndoSource(t.id)
			if skip[UndoItem{UserMemoryItem, t.section}] {
				// Nothing to revert. The skip entry keeps the section, so it
				// is no later change, and leaves the edit's state as it is:
				// a later undo tries again.
				keep := MemorySection{Scope: ScopeUser, Section: t.section}
				if cur != nil {
					keep = *cur
				}
				b := memoryRev(&keep)
				if _, err := logUserMemory(tx, now, src, from, t.section, "skip", b, keep); err != nil {
					return err
				}
				continue
			}
			// An undo takes the section from the edit's after to its
			// before, a redo back.
			before, err := parseMemoryRev(t.before)
			if err != nil {
				return err
			}
			after, err := parseMemoryRev(t.after)
			if err != nil {
				return err
			}
			was, want := after, before
			if o.Redo {
				was, want = before, after
			}
			if !sameMemory(cur, was) { // changed without an entry
				res.Conflicts = append(res.Conflicts, UndoConflict{Item: UndoItem{UserMemoryItem, t.section}, Reason: ConflictChanged,
					Detail: "the memory section changed since"})
				continue
			}
			next := MemorySection{Scope: ScopeUser, Section: t.section, Revision: 1, Source: src, UpdatedAt: now}
			if cur != nil {
				next.Revision = cur.Revision + 1
			}
			if want != nil {
				next.Content = want.Content
			}
			if err := putMemory(tx, "user_memory", next); err != nil {
				return err
			}
			if _, err := logUserMemory(tx, now, src, from, t.section, "undo", memoryRev(cur), next); err != nil {
				return err
			}
			reverted++
			if cur != nil {
				*cur = next
			} else {
				all = append(all, next)
			}
		}
		// As Undo: the plan counts the edits reverted, none when all were
		// skipped.
		res.Plan.Changes = reverted
		if len(res.Conflicts) > 0 {
			return ErrUndoConflict
		}
		if o.Check {
			return errChecked
		}
		return nil
	})
	if errors.Is(err, errChecked) {
		err = nil
	}
	if err != nil && !errors.Is(err, ErrUndoConflict) {
		return res, fmt.Errorf("store: undo of user memory: %w", err)
	}
	return res, err
}

// laterUser lists, as conflicts, edits of a section after the undo's
// first entry for it that aren't in the undo, as later does for
// project.db: the later entries of one chain don't count when they are
// even in number, or when the chain started before and is undone now.
func (r *Registry) laterUser(tx *sql.Tx, first map[string]int64, set map[int64]bool, res *UserMemoryUndo) error {
	type later struct {
		id      int64
		section string
		src     id.Source
		at      string
	}
	var ls []later
	for _, s := range slices.Sorted(maps.Keys(first)) {
		got, err := queryTx(tx, `SELECT id, section, source, at FROM user_memory_changes
			WHERE section = ? AND id > ? AND op <> 'skip' ORDER BY id`, []any{s, first[s]},
			func(r *sql.Rows) (later, error) {
				var l later
				return l, r.Scan(&l.id, &l.section, &l.src, &l.at)
			})
		if err != nil {
			return err
		}
		for _, l := range got {
			if !set[l.id] {
				ls = append(ls, l)
			}
		}
	}
	if len(ls) == 0 {
		return nil
	}
	ids := make([]int64, len(ls))
	for i, l := range ls {
		ids[i] = l.id
	}
	chains, err := userChains(tx, ids)
	if err != nil {
		return err
	}
	count := map[int64]int{}
	started := map[int64]bool{}
	for _, l := range ls {
		root := chains[l.id].root
		count[root]++
		if l.id == root {
			started[root] = true
		}
	}
	listed := map[string]bool{}
	for _, l := range ls {
		ch := chains[l.id]
		if count[ch.root]%2 == 0 || (!started[ch.root] && ch.undone) || listed[l.section] {
			continue
		}
		listed[l.section] = true
		x := UndoConflict{Item: UndoItem{UserMemoryItem, l.section}, Reason: ConflictChanged, Source: l.src,
			Detail: "the memory section changed since"}
		if at, err := parseTime(l.at); err == nil {
			x.At = &at
		}
		res.Conflicts = append(res.Conflicts, x)
	}
	return nil
}

// userChains returns the chain of each of the entries ids: its base
// entry, and whether it is undone now, an odd number of undos that aren't
// skips (userTip), as roleChains.
func userChains(tx *sql.Tx, ids []int64) (map[int64]numberedChain, error) {
	out := map[int64]numberedChain{}
	for _, n := range ids {
		out[n] = numberedChain{root: n}
	}
	js, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	type row struct {
		start int64
		ch    numberedChain
	}
	got, err := queryTx(tx, `WITH RECURSIVE up(start, id, source) AS (
			SELECT id, id, source FROM user_memory_changes WHERE id IN (SELECT value FROM json_each(?1))
			UNION ALL
			SELECT up.start, p.id, p.source FROM up JOIN user_memory_changes p
				ON up.source LIKE 'undo:user_memory:%' AND p.id = CAST(substr(up.source, 18) AS INTEGER)),
		roots(start, root) AS (SELECT start, id FROM up WHERE source NOT LIKE 'undo:user_memory:%'),
		down(root, id, depth, eff) AS (
			SELECT DISTINCT root, root, 0, 0 FROM roots
			UNION ALL
			SELECT down.root, c.id, down.depth + 1, down.eff + (c.op <> 'skip')
			FROM down JOIN user_memory_changes c ON c.source = 'undo:user_memory:' || down.id),
		tips(root, eff, depth) AS (SELECT root, eff, max(depth) FROM down GROUP BY root)
		SELECT r.start, r.root, t.eff % 2 = 1 FROM roots r JOIN tips t ON t.root = r.root`, []any{string(js)}, func(r *sql.Rows) (row, error) {
		var x row
		return x, r.Scan(&x.start, &x.ch.root, &x.ch.undone)
	})
	for _, x := range got {
		out[x.start] = x.ch
	}
	return out, err
}

// UserMemoryChangeUndone reports whether entry n of user memory's log is
// undone now: its chain has an odd number of undos that aren't skips. A
// chip asks, so it offers redo after a reload or a turn's undo (SPEC 3.3).
func (r *Registry) UserMemoryChangeUndone(ctx context.Context, n int64) (bool, error) {
	var undone bool
	err := r.tx(ctx, func(tx *sql.Tx) error {
		tips, err := userTips(tx, "[]", fmt.Sprintf("[%d]", n))
		if err != nil || len(tips) == 0 {
			return err
		}
		undone = tips[0].undone()
		return nil
	})
	return undone, err
}

// ChangeUndone is UserMemoryChangeUndone for a change in project.db: its
// items are all undone (undoChain). An undo that skipped them left them
// in effect, as they are. A change past the undo window reads as in
// effect: it is history, and the cleanup may have dropped what its state
// is read from.
func (p *ProjectDB) ChangeUndone(ctx context.Context, c id.Change) (bool, error) {
	return readTx(ctx, p.DB, func(tx *sql.Tx) (bool, error) {
		chains, err := undoChains(tx, undoBase{srcs: "[]", ids: idsJSON([]id.Change{c})})
		if err != nil || len(chains) == 0 {
			return false, err
		}
		mark, err := cleanedTo(tx)
		if err != nil {
			return false, err
		}
		ch := chains[0]
		if ch.past(mark) {
			return false, nil // history only: as it is, in effect
		}
		if len(ch.items) == 0 {
			return ch.tip().depth%2 == 1, nil
		}
		for _, it := range ch.items {
			if !it.undone() {
				return false, nil
			}
		}
		return true, nil
	})
}

// userEntryGone reports whether entry n of user memory's log was dropped
// by CleanUserMemory. Entry numbers are never reused (AUTOINCREMENT).
func (r *Registry) userEntryGone(ctx context.Context, n int64) (bool, error) {
	var gone bool
	err := r.tx(ctx, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT NOT EXISTS (SELECT 1 FROM user_memory_changes WHERE id = ?)
			AND ? <= coalesce((SELECT seq FROM sqlite_sequence WHERE name = 'user_memory_changes'), 0)`, n, n).Scan(&gone)
	})
	return gone, err
}

// CleanUserMemory drops the entries of user memory's log older than keep
// (0 means ChangeKeep), and with them their undo, as CleanChanges does
// for a project (SPEC 2.5, 2.6). A chain cut by the cleanup reads as a
// later edit, so an undo across it is refused rather than wrong.
func (r *Registry) CleanUserMemory(ctx context.Context, keep time.Duration) (int, error) {
	if keep <= 0 {
		keep = ChangeKeep
	}
	var n int64
	err := r.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec(`DELETE FROM user_memory_changes WHERE at < ?`, formatTime(time.Now().Add(-keep)))
		if err != nil {
			return err
		}
		n, err = res.RowsAffected()
		return err
	})
	return int(n), err
}
