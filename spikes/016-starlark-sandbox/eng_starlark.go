package main

import (
	"context"
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"
)

// lastStarlarkSteps is the step count of the last run (for the report).
var lastStarlarkSteps uint64

type starlarkEngine struct{}

func (starlarkEngine) Name() string { return "starlark" }

// The script sets a global `result`. Recursion stays off (the default).
var starlarkOpts = &syntax.FileOptions{While: true, TopLevelControl: true, GlobalReassign: true, Set: true}

func (starlarkEngine) Run(ctx context.Context, src string, o RunOpts) (any, error) {
	th := &starlark.Thread{Name: "script", Print: func(*starlark.Thread, string) {}}
	// th.Load stays nil, so load() fails.
	if !o.Default && !o.NoSteps {
		th.SetMaxExecutionSteps(stepLimit)
	}
	stop := context.AfterFunc(ctx, func() { th.Cancel(causeText(ctx)) })
	defer stop()
	opts := starlarkOpts
	if o.Default {
		opts = &syntax.FileOptions{TopLevelControl: true, GlobalReassign: true}
	}
	pre := starlark.StringDict{}
	if o.DB != nil {
		pre = starlarkHost(ctx, o.DB)
	}
	g, err := starlark.ExecFileOptions(opts, th, "script.star", src, pre)
	lastStarlarkSteps = th.ExecutionSteps()
	if err != nil {
		return nil, err
	}
	v, ok := g["result"]
	if !ok {
		return nil, nil
	}
	return fromStarlark(v)
}

func starlarkHost(ctx context.Context, d *DB) starlark.StringDict {
	query := starlark.NewBuiltin("db.query", func(th *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kw []starlark.Tuple) (starlark.Value, error) {
		var q string
		var params *starlark.List
		if err := starlark.UnpackArgs("query", args, kw, "sql", &q, "params?", &params); err != nil {
			return nil, err
		}
		var ps []any
		if params != nil {
			for i := 0; i < params.Len(); i++ {
				v, err := fromStarlark(params.Index(i))
				if err != nil {
					return nil, err
				}
				ps = append(ps, v)
			}
		}
		rows, err := d.Query(ctx, q, ps)
		if err != nil {
			return nil, err
		}
		out := make([]starlark.Value, len(rows.Data))
		for i, r := range rows.Data {
			dict := starlark.NewDict(len(rows.Cols))
			for j, c := range rows.Cols {
				dict.SetKey(starlark.String(c), toStarlark(r[j]))
			}
			out[i] = dict
		}
		return starlark.NewList(out), nil
	})
	return starlark.StringDict{
		"db": &starlarkstruct.Module{Name: "db", Members: starlark.StringDict{"query": query}},
		"round": starlark.NewBuiltin("round", func(th *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kw []starlark.Tuple) (starlark.Value, error) {
			var x starlark.Value
			digits := 0
			if err := starlark.UnpackArgs("round", args, kw, "x", &x, "digits?", &digits); err != nil {
				return nil, err
			}
			f, ok := starlark.AsFloat(x)
			if !ok {
				return nil, fmt.Errorf("round: want a number")
			}
			return starlark.Float(fnRound(f, digits)), nil
		}),
		"add_days": starlark.NewBuiltin("add_days", func(th *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kw []starlark.Tuple) (starlark.Value, error) {
			var d string
			var n int
			if err := starlark.UnpackArgs("add_days", args, kw, "d", &d, "n", &n); err != nil {
				return nil, err
			}
			s, err := fnAddDays(d, n)
			return starlark.String(s), err
		}),
		"format_date": starlark.NewBuiltin("format_date", func(th *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kw []starlark.Tuple) (starlark.Value, error) {
			var d, layout string
			if err := starlark.UnpackArgs("format_date", args, kw, "d", &d, "layout", &layout); err != nil {
				return nil, err
			}
			s, err := fnFormatDate(d, layout)
			return starlark.String(s), err
		}),
	}
}

func toStarlark(v any) starlark.Value {
	switch x := v.(type) {
	case nil:
		return starlark.None
	case int64:
		return starlark.MakeInt64(x)
	case float64:
		return starlark.Float(x)
	case string:
		return starlark.String(x)
	case bool:
		return starlark.Bool(x)
	}
	return starlark.String(fmt.Sprint(v))
}

func fromStarlark(v starlark.Value) (any, error) {
	switch x := v.(type) {
	case starlark.NoneType:
		return nil, nil
	case starlark.Bool:
		return bool(x), nil
	case starlark.Int:
		if i, ok := x.Int64(); ok {
			return i, nil
		}
		return fmt.Sprintf("big int (%d bits)", x.BigInt().BitLen()), nil
	case starlark.Float:
		return float64(x), nil
	case starlark.String:
		if len(x) > 64 {
			return fmt.Sprintf("string (%d bytes)", len(x)), nil
		}
		return string(x), nil
	case *starlark.List:
		out := make([]any, x.Len())
		for i := range out {
			e, err := fromStarlark(x.Index(i))
			if err != nil {
				return nil, err
			}
			out[i] = e
		}
		return out, nil
	case *starlark.Dict:
		out := make(map[string]any, x.Len())
		for _, it := range x.Items() {
			k, ok := it[0].(starlark.String)
			if !ok {
				return nil, fmt.Errorf("dict key must be a string")
			}
			e, err := fromStarlark(it[1])
			if err != nil {
				return nil, err
			}
			out[string(k)] = e
		}
		return out, nil
	}
	return nil, fmt.Errorf("unsupported result type %s", v.Type())
}
