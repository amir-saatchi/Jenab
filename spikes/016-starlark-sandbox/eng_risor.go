package main

import (
	"context"
	"fmt"

	"github.com/deepnoodle-ai/risor/v2"
	"github.com/deepnoodle-ai/risor/v2/pkg/object"
)

type risorEngine struct{}

func (risorEngine) Name() string { return "risor" }

// The script's last expression is the result.
func (risorEngine) Run(ctx context.Context, src string, o RunOpts) (any, error) {
	env := risor.Builtins()
	var opts []risor.Option
	if !o.Default {
		delete(env, "rand") // sandbox: no randomness
		if !o.NoSteps {
			opts = append(opts, risor.WithMaxSteps(stepLimit))
		}
		opts = append(opts, risor.WithMaxStackDepth(stackLimit))
	}
	if o.DB != nil {
		risorHost(ctx, env, o.DB)
	}
	opts = append(opts, risor.WithEnv(env))
	code, err := risor.Compile(ctx, src, opts...)
	if err != nil {
		return nil, err
	}
	v, err := risor.Run(ctx, code, opts...)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w (%s)", err, causeText(ctx))
		}
		return nil, err
	}
	return normalize(v), nil
}

func risorHost(ctx context.Context, env map[string]any, d *DB) {
	query := object.NewBuiltin("query", func(_ context.Context, args ...object.Object) (object.Object, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("query: want sql")
		}
		q, ok := args[0].(*object.String)
		if !ok {
			return nil, fmt.Errorf("query: sql must be a string")
		}
		var ps []any
		if len(args) > 1 {
			if l, ok := args[1].(*object.List); ok {
				for _, e := range l.Value() {
					ps = append(ps, e.Interface())
				}
			}
		}
		rows, err := d.Query(ctx, q.Value(), ps)
		if err != nil {
			return nil, err
		}
		out := make([]object.Object, len(rows.Data))
		for i, r := range rows.Data {
			m := make(map[string]object.Object, len(rows.Cols))
			for j, c := range rows.Cols {
				m[c] = object.FromGoType(r[j])
			}
			out[i] = object.NewMap(m)
		}
		return object.NewList(out), nil
	})
	env["db"] = object.NewBuiltinsModule("db", map[string]object.Object{"query": query})
	env["round"] = object.NewBuiltin("round", func(_ context.Context, args ...object.Object) (object.Object, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("round: want 2 arguments")
		}
		x, err := risorFloat(args[0])
		if err != nil {
			return nil, err
		}
		n, ok := args[1].(*object.Int)
		if !ok {
			return nil, fmt.Errorf("round: digits must be int")
		}
		return object.NewFloat(fnRound(x, int(n.Value()))), nil
	})
	env["add_days"] = object.NewBuiltin("add_days", func(_ context.Context, args ...object.Object) (object.Object, error) {
		s, ok1 := args[0].(*object.String)
		n, ok2 := args[1].(*object.Int)
		if len(args) != 2 || !ok1 || !ok2 {
			return nil, fmt.Errorf("add_days: want (string, int)")
		}
		r, err := fnAddDays(s.Value(), int(n.Value()))
		if err != nil {
			return nil, err
		}
		return object.NewString(r), nil
	})
	env["format_date"] = object.NewBuiltin("format_date", func(_ context.Context, args ...object.Object) (object.Object, error) {
		s, ok1 := args[0].(*object.String)
		l, ok2 := args[1].(*object.String)
		if len(args) != 2 || !ok1 || !ok2 {
			return nil, fmt.Errorf("format_date: want (string, string)")
		}
		r, err := fnFormatDate(s.Value(), l.Value())
		if err != nil {
			return nil, err
		}
		return object.NewString(r), nil
	})
}

func risorFloat(o object.Object) (float64, error) {
	switch x := o.(type) {
	case *object.Float:
		return x.Value(), nil
	case *object.Int:
		return float64(x.Value()), nil
	}
	return 0, fmt.Errorf("want a number, got %s", o.Type())
}
