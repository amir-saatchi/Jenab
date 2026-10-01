package main

// Test 5: parse + validate speed for a typical 5 KB pipeline and a 1 MB one.

import (
	"encoding/json"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"
)

// bigPipeline repeats the steps of valid.yaml (with new ids) until the file reaches size bytes.
func bigPipeline(size int) ([]byte, int) {
	src := string(mustRead("fixtures/valid.yaml"))
	i := strings.Index(src, "steps:\n")
	headPart, steps := src[:i+len("steps:\n")], src[i+len("steps:\n"):]
	var b strings.Builder
	b.WriteString(headPart)
	n := 0
	for r := 0; b.Len() < size; r++ {
		b.WriteString(strings.ReplaceAll(steps, "- id: ", fmt.Sprintf("- id: r%d_", r)))
		n += strings.Count(steps, "- id: ")
	}
	return []byte(b.String()), n
}

func p50(runs int, f func()) time.Duration {
	f() // warm up
	d := make([]time.Duration, runs)
	for i := range d {
		s := clock()
		f()
		d[i] = since(s)
	}
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	return d[runs/2]
}

func dur(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%.0f µs", float64(d)/1e3)
	case d < time.Second:
		return fmt.Sprintf("%.1f ms", float64(d)/1e6)
	}
	return fmt.Sprintf("%.2f s", d.Seconds())
}

func speed() {
	h2("5. Speed (p50)")
	small, smallSteps := bigPipeline(5 * 1024)
	large, largeSteps := bigPipeline(1 << 20)
	sizes := []struct {
		src   []byte
		steps int
		runs  int
	}{{small, smallSteps, 300}, {large, largeSteps, 15}}
	fmt.Printf("Inputs: valid.yaml with its steps repeated. Small: %d bytes, %d steps, %d runs. Large: %d bytes, %d steps, %d runs.\n",
		len(small), smallSteps, sizes[0].runs, len(large), largeSteps, sizes[1].runs)
	fmt.Println("All inputs are valid, so no error path is timed. Timer: Windows performance counter (clock.go).")
	fmt.Println()
	type op struct {
		name string
		f    func(src []byte) func()
	}
	sch := newSanthosh(schemaBytes, true)
	ksch := newKaptinlin(schemaBytes, true)
	gsch := newGoogle(schemaBytes)
	var ops []op
	for i := range loaders {
		n, l := natives[i], loaders[i]
		ops = append(ops,
			op{n.name + " native decode into any", func(src []byte) func() { return func() { n.decode(src) } }},
			op{n.name + " Burrow loader (tree + walk + positions)", func(src []byte) func() { return func() { l.load(src) } }},
		)
	}
	ops = append(ops,
		op{"santhosh v6 validate", func(src []byte) func() {
			d, _ := loadV3(src)
			return func() {
				if err := sch.Validate(d.Value); err != nil {
					panic(err)
				}
			}
		}},
		op{"kaptinlin v0.7.7 validate", func(src []byte) func() {
			d, _ := loadV3(src)
			v := jsonAny(d.Value)
			return func() {
				if !ksch.Validate(v).IsValid() {
					panic("invalid")
				}
			}
		}},
		op{"google v0.4.3 validate", func(src []byte) func() {
			d, _ := loadV3(src)
			v := jsonAny(d.Value)
			return func() {
				if err := gsch.Validate(v); err != nil {
					panic(err)
				}
			}
		}},
		op{"**yaml/v3 Burrow loader + santhosh validate**", func(src []byte) func() {
			return func() {
				d, err := loadV3(src)
				if err != nil {
					panic(err)
				}
				if err := sch.Validate(d.Value); err != nil {
					panic(err)
				}
			}
		}},
		op{"same config as JSON: encoding/json decode + santhosh validate", func(src []byte) func() {
			d, _ := loadV3(src)
			js, _ := json.Marshal(d.Value)
			return func() {
				var v any
				json.Unmarshal(js, &v)
				if err := sch.Validate(v); err != nil {
					panic(err)
				}
			}
		}},
		op{"same config as JSON: yaml/v3 Burrow loader + santhosh validate", func(src []byte) func() {
			d, _ := loadV3(src)
			js, _ := json.Marshal(d.Value)
			return func() {
				d, err := loadV3(js)
				if err != nil {
					panic(err)
				}
				sch.Validate(d.Value)
			}
		}},
	)
	head("operation", fmt.Sprintf("%d KB", len(small)/1024), fmt.Sprintf("%d KB", len(large)/1024))
	for _, o := range ops {
		cells := []any{o.name}
		for _, s := range sizes {
			runtime.GC()
			cells = append(cells, dur(p50(s.runs, o.f(s.src))))
		}
		row(cells...)
	}
}
