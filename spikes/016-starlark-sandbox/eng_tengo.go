package main

import (
	"context"
	"fmt"
	"math"

	"github.com/d5/tengo/v2"
)

type tengoEngine struct{}

func (tengoEngine) Name() string { return "tengo" }

// The script sets a global `result`. No imports are set, so import() fails.
func (tengoEngine) Run(ctx context.Context, src string, o RunOpts) (any, error) {
	if o.Default {
		tengo.MaxStringLen, tengo.MaxBytesLen = math.MaxInt32, math.MaxInt32
	} else {
		tengo.MaxStringLen, tengo.MaxBytesLen = strLimit, strLimit
	}
	s := tengo.NewScript([]byte(src))
	if !o.Default && !o.NoSteps {
		s.SetMaxAllocs(allocLimit)
	}
	if o.DB != nil {
		tengoHost(ctx, s, o.DB)
	}
	c, err := s.RunContext(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w (%s)", err, causeText(ctx))
		}
		return nil, err
	}
	if !c.IsDefined("result") {
		return nil, nil
	}
	return tengoValue(c.Get("result").Object()), nil
}

func tengoValue(o tengo.Object) any {
	if s, ok := o.(*tengo.String); ok && len(s.Value) > 64 {
		return fmt.Sprintf("string (%d bytes)", len(s.Value))
	}
	if b, ok := o.(*tengo.Bytes); ok {
		return fmt.Sprintf("bytes (%d)", len(b.Value))
	}
	return normalize(tengo.ToInterface(o))
}

func tengoHost(ctx context.Context, s *tengo.Script, d *DB) {
	fn := func(name string, f tengo.CallableFunc) *tengo.UserFunction {
		return &tengo.UserFunction{Name: name, Value: f}
	}
	query := fn("query", func(args ...tengo.Object) (tengo.Object, error) {
		if len(args) < 1 {
			return nil, tengo.ErrWrongNumArguments
		}
		q, ok := tengo.ToString(args[0])
		if !ok {
			return nil, tengo.ErrInvalidArgumentType{Name: "sql", Expected: "string", Found: args[0].TypeName()}
		}
		var ps []any
		if len(args) > 1 {
			if a, ok := args[1].(*tengo.Array); ok {
				for _, e := range a.Value {
					ps = append(ps, tengo.ToInterface(e))
				}
			}
		}
		rows, err := d.Query(ctx, q, ps)
		if err != nil {
			return &tengo.Error{Value: &tengo.String{Value: err.Error()}}, nil
		}
		out := make([]tengo.Object, len(rows.Data))
		for i, r := range rows.Data {
			m := make(map[string]tengo.Object, len(rows.Cols))
			for j, c := range rows.Cols {
				v, _ := tengo.FromInterface(r[j])
				m[c] = v
			}
			out[i] = &tengo.Map{Value: m}
		}
		return &tengo.Array{Value: out}, nil
	})
	s.Add("db", &tengo.ImmutableMap{Value: map[string]tengo.Object{"query": query}})
	s.Add("round", fn("round", func(args ...tengo.Object) (tengo.Object, error) {
		if len(args) != 2 {
			return nil, tengo.ErrWrongNumArguments
		}
		x, _ := tengo.ToFloat64(args[0])
		n, _ := tengo.ToInt(args[1])
		return &tengo.Float{Value: fnRound(x, n)}, nil
	}))
	s.Add("add_days", fn("add_days", func(args ...tengo.Object) (tengo.Object, error) {
		if len(args) != 2 {
			return nil, tengo.ErrWrongNumArguments
		}
		d, _ := tengo.ToString(args[0])
		n, _ := tengo.ToInt(args[1])
		r, err := fnAddDays(d, n)
		if err != nil {
			return nil, err
		}
		return &tengo.String{Value: r}, nil
	}))
	s.Add("format_date", fn("format_date", func(args ...tengo.Object) (tengo.Object, error) {
		if len(args) != 2 {
			return nil, tengo.ErrWrongNumArguments
		}
		d, _ := tengo.ToString(args[0])
		l, _ := tengo.ToString(args[1])
		r, err := fnFormatDate(d, l)
		if err != nil {
			return nil, err
		}
		return &tengo.String{Value: r}, nil
	}))
}
