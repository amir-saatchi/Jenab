// SPIKE-013: which YAML library and which JSON Schema validator should Burrow use?
//
// Usage: go run . > results.md
package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

func main() {
	if c := os.Getenv("SPIKE013_CHILD"); c != "" {
		child(c) // isolated run for the bomb and depth tests (safety.go)
		return
	}
	fmt.Printf("# SPIKE-013 results\n\nGo %s, %s/%s, %s.\n\n", runtime.Version(), runtime.GOOS, runtime.GOARCH, time.Now().Format("2006-01-02"))
	fmt.Println("Libraries: go.yaml.in/yaml/v3 v3.0.5, go.yaml.in/yaml/v4 v4.0.0-rc.6, github.com/goccy/go-yaml v1.19.2;")
	fmt.Println("github.com/santhosh-tekuri/jsonschema/v6 v6.0.3, github.com/kaptinlin/jsonschema v0.7.7, github.com/google/jsonschema-go v0.4.3.")
	fmt.Println()
	fmt.Println("\"Burrow loader\" = parse to the library's syntax tree, then one Go walk that builds the value, records a line:column per JSON pointer,")
	fmt.Println("and rejects anchors, aliases, merge keys, tags, non-string keys, duplicate keys and nesting over 64 levels (load.go).")
	fmt.Println()
	validSample()
	brokenFiles()
	yaml11()
	safety()
	speed()
	display()
	formats()
}

func h2(s string)      { fmt.Printf("\n## %s\n\n", s) }
func row(cells ...any) { fmt.Println("| " + strings.Join(strs(cells), " | ") + " |") }
func head(cells ...string) {
	fmt.Println("| " + strings.Join(cells, " | ") + " |")
	fmt.Println("|" + strings.Repeat("---|", len(cells)))
}

func strs(cells []any) []string {
	out := make([]string, len(cells))
	for i, c := range cells {
		out[i] = strings.ReplaceAll(fmt.Sprint(c), "|", `\|`)
	}
	return out
}

func md(s string) string {
	s = strings.ReplaceAll(s, "\n", "↵")
	if len(s) > 160 {
		s = s[:157] + "..."
	}
	return "`" + strings.ReplaceAll(s, "`", "'") + "`"
}
