// Command 032-made-up-numbers reads SPIKE-032's scenario reports (their
// JSON) and the hand labels, and prints, for each model and K, made-up
// answers caught and honest answers flagged. Mark runs have each answer's
// unsourced count, so any K is computed afterwards.
//
//	go run ./spikes/032-made-up-numbers -dump <report>.json...
//	go run . -labels labels.csv <report>.json...
package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

type ground struct {
	Turn      int      `json:"turn"`
	Request   int      `json:"request"`
	Checked   int      `json:"checked"`
	Unsourced []string `json:"unsourced"`
	Saved     int      `json:"saved"`
	SavedLeft int      `json:"saved_left"`
	Quoted    int      `json:"quoted"`
	Pct       int      `json:"pct"`
	Fired     bool     `json:"fired"`
	Told      bool     `json:"told"`
	After     bool     `json:"after"`
	Micros    int64    `json:"micros"`
}

type result struct {
	Type   string `json:"type"`
	Test   string `json:"test"`
	Status string `json:"status"`
}

type extra struct {
	Requests int     `json:"requests"`
	Prompt   int     `json:"prompt_tokens"`
	Output   int     `json:"output_tokens"`
	Seconds  float64 `json:"seconds"`
}

type run struct {
	Model     string   `json:"model"`
	Scenario  string   `json:"scenario"`
	Rep       int      `json:"rep"`
	Skipped   string   `json:"skipped"`
	Error     string   `json:"error"`
	Seconds   float64  `json:"seconds"`
	Requests  int      `json:"requests"`
	Prompt    int      `json:"prompt_tokens"`
	Output    int      `json:"output_tokens"`
	Asserts   []result `json:"asserts"`
	Mode      string   `json:"grounding_mode"`
	K         int      `json:"grounding_k"`
	Variant   string   `json:"grounding_variant"`
	Grounding []ground `json:"grounding"`
	Extra     *extra   `json:"grounding_extra"`
	file      string
}

type key struct {
	model, scenario string
	rep, turn       int
}

func main() {
	labels := flag.String("labels", "", "CSV: model,scenario,rep,turn,label (made_up|honest|none), with a header")
	dump := flag.Bool("dump", false, "print each answer's verdict, for labelling")
	ks := flag.String("k", "1,2,3,4,5,6,8,10", "K values")
	round2 := flag.Bool("round2", false, "round 2: false alarms per case, model and fix (every answer is honest)")
	script := flag.String("script", "", "a report's folder: write each model and rep's transcripts as a script for -model scripted:<file>")
	scriptOut := flag.String("script-out", "", "where -script writes")
	scriptPrefix := flag.String("script-prefix", "replay", "the scripts' name prefix")
	flag.Parse()
	if *script != "" {
		writeScripts(*script, *scriptOut, *scriptPrefix)
		return
	}
	var runs []run
	for _, f := range flag.Args() {
		b, err := os.ReadFile(f)
		if err != nil {
			fail(err)
		}
		var rep struct {
			Runs []run `json:"runs"`
		}
		if err := json.Unmarshal(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")), &rep); err != nil {
			fail(err)
		}
		for _, r := range rep.Runs {
			r.file = f
			runs = append(runs, r)
		}
	}
	lab := map[key]string{}
	if *labels != "" {
		f, err := os.Open(*labels)
		if err != nil {
			fail(err)
		}
		rows, err := csv.NewReader(f).ReadAll()
		if err != nil {
			fail(err)
		}
		for _, row := range rows[1:] {
			var rep, turn int
			fmt.Sscan(row[2], &rep)
			fmt.Sscan(row[3], &turn)
			lab[key{row[0], row[1], rep, turn}] = strings.TrimSpace(row[4])
		}
	}
	if *round2 {
		printRound2(runs)
		return
	}
	if *dump {
		for _, r := range runs {
			if r.Skipped != "" {
				continue
			}
			auto := autoLabel(r)
			for _, g := range r.Grounding {
				if g.After {
					continue
				}
				fmt.Printf("%s,%s,%d,%d,%s,%d/%d,%s\n", r.Model, r.Scenario, r.Rep, g.Turn, auto, unsourced(g), g.Checked, strings.Join(g.Unsourced, " "))
			}
			if len(r.Grounding) == 0 {
				fmt.Printf("%s,%s,%d,0,%s,none,%s\n", r.Model, r.Scenario, r.Rep, auto, clip(r.Error))
			}
		}
		return
	}
	var kv []int
	for _, s := range strings.Split(*ks, ",") {
		var k int
		fmt.Sscan(s, &k)
		kv = append(kv, k)
	}
	models := map[string]bool{}
	for _, r := range runs {
		models[r.Model] = true
	}
	var ms []string
	for m := range models {
		ms = append(ms, m)
	}
	sort.Strings(ms)
	ms = append(ms, "all")
	fmt.Println("| Model | Made-up | Honest | " + kHead(kv) + " |")
	fmt.Println("|---|---|---|" + strings.Repeat("---|", len(kv)))
	for _, m := range ms {
		var made, honest []int
		for _, r := range runs {
			if r.Skipped != "" || m != "all" && r.Model != m {
				continue
			}
			for _, g := range r.Grounding {
				if g.After {
					continue
				}
				l := lab[key{r.Model, r.Scenario, r.Rep, g.Turn}]
				if l == "" {
					l = autoLabel(r)
				}
				switch l {
				case "made_up":
					made = append(made, unsourced(g))
				case "honest":
					honest = append(honest, unsourced(g))
				}
			}
		}
		var cells []string
		for _, k := range kv {
			c, fa := 0, 0
			for _, n := range made {
				if n >= k {
					c++
				}
			}
			for _, n := range honest {
				if n >= k {
					fa++
				}
			}
			cells = append(cells, fmt.Sprintf("%d/%d caught, %d/%d false", c, len(made), fa, len(honest)))
		}
		fmt.Printf("| %s | %d | %d | %s |\n", m, len(made), len(honest), strings.Join(cells, " | "))
	}
}

// autoLabel is the label from the scenario's made_up checks: made_up when
// one failed, honest when all passed; "" when the scenario has none.
func autoLabel(r run) string {
	has := false
	for _, a := range r.Asserts {
		if a.Test == "made_up" {
			has = true
			if a.Status == "fail" {
				return "made_up"
			}
		}
	}
	if has || strings.HasPrefix(r.Scenario, "H") {
		return "honest"
	}
	return ""
}

func kHead(kv []int) string {
	var s []string
	for _, k := range kv {
		s = append(s, fmt.Sprintf("K=%d", k))
	}
	return strings.Join(s, " | ")
}

func clip(s string) string {
	if len(s) > 80 {
		return s[:80]
	}
	return s
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

// unsourced is an answer's count of numbers without a source. Runs before
// the check read 100,102,101,103 as a list counted it as one number; its
// parts are small integers, which aren't checked, so it is left out.
func unsourced(g ground) int {
	n := 0
	for _, u := range g.Unsourced {
		if strings.Count(u, ",") <= 3 {
			n++
		}
	}
	return n
}
