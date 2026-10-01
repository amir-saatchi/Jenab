package main

// refCheck is copied unchanged from SPIKE-021 main.go.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// refCheck validates the SPEC 9 example through the tools and runs the assertions on the
// reference states; all must pass before any model run.
func refCheck() bool {
	ok := true
	say := func(format string, a ...any) { fmt.Fprintf(out, format+"\n", a...) }
	tmp, _ := os.MkdirTemp("", "spike021-ref-")
	defer os.RemoveAll(tmp)
	report := func(name string, cs []Check) {
		for _, c := range cs {
			if !c.OK {
				ok = false
				say("  FAIL %s: %s", c.Name, c.Why)
			}
		}
		say("%s: %d checks", name, len(cs))
	}
	// T1
	p, err := newProject(filepath.Join(tmp, "t1"))
	if err != nil {
		say("project: %v", err)
		return false
	}
	res, wc := p.callTool("apply_migration", refT1)
	say("T1 migration (SPEC 9.1):\n%s", res)
	if wc == nil || !wc.OK {
		ok = false
	}
	report("T1 assertions on the reference", checkT1(p))
	p.Close()
	for i := 2; i <= 4; i++ {
		p, err := buildState(filepath.Join(tmp, fmt.Sprintf("s%d", i)), i)
		if err != nil {
			say("state before T%d: %v", i, err)
			return false
		}
		say("state before T%d built (reference configs of the earlier tasks validated)", i)
		switch i {
		case 2:
			r, _ := p.callTool("save_pipeline", yamlJSON(refPipeline))
			say("T2 pipeline (SPEC 9.2):\n%s", r)
			report("T2 assertions on the reference", checkT2(p))
		case 3:
			for _, v := range refViewsT3 {
				tool := "save_view"
				if strings.Contains(v, "type: form") {
					tool = "save_form"
				}
				r, _ := p.callTool(tool, yamlJSON(v))
				say("T3 %s:\n%s", tool, r)
			}
			report("T3 assertions on the reference", checkT3(p))
		case 4:
			r, wc := p.callTool("apply_migration", refT4Args())
			say("T4 migration + dependents (SPEC 9.5):\n%s", r)
			if wc == nil || !wc.OK {
				ok = false
			}
			report("T4 assertions on the reference", checkT4(p))
		}
		p.Close()
	}
	say("reference check: %v", map[bool]string{true: "all passed", false: "FAILED"}[ok])
	return ok
}
