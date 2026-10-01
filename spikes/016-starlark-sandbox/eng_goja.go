package main

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/dop251/goja"
)

type gojaEngine struct{}

func (gojaEngine) Name() string { return "goja" }

// The script's last expression is the result.
func (gojaEngine) Run(ctx context.Context, src string, o RunOpts) (any, error) {
	vm := goja.New()
	if !o.Default {
		vm.SetMaxCallStackSize(stackLimit)
		// Sandbox: remove time and randomness.
		vm.GlobalObject().Delete("Date")
		vm.Get("Math").ToObject(vm).Delete("random")
	}
	stop := context.AfterFunc(ctx, func() { vm.Interrupt(causeText(ctx)) })
	defer stop()
	if o.DB != nil {
		gojaHost(ctx, vm, o.DB)
	}
	v, err := vm.RunString(src)
	if err != nil {
		var so *goja.StackOverflowError
		if errors.As(err, &so) {
			return nil, fmt.Errorf("stack overflow: %w", err)
		}
		return nil, err
	}
	return fromGoja(v), nil
}

func gojaHost(ctx context.Context, vm *goja.Runtime, d *DB) {
	db := vm.NewObject()
	db.Set("query", func(q string, params []any) ([]any, error) {
		rows, err := d.Query(ctx, q, params)
		if err != nil {
			return nil, err
		}
		out := make([]any, len(rows.Data))
		for i, r := range rows.Data {
			obj := vm.NewObject()
			for j, c := range rows.Cols {
				obj.Set(c, r[j])
			}
			out[i] = obj
		}
		return out, nil
	})
	vm.Set("db", db)
	vm.Set("round", fnRound)
	vm.Set("add_days", fnAddDays)
	vm.Set("format_date", fnFormatDate)
}

func fromGoja(v goja.Value) any {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	switch x := v.Export().(type) {
	case *big.Int:
		return fmt.Sprintf("big int (%d bits)", x.BitLen())
	default:
		return normalize(x)
	}
}

// normalize turns exported values into plain JSON-like Go values.
func normalize(v any) any {
	switch x := v.(type) {
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = normalize(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = normalize(e)
		}
		return out
	case string:
		if len(x) > 64 {
			return fmt.Sprintf("string (%d bytes)", len(x))
		}
		return x
	case int:
		return int64(x)
	case float64:
		if x == float64(int64(x)) && x < 1e15 && x > -1e15 {
			return int64(x)
		}
		return x
	}
	return v
}
