package main

// Test 6: stored JSON -> YAML for display/editing. Key order, quoting, block scalars.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	goccy "github.com/goccy/go-yaml"
	yaml3 "go.yaml.in/yaml/v3"
	yaml4 "go.yaml.in/yaml/v4"
)

// v3/v4: parse the JSON as YAML (JSON is valid YAML) to a Node, which keeps key order; switch
// every node to block/plain style, multi-line strings to literal style, and encode the Node.
func displayV3(js []byte) ([]byte, error) {
	var n yaml3.Node
	if err := yaml3.Unmarshal(js, &n); err != nil {
		return nil, err
	}
	var fix func(*yaml3.Node)
	fix = func(n *yaml3.Node) {
		n.Style = 0
		if n.Kind == yaml3.ScalarNode && n.Tag == "!!str" && strings.Contains(n.Value, "\n") {
			n.Style = yaml3.LiteralStyle
		}
		for _, c := range n.Content {
			fix(c)
		}
	}
	fix(&n)
	var buf bytes.Buffer
	enc := yaml3.NewEncoder(&buf)
	enc.SetIndent(2)
	err := enc.Encode(&n)
	enc.Close()
	return buf.Bytes(), err
}

func displayV4(js []byte) ([]byte, error) {
	var n yaml4.Node
	if err := yaml4.Load(js, &n); err != nil {
		return nil, err
	}
	var fix func(*yaml4.Node)
	fix = func(n *yaml4.Node) {
		n.Style = 0
		if n.Kind == yaml4.ScalarNode && n.Tag == "!!str" && strings.Contains(n.Value, "\n") {
			n.Style = yaml4.LiteralStyle
		}
		for _, c := range n.Content {
			fix(c)
		}
	}
	fix(&n)
	return yaml4.Dump(&n, yaml4.WithV4Defaults())
}

// goccy: decode into an ordered MapSlice and marshal with literal style for multi-line strings.
func displayGoccy(js []byte) ([]byte, error) {
	var v any
	if err := goccy.UnmarshalWithOptions(js, &v, goccy.UseOrderedMap()); err != nil {
		return nil, err
	}
	return goccy.MarshalWithOptions(v, goccy.Indent(2), goccy.IndentSequence(true), goccy.UseLiteralStyleIfMultiline(true))
}

var displayers = []struct {
	name string
	f    func([]byte) ([]byte, error)
}{{"yaml/v3", displayV3}, {"yaml/v4", displayV4}, {"goccy", displayGoccy}}

// keyOrder lists every mapping key depth-first, read with yaml/v3 Node (keeps order).
func keyOrder(src []byte) []string {
	var n yaml3.Node
	if err := yaml3.Unmarshal(src, &n); err != nil {
		return []string{"parse error: " + err.Error()}
	}
	var out []string
	var walk func(*yaml3.Node)
	walk = func(n *yaml3.Node) {
		if n.Kind == yaml3.MappingNode {
			for i := 0; i+1 < len(n.Content); i += 2 {
				out = append(out, n.Content[i].Value)
				walk(n.Content[i+1])
			}
			return
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(&n)
	return out
}

func display() {
	h2("6. JSON → YAML for display")
	fmt.Println("Stored JSON is turned into YAML for the editor. v3/v4: JSON parsed to `yaml.Node`, styles reset, multi-line strings set to literal `|`.")
	fmt.Println("goccy: decoded with `UseOrderedMap`, marshalled with `UseLiteralStyleIfMultiline`. \"round trip\" = the YAML loads back (Burrow loader of")
	fmt.Println("the same library, and yaml/v3 as a second reader) to exactly the stored JSON value.")
	fmt.Println()
	head("library", "input", "key order kept", "round trip, same lib", "round trip, read by v3", "SQL as block scalar", "strings that changed")
	outputs := map[string]string{}
	for _, f := range []string{"valid.json", "display.json"} {
		js := mustRead("fixtures/" + f)
		var want any
		json.Unmarshal(js, &want)
		order := keyOrder(js)
		for li, d := range displayers {
			y, err := d.f(js)
			if err != nil {
				row(d.name, f, "error: "+md(err.Error()))
				continue
			}
			if f == "display.json" {
				outputs[d.name] = string(y)
			}
			rt := func(l loader) (bool, []string) {
				doc, err := l.load(y)
				if err != nil {
					return false, []string{err.Error()}
				}
				got := jsonAny(doc.Value)
				if reflect.DeepEqual(got, want) {
					return true, nil
				}
				var diff []string
				if gm, ok := got.(map[string]any); ok {
					wm := want.(map[string]any)
					for k, wv := range wm {
						if !reflect.DeepEqual(gm[k], wv) {
							diff = append(diff, fmt.Sprintf("%s: %v -> %v", k, show(wv), show(gm[k])))
						}
					}
				}
				sort.Strings(diff)
				return false, diff
			}
			same, diff := rt(loaders[li])
			byV3, diff3 := rt(loaders[0])
			changed := "none"
			if len(diff)+len(diff3) > 0 {
				changed = md(strings.Join(append(diff, diff3...), "; "))
			}
			row(d.name, f, yesNo(reflect.DeepEqual(keyOrder(y), order)), yesNo(same), yesNo(byV3),
				yesNo(strings.Contains(string(y), "sql: |")), changed)
		}
	}
	for _, d := range displayers {
		fmt.Printf("\n`display.json` rendered by %s:\n\n```yaml\n%s```\n", d.name, outputs[d.name])
	}
}
