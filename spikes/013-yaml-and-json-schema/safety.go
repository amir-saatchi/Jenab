package main

// Test 4: duplicate keys, alias bombs, deep nesting, merge keys. Bombs and deep inputs run in a
// child process (this same binary) with a 10 s timeout and a 1 GB memory watchdog, so a crash
// or runaway allocation cannot take the whole run down.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime/metrics"
	"strconv"
	"strings"
	"time"

	sjs "github.com/santhosh-tekuri/jsonschema/v6"
)

const memCap = 1 << 30

func bomb(levels int) []byte {
	var b strings.Builder
	b.WriteString("a0: &a0 [" + strings.TrimSuffix(strings.Repeat(`"lol",`, 10), ",") + "]\n")
	for i := 1; i <= levels; i++ {
		refs := strings.TrimSuffix(strings.Repeat(fmt.Sprintf("*a%d,", i-1), 10), ",")
		fmt.Fprintf(&b, "a%d: &a%d [%s]\n", i, i, refs)
	}
	return []byte(b.String())
}

func deepFlow(n int) []byte {
	return []byte("v: " + strings.Repeat("[", n) + strings.Repeat("]", n) + "\n")
}
func deepSeq(n int) []byte { return []byte(strings.Repeat("- ", n) + "x\n") }

func input(kind string, n int) []byte {
	switch kind {
	case "bomb":
		return bomb(n)
	case "flow":
		return deepFlow(n)
	case "seq":
		return deepSeq(n)
	}
	panic(kind)
}

func memNow() uint64 {
	s := []metrics.Sample{{Name: "/memory/classes/total:bytes"}}
	metrics.Read(s)
	return s[0].Value.Uint64()
}

// child runs one case: SPIKE013_CHILD=kind|n|mode|lib and prints status|ms|peakMB|detail.
func child(spec string) {
	p := strings.Split(spec, "|")
	n, _ := strconv.Atoi(p[1])
	src := input(p[0], n)
	mode, lib := p[2], p[3]
	start := time.Now()
	var peak uint64
	go func() {
		for {
			m := memNow()
			if m > peak {
				peak = m
			}
			if m > memCap {
				fmt.Printf("killed|%d|%d|memory over 1 GB\n", time.Since(start).Milliseconds(), peak>>20)
				os.Exit(0)
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()
	var err error
	var detail string
	switch mode {
	case "native", "tree":
		list := natives
		if mode == "tree" {
			list = treeOnly
		}
		for _, x := range list {
			if x.name == lib {
				var v any
				v, err = x.decode(src)
				if err == nil && mode == "native" {
					b, _ := json.Marshal(v)
					detail = fmt.Sprintf("value built, %d bytes as JSON", len(b))
				}
			}
		}
	case "burrow":
		for _, l := range loaders {
			if l.name == lib {
				_, err = l.load(src)
			}
		}
	}
	if m := memNow(); m > peak {
		peak = m
	}
	ms := time.Since(start).Milliseconds()
	if err != nil {
		fmt.Printf("error|%d|%d|%s\n", ms, peak>>20, strings.SplitN(err.Error(), "\n", 2)[0])
		return
	}
	if detail == "" {
		detail = "accepted"
	}
	fmt.Printf("ok|%d|%d|%s\n", ms, peak>>20, detail)
}

func runChild(spec string) string {
	exe, _ := os.Executable()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe)
	cmd.Env = append(os.Environ(), "SPIKE013_CHILD="+spec)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if ctx.Err() != nil {
		return "**timeout after 10 s (killed)**"
	}
	line := strings.TrimSpace(out.String())
	if err != nil || line == "" {
		e := errb.String()
		if strings.Contains(e, "stack exceeds") || strings.Contains(e, "stack overflow") {
			return "**crash: Go stack overflow (fatal, cannot be recovered)**"
		}
		first := strings.SplitN(strings.TrimSpace(e), "\n", 2)[0]
		return "**crash: " + first + "**"
	}
	f := strings.SplitN(line, "|", 4)
	switch f[0] {
	case "killed":
		return fmt.Sprintf("**killed at %s ms, over 1 GB**", f[1])
	case "error":
		return fmt.Sprintf("stopped: %s (%s ms, %s MB)", md(f[3]), f[1], f[2])
	}
	return fmt.Sprintf("%s (%s ms, %s MB)", f[3], f[1], f[2])
}

func safety() {
	h2("4. Duplicate keys, alias bombs, deep nesting, merge keys")
	dupKeys()
	fmt.Println()
	fmt.Println("### Alias bomb (billion laughs) and deep nesting")
	fmt.Println()
	fmt.Println("Each cell is a separate child process (10 s timeout, killed above 1 GB of Go memory). MB = peak Go memory of the child.")
	fmt.Println("tree = parse to the syntax tree only; native = library decode into `any`; Burrow = load.go (rejects aliases, depth over 64).")
	fmt.Println()
	head("input", "library", "tree", "native", "Burrow loader")
	cases := []struct {
		kind  string
		n     int
		label string
	}{
		{"bomb", 3, "bomb, 3 levels (10^4 strings)"},
		{"bomb", 5, "bomb, 5 levels (10^6 strings)"},
		{"bomb", 8, "bomb, 8 levels (10^9 strings)"},
		{"flow", 1000, "`[` nested 1,000 deep"},
		{"flow", 10000, "`[` nested 10,000 deep"},
		{"flow", 1000000, "`[` nested 1,000,000 deep"},
		{"seq", 1000, "`- ` nested 1,000 deep"},
		{"seq", 100000, "`- ` nested 100,000 deep"},
	}
	for _, c := range cases {
		for _, l := range loaders {
			cells := []any{c.label + fmt.Sprintf(" (%d bytes)", len(input(c.kind, c.n))), l.name}
			for _, mode := range []string{"tree", "native", "burrow"} {
				cells = append(cells, runChild(fmt.Sprintf("%s|%d|%s|%s", c.kind, c.n, mode, l.name)))
			}
			row(cells...)
		}
	}
	fmt.Println()
	mergeKeys()
}

func dupKeys() {
	fmt.Println("### Duplicate keys")
	fmt.Println()
	head("input", "library", "tree only", "native decode", "Burrow loader")
	inputs := []string{"id: a\nid: b\n", "steps:\n  - id: a\n    use: x\n    use: y\n", "{\"id\": \"a\", \"id\": \"b\"}"}
	for _, in := range inputs {
		for i, l := range loaders {
			cells := []any{md(in), l.name}
			_, err := treeOnly[i].decode([]byte(in))
			cells = append(cells, errCell(err))
			_, err = natives[i].decode([]byte(in))
			cells = append(cells, errCell(err))
			_, err = l.load([]byte(in))
			cells = append(cells, errCell(err))
			row(cells...)
		}
	}
	js := `{"id": "a", "id": "b"}`
	var v any
	err := json.Unmarshal([]byte(js), &v)
	row(md(js), "encoding/json", "-", errCell(err)+" → "+md(show(v)), "-")
	sv, err := sjs.UnmarshalJSON(strings.NewReader(js))
	row(md(js), "santhosh UnmarshalJSON", "-", errCell(err)+" → "+md(show(sv)), "-")
}

func errCell(err error) string {
	if err == nil {
		return "**accepted**"
	}
	return "rejected: " + md(strings.SplitN(err.Error(), "\n", 2)[0])
}

func mergeKeys() {
	fmt.Println("### Merge keys `<<`")
	fmt.Println()
	src := "defaults: &d { timeout: 60s, continue_on_error: true }\nstep:\n  <<: *d\n  id: price\n  timeout: 10s\n"
	fmt.Println("Input: " + md(src))
	fmt.Println()
	head("library", "native `step`", "Burrow loader")
	for i, l := range loaders {
		v, err := natives[i].decode([]byte(src))
		nat := errCell(err)
		if err == nil {
			if m, ok := v.(map[string]any); ok {
				nat = md(show(m["step"]))
			} else {
				nat = md(show(v))
			}
		}
		_, err = l.load([]byte(src))
		row(l.name, nat, errCell(err))
	}
}
