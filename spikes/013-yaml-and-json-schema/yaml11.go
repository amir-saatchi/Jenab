package main

// Test 3: YAML 1.1 surprises. What each library makes of plain scalars that YAML 1.1 treats
// specially, and what the Burrow loader passes on to the validator.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

func show(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return fmt.Sprintf("string %q", x)
	case time.Time:
		return "time.Time " + x.Format(time.RFC3339)
	case map[string]any:
		var ks []string
		for k, e := range x {
			ks = append(ks, fmt.Sprintf("%q: %s", k, show(e)))
		}
		sort.Strings(ks)
		return "map[string]any{" + strings.Join(ks, ", ") + "}"
	case map[any]any:
		var ks []string
		for k, e := range x {
			ks = append(ks, fmt.Sprintf("%T %v: %s", k, k, show(e)))
		}
		sort.Strings(ks)
		return "map[any]any{" + strings.Join(ks, ", ") + "}"
	}
	return fmt.Sprintf("%T %v", v, v)
}

func yaml11() {
	h2("3. YAML 1.1 surprises")
	fmt.Println("Each input is decoded as `v: <input>` (key rows: the whole document). Native = the library's own `Unmarshal`/`Load` into `any`.")
	fmt.Println("Burrow loader = load.go (the value the validator sees). \"json.Marshal\" = can the native value be stored as JSON as-is.")
	fmt.Println()
	values := []string{"no", "on", "yes", "off", "y", "True", "0755", "010", "0o755", "0x1F", "1_000", "+1", "1e3", ".inf",
		"2026-09-28", "2026-09-28T08:00:00Z", "~", "12:30", "\"0755\""}
	keys := []string{"on: 1", "no: 1", "1: x", "2026-09-28: x", "~: x"}
	hdr := []string{"input"}
	for _, n := range natives {
		hdr = append(hdr, n.name+" native")
	}
	hdr = append(hdr, "Burrow loader (v3 / v4 / goccy)")
	head(hdr...)
	run := func(label string, doc string, pick func(any) any) {
		cells := []any{md(label)}
		for _, n := range natives {
			v, err := n.decode([]byte(doc))
			if err != nil {
				cells = append(cells, "error: "+md(err.Error()))
				continue
			}
			x := pick(v)
			s := show(x)
			if _, err := json.Marshal(x); err != nil {
				s += " (json.Marshal fails)"
			}
			cells = append(cells, md(s))
		}
		var bl []string
		for _, l := range loaders {
			d, err := l.load([]byte(doc))
			if err != nil {
				bl = append(bl, "error: "+err.Error())
				continue
			}
			bl = append(bl, show(pick(d.Value)))
		}
		if bl[0] == bl[1] && bl[1] == bl[2] {
			cells = append(cells, md(bl[0])+" (all three)")
		} else {
			cells = append(cells, md(strings.Join(bl, " / ")))
		}
		row(cells...)
	}
	for _, v := range values {
		run(v, "v: "+v+"\n", func(x any) any {
			switch m := x.(type) {
			case map[string]any:
				return m["v"]
			case map[any]any:
				return m["v"]
			}
			return x
		})
	}
	for _, k := range keys {
		run(k, k+"\n", func(x any) any { return x })
	}

	fmt.Println()
	fmt.Println("### JSON read by the YAML loaders")
	fmt.Println()
	fmt.Println("Configs may arrive as JSON. Same value as `encoding/json` (after both go through `encoding/json` once)?")
	fmt.Println()
	jsons := []string{`{"v": 1e3}`, `{"v": 1E+2}`, `{"v": 1.0}`, `{"v": -0.5}`, `{"v": 12345678901234567890}`,
		`{"v": "é😀"}`, `{"v": "a\tb\/c"}`, `{"v": null}`, `{"v":"x","w":[1,{"a":true}]}`}
	head("JSON", "encoding/json", "yaml/v3", "yaml/v4", "goccy")
	for _, js := range jsons {
		var want any
		json.Unmarshal([]byte(js), &want)
		cells := []any{md(js), md(show(want.(map[string]any)["v"]))}
		for _, l := range loaders {
			d, err := l.load([]byte(js))
			if err != nil {
				cells = append(cells, "error: "+md(err.Error()))
				continue
			}
			got := jsonAny(d.Value)
			if fmt.Sprint(got) == fmt.Sprint(want) {
				cells = append(cells, "same")
			} else {
				cells = append(cells, "**differs**: "+md(show(got.(map[string]any)["v"])))
			}
		}
		row(cells...)
	}
}
