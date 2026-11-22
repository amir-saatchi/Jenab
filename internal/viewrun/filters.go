package viewrun

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/view"
)

// Filter is a filter control and what it bound (SPEC 5.2). It maps onto
// the kit's FilterSpec (Label, Control, Required, Options as text) and the
// value the control starts with.
type Filter struct {
	Param    string
	Control  string // text, number, date, select or checkbox
	Label    string
	Required bool
	// Options are a select's choices: its list, or the rows of its
	// options query, at most MaxOptions.
	Options []any
	// Value is what the filter bound: nil (NULL), a string (text, a
	// select's text, and a date as ISO text, 2026-11-20), an int64 or a
	// float64 (a number, a select's number), or a bool (a checkbox).
	Value any
	// Error is why the filter's value or options couldn't be read; the
	// filter then binds its default.
	Error string
}

// MaxOptions is the most choices a select's options query gives.
const MaxOptions = 1000

// NeedsValue is a required filter's Error when it was given no value: it
// binds its default, or a select its first option.
const NeedsValue = "This filter needs a value."

// filters resolves fs with the values given: defaults for those not
// given, relative dates in the project's local time, NULL for a cleared
// optional filter, and a select's options, their queries run at once. It
// returns the filters and what each binds, by parameter. A value given
// as "" or blanks is cleared, as nil is.
func (r *Runner) filters(ctx context.Context, fs []view.Filter, given Values, opts *options) ([]Filter, map[string]any) {
	out := make([]Filter, 0, len(fs))
	values := make(map[string]any, len(fs))
	today := r.now()
	optionsOf := make([][]any, len(fs))
	problems := make([]string, len(fs))
	var wg sync.WaitGroup
	for i, f := range fs {
		if f.Options == nil {
			continue
		}
		if f.Options.Query == "" {
			optionsOf[i] = f.Options.List
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if p := recover(); p != nil {
					problems[i] = fmt.Sprintf("Can't read the options: something went wrong: %v.", p)
				}
			}()
			optionsOf[i], problems[i] = opts.get(ctx, f.Options.Query)
		}()
	}
	wg.Wait()
	for i, f := range fs {
		rf := Filter{Param: f.Param, Control: f.Control, Label: f.Label, Required: f.Required, Options: optionsOf[i], Error: problems[i]}
		v, has := given[f.Param]
		if has {
			var problem string
			v, problem = readValue(f, v, today, rf.Options)
			switch {
			case problem != "":
				rf.Error, has = problem, false
			case v == nil && f.Required && f.Control != "checkbox":
				// A required checkbox unchecked is false, not missing.
				rf.Error = NeedsValue
			}
		}
		if !has || v == nil && f.Required {
			v = defaultValue(f, today)
		}
		if v == nil && f.Required {
			switch f.Control {
			case "select":
				if len(rf.Options) > 0 {
					v = rf.Options[0]
				}
			case "checkbox":
				v = false
			}
		}
		if f.Control == "checkbox" && v == false && !f.Required {
			v = nil // an optional checkbox unchecked binds NULL
		}
		rf.Value = v
		values[f.Param] = v
		out = append(out, rf)
	}
	return out, values
}

// defaultValue is f's default as it binds: a date's relative form counted
// back from today.
func defaultValue(f view.Filter, today time.Time) any {
	if f.Default == nil {
		return nil
	}
	if f.Control == "date" {
		if s, ok := f.Default.(string); ok {
			if t, ok := view.RelativeDate(s, today); ok {
				return t.Format(time.DateOnly)
			}
		}
		return nil
	}
	return f.Default
}

// readValue reads a value given for f; problem says why it can't bind.
// "" and blanks are no value, as nil is.
func readValue(f view.Filter, v any, today time.Time, options []any) (any, string) {
	if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
		v = nil
	}
	if v == nil {
		return nil, ""
	}
	switch f.Control {
	case "date":
		switch x := v.(type) {
		case string:
			// The relative forms too, as the agent's open_page may give them.
			if t, ok := view.RelativeDate(x, today); ok {
				return t.Format(time.DateOnly), ""
			}
		case time.Time:
			return x.Format(time.DateOnly), ""
		}
		return nil, fmt.Sprintf("%v is not a date; write it as 2026-11-20", v)
	case "number":
		switch n := number(v).(type) {
		case int64:
			return n, ""
		case float64:
			if !math.IsNaN(n) && !math.IsInf(n, 0) {
				return n, ""
			}
		}
		return nil, fmt.Sprintf("%v is not a number", v)
	case "checkbox":
		if b, ok := boolOf(v); ok {
			return b, ""
		}
		return nil, fmt.Sprintf("%v is not true or false", v)
	case "select":
		// The kit's select binds an option's text: bind the option itself,
		// so a number stays a number.
		if s, ok := v.(string); ok {
			for _, o := range options {
				if text(o) == s {
					return o, ""
				}
			}
		}
		return v, ""
	}
	switch x := v.(type) {
	case string, int64, float64, bool:
		return x, ""
	}
	return text(v), ""
}

// options runs the options queries of one run, each once.
type options struct {
	r  *Runner
	mu sync.Mutex
	m  map[string]*optionsCall
}

type optionsCall struct {
	done    chan struct{}
	options []any
	problem string
}

func newOptions(r *Runner) *options { return &options{r: r, m: map[string]*optionsCall{}} }

// get returns the first column of q's rows, at most MaxOptions; problem
// says why there are none.
func (o *options) get(ctx context.Context, q string) ([]any, string) {
	o.mu.Lock()
	c, ok := o.m[q]
	if !ok {
		c = &optionsCall{done: make(chan struct{})}
		o.m[q] = c
	}
	o.mu.Unlock()
	if ok {
		select {
		case <-c.done:
		case <-ctx.Done():
			return nil, ""
		}
		return c.options, c.problem
	}
	defer close(c.done)
	c.options, c.problem = o.r.Options(ctx, q)
	return c.options, c.problem
}

// Options runs a select's options query (SPEC 5.2, 5.5): one column, no
// parameters. It returns the first column's values, at most MaxOptions,
// or why there are none.
func (r *Runner) Options(ctx context.Context, q string) ([]any, string) {
	// One more than kept, so a full list doesn't make the worker count
	// the query again.
	rows, err := r.Source.ViewQuery(ctx, q, nil, MaxOptions+1)
	if err != nil {
		return nil, "Can't read the options: " + queryMessage(err)
	}
	vals := rows.Values[:min(len(rows.Values), MaxOptions)]
	out := make([]any, 0, len(vals))
	for _, row := range vals {
		if len(row) > 0 && row[0] != nil {
			out = append(out, textOrNumber(row[0]))
		}
	}
	return out, ""
}
