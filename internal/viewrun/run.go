// Package viewrun runs saved views (SPEC 5.2–5.4, 5.7, 5.10): it turns a
// parsed view (internal/view) and its filter values into what its block
// draws, in plain Go types that the kit's blocks take with little more
// than a copy (P3-09 maps them).
//
// Every query goes through the project's query workers on the read-only
// path (store.ProjectDB.ViewQuery), with 2 s each and only named
// parameters bound, so a view never waits for the writer and never runs
// SQL in the app. The blocks of a page query in parallel and each answers
// on its own. Nothing here calls a model or opens a connection (R-48):
// the Runner reaches the project only through Source.
package viewrun

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/sqlguard"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/view"
)

// Source is what the runtime reads from a project: its queries, on the
// read-only path with a view's deadline, and whether bucket keys have a
// file. *store.ProjectDB is one.
type Source interface {
	// ViewQuery runs a guarded read-only query with a view's 2 s and
	// returns at most max rows (store.ProjectDB.ViewQuery).
	ViewQuery(ctx context.Context, q string, args []any, max int) (store.Rows, error)
	// ObjectsExist reports which keys the bucket has a file at.
	ObjectsExist(ctx context.Context, keys []string) (map[string]bool, error)
}

var _ Source = (*store.ProjectDB)(nil)

// Runner runs views on one project. It is safe for concurrent use.
type Runner struct {
	Source Source
	// Now is the time in the project's local zone, for relative date
	// filters (today-90d); nil is time.Now, in the computer's zone.
	Now func() time.Time
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// Values are filter values by parameter name, as the filter controls bind
// them: a string, int64, float64, bool, or nil. A parameter that is there
// with nil was cleared, and binds NULL unless its filter is required; one
// that isn't there takes its filter's default.
type Values map[string]any

// ViewRequest is what one view runs with.
type ViewRequest struct {
	// Values are the view's own filters' values. On a page, the page's
	// filters (PageRequest.Values) win for the parameters they have.
	Values Values
	// Table is a table's sort and page; the zero value is its
	// default_sort and the first page.
	Table TableRequest
}

// TableRequest is the sort and page a table shows.
type TableRequest struct {
	// Sort is the column to sort by; nil, or a column the table doesn't
	// show or can't sort (image, file), is the view's default_sort.
	Sort *Sort
	// Page is the page shown, from 0.
	Page int
}

// Sort is a table's sort: one of its columns' fields, as written.
type Sort struct {
	Field string
	Desc  bool
}

// Block is what one data block shows: its view's data, or why there is
// none. Error and Notice are the kit's BlockState's (its Loading is the
// page screen's own, until the Block arrives).
type Block struct {
	// Place is where the block is on its page: its row and its place in
	// the row, then the same inside a card ("2.0.0.1"); "" for a view run
	// on its own (Runner.View).
	Place string
	View  string // the view's ID
	Type  string // table, chart, stat or form
	Title string
	// Filters are the view's own filters, with what they bound, and
	// their options. On a page, those the page's filters cover are left
	// out: the page shows its own.
	Filters []Filter
	// Params are the parameters the view's query binds, so a page re-runs
	// only the blocks a changed filter reaches.
	Params []string
	// Error is why the block can't show its data, in place of it; ""
	// for none.
	Error string
	// Notice is a line above the data, as the 10,000-row notice.
	Notice string
	// One of these, by Type, unless Error is set; a form has none.
	Table *Table
	Chart *Chart
	Stat  *Stat
	// Took is how long the block took, its queries and the work on
	// their rows.
	Took time.Duration
}

// View runs v on its own, with its own filters.
func (r *Runner) View(ctx context.Context, v *view.View, req ViewRequest) Block {
	return r.safeView(ctx, v, req, pageBinding{}, newOptions(r))
}

// safeView is view, with a panic turned into an error on the block only.
func (r *Runner) safeView(ctx context.Context, v *view.View, req ViewRequest, pb pageBinding, opts *options) (b Block) {
	defer func() {
		if p := recover(); p != nil {
			b = Block{View: v.ID, Type: v.Type, Title: v.Title, Error: fmt.Sprintf("Something went wrong showing this view: %v.", p)}
		}
	}()
	return r.view(ctx, v, req, pb, opts)
}

// PageRequest is what a page runs with.
type PageRequest struct {
	// Values are the page's filters' values.
	Values Values
	// Blocks are each block's own filter values and table state, by its
	// place (Block.Place), so two blocks of one view keep their own.
	Blocks map[string]ViewRequest
	// Only, when not nil, runs only the blocks at these places, as when
	// a filter changed that only they use; nil runs them all.
	Only []string
}

// Answer is one answer of a page's run: first the page's filters, then
// each block as it answers.
type Answer struct {
	// Filters, in the first answer only, are the page's filters with
	// what they bound and their options.
	Filters []Filter
	// Block is one block, in each answer after the first.
	Block *Block
}

// Page runs the data blocks of p in parallel, each view taken from views
// by ID. out gets the page's filters first, then each block as it
// answers: one call at a time, from the goroutine that called Page, with
// no lock held. A view that views can't give, or a block that panics, is
// an error on its block. When ctx ends, the queries still running are
// stopped (their workers are killed), out gets nothing more, and Page
// returns ctx's error once every block has stopped.
func (r *Runner) Page(ctx context.Context, p *view.Page, views Views, req PageRequest, out func(Answer)) error {
	opts := newOptions(r)
	filters, values := r.filters(ctx, p.Filters, req.Values, opts)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	out(Answer{Filters: filters})
	only := map[string]bool{}
	for _, pl := range req.Only {
		only[pl] = true
	}
	answers := make(chan *Block)
	var wg sync.WaitGroup
	for _, pv := range Places(p) {
		if req.Only != nil && !only[pv.Place] {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			b := r.placed(ctx, pv, views, req.Blocks[pv.Place], pageBinding{p.Filters, values}, opts)
			select {
			case answers <- &b:
			case <-ctx.Done():
			}
		}()
	}
	go func() {
		wg.Wait()
		close(answers)
	}()
	for b := range answers {
		if ctx.Err() == nil {
			out(Answer{Block: b})
		}
	}
	return ctx.Err()
}

// placed runs the view of one block of a page.
func (r *Runner) placed(ctx context.Context, pv PlacedView, views Views, req ViewRequest, pb pageBinding, opts *options) (b Block) {
	defer func() {
		if p := recover(); p != nil {
			b = Block{View: pv.View, Error: fmt.Sprintf("Something went wrong showing this view: %v.", p)}
		}
		b.Place = pv.Place
	}()
	v, err := views(pv.View)
	if err != nil {
		return Block{View: pv.View, Error: err.Error()}
	}
	return r.safeView(ctx, v, req, pb, opts)
}

// Views gives a page's views by ID; it is called from several
// goroutines at once. Its error, shown on the view's block, says why there
// is none, as "There is no view x.".
type Views func(id string) (*view.View, error)

// ViewMap gives the views in m, and "There is no view x." for the others.
func ViewMap(m map[string]*view.View) Views {
	return func(id string) (*view.View, error) {
		if v := m[id]; v != nil {
			return v, nil
		}
		return nil, fmt.Errorf("There is no view %s.", id)
	}
}

// PlacedView is a view block of a page and where it is.
type PlacedView struct {
	Place string // as Block.Place
	View  string // the view's ID
}

// Places lists the view blocks of p in reading order: row by row, and in
// a card, its rows where the card is.
func Places(p *view.Page) []PlacedView {
	var out []PlacedView
	var walk func(prefix string, rows [][]view.Block)
	walk = func(prefix string, rows [][]view.Block) {
		for i, row := range rows {
			for j, b := range row {
				place := prefix + strconv.Itoa(i) + "." + strconv.Itoa(j)
				if vb, ok := b.Spec.(*view.ViewBlock); ok {
					out = append(out, PlacedView{Place: place, View: vb.View})
				}
				if len(b.Rows) > 0 {
					walk(place+".", b.Rows)
				}
			}
		}
	}
	walk("", p.Rows)
	return out
}

// pageBinding is a page's filters and the values they bound, which win
// over a view's own for the parameters they have.
type pageBinding struct {
	filters []view.Filter
	values  map[string]any
}

func (pb pageBinding) has(param string) bool {
	for _, f := range pb.filters {
		if f.Param == param {
			return true
		}
	}
	return false
}

// view runs one view, its filters resolved and the page's (if any)
// applied.
func (r *Runner) view(ctx context.Context, v *view.View, req ViewRequest, pb pageBinding, opts *options) (b Block) {
	begin := time.Now()
	b = Block{View: v.ID, Type: v.Type, Title: v.Title}
	defer func() { b.Took = time.Since(begin) }()
	var own []view.Filter
	for _, f := range v.Filters {
		if !pb.has(f.Param) {
			own = append(own, f)
		}
	}
	var values map[string]any
	b.Filters, values = r.filters(ctx, own, req.Values, opts)
	if v.Query == "" {
		return b // a form: P3-06 runs its load and its options
	}
	names, err := params(v.Query)
	if err != nil {
		b.Error = queryMessage(err)
		return b
	}
	b.Params = names
	args := make([]any, 0, len(names))
	for _, n := range names {
		var val any
		switch {
		case pb.has(n):
			val = pb.values[n]
		default:
			if x, ok := values[n]; ok {
				val = x
			} else {
				val = v.Params[n]
			}
		}
		args = append(args, sql.Named(n, val))
	}
	var err2 error
	switch spec := v.Spec.(type) {
	case *view.Table:
		b.Table, b.Notice, err2 = r.table(ctx, v, spec, args, req.Table)
	case *view.Chart:
		b.Chart, b.Notice, err2 = r.chart(ctx, v, spec, args)
	case *view.Stat:
		b.Stat, err2 = r.stat(ctx, v, spec, args)
	default:
		err2 = fmt.Errorf("a %s view can't be shown here", v.Type)
	}
	if err2 != nil {
		b.Error = queryMessage(err2)
	}
	return b
}

// params returns the :name parameters of q, without the colon. A
// parameter written another way, which validation refuses, is an error.
func params(q string) ([]string, error) {
	ps, err := sqlguard.Params(q)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		if p.Name == "" || p.Name[0] != ':' {
			return nil, fmt.Errorf("the query has the parameter %s; only :name parameters are bound", cmpOr(p.Name, "?"+strconv.Itoa(p.Number)))
		}
		out = append(out, p.Name[1:])
	}
	return out, nil
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// wrapped puts q, which must pass the guard's text check, inside
// brackets for an outer statement, as SPEC 5.3 and 5.4 wrap it: its
// final ';' dropped, and the bracket closed on a line of its own so a
// final -- comment can't hide it.
func wrapped(q string) (string, error) {
	body, err := sqlguard.Body(q)
	if err != nil {
		return "", err
	}
	return "(\n" + body + "\n)", nil
}

// quoteName quotes a result column's name for SQL.
func quoteName(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// timedOut reports whether err is a query that ran out of its 2 s while
// the caller still waited.
func timedOut(ctx context.Context, err error) bool {
	return errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil
}

// queryMessage words a failed query for its block.
func queryMessage(err error) string {
	var re *sqlguard.RejectError
	switch {
	case errors.As(err, &re):
		return "The query was refused: " + re.Reason
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Sprintf("The query took longer than %v and was stopped. Make it faster, or aggregate in the query.", store.ViewTimeout)
	case errors.Is(err, store.ErrClosed):
		return "The project is closed."
	}
	msg := err.Error()
	msg = strings.TrimPrefix(msg, "SQL logic error: ")
	if i := strings.LastIndex(msg, " ("); i > 0 && strings.HasSuffix(msg, ")") {
		msg = msg[:i]
	}
	if msg != "" {
		msg = strings.ToUpper(msg[:1]) + msg[1:]
	}
	if !strings.HasSuffix(msg, ".") {
		msg += "."
	}
	return msg
}

// columnIndex returns the index of each field among cols, matched as
// written, as validation matches them (P3-02); a field the query doesn't
// return is an error naming the columns it has.
func columnIndex(cols []string, fields ...string) ([]int, error) {
	out := make([]int, len(fields))
	for i, f := range fields {
		out[i] = -1
		for j, c := range cols {
			if c == f {
				out[i] = j
				break
			}
		}
		if out[i] < 0 {
			return nil, fmt.Errorf("'%s' is not a result column of the query; it has %s; names are matched as written", f, strings.Join(cols, ", "))
		}
	}
	return out, nil
}
