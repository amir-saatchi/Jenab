package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"golang.org/x/text/unicode/norm"
)

// Views and pages (SPEC 5) are configs: the YAML the agent wrote, kept as
// written, comments too, with a revision. Each save and delete is one
// change with its row entry, so Undo turn, rewind's Undo changes and the
// conflict check cover them as they do memory sections (SPEC 2.5, 2.6).
// The store doesn't read the YAML: a config is validated before it is
// saved (P3-02), and the caller gives its type and title.

// ConfigKind is a view or a page.
type ConfigKind string

const (
	KindView ConfigKind = "view"
	KindPage ConfigKind = "page"
)

// ConfigKinds are the kinds, in the order lists give them.
var ConfigKinds = []ConfigKind{KindView, KindPage}

// The tables of the kinds, and their UndoItem tables.
const (
	viewTable = "_jenab_views"
	pageTable = "_jenab_pages"
)

func (k ConfigKind) table() string {
	switch k {
	case KindView:
		return viewTable
	case KindPage:
		return pageTable
	}
	return ""
}

func (k ConfigKind) other() ConfigKind {
	if k == KindView {
		return KindPage
	}
	return KindView
}

// configTables is project.db format step 6: views and pages (SPEC 2.2,
// P3-01). A deleted config keeps its row with an empty config, so its revision
// goes on counting, as memory's sections do; the lists leave it out.
var configTables = execAll(
	`CREATE TABLE _jenab_views (
		id         TEXT PRIMARY KEY,
		type       TEXT NOT NULL,
		title      TEXT NOT NULL,
		config     TEXT NOT NULL,
		revision   INTEGER NOT NULL,
		updated_at TEXT NOT NULL,
		updated_by TEXT NOT NULL
	) WITHOUT ROWID`,
	`CREATE TABLE _jenab_pages (
		id         TEXT PRIMARY KEY,
		type       TEXT NOT NULL,
		title      TEXT NOT NULL,
		config     TEXT NOT NULL,
		revision   INTEGER NOT NULL,
		updated_at TEXT NOT NULL,
		updated_by TEXT NOT NULL
	) WITHOUT ROWID`,
)

// Config is a saved view or page. A deleted one has YAML "".
type Config struct {
	Kind ConfigKind `json:"kind"`
	ID   string     `json:"id"`
	// Type is a view's type (table, chart, stat, form, …); a page's is
	// "page".
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	YAML      string    `json:"yaml"` // as the agent wrote it
	Revision  int       `json:"revision"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy id.Source `json:"updated_by"` // who wrote it last
}

// Live reports whether the config is there, not deleted.
func (c Config) Live() bool { return c.YAML != "" }

// Target is the config's change-log target, view:<id> or page:<id>.
func (c Config) Target() string { return ConfigTarget(c.Kind, c.ID) }

// ConfigTarget is a config's change-log target (SPEC 2.5).
func ConfigTarget(kind ConfigKind, cid string) string { return string(kind) + ":" + cid }

// ParseConfigTarget splits a target view:<id> or page:<id>.
func ParseConfigTarget(target string) (ConfigKind, string, bool) {
	k, cid, ok := strings.Cut(target, ":")
	if !ok || cid == "" || ConfigKind(k).table() == "" {
		return "", "", false
	}
	return ConfigKind(k), cid, true
}

// ConfigChange is what a write or an undo did to one config, as the
// project:configs event says it (SPEC 5.9): open pages read it again, and
// a deleted page closes.
type ConfigChange struct {
	Kind     ConfigKind `json:"kind"`
	ID       string     `json:"id"`
	Op       string     `json:"op"`       // save or delete
	Revision int        `json:"revision"` // after it
}

func changeOf(c Config) ConfigChange {
	op := "save"
	if !c.Live() {
		op = "delete"
	}
	return ConfigChange{Kind: c.Kind, ID: c.ID, Op: op, Revision: c.Revision}
}

// ConfigSave is a save of one config, already validated.
type ConfigSave struct {
	Kind  ConfigKind
	ID    string
	Type  string // a view's type; ignored for a page
	Title string
	YAML  string
	// Expected is the revision the writer read, to refuse a stale save;
	// 0 saves over whatever is there.
	Expected int
	Source   id.Source
}

// ConfigDelete is a delete of one config. Kind "" finds the view or the
// page with the ID, as delete_config(id) does (SPEC 8.1).
type ConfigDelete struct {
	Kind     ConfigKind
	ID       string
	Expected int // as in ConfigSave
	Source   id.Source
}

// ConfigWrite says what a save or a delete did.
type ConfigWrite struct {
	Config Config    `json:"config"` // after it
	Op     string    `json:"op"`     // save or delete; "" when it changed nothing
	Change id.Change `json:"change,omitempty"`
}

var (
	// ErrConfigIDTaken is a save of a view with a page's ID, or the
	// reverse: delete_config and read_config take an ID alone, so views and
	// pages share their IDs. It matches ErrInvalid.
	ErrConfigIDTaken = errors.New("store: a view or a page already has this ID")
)

// ConfigConflictError is a save or a delete whose expected revision is
// stale. Current is the config as it is now.
type ConfigConflictError struct{ Current Config }

func (e *ConfigConflictError) Error() string {
	return fmt.Sprintf("store: %s is at revision %d", e.Current.Target(), e.Current.Revision)
}

func (e *ConfigConflictError) Is(target error) bool { return target == ErrConflict }

// configID is a config's ID: letters, digits, _ and -, starting with a
// letter or digit, as in btc_price_chart (SPEC 5.1, 9.3).
var configID = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N}\p{Mn}_\x{200C}-]*$`)

// MaxConfigID is the longest config ID, in characters.
const MaxConfigID = 64

// NormConfigID is the form a config ID is stored and looked up in: NFC,
// so the composed and decomposed spellings of an ID (آ as one letter, or
// as ا with a mark) are one config. The store normalizes the IDs it is
// given; P3-02 should normalize the YAML's id the same way.
func NormConfigID(cid string) string { return norm.NFC.String(cid) }

// CheckConfigID checks a view's or a page's ID: 1 to 64 letters, digits,
// _ or -, starting with a letter or digit.
func CheckConfigID(cid string) error {
	if utf8.RuneCountInString(cid) > MaxConfigID || !configID.MatchString(cid) {
		return fault("%w: an ID is 1 to %d letters, digits, _ or -, starting with a letter or digit", ErrInvalid, MaxConfigID)
	}
	return nil
}

const configCols = "id, type, title, config, revision, updated_at, updated_by"

func scanConfig(kind ConfigKind) func(*sql.Rows) (Config, error) {
	return func(r *sql.Rows) (Config, error) {
		c := Config{Kind: kind}
		var at string
		if err := r.Scan(&c.ID, &c.Type, &c.Title, &c.YAML, &c.Revision, &at, &c.UpdatedBy); err != nil {
			return c, err
		}
		var err error
		c.UpdatedAt, err = parseTime(at)
		return c, err
	}
}

// configIn reads one config's row, a deleted one too; nil if it never was.
func configIn(tx *sql.Tx, kind ConfigKind, cid string) (*Config, error) {
	cs, err := queryTx(tx, "SELECT "+configCols+" FROM "+kind.table()+" WHERE id = ?", []any{cid}, scanConfig(kind))
	if err != nil || len(cs) == 0 {
		return nil, err
	}
	return &cs[0], nil
}

// liveIn reads the live config with the ID, of kind, or of either kind
// when kind is "".
func liveIn(tx *sql.Tx, kind ConfigKind, cid string) (*Config, error) {
	kinds := ConfigKinds
	if kind != "" {
		kinds = []ConfigKind{kind}
	}
	for _, k := range kinds {
		c, err := configIn(tx, k, cid)
		if err != nil {
			return nil, err
		}
		if c != nil && c.Live() {
			return c, nil
		}
	}
	return nil, nil
}

// ConfigsTx lists the live configs of kind in tx, by ID, such as for a
// migration's dependents (SPEC 7.5, P3-04).
func ConfigsTx(tx *sql.Tx, kind ConfigKind) ([]Config, error) {
	if kind.table() == "" {
		return nil, fault("%w: unknown config kind %q", ErrInvalid, kind)
	}
	return queryTx(tx, "SELECT "+configCols+" FROM "+kind.table()+" WHERE config <> '' ORDER BY id", nil, scanConfig(kind))
}

func putConfig(tx *sql.Tx, c Config) error {
	_, err := tx.Exec(`INSERT INTO `+c.Kind.table()+` (`+configCols+`) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET type = excluded.type, title = excluded.title, config = excluded.config,
			revision = excluded.revision, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
		c.ID, c.Type, c.Title, c.YAML, c.Revision, formatTime(c.UpdatedAt), string(c.UpdatedBy))
	return err
}

// configRev is a config as the change log keeps it in before and after:
// NULL for a config that never was.
func configRev(c *Config) sql.NullString {
	if c == nil {
		return sql.NullString{}
	}
	b, _ := json.Marshal(c)
	return sql.NullString{String: string(b), Valid: true}
}

func parseConfigRev(s sql.NullString) (*Config, error) {
	if !s.Valid {
		return nil, nil
	}
	var c Config
	if err := json.Unmarshal([]byte(s.String), &c); err != nil {
		return nil, fmt.Errorf("store: a config revision: %w", err)
	}
	return &c, nil
}

// sameConfig reports whether a config is as a revision left it; one that
// isn't there is as a deleted one. The revision isn't compared, as for
// memory (sameMemory): an undo writes a new one.
func sameConfig(cur, rev *Config) bool {
	var a, b Config
	if cur != nil {
		a = *cur
	}
	if rev != nil {
		b = *rev
	}
	if !a.Live() || !b.Live() {
		return a.Live() == b.Live()
	}
	return a.YAML == b.YAML && a.Type == b.Type && a.Title == b.Title
}

// configKind is the row entry kind of a change from before to after.
func configKind(before, after *Config) string {
	switch {
	case before == nil || !before.Live():
		return rowInserted
	case !after.Live():
		return rowDeleted
	}
	return rowUpdated
}

// checkExpected refuses a write whose expected revision isn't cur's.
func checkExpected(kind ConfigKind, cid string, cur *Config, expected int) error {
	switch {
	case expected < 0:
		return fault("%w: the expected revision is 0 or more", ErrInvalid)
	case expected == 0:
		return nil
	case cur == nil:
		return &ConfigConflictError{Current: Config{Kind: kind, ID: cid}}
	case cur.Revision != expected:
		return &ConfigConflictError{Current: *cur}
	}
	return nil
}

// writeConfig puts next in place of cur, with its change-log entry.
func writeConfig(tx *sql.Tx, cur *Config, next Config, op string) (ConfigWrite, error) {
	if err := putConfig(tx, next); err != nil {
		return ConfigWrite{}, err
	}
	c := newChange(next.UpdatedBy, next.Target(), op)
	c.before, c.after = configRev(cur), configRev(&next)
	row := rowEntry{table: next.Kind.table(), key: next.Target(), kind: configKind(cur, &next), before: c.before}
	if err := writeChange(tx, c, []rowEntry{row}); err != nil {
		return ConfigWrite{}, err
	}
	return ConfigWrite{Config: next, Op: op, Change: c.id}, nil
}

// SaveConfigTx saves s in tx with its change-log entry: target
// view:<id> or page:<id>, op save, before and after the revisions. It is
// ProjectDB.SaveConfig inside a transaction the caller holds, such as a
// migration updating its dependents (SPEC 7.5); the caller publishes the
// change after the commit.
func SaveConfigTx(tx *sql.Tx, s ConfigSave) (ConfigWrite, error) {
	if err := checkSource(s.Source); err != nil {
		return ConfigWrite{}, err
	}
	if s.Kind.table() == "" {
		return ConfigWrite{}, fault("%w: unknown config kind %q", ErrInvalid, s.Kind)
	}
	s.ID = NormConfigID(s.ID)
	if err := CheckConfigID(s.ID); err != nil {
		return ConfigWrite{}, err
	}
	if s.Kind == KindPage {
		s.Type = string(KindPage)
	}
	switch {
	case strings.TrimSpace(s.YAML) == "":
		return ConfigWrite{}, fault("%w: a config can't be empty", ErrInvalid)
	case !utf8.ValidString(s.YAML) || !utf8.ValidString(s.Title) || !utf8.ValidString(s.Type):
		return ConfigWrite{}, fault("%w: a config is UTF-8 text", ErrInvalid)
	case s.Type == "":
		return ConfigWrite{}, fault("%w: a view needs its type", ErrInvalid)
	}
	cur, err := configIn(tx, s.Kind, s.ID)
	if err != nil {
		return ConfigWrite{}, err
	}
	if err := checkExpected(s.Kind, s.ID, cur, s.Expected); err != nil {
		return ConfigWrite{}, err
	}
	other, err := liveIn(tx, s.Kind.other(), s.ID)
	if err != nil {
		return ConfigWrite{}, err
	}
	if other != nil {
		return ConfigWrite{}, fault("%w: the %s %s", ErrConfigIDTaken, other.Kind, other.ID)
	}
	next := Config{Kind: s.Kind, ID: s.ID, Type: s.Type, Title: s.Title, YAML: s.YAML, Revision: 1, UpdatedAt: now(), UpdatedBy: s.Source}
	if cur != nil {
		if sameConfig(cur, &next) {
			return ConfigWrite{Config: *cur}, nil
		}
		next.Revision = cur.Revision + 1
	}
	return writeConfig(tx, cur, next, "save")
}

// deleteConfigTx deletes a live config in tx with its change-log entry,
// op delete. A config that isn't there is ErrNotFound.
func deleteConfigTx(tx *sql.Tx, d ConfigDelete) (ConfigWrite, error) {
	if err := checkSource(d.Source); err != nil {
		return ConfigWrite{}, err
	}
	if d.Kind != "" && d.Kind.table() == "" {
		return ConfigWrite{}, fault("%w: unknown config kind %q", ErrInvalid, d.Kind)
	}
	d.ID = NormConfigID(d.ID)
	cur, err := liveIn(tx, d.Kind, d.ID)
	if err != nil {
		return ConfigWrite{}, err
	}
	if cur == nil {
		what := "view or page"
		if d.Kind != "" {
			what = string(d.Kind)
		}
		return ConfigWrite{}, fmt.Errorf("%w: no %s %q", ErrNotFound, what, d.ID)
	}
	if err := checkExpected(cur.Kind, cur.ID, cur, d.Expected); err != nil {
		return ConfigWrite{}, err
	}
	next := *cur
	next.YAML, next.Revision, next.UpdatedAt, next.UpdatedBy = "", cur.Revision+1, now(), d.Source
	return writeConfig(tx, cur, next, "delete")
}

// SaveConfig saves a view or a page as an interactive write with its
// change-log entry (SPEC 2.5, 7.3). A stale s.Expected fails with a
// *ConfigConflictError, an ID the other kind has with ErrConfigIDTaken. A
// save of what is there already writes nothing. A deleted config saved
// again goes on from its revision.
func (p *ProjectDB) SaveConfig(ctx context.Context, s ConfigSave) (ConfigWrite, error) {
	return doThen(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (ConfigWrite, error) { return SaveConfigTx(tx, s) }, p.configWritten)
}

// DeleteConfig deletes a view or a page, as SaveConfig saves one, with op
// delete. Its row stays with no config, so its revision goes on.
func (p *ProjectDB) DeleteConfig(ctx context.Context, d ConfigDelete) (ConfigWrite, error) {
	return doThen(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (ConfigWrite, error) { return deleteConfigTx(tx, d) }, p.configWritten)
}

// configWritten publishes a committed save or delete; a save that changed
// nothing has no op and sends nothing.
func (p *ProjectDB) configWritten(w ConfigWrite) {
	if w.Op != "" {
		p.configsChanged([]ConfigChange{changeOf(w.Config)})
	}
}

// Config reads the live config with the ID: of kind, or of either kind
// when kind is "". One that isn't there, or was deleted, is ErrNotFound.
func (p *ProjectDB) Config(ctx context.Context, kind ConfigKind, cid string) (Config, error) {
	if kind != "" && kind.table() == "" {
		return Config{}, fault("%w: unknown config kind %q", ErrInvalid, kind)
	}
	cid = NormConfigID(cid)
	return readTx(ctx, p.DB, func(tx *sql.Tx) (Config, error) {
		c, err := liveIn(tx, kind, cid)
		if err != nil {
			return Config{}, err
		}
		if c == nil {
			return Config{}, fmt.Errorf("%w: no config %q", ErrNotFound, cid)
		}
		return *c, nil
	})
}

// Configs lists the live configs of kind, by ID.
func (p *ProjectDB) Configs(ctx context.Context, kind ConfigKind) ([]Config, error) {
	return readTx(ctx, p.DB, func(tx *sql.Tx) ([]Config, error) { return ConfigsTx(tx, kind) })
}

// configHook is the function a project's config changes go to.
type configHook struct {
	mu sync.Mutex
	fn func([]ConfigChange)
}

// OnConfigs sets fn to get every change to views and pages once it is
// committed: saves, deletes, and the undos and redos that revert them.
// It runs on the writer right after the commit, so changes come in the
// order they were committed, and it must not block, as it holds up every
// write. The project sends
// them as the project:configs event (SPEC 5.9).
func (p *ProjectDB) OnConfigs(fn func([]ConfigChange)) {
	p.configs.mu.Lock()
	defer p.configs.mu.Unlock()
	p.configs.fn = fn
}

func (p *ProjectDB) configsChanged(cs []ConfigChange) {
	if len(cs) == 0 {
		return
	}
	p.configs.mu.Lock()
	fn := p.configs.fn
	p.configs.mu.Unlock()
	if fn != nil {
		fn(cs)
	}
}

// revertConfig puts a view or a page back as it was before the change c,
// with a new revision, as revertMemory does a memory section: a first
// save is deleted, a delete brings the config back. A config changed since
// is a conflict; so is one whose ID the other kind took meanwhile.
func (u *undoRun) revertConfig(c *undoChange) error {
	kind, cid, ok := ParseConfigTarget(c.target)
	if !ok {
		return fmt.Errorf("store: %q is no config target", c.target)
	}
	now, err := parseConfigRev(c.after)
	if err != nil {
		return err
	}
	want, err := parseConfigRev(c.before)
	if err != nil {
		return err
	}
	cur, err := configIn(u.tx, kind, cid)
	if err != nil {
		return err
	}
	item := UndoItem{kind.table(), c.target}
	if !sameConfig(cur, now) {
		if !u.has(item) { // later has it already when a logged change did it
			u.conflict(UndoConflict{Item: item, Reason: ConflictChanged, Detail: "the " + string(kind) + " changed since"})
		}
		return nil
	}
	if want != nil && want.Live() {
		other, err := liveIn(u.tx, kind.other(), cid)
		if err != nil {
			return err
		}
		if other != nil {
			u.conflict(UndoConflict{Item: item, Reason: ConflictConstraint, Reached: &UndoItem{other.Kind.table(), other.Target()},
				Detail: "a " + string(other.Kind) + " has this ID now"})
			return nil
		}
	}
	w := u.entry(c.id)
	next := Config{Kind: kind, ID: cid, Revision: 1, UpdatedAt: w.at, UpdatedBy: w.source}
	if cur != nil {
		next.Type, next.Title, next.Revision = cur.Type, cur.Title, cur.Revision+1
	}
	if want != nil && want.Live() {
		next.Type, next.Title, next.YAML = want.Type, want.Title, want.YAML
	}
	if err := putConfig(u.tx, next); err != nil {
		return err
	}
	before := configRev(cur)
	if err := u.log(c.id, []rowEntry{{table: kind.table(), key: c.target, kind: configKind(cur, &next), before: before}}); err != nil {
		return err
	}
	w.before, w.after = before, configRev(&next)
	u.configs = append(u.configs, changeOf(next))
	return nil
}
