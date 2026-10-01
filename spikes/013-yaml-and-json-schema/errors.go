package main

// Tests 1 and 2: the valid sample, and the broken files with their expected errors.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var schemaBytes = mustRead("fixtures/pipeline.schema.json")

func allValidators() []validator {
	return []validator{santhoshValidator(schemaBytes), kaptinlinValidator(schemaBytes), googleValidator(schemaBytes)}
}

func validSample() {
	h2("1. Valid sample pipeline")
	fmt.Println("`fixtures/valid.yaml` (SPEC 9.2 plus inputs, retry, a block-scalar SQL and app.notify) and `fixtures/valid.json`, the same config written by hand as JSON.")
	fmt.Println()
	vals := allValidators()
	hdr := []string{"loader", "file", "loads"}
	for _, v := range vals {
		hdr = append(hdr, v.name)
	}
	hdr = append(hdr, "same value as encoding/json of valid.json")
	head(hdr...)
	var ref any
	json.Unmarshal(mustRead("fixtures/valid.json"), &ref)
	for _, l := range loaders {
		for _, f := range []string{"valid.yaml", "valid.json"} {
			d, err := l.load(mustRead("fixtures/" + f))
			if err != nil {
				row(l.name, f, "error: "+md(err.Error()))
				continue
			}
			cells := []any{l.name, f, "yes"}
			for _, v := range vals {
				errs := v.validate(d.Value)
				if len(errs) == 0 {
					cells = append(cells, "valid")
				} else {
					cells = append(cells, fmt.Sprintf("%d errors: %s", len(errs), md(errs[0].Msg)))
				}
			}
			cells = append(cells, yesNo(reflect.DeepEqual(jsonAny(d.Value), ref)))
			row(cells...)
		}
	}
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "**no**"
}

type expect struct {
	line    int
	ptr, kw string
}

var expectRe = regexp.MustCompile(`(?m)^# expect: (\d+) (\S+)(?: (\S+))?`)

func readExpect(path string) []expect {
	src := mustRead(path)
	if b, err := os.ReadFile(path + ".expect"); err == nil {
		src = b
	}
	var out []expect
	for _, m := range expectRe.FindAllStringSubmatch(string(src), -1) {
		l, _ := strconv.Atoi(m[1])
		e := expect{line: l, ptr: m[2], kw: m[3]}
		if e.ptr == "#" {
			e.ptr = ""
		}
		if e.ptr == "parse" {
			e.ptr, e.kw = "", "parse"
		}
		out = append(out, e)
	}
	return out
}

// matches: same keyword, and the same pointer (kaptinlin reports unknown fields and bad
// property names on the parent object, so the parent also counts there).
func matches(e expect, v VErr) bool {
	if e.kw != v.Keyword {
		return false
	}
	if e.ptr == v.Ptr {
		return true
	}
	if e.kw == "additionalProperties" || e.kw == "propertyNames" {
		return v.Ptr == e.ptr[:strings.LastIndex(e.ptr, "/")]
	}
	return false
}

func brokenFiles() {
	h2("2. Broken files: all errors in one pass, line mapping, messages")
	fmt.Println("Each file in `fixtures/broken/` lists its expected errors as `# expect: LINE POINTER KEYWORD` (JSON files use a `.expect` side file).")
	fmt.Println("\"found\" = expected errors reported in one Validate call; \"extra\" = other errors reported. Lines are taken from the santhosh")
	fmt.Println("errors mapped through each loader's pointer map (key position for unknown/missing fields and whole objects, value position otherwise).")
	fmt.Println()
	files, _ := filepath.Glob("fixtures/broken/*")
	var fixtures []string
	for _, f := range files {
		if !strings.HasSuffix(f, ".expect") {
			fixtures = append(fixtures, f)
		}
	}
	sort.Strings(fixtures)
	vals := allValidators()
	head("file", "expected", "santhosh found / extra", "kaptinlin found / extra", "google errors returned", "lines right v3 / v4 / goccy", "v3, v4, goccy same line:col")

	type msgs struct {
		file  string
		lines []string
	}
	var santhoshMsgs []msgs
	otherMsgs := map[string]map[string][]string{} // file -> validator -> messages
	totals := map[string][2]int{}
	for _, f := range fixtures {
		exp := readExpect(f)
		src := mustRead(f)
		name := filepath.Base(f)
		docs := make([]*Doc, len(loaders))
		var loadErrs []error
		for i, l := range loaders {
			docs[i], _ = nil, error(nil)
			d, err := l.load(src)
			docs[i] = d
			loadErrs = append(loadErrs, err)
		}
		if len(exp) == 1 && exp[0].kw == "parse" {
			var lines []string
			var parts []string
			for i, l := range loaders {
				err := loadErrs[i]
				if err == nil {
					parts = append(parts, "**no error**")
					continue
				}
				le, _ := err.(*LoadError)
				ok := le != nil && le.Pos.Line == exp[0].line
				parts = append(parts, fmt.Sprintf("%s (line %d)", yesNo(ok), le.Pos.Line))
				lines = append(lines, fmt.Sprintf("%s: %s", l.name, err))
			}
			row(name, fmt.Sprintf("parse error at line %d", exp[0].line), "-", "-", "-", strings.Join(parts, " / "), "-")
			santhoshMsgs = append(santhoshMsgs, msgs{name, lines})
			continue
		}
		if loadErrs[0] != nil {
			row(name, len(exp), "load error: "+md(loadErrs[0].Error()))
			continue
		}
		cells := []any{name, len(exp)}
		var sErrs []VErr
		for vi, v := range vals {
			errs := v.validate(docs[0].Value)
			if vi == 0 {
				sErrs = errs
			}
			if v.name == "google v0.4.3" {
				cells = append(cells, len(errs))
			} else {
				found, extra := 0, 0
				for _, e := range exp {
					for _, ve := range errs {
						if matches(e, ve) {
							found++
							break
						}
					}
				}
				for _, ve := range errs {
					hit := false
					for _, e := range exp {
						if matches(e, ve) {
							hit = true
						}
					}
					if !hit {
						extra++
					}
				}
				t := totals[v.name]
				t[0] += found
				t[1] += extra
				totals[v.name] = t
				cells = append(cells, fmt.Sprintf("%d / %d", found, extra))
			}
			if vi > 0 {
				var ls []string
				for _, e := range errs {
					ls = append(ls, fmt.Sprintf("%s %s: %s", e.Ptr, e.Keyword, e.Msg))
				}
				if otherMsgs[name] == nil {
					otherMsgs[name] = map[string][]string{}
				}
				otherMsgs[name][v.name] = ls
			}
		}
		// line check per loader, using santhosh errors
		var parts []string
		same := true
		for li := range loaders {
			d := docs[li]
			if d == nil {
				parts = append(parts, "load error")
				continue
			}
			right := 0
			for _, e := range exp {
				for _, ve := range sErrs {
					if matches(e, ve) {
						if d.posFor(ve.Ptr, ve.Keyword).Line == e.line {
							right++
						}
						break
					}
				}
			}
			parts = append(parts, fmt.Sprintf("%d/%d", right, len(exp)))
		}
		var diffs []string
		for _, ve := range sErrs {
			p0 := docs[0].posFor(ve.Ptr, ve.Keyword)
			for li := 1; li < len(loaders); li++ {
				if docs[li] == nil {
					continue
				}
				if p := docs[li].posFor(ve.Ptr, ve.Keyword); p != p0 {
					same = false
					diffs = append(diffs, fmt.Sprintf("%s: v3 %s, %s %s", specPath(ve.Ptr), p0, loaders[li].name, p))
				}
			}
		}
		sameCell := "yes"
		if !same {
			sameCell = "no: " + strings.Join(diffs, "; ")
		}
		cells = append(cells, strings.Join(parts, " / "), sameCell)
		row(cells...)
		var lines []string
		for _, e := range sErrs {
			lines = append(lines, llmLine(docs[0], e))
		}
		santhoshMsgs = append(santhoshMsgs, msgs{name, lines})
	}
	fmt.Println()
	for _, v := range vals[:2] {
		fmt.Printf("Totals for %s: %d expected errors found, %d extra.\n\n", v.name, totals[v.name][0], totals[v.name][1])
	}
	// google returns one error; is it always the same one?
	d18, _ := loadV3(mustRead("fixtures/broken/b18_many_errors.yaml"))
	distinct := map[string]bool{}
	for i := 0; i < 50; i++ {
		distinct[vals[2].validate(d18.Value)[0].Msg] = true
	}
	fmt.Printf("google v0.4.3 on b18_many_errors.yaml, 50 runs: %d different first errors returned.\n\n", len(distinct))
	fmt.Println("b15: santhosh v6.0.3 puts the `propertyNames` error at the wrong path. Its validator stores its internal location slice")
	fmt.Println("without copying it (`verr.InstanceLocation = vd.vloc` in validator.go), so a later property overwrites it. Other keywords copy it.")
	fmt.Println()
	fmt.Println("### Messages Burrow would return (santhosh v6 errors, yaml/v3 positions, SPEC path)")
	fmt.Println()
	fmt.Println("```")
	for _, m := range santhoshMsgs {
		fmt.Printf("%s\n", m.file)
		for _, l := range m.lines {
			fmt.Printf("  %s\n", l)
		}
	}
	fmt.Println("```")
	fmt.Println()
	fmt.Println("### Raw messages from the other validators (selected files)")
	fmt.Println()
	fmt.Println("```")
	for _, f := range []string{"b07_typo_in_with.yaml", "b14_nested_step_errors.yaml", "b18_many_errors.yaml"} {
		for _, v := range vals[1:] {
			fmt.Printf("%s, %s\n", f, v.name)
			for _, l := range otherMsgs[f][v.name] {
				fmt.Printf("  %s\n", strings.ReplaceAll(l, "\n", "\n  "))
			}
		}
	}
	fmt.Println("```")
}
