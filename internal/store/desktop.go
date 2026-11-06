package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
)

// Desktop macros and recipes (SPEC 8.11, TASK-038): app-wide, like user
// memory, so they live in registry.db. A macro's steps are JSON the
// desktop package reads; the store keeps them as they are.

// desktopTables is registry.db format step 3.
var desktopTables = execAll(
	`CREATE TABLE desktop_macros (
		id             INTEGER PRIMARY KEY,
		app            TEXT NOT NULL,
		site           TEXT NOT NULL DEFAULT '',
		name           TEXT NOT NULL,
		description    TEXT NOT NULL,
		params         TEXT NOT NULL,
		steps          TEXT NOT NULL,
		version        INTEGER NOT NULL DEFAULT 1,
		made_at        TEXT NOT NULL,
		made_by_model  TEXT NOT NULL,
		made_in_chat   TEXT NOT NULL DEFAULT '',
		untrusted      INTEGER NOT NULL DEFAULT 1,
		last_worked_at TEXT,
		last_run_at    TEXT,
		runs           INTEGER NOT NULL DEFAULT 0,
		failures       INTEGER NOT NULL DEFAULT 0,
		fail_streak    INTEGER NOT NULL DEFAULT 0,
		removed_at     TEXT,
		removed_why    TEXT NOT NULL DEFAULT '',
		UNIQUE (app, site, name)
	)`,
	`CREATE TABLE desktop_recipes (
		id            INTEGER PRIMARY KEY,
		app           TEXT NOT NULL,
		site          TEXT NOT NULL DEFAULT '',
		name          TEXT NOT NULL,
		text          TEXT NOT NULL,
		source        TEXT NOT NULL CHECK (source IN ('app', 'agent', 'user')),
		app_version   TEXT NOT NULL DEFAULT '',
		made_at       TEXT NOT NULL,
		made_by_model TEXT NOT NULL DEFAULT '',
		made_in_chat  TEXT NOT NULL DEFAULT '',
		untrusted     INTEGER NOT NULL DEFAULT 0,
		last_used_at  TEXT,
		uses          INTEGER NOT NULL DEFAULT 0,
		removed_at    TEXT,
		removed_why   TEXT NOT NULL DEFAULT '',
		UNIQUE (app, site, name, source)
	)`,
	// Undo and redo for a save's notice, like user_memory_changes (3.3):
	// the row before (NULL when the save made it) and after.
	`CREATE TABLE desktop_saved_changes (
		id      INTEGER PRIMARY KEY AUTOINCREMENT,
		at      TEXT NOT NULL,
		kind    TEXT NOT NULL CHECK (kind IN ('macro', 'recipe')),
		item_id INTEGER NOT NULL,
		before  TEXT,
		after   TEXT NOT NULL,
		chat_id TEXT NOT NULL DEFAULT '',
		undone  INTEGER NOT NULL DEFAULT 0
	)`,
	// The last 20 replays per macro, for Settings: no screen text, the
	// reason's names clipped, the log's length only (8.10's log rule).
	`CREATE TABLE desktop_macro_runs (
		id        INTEGER PRIMARY KEY,
		macro_id  INTEGER NOT NULL REFERENCES desktop_macros(id) ON DELETE CASCADE,
		version   INTEGER NOT NULL,
		at        TEXT NOT NULL,
		ok        INTEGER NOT NULL,
		failed_at INTEGER,
		reason    TEXT NOT NULL DEFAULT '',
		skipped   INTEGER NOT NULL DEFAULT 0,
		seconds   REAL NOT NULL,
		log_chars INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE INDEX desktop_macro_runs_macro ON desktop_macro_runs (macro_id, id)`,
)

// Removal (8.11): a macro after MacroFailLimit failed replays in a row or
// MacroUnused without a run; an agent's recipe after RecipeUnused; what
// was removed, and Undo rows, are kept for KeepRemoved.
const (
	MacroFailLimit = 3
	MacroUnused    = 90 * 24 * time.Hour
	RecipeUnused   = 365 * 24 * time.Hour
	KeepRemoved    = 30 * 24 * time.Hour
	keepRuns       = 20
)

// Recipe sources: shipped with Jenab, saved by an agent, or written or
// edited by the user.
const (
	RecipeApp   = "app"
	RecipeAgent = "agent"
	RecipeUser  = "user"
)

// DesktopMacro is a row of desktop_macros. Steps are the desktop
// package's JSON. App is the program's name in lower case without .exe;
// Site is a browser macro's origin.
type DesktopMacro struct {
	ID           int64           `json:"id"`
	App          string          `json:"app"`
	Site         string          `json:"site"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Params       []string        `json:"params"`
	Steps        json.RawMessage `json:"steps"`
	Version      int             `json:"version"`
	MadeAt       time.Time       `json:"made_at"`
	MadeByModel  string          `json:"made_by_model"`
	MadeInChat   string          `json:"made_in_chat"`
	Untrusted    bool            `json:"untrusted"`
	LastWorkedAt time.Time       `json:"last_worked_at"`
	LastRunAt    time.Time       `json:"last_run_at"`
	Runs         int             `json:"runs"`
	Failures     int             `json:"failures"`
	FailStreak   int             `json:"fail_streak"`
	RemovedAt    time.Time       `json:"removed_at"`
	RemovedWhy   string          `json:"removed_why"`
}

// Worked reports whether a replay of the macro has finished.
func (m DesktopMacro) Worked() bool { return m.Runs > m.Failures }

// DesktopRecipe is a row of desktop_recipes.
type DesktopRecipe struct {
	ID          int64     `json:"id"`
	App         string    `json:"app"`
	Site        string    `json:"site"`
	Name        string    `json:"name"`
	Text        string    `json:"text"`
	Source      string    `json:"source"`
	AppVersion  string    `json:"app_version"`
	MadeAt      time.Time `json:"made_at"`
	MadeByModel string    `json:"made_by_model"`
	MadeInChat  string    `json:"made_in_chat"`
	Untrusted   bool      `json:"untrusted"`
	LastUsedAt  time.Time `json:"last_used_at"`
	Uses        int       `json:"uses"`
	RemovedAt   time.Time `json:"removed_at"`
	RemovedWhy  string    `json:"removed_why"`
}

// DesktopRun is a row of desktop_macro_runs.
type DesktopRun struct {
	MacroID  int64     `json:"macro_id"`
	Version  int       `json:"version"`
	At       time.Time `json:"at"`
	OK       bool      `json:"ok"`
	FailedAt int       `json:"failed_at"` // the step number; 0 when it worked
	Reason   string    `json:"reason"`
	Skipped  int       `json:"skipped"`
	Seconds  float64   `json:"seconds"`
	LogChars int       `json:"log_chars"`
}

// DesktopSave is what a save did: the row now, and its change for the
// notice's Undo.
type DesktopSave struct {
	Macro  *DesktopMacro
	Recipe *DesktopRecipe
	Change int64
}

// DesktopUndo says what an Undo or Redo did. Changed: the item changed
// since, so nothing was done. Undone is the save's state now.
type DesktopUndo struct {
	Done    bool `json:"done"`
	Changed bool `json:"changed"`
	Undone  bool `json:"undone"`
}

const macroCols = `id, app, site, name, description, params, steps, version, made_at, made_by_model, made_in_chat, untrusted,
	last_worked_at, last_run_at, runs, failures, fail_streak, removed_at, removed_why`

func scanMacro(r *sql.Rows) (DesktopMacro, error) {
	var m DesktopMacro
	var params, steps, made string
	var worked, run, removed sql.NullString
	if err := r.Scan(&m.ID, &m.App, &m.Site, &m.Name, &m.Description, &params, &steps, &m.Version, &made, &m.MadeByModel, &m.MadeInChat,
		&m.Untrusted, &worked, &run, &m.Runs, &m.Failures, &m.FailStreak, &removed, &m.RemovedWhy); err != nil {
		return m, err
	}
	if err := json.Unmarshal([]byte(params), &m.Params); err != nil {
		return m, fmt.Errorf("store: macro %d's params: %w", m.ID, err)
	}
	m.Steps = json.RawMessage(steps)
	var err error
	if m.MadeAt, err = parseTime(made); err != nil {
		return m, err
	}
	for _, t := range []struct {
		s sql.NullString
		t *time.Time
	}{{worked, &m.LastWorkedAt}, {run, &m.LastRunAt}, {removed, &m.RemovedAt}} {
		if t.s.Valid {
			if *t.t, err = parseTime(t.s.String); err != nil {
				return m, err
			}
		}
	}
	return m, nil
}

const recipeCols = `id, app, site, name, text, source, app_version, made_at, made_by_model, made_in_chat, untrusted, last_used_at, uses, removed_at, removed_why`

func scanRecipe(r *sql.Rows) (DesktopRecipe, error) {
	var d DesktopRecipe
	var made string
	var used, removed sql.NullString
	if err := r.Scan(&d.ID, &d.App, &d.Site, &d.Name, &d.Text, &d.Source, &d.AppVersion, &made, &d.MadeByModel, &d.MadeInChat,
		&d.Untrusted, &used, &d.Uses, &removed, &d.RemovedWhy); err != nil {
		return d, err
	}
	var err error
	if d.MadeAt, err = parseTime(made); err != nil {
		return d, err
	}
	if used.Valid {
		if d.LastUsedAt, err = parseTime(used.String); err != nil {
			return d, err
		}
	}
	if removed.Valid {
		if d.RemovedAt, err = parseTime(removed.String); err != nil {
			return d, err
		}
	}
	return d, nil
}

// DesktopMacros lists the macros of app ("" for every app), without the
// removed ones, by app, site and name.
func (r *Registry) DesktopMacros(ctx context.Context, app string) ([]DesktopMacro, error) {
	var out []DesktopMacro
	err := r.tx(ctx, func(tx *sql.Tx) error {
		var err error
		out, err = queryTx(tx, "SELECT "+macroCols+" FROM desktop_macros WHERE removed_at IS NULL AND (?1 = '' OR app = ?1) ORDER BY app, site, name",
			[]any{app}, scanMacro)
		return err
	})
	return out, err
}

// DesktopMacro returns a macro by its id, removed or not, or ErrNotFound.
func (r *Registry) DesktopMacro(ctx context.Context, mid int64) (DesktopMacro, error) {
	var m DesktopMacro
	err := r.tx(ctx, func(tx *sql.Tx) error {
		var err error
		m, err = macroByID(tx, mid)
		return err
	})
	return m, err
}

func macroByID(tx *sql.Tx, mid int64) (DesktopMacro, error) {
	ms, err := queryTx(tx, "SELECT "+macroCols+" FROM desktop_macros WHERE id = ?", []any{mid}, scanMacro)
	if err != nil {
		return DesktopMacro{}, err
	}
	if len(ms) == 0 {
		return DesktopMacro{}, fmt.Errorf("desktop macro %d: %w", mid, ErrNotFound)
	}
	return ms[0], nil
}

func macroByName(tx *sql.Tx, app, site, name string) (*DesktopMacro, error) {
	ms, err := queryTx(tx, "SELECT "+macroCols+" FROM desktop_macros WHERE app = ? AND site = ? AND name = ?", []any{app, site, name}, scanMacro)
	if err != nil || len(ms) == 0 {
		return nil, err
	}
	return &ms[0], nil
}

// putMacro writes every column of m, by its id; an id of 0 inserts it.
func putMacro(tx *sql.Tx, m *DesktopMacro) error {
	params, err := json.Marshal(m.Params)
	if err != nil {
		return err
	}
	if m.Params == nil {
		params = []byte("[]")
	}
	args := []any{m.App, m.Site, m.Name, m.Description, string(params), string(m.Steps), m.Version, formatTime(m.MadeAt), m.MadeByModel, m.MadeInChat,
		m.Untrusted, timeOrNull(m.LastWorkedAt), timeOrNull(m.LastRunAt), m.Runs, m.Failures, m.FailStreak, timeOrNull(m.RemovedAt), m.RemovedWhy}
	if m.ID == 0 {
		res, err := tx.Exec(`INSERT INTO desktop_macros (app, site, name, description, params, steps, version, made_at, made_by_model, made_in_chat, untrusted,
			last_worked_at, last_run_at, runs, failures, fail_streak, removed_at, removed_why) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, args...)
		if err != nil {
			return err
		}
		m.ID, err = res.LastInsertId()
		return err
	}
	_, err = tx.Exec(`INSERT INTO desktop_macros (id, app, site, name, description, params, steps, version, made_at, made_by_model, made_in_chat, untrusted,
			last_worked_at, last_run_at, runs, failures, fail_streak, removed_at, removed_why) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET app = excluded.app, site = excluded.site, name = excluded.name, description = excluded.description,
			params = excluded.params, steps = excluded.steps, version = excluded.version, made_at = excluded.made_at,
			made_by_model = excluded.made_by_model, made_in_chat = excluded.made_in_chat, untrusted = excluded.untrusted,
			last_worked_at = excluded.last_worked_at, last_run_at = excluded.last_run_at, runs = excluded.runs, failures = excluded.failures,
			fail_streak = excluded.fail_streak, removed_at = excluded.removed_at, removed_why = excluded.removed_why`, append([]any{m.ID}, args...)...)
	return err
}

// SaveDesktopMacro saves an agent's macro (8.11). One of the same app,
// site and name gets a new version: its runs stay, its failures in a row
// start again. A removed one is replaced whole. The change is logged for
// the notice's Undo. The caller has checked the steps and params.
func (r *Registry) SaveDesktopMacro(ctx context.Context, m DesktopMacro, now time.Time) (DesktopSave, error) {
	var s DesktopSave
	err := r.tx(ctx, func(tx *sql.Tx) error {
		old, err := macroByName(tx, m.App, m.Site, m.Name)
		if err != nil {
			return err
		}
		m.ID, m.Version, m.MadeAt = 0, 1, now
		m.LastWorkedAt, m.LastRunAt, m.Runs, m.Failures, m.FailStreak = now, time.Time{}, 0, 0, 0
		m.RemovedAt, m.RemovedWhy = time.Time{}, ""
		var before any
		if old != nil {
			m.ID = old.ID
			if old.RemovedAt.IsZero() {
				m.Version, m.MadeAt = old.Version+1, old.MadeAt
				m.LastRunAt, m.Runs, m.Failures = old.LastRunAt, old.Runs, old.Failures
				b, err := json.Marshal(old)
				if err != nil {
					return err
				}
				before = string(b)
			} else if _, err := tx.Exec("DELETE FROM desktop_macro_runs WHERE macro_id = ?", old.ID); err != nil {
				return err
			}
		}
		if err := putMacro(tx, &m); err != nil {
			return err
		}
		after, err := json.Marshal(m)
		if err != nil {
			return err
		}
		res, err := tx.Exec("INSERT INTO desktop_saved_changes (at, kind, item_id, before, after, chat_id) VALUES (?, 'macro', ?, ?, ?, ?)",
			formatTime(now), m.ID, before, string(after), m.MadeInChat)
		if err != nil {
			return err
		}
		s.Macro = &m
		s.Change, err = res.LastInsertId()
		return err
	})
	return s, err
}

// EditDesktopMacro is the user's edit in Settings: the description, the
// params and the steps, checked by the caller.
func (r *Registry) EditDesktopMacro(ctx context.Context, mid int64, description string, params []string, steps json.RawMessage) (DesktopMacro, error) {
	var m DesktopMacro
	err := r.tx(ctx, func(tx *sql.Tx) error {
		var err error
		if m, err = macroByID(tx, mid); err != nil {
			return err
		}
		if !m.RemovedAt.IsZero() {
			return fmt.Errorf("desktop macro %d: %w", mid, ErrNotFound)
		}
		m.Description, m.Params, m.Steps = description, params, steps
		m.Version++
		m.FailStreak = 0
		return putMacro(tx, &m)
	})
	return m, err
}

// RemoveDesktopMacro removes a macro; Settings lists it for KeepRemoved.
func (r *Registry) RemoveDesktopMacro(ctx context.Context, mid int64, why string, now time.Time) error {
	return r.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec("UPDATE desktop_macros SET removed_at = ?, removed_why = ? WHERE id = ? AND removed_at IS NULL", formatTime(now), why, mid)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("desktop macro %d: %w", mid, ErrNotFound)
		}
		return nil
	})
}

// RecordDesktopRun records a replay: the macro's counts, its last 20
// runs, and after MacroFailLimit failures in a row its removal, which
// removed reports.
func (r *Registry) RecordDesktopRun(ctx context.Context, run DesktopRun) (removed bool, err error) {
	err = r.tx(ctx, func(tx *sql.Tx) error {
		m, err := macroByID(tx, run.MacroID)
		if err != nil {
			return err
		}
		var failedAt any
		if !run.OK {
			failedAt = run.FailedAt
		}
		if _, err := tx.Exec(`INSERT INTO desktop_macro_runs (macro_id, version, at, ok, failed_at, reason, skipped, seconds, log_chars)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, m.ID, run.Version, formatTime(run.At), run.OK, failedAt, clipRunes(run.Reason, 300), run.Skipped, run.Seconds, run.LogChars); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM desktop_macro_runs WHERE macro_id = ?1 AND id NOT IN
			(SELECT id FROM desktop_macro_runs WHERE macro_id = ?1 ORDER BY id DESC LIMIT ?2)`, m.ID, keepRuns); err != nil {
			return err
		}
		m.Runs++
		m.LastRunAt = run.At
		if run.OK {
			m.FailStreak, m.LastWorkedAt = 0, run.At
		} else {
			m.Failures++
			m.FailStreak++
			if m.FailStreak >= MacroFailLimit && m.RemovedAt.IsZero() {
				m.RemovedAt, m.RemovedWhy, removed = run.At, fmt.Sprintf("failed %d replays in a row", m.FailStreak), true
			}
		}
		return putMacro(tx, &m)
	})
	return removed, err
}

// DesktopRuns is a macro's last runs, newest first.
func (r *Registry) DesktopRuns(ctx context.Context, mid int64) ([]DesktopRun, error) {
	var out []DesktopRun
	err := r.tx(ctx, func(tx *sql.Tx) error {
		var err error
		out, err = queryTx(tx, `SELECT macro_id, version, at, ok, coalesce(failed_at, 0), reason, skipped, seconds, log_chars
			FROM desktop_macro_runs WHERE macro_id = ? ORDER BY id DESC`, []any{mid}, func(rs *sql.Rows) (DesktopRun, error) {
			var d DesktopRun
			var at string
			if err := rs.Scan(&d.MacroID, &d.Version, &at, &d.OK, &d.FailedAt, &d.Reason, &d.Skipped, &d.Seconds, &d.LogChars); err != nil {
				return d, err
			}
			var err error
			d.At, err = parseTime(at)
			return d, err
		})
		return err
	})
	return out, err
}

// DesktopRecipes lists the recipes of the given apps (none for every
// app), without the removed ones, by app, site, name and source.
func (r *Registry) DesktopRecipes(ctx context.Context, apps ...string) ([]DesktopRecipe, error) {
	all, err := json.Marshal(apps)
	if err != nil {
		return nil, err
	}
	var out []DesktopRecipe
	err = r.tx(ctx, func(tx *sql.Tx) error {
		var err error
		out, err = queryTx(tx, "SELECT "+recipeCols+` FROM desktop_recipes WHERE removed_at IS NULL
			AND (json_array_length(?1) = 0 OR app IN (SELECT value FROM json_each(?1))) ORDER BY app, site, name, source`, []any{string(all)}, scanRecipe)
		return err
	})
	return out, err
}

func recipeByID(tx *sql.Tx, rid int64) (DesktopRecipe, error) {
	rs, err := queryTx(tx, "SELECT "+recipeCols+" FROM desktop_recipes WHERE id = ?", []any{rid}, scanRecipe)
	if err != nil {
		return DesktopRecipe{}, err
	}
	if len(rs) == 0 {
		return DesktopRecipe{}, fmt.Errorf("desktop recipe %d: %w", rid, ErrNotFound)
	}
	return rs[0], nil
}

func recipeByName(tx *sql.Tx, app, site, name, source string) (*DesktopRecipe, error) {
	rs, err := queryTx(tx, "SELECT "+recipeCols+" FROM desktop_recipes WHERE app = ? AND site = ? AND name = ? AND source = ?",
		[]any{app, site, name, source}, scanRecipe)
	if err != nil || len(rs) == 0 {
		return nil, err
	}
	return &rs[0], nil
}

func putRecipe(tx *sql.Tx, d *DesktopRecipe) error {
	args := []any{d.App, d.Site, d.Name, d.Text, d.Source, d.AppVersion, formatTime(d.MadeAt), d.MadeByModel, d.MadeInChat, d.Untrusted,
		timeOrNull(d.LastUsedAt), d.Uses, timeOrNull(d.RemovedAt), d.RemovedWhy}
	if d.ID == 0 {
		res, err := tx.Exec(`INSERT INTO desktop_recipes (app, site, name, text, source, app_version, made_at, made_by_model, made_in_chat, untrusted,
			last_used_at, uses, removed_at, removed_why) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, args...)
		if err != nil {
			return err
		}
		d.ID, err = res.LastInsertId()
		return err
	}
	_, err := tx.Exec(`INSERT INTO desktop_recipes (id, app, site, name, text, source, app_version, made_at, made_by_model, made_in_chat, untrusted,
			last_used_at, uses, removed_at, removed_why) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET app = excluded.app, site = excluded.site, name = excluded.name, text = excluded.text,
			source = excluded.source, app_version = excluded.app_version, made_at = excluded.made_at, made_by_model = excluded.made_by_model,
			made_in_chat = excluded.made_in_chat, untrusted = excluded.untrusted, last_used_at = excluded.last_used_at, uses = excluded.uses,
			removed_at = excluded.removed_at, removed_why = excluded.removed_why`, append([]any{d.ID}, args...)...)
	return err
}

// SaveDesktopRecipe saves an agent's recipe, replacing its own recipe of
// that name; the change is logged for the notice's Undo.
func (r *Registry) SaveDesktopRecipe(ctx context.Context, d DesktopRecipe, now time.Time) (DesktopSave, error) {
	var s DesktopSave
	err := r.tx(ctx, func(tx *sql.Tx) error {
		d.Source, d.MadeAt, d.LastUsedAt, d.Uses, d.RemovedAt, d.RemovedWhy, d.ID = RecipeAgent, now, now, 0, time.Time{}, "", 0
		old, err := recipeByName(tx, d.App, d.Site, d.Name, RecipeAgent)
		if err != nil {
			return err
		}
		var before any
		if old != nil {
			d.ID = old.ID
			if old.RemovedAt.IsZero() {
				d.Uses = old.Uses
				b, err := json.Marshal(old)
				if err != nil {
					return err
				}
				before = string(b)
			}
		}
		if err := putRecipe(tx, &d); err != nil {
			return err
		}
		after, err := json.Marshal(d)
		if err != nil {
			return err
		}
		res, err := tx.Exec("INSERT INTO desktop_saved_changes (at, kind, item_id, before, after, chat_id) VALUES (?, 'recipe', ?, ?, ?, ?)",
			formatTime(now), d.ID, before, string(after), d.MadeInChat)
		if err != nil {
			return err
		}
		s.Recipe = &d
		s.Change, err = res.LastInsertId()
		return err
	})
	return s, err
}

// WriteDesktopRecipe is the user's recipe from Settings: a new one when
// rid is 0, or an edit. An edit of a shipped recipe makes the user's own
// of that name, which the agent then sees instead; any other edit makes
// the recipe the user's. The user's recipes are never removed.
func (r *Registry) WriteDesktopRecipe(ctx context.Context, rid int64, d DesktopRecipe, now time.Time) (DesktopRecipe, error) {
	err := r.tx(ctx, func(tx *sql.Tx) error {
		if rid != 0 {
			old, err := recipeByID(tx, rid)
			if err != nil {
				return err
			}
			if !old.RemovedAt.IsZero() {
				return fmt.Errorf("desktop recipe %d: %w", rid, ErrNotFound)
			}
			d.App, d.Site, d.Name = old.App, old.Site, old.Name
			if old.Source != RecipeApp {
				d.ID = old.ID
			}
		}
		mine, err := recipeByName(tx, d.App, d.Site, d.Name, RecipeUser)
		if err != nil {
			return err
		}
		if mine != nil && mine.ID != d.ID {
			if d.ID != 0 { // an agent's recipe edited onto the user's own: one is enough
				if _, err := tx.Exec("DELETE FROM desktop_recipes WHERE id = ?", d.ID); err != nil {
					return err
				}
			}
			d.ID = mine.ID
		}
		d.Source, d.MadeAt, d.LastUsedAt, d.RemovedAt, d.RemovedWhy, d.Untrusted = RecipeUser, now, now, time.Time{}, "", false
		d.MadeByModel, d.MadeInChat, d.AppVersion = "", "", ""
		return putRecipe(tx, &d)
	})
	return d, err
}

// RemoveDesktopRecipe removes a recipe; Settings lists it for KeepRemoved.
// A shipped one comes back with the next update of Jenab.
func (r *Registry) RemoveDesktopRecipe(ctx context.Context, rid int64, why string, now time.Time) error {
	return r.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec("UPDATE desktop_recipes SET removed_at = ?, removed_why = ? WHERE id = ? AND removed_at IS NULL", formatTime(now), why, rid)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("desktop recipe %d: %w", rid, ErrNotFound)
		}
		return nil
	})
}

// UseDesktopRecipes marks recipes as used: shown to an agent.
func (r *Registry) UseDesktopRecipes(ctx context.Context, ids []int64, now time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	b, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	return r.tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE desktop_recipes SET uses = uses + 1, last_used_at = ? WHERE id IN (SELECT value FROM json_each(?))", formatTime(now), string(b))
		return err
	})
}

// ShipDesktopRecipes puts the recipes that come with Jenab in place: each
// replaces the shipped one of its name, keeping its uses. A shipped one
// the user removed stays removed until the text changes.
func (r *Registry) ShipDesktopRecipes(ctx context.Context, ds []DesktopRecipe, version string, now time.Time) error {
	return r.tx(ctx, func(tx *sql.Tx) error {
		for _, d := range ds {
			old, err := recipeByName(tx, d.App, d.Site, d.Name, RecipeApp)
			if err != nil {
				return err
			}
			d.Source, d.AppVersion, d.MadeAt, d.ID = RecipeApp, version, now, 0
			if old != nil {
				if old.Text == d.Text {
					continue
				}
				d.ID, d.Uses, d.LastUsedAt = old.ID, old.Uses, old.LastUsedAt
			}
			if err := putRecipe(tx, &d); err != nil {
				return err
			}
		}
		return nil
	})
}

// PruneDesktop applies the removal rules (8.11) at now: macros unused for
// MacroUnused, agents' recipes unused for RecipeUnused; then it deletes
// what was removed, and Undo rows, older than KeepRemoved.
func (r *Registry) PruneDesktop(ctx context.Context, now time.Time) error {
	return r.tx(ctx, func(tx *sql.Tx) error {
		at := formatTime(now)
		if _, err := tx.Exec(`UPDATE desktop_macros SET removed_at = ?1, removed_why = 'not used for 90 days'
			WHERE removed_at IS NULL AND max(made_at, coalesce(last_worked_at, ''), coalesce(last_run_at, '')) < ?2`, at, formatTime(now.Add(-MacroUnused))); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE desktop_recipes SET removed_at = ?1, removed_why = 'not used for a year'
			WHERE removed_at IS NULL AND source = 'agent' AND max(made_at, coalesce(last_used_at, '')) < ?2`, at, formatTime(now.Add(-RecipeUnused))); err != nil {
			return err
		}
		old := formatTime(now.Add(-KeepRemoved))
		for _, q := range []string{
			"DELETE FROM desktop_macros WHERE removed_at < ?",
			"DELETE FROM desktop_recipes WHERE removed_at < ? AND source <> 'app'",
			"DELETE FROM desktop_saved_changes WHERE at < ?",
		} {
			if _, err := tx.Exec(q, old); err != nil {
				return err
			}
		}
		return nil
	})
}

// DesktopRemoved lists the macros and recipes removed since since,
// newest first.
func (r *Registry) DesktopRemoved(ctx context.Context, since time.Time) (ms []DesktopMacro, rs []DesktopRecipe, err error) {
	err = r.tx(ctx, func(tx *sql.Tx) error {
		s := formatTime(since)
		if ms, err = queryTx(tx, "SELECT "+macroCols+" FROM desktop_macros WHERE removed_at >= ? ORDER BY removed_at DESC", []any{s}, scanMacro); err != nil {
			return err
		}
		rs, err = queryTx(tx, "SELECT "+recipeCols+" FROM desktop_recipes WHERE removed_at >= ? ORDER BY removed_at DESC", []any{s}, scanRecipe)
		return err
	})
	return ms, rs, err
}

// ErrDesktopChanged: the macro or recipe changed after the save, so its
// Undo leaves it as it is.
var ErrDesktopChanged = errors.New("store: the macro or recipe changed since it was saved")

type savedChange struct {
	kind          string
	item          int64
	before, after sql.NullString
	undone        bool
}

func changeByID(tx *sql.Tx, n int64) (savedChange, error) {
	var c savedChange
	err := tx.QueryRow("SELECT kind, item_id, before, after, undone FROM desktop_saved_changes WHERE id = ?", n).Scan(&c.kind, &c.item, &c.before, &c.after, &c.undone)
	if errors.Is(err, sql.ErrNoRows) {
		return c, fmt.Errorf("desktop change %d: %w", n, ErrNotFound)
	}
	return c, err
}

// DesktopSaveUndone reports whether a save's change is undone now.
func (r *Registry) DesktopSaveUndone(ctx context.Context, n int64) (bool, error) {
	var undone bool
	err := r.tx(ctx, func(tx *sql.Tx) error {
		c, err := changeByID(tx, n)
		undone = c.undone
		return err
	})
	return undone, err
}

// UndoDesktopSave reverts a save from its notice, or with redo puts it
// back. The item must be as the save (or the undo) left it, else nothing
// changes and the result says Changed.
func (r *Registry) UndoDesktopSave(ctx context.Context, n int64, redo bool) (DesktopUndo, error) {
	var u DesktopUndo
	err := r.tx(ctx, func(tx *sql.Tx) error {
		c, err := changeByID(tx, n)
		if err != nil {
			return err
		}
		u.Undone = c.undone
		if c.undone != redo {
			return nil // already where it was asked to go
		}
		from, to := c.after, c.before
		if redo {
			from, to = c.before, c.after
		}
		same, err := savedIs(tx, c.kind, c.item, from)
		if err != nil {
			return err
		}
		if !same {
			u.Changed = true
			return nil
		}
		if err := restoreSaved(tx, c.kind, c.item, to); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE desktop_saved_changes SET undone = ? WHERE id = ?", !redo, n); err != nil {
			return err
		}
		u.Done, u.Undone = true, !redo
		return nil
	})
	return u, err
}

// savedIs reports whether the item is as want (JSON of the row; NULL for
// no row, or a removed one): its content, not its counts.
func savedIs(tx *sql.Tx, kind string, item int64, want sql.NullString) (bool, error) {
	if kind == "macro" {
		m, err := macroByID(tx, item)
		if errors.Is(err, ErrNotFound) || err == nil && !m.RemovedAt.IsZero() {
			return !want.Valid, nil
		}
		if err != nil || !want.Valid {
			return false, err
		}
		var w DesktopMacro
		if err := json.Unmarshal([]byte(want.String), &w); err != nil {
			return false, err
		}
		return m.Version == w.Version && m.Description == w.Description && string(m.Steps) == string(w.Steps) && slices.Equal(m.Params, w.Params), nil
	}
	d, err := recipeByID(tx, item)
	if errors.Is(err, ErrNotFound) || err == nil && !d.RemovedAt.IsZero() {
		return !want.Valid, nil
	}
	if err != nil || !want.Valid {
		return false, err
	}
	var w DesktopRecipe
	if err := json.Unmarshal([]byte(want.String), &w); err != nil {
		return false, err
	}
	return d.Text == w.Text && d.Source == w.Source, nil
}

// restoreSaved puts the item back as to says, or deletes it for NULL.
func restoreSaved(tx *sql.Tx, kind string, item int64, to sql.NullString) error {
	table := "desktop_macros"
	if kind == "recipe" {
		table = "desktop_recipes"
	}
	if !to.Valid {
		_, err := tx.Exec("DELETE FROM "+table+" WHERE id = ?", item)
		return err
	}
	if kind == "macro" {
		var m DesktopMacro
		if err := json.Unmarshal([]byte(to.String), &m); err != nil {
			return err
		}
		m.ID = item
		return putMacro(tx, &m)
	}
	var d DesktopRecipe
	if err := json.Unmarshal([]byte(to.String), &d); err != nil {
		return err
	}
	d.ID = item
	return putRecipe(tx, &d)
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
