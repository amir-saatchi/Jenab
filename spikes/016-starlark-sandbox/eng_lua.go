package main

import (
	"context"
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

type luaEngine struct{}

func (luaEngine) Name() string { return "gopher-lua" }

// The script returns its result with `return`.
func (luaEngine) Run(ctx context.Context, src string, o RunOpts) (any, error) {
	var L *lua.LState
	if o.Default {
		L = lua.NewState()
	} else {
		L = lua.NewState(lua.Options{SkipOpenLibs: true, CallStackSize: stackLimit, RegistrySize: 1024 * 20, RegistryMaxSize: 1024 * 256, MinimizeStackMemory: true})
		for _, lib := range []struct {
			name string
			fn   lua.LGFunction
		}{{lua.BaseLibName, lua.OpenBase}, {lua.TabLibName, lua.OpenTable}, {lua.StringLibName, lua.OpenString}, {lua.MathLibName, lua.OpenMath}} {
			L.Push(L.NewFunction(lib.fn))
			L.Push(lua.LString(lib.name))
			L.Call(1, 0)
		}
		// Sandbox: the base library can read files and load code; math has randomness.
		for _, name := range []string{"dofile", "loadfile", "load", "loadstring", "require", "module", "collectgarbage", "print", "_printregs", "newproxy"} {
			L.SetGlobal(name, lua.LNil)
		}
		m := L.GetGlobal("math").(*lua.LTable)
		m.RawSetString("random", lua.LNil)
		m.RawSetString("randomseed", lua.LNil)
	}
	defer L.Close()
	L.SetContext(ctx)
	if o.DB != nil {
		luaHost(ctx, L, o.DB)
	}
	top := L.GetTop()
	if err := L.DoString(src); err != nil {
		return nil, err
	}
	if L.GetTop() == top {
		return nil, nil
	}
	return fromLua(L.Get(-1), 0)
}

func luaHost(ctx context.Context, L *lua.LState, d *DB) {
	db := L.NewTable()
	L.SetField(db, "query", L.NewFunction(func(L *lua.LState) int {
		q := L.CheckString(1)
		var ps []any
		if t, ok := L.Get(2).(*lua.LTable); ok {
			t.ForEach(func(_, v lua.LValue) {
				x, _ := fromLua(v, 0)
				ps = append(ps, x)
			})
		}
		rows, err := d.Query(ctx, q, ps)
		if err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}
		out := L.CreateTable(len(rows.Data), 0)
		for _, r := range rows.Data {
			row := L.CreateTable(0, len(rows.Cols))
			for j, c := range rows.Cols {
				row.RawSetString(c, toLua(r[j]))
			}
			out.Append(row)
		}
		L.Push(out)
		return 1
	}))
	L.SetGlobal("db", db)
	L.SetGlobal("round", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(fnRound(float64(L.CheckNumber(1)), L.OptInt(2, 0))))
		return 1
	}))
	L.SetGlobal("add_days", L.NewFunction(func(L *lua.LState) int {
		s, err := fnAddDays(L.CheckString(1), L.CheckInt(2))
		if err != nil {
			L.RaiseError("%s", err.Error())
		}
		L.Push(lua.LString(s))
		return 1
	}))
	L.SetGlobal("format_date", L.NewFunction(func(L *lua.LState) int {
		s, err := fnFormatDate(L.CheckString(1), L.CheckString(2))
		if err != nil {
			L.RaiseError("%s", err.Error())
		}
		L.Push(lua.LString(s))
		return 1
	}))
}

func toLua(v any) lua.LValue {
	switch x := v.(type) {
	case nil:
		return lua.LNil
	case int64:
		return lua.LNumber(x)
	case float64:
		return lua.LNumber(x)
	case string:
		return lua.LString(x)
	case bool:
		return lua.LBool(x)
	}
	return lua.LString(fmt.Sprint(v))
}

func fromLua(v lua.LValue, depth int) (any, error) {
	if depth > 50 {
		return nil, fmt.Errorf("result nested too deep")
	}
	switch x := v.(type) {
	case *lua.LNilType:
		return nil, nil
	case lua.LBool:
		return bool(x), nil
	case lua.LNumber:
		f := float64(x)
		if f == float64(int64(f)) && f < 1e15 && f > -1e15 {
			return int64(f), nil
		}
		return f, nil
	case lua.LString:
		if len(x) > 64 {
			return fmt.Sprintf("string (%d bytes)", len(x)), nil
		}
		return string(x), nil
	case *lua.LTable:
		if n := x.Len(); n > 0 {
			out := make([]any, 0, n)
			for i := 1; i <= n; i++ {
				e, err := fromLua(x.RawGetInt(i), depth+1)
				if err != nil {
					return nil, err
				}
				out = append(out, e)
			}
			return out, nil
		}
		out := map[string]any{}
		var err error
		x.ForEach(func(k, e lua.LValue) {
			if err != nil {
				return
			}
			var ge any
			ge, err = fromLua(e, depth+1)
			out[k.String()] = ge
		})
		return out, err
	}
	return v.String(), nil
}
