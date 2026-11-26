package viewwrite

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/view"
)

// Acted is what a row action or a view's action did, or asks for.
type Acted struct {
	// Confirm is the question to ask before the action writes, as "Delete
	// 3 rows?"; nothing was written. The page asks it and runs the action
	// again with confirmed.
	Confirm string `json:"confirm,omitempty"`
	// OpenForm is the form to open, with Key the row it edits.
	OpenForm string `json:"open_form,omitempty"`
	Key      any    `json:"key,omitempty"`
	// OpenPage is the page a view's action opens.
	OpenPage string `json:"open_page,omitempty"`
	// Change is the write's change-log entry, which the notice's Undo
	// undoes; Table the table it wrote, so the page re-runs its blocks.
	Change id.Change `json:"change,omitempty"`
	Table  string    `json:"table,omitempty"`
	// Rows are the rows written; Skipped those that were no longer there.
	Rows    int `json:"rows,omitempty"`
	Skipped int `json:"skipped,omitempty"`
	// Run is the run a run_pipeline action started.
	Run id.Run `json:"run,omitempty"`
	// Notice is the line the page shows, with Undo when Change is set.
	Notice string `json:"notice,omitempty"`
	// Problem is why the action did nothing: "Choose a row first.",
	// "already running since 08:00".
	Problem string `json:"problem,omitempty"`
}

// RowAction runs the row action index of the table view v on the rows
// with keys, the rows_from key values of the chosen rows of the loaded
// page (viewrun.Table.Keys). An action with confirm only asks until
// confirmed is true. A bulk action takes at most MaxBulk rows, written as
// one request with one change-log entry; an action without bulk takes
// one row.
func (w *Writer) RowAction(ctx context.Context, v *view.View, index int, keys []any, confirmed bool) (Acted, error) {
	t, ok := v.Spec.(*view.Table)
	if !ok || t == nil {
		return Acted{}, badRequest("%s is a %s view, not a table", v.ID, v.Type)
	}
	if index < 0 || index >= len(t.RowActions) {
		return Acted{}, badRequest("%s has no row action %d", v.ID, index)
	}
	if t.RowsFrom == nil {
		return Acted{}, badRequest("%s has row actions but no rows_from", v.ID)
	}
	a := t.RowActions[index]
	switch {
	case len(keys) == 0:
		return Acted{Problem: "Choose a row first."}, nil
	case len(keys) > 1 && !a.Bulk:
		return Acted{}, badRequest("the row action %q of %s takes one row, not %d", a.Label, v.ID, len(keys))
	}
	shape, err := w.DB.TableShape(ctx, t.RowsFrom.Table)
	if errors.Is(err, store.ErrNotFound) {
		return Acted{Problem: fmt.Sprintf("There is no table %s any more.", t.RowsFrom.Table)}, nil
	}
	if err != nil {
		return Acted{}, err
	}
	keyCol := t.RowsFrom.Key
	if col, ok := shape.Column(keyCol); ok {
		keyCol = col.Name
	}
	// Each key is read for its column's type; a key given twice is one
	// row.
	var rows []store.Row
	seen := map[any]bool{}
	for _, k := range keys {
		k, err := keyOf(shape, keyCol, k)
		if err != nil {
			return Acted{}, err
		}
		if !seen[k] {
			seen[k] = true
			rows = append(rows, store.Row{keyCol: k})
		}
	}
	if len(rows) > MaxBulk {
		return Acted{Problem: fmt.Sprintf("At most %d rows at once; %d are chosen.", MaxBulk, len(rows))}, nil
	}
	if a.OpenForm != "" {
		return Acted{OpenForm: a.OpenForm, Key: rows[0][keyCol]}, nil
	}
	if a.Confirm && !confirmed {
		return Acted{Confirm: confirmQuestion(a, len(rows))}, nil
	}
	src := Source(v.ID)
	out := Acted{Table: shape.Name}
	var res store.WriteResult
	var set store.Row
	var problem string
	switch {
	case a.Delete:
		res, err = w.DB.Delete(ctx, limit.Interactive, src, shape.Name, rows)
		out.Rows = res.Deleted
		out.Notice = fmt.Sprintf("Deleted %s from %s.", plural(res.Deleted, "row"), shape.Name)
	case a.Set != nil:
		set, problem = w.setRow(a.Set, shape, keyCol)
		if problem != "" {
			return Acted{Problem: problem}, nil
		}
		res, err = w.DB.Update(ctx, limit.Interactive, src, store.UpdateReq{Table: shape.Name, Set: set, Keys: rows})
		out.Rows = res.Updated
		out.Notice = fmt.Sprintf("Updated %s in %s.", plural(res.Updated, "row"), shape.Name)
	default:
		return Acted{}, badRequest("the row action %q of %s does nothing", a.Label, v.ID)
	}
	if err != nil {
		if key, ok := store.MissingObject(err); ok {
			return Acted{Problem: fmt.Sprintf("Not saved: there is no file %s.", key)}, nil
		}
		var ce *store.ConstraintError
		if errors.As(err, &ce) {
			if a.Delete {
				_, msg := constraintText(ce, shape, nil, nil, func(string) string { return "" }, "delete", w.referrers(ctx, shape.Name)...)
				return Acted{Problem: "Not deleted: " + msg + "."}, nil
			}
			// A set action has no fields: its columns stand for them.
			cols := slices.Collect(maps.Keys(set))
			field, msg := constraintText(ce, shape, set, cols, func(c string) string {
				for k := range set {
					if strings.EqualFold(k, c) {
						return k
					}
				}
				return ""
			}, "set")
			if field != "" {
				msg = field + " " + msg
			}
			return Acted{Problem: "Not saved: " + msg + "."}, nil
		}
		if errors.Is(err, store.ErrInvalid) && ctx.Err() == nil {
			return Acted{Problem: "Not saved: " + message(err)}, nil
		}
		return Acted{}, err
	}
	out.Change, out.Skipped = res.Change, res.Skipped
	if out.Rows == 0 {
		out.Notice = ""
		out.Problem = "The rows are no longer there."
		if len(keys) == 1 {
			out.Problem = "This row is no longer there."
		}
	} else if res.Cascaded > 0 {
		out.Notice = strings.TrimSuffix(out.Notice, ".") + fmt.Sprintf(", and %s that refer to them.", plural(res.Cascaded, "row"))
	}
	return out, nil
}

// setRow makes the row a set action writes: its literals, now() as the
// UTC time in RFC 3339 and today() as the project's date, each checked
// against its column as a form's values are.
func (w *Writer) setRow(set map[string]any, shape store.Table, keyCol string) (store.Row, string) {
	row := store.Row{}
	for _, field := range slices.Sorted(maps.Keys(set)) {
		col, ok := shape.Column(field)
		if !ok {
			return nil, fmt.Sprintf("Not saved: %s has no column %s any more.", shape.Name, field)
		}
		if strings.EqualFold(col.Name, keyCol) || slices.ContainsFunc(shape.Key, func(k string) bool { return strings.EqualFold(k, col.Name) }) {
			return nil, fmt.Sprintf("Not saved: %s is the key of %s, which set can't change.", col.Name, shape.Name)
		}
		var val any = set[field]
		switch val {
		case "now()":
			val = dateText(w.now().UTC().Format(time.RFC3339)) // as pipelines write it (SPEC 6.4)
		case "today()":
			val = dateText(w.now().Format(time.DateOnly))
		}
		if i, ok := val.(int); ok { // a literal from Go, not YAML
			val = int64(i)
		}
		fitted, problem := fit(val, col.Type, shape.Strict)
		if problem != "" {
			return nil, fmt.Sprintf("Not saved: %s %s.", field, problem)
		}
		if fitted == nil && col.NotNull {
			return nil, fmt.Sprintf("Not saved: %s needs a value.", field)
		}
		row[col.Name] = fitted
	}
	if len(row) == 0 {
		return nil, "Not saved: the action sets nothing."
	}
	return row, ""
}

// referrers are the tables whose foreign keys refer to table, with no
// action that deletes or changes their rows with it: those a delete can
// be stopped by. On an error it gives none; the message then names none.
func (w *Writer) referrers(ctx context.Context, table string) []string {
	names, err := w.DB.Tables(ctx)
	if err != nil {
		return nil
	}
	var out []string
	for _, n := range names {
		t, err := w.DB.TableShape(ctx, n)
		if err != nil {
			continue
		}
		for _, fk := range t.ForeignKeys {
			action := strings.ToUpper(fk.OnDelete)
			if strings.EqualFold(fk.Table, table) && action != "CASCADE" && action != "SET NULL" && action != "SET DEFAULT" {
				out = append(out, t.Name)
				break
			}
		}
	}
	return out
}

// confirmQuestion is what an action with confirm asks.
func confirmQuestion(a view.RowAction, n int) string {
	label := cmp.Or(a.Label, "Change")
	if a.Delete {
		if n == 1 {
			return "Delete this row?"
		}
		return fmt.Sprintf("Delete %d rows?", n)
	}
	if n == 1 {
		return label + " this row?"
	}
	return fmt.Sprintf("%s %d rows?", label, n)
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}

// Action runs the action index of view v, a button of its page block: a
// run_pipeline starts its pipeline, as form:<view_id>; open_page and
// open_form say what to open. An action with confirm only asks until
// confirmed is true.
func (w *Writer) Action(ctx context.Context, v *view.View, index int, confirmed bool) (Acted, error) {
	if index < 0 || index >= len(v.Actions) {
		return Acted{}, badRequest("%s has no action %d", v.ID, index)
	}
	a := v.Actions[index]
	switch {
	case a.OpenPage != "":
		return Acted{OpenPage: a.OpenPage}, nil
	case a.OpenForm != "":
		return Acted{OpenForm: a.OpenForm}, nil
	case a.RunPipeline == "":
		return Acted{}, badRequest("the action %q of %s does nothing", a.Label, v.ID)
	}
	if a.Confirm && !confirmed {
		return Acted{Confirm: fmt.Sprintf("Run %s?", a.RunPipeline)}, nil
	}
	run, problem, err := w.runPipeline(ctx, a.RunPipeline, nil, Source(v.ID))
	if err != nil {
		return Acted{}, err
	}
	if problem != "" {
		return Acted{Problem: problem}, nil
	}
	return Acted{Run: run, Notice: fmt.Sprintf("Started %s.", a.RunPipeline)}, nil
}
