package scenario

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Report is a set's run on its models.
type Report struct {
	Set     string    `json:"set"`
	Dir     string    `json:"-"`
	Started time.Time `json:"started"`
	Took    float64   `json:"took_seconds"`
	Scale   float64   `json:"scale"`
	Reps    int       `json:"reps"`
	Models  []string  `json:"models"`
	Summary []Summary `json:"summary"`
	Runs    []Run     `json:"runs"`
}

// Run is one scenario on one model. Error is a run that broke, such as a
// provider error or a timeout; it doesn't pass, but its checks still
// count.
type Run struct {
	Model    string  `json:"model"`
	Scenario string  `json:"scenario"`
	Rep      int     `json:"rep"`
	Skipped  string  `json:"skipped,omitempty"`
	Error    string  `json:"error,omitempty"`
	Seconds  float64 `json:"seconds"`
	Turns    int     `json:"turns"`
	Requests int     `json:"requests"`
	// SubRequests are the subagents' requests, such as the schema
	// agent's; the token counts are the chat's only.
	SubRequests int      `json:"subagent_requests,omitempty"`
	Prompt      int      `json:"prompt_tokens"` // all requests: input, cache read and cache write
	Input       int      `json:"input_tokens"`  // not read from or written to the cache
	CacheRead   int      `json:"cache_read_tokens"`
	CacheWrite  int      `json:"cache_write_tokens"`
	PeakPrompt  int      `json:"peak_prompt_tokens"`
	Output      int      `json:"output_tokens"`
	Asserts     []Result `json:"asserts"`
	// SPIKE-032's number check: its mode and K, its verdicts, and what the
	// requests after its notice cost.
	GroundMode    string         `json:"grounding_mode,omitempty"`
	GroundK       int            `json:"grounding_k,omitempty"`
	GroundVariant string         `json:"grounding_variant,omitempty"` // round 2's switches
	Grounding     []GroundResult `json:"grounding,omitempty"`
	GroundExtra   *GroundExtra   `json:"grounding_extra,omitempty"`
	Transcript    string         `json:"-"`
}

// Passed reports whether the run didn't break and no check failed;
// "info" checks don't count.
func (r Run) Passed() bool {
	if r.Skipped != "" || r.Error != "" {
		return false
	}
	for _, a := range r.Asserts {
		if a.Status == Fail && a.Test != "info" {
			return false
		}
	}
	return true
}

// Summary is one model's numbers.
type Summary struct {
	Model   string          `json:"model"`
	Runs    int             `json:"runs"` // not skipped
	Passed  int             `json:"passed"`
	Errors  int             `json:"errors"`
	Skipped int             `json:"skipped"`
	Checks  map[string]Rate `json:"checks"` // by assertion type, "info" left out
	Prompt  int             `json:"avg_prompt_tokens"`
	Output  int             `json:"avg_output_tokens"`
	Seconds float64         `json:"avg_seconds"`
	// CacheRead is the share of all prompt tokens read from the cache.
	CacheRead float64 `json:"cache_read_share"`
	// Spread is, with 3 or more reps, the least and the most of each rep's
	// pass share: of the runs (spreadRuns) and of each check.
	Spread map[string]Range `json:"spread,omitempty"`
}

// Range is the least and the most of a share across reps.
type Range struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

func (r Range) String() string {
	return fmt.Sprintf("%.0f–%.0f %%", r.Min*100, r.Max*100)
}

// spreadFrom is the minimum number of reps for a spread.
const spreadFrom = 3

// spreadRuns is the Spread key of the runs' pass share; the other keys
// are check types.
const spreadRuns = "runs"

// Rate counts one check's results.
type Rate struct {
	Pass int `json:"pass"`
	Fail int `json:"fail"`
	Skip int `json:"skip"`
}

func (r Rate) String() string {
	if r.Pass+r.Fail == 0 {
		return "–"
	}
	return fmt.Sprintf("%d/%d", r.Pass, r.Pass+r.Fail)
}

func (rep *Report) summarize() {
	rep.Summary = nil
	for _, m := range rep.Models {
		s := Summary{Model: m, Checks: map[string]Rate{}}
		var prompt, output, cached int
		var secs float64
		for _, r := range rep.Runs {
			if r.Model != m {
				continue
			}
			if r.Skipped != "" {
				s.Skipped++
				continue
			}
			s.Runs++
			if r.Passed() {
				s.Passed++
			}
			if r.Error != "" {
				s.Errors++
			}
			prompt, output, cached, secs = prompt+r.Prompt, output+r.Output, cached+r.CacheRead, secs+r.Seconds
			for _, a := range r.Asserts {
				if a.Test == "info" {
					continue
				}
				c := s.Checks[a.Type]
				switch a.Status {
				case Pass:
					c.Pass++
				case Fail:
					c.Fail++
				default:
					c.Skip++
				}
				s.Checks[a.Type] = c
			}
		}
		if s.Runs > 0 {
			s.Prompt, s.Output, s.Seconds = prompt/s.Runs, output/s.Runs, secs/float64(s.Runs)
		}
		if prompt > 0 {
			s.CacheRead = float64(cached) / float64(prompt)
		}
		if rep.Reps >= spreadFrom {
			s.Spread = rep.spread(m)
		}
		rep.Summary = append(rep.Summary, s)
	}
}

// spread is each rep's pass share of the runs and of each check, as the
// least and the most across the reps.
func (rep *Report) spread(model string) map[string]Range {
	out := map[string]Range{}
	add := func(key string, share float64) {
		r, ok := out[key]
		if !ok {
			r = Range{Min: share, Max: share}
		}
		r.Min, r.Max = min(r.Min, share), max(r.Max, share)
		out[key] = r
	}
	for k := 1; k <= rep.Reps; k++ {
		runs, passed := 0, 0
		checks := map[string]Rate{}
		for _, r := range rep.Runs {
			if r.Model != model || r.Rep != k || r.Skipped != "" {
				continue
			}
			runs++
			if r.Passed() {
				passed++
			}
			for _, a := range r.Asserts {
				if a.Test == "info" {
					continue
				}
				c := checks[a.Type]
				switch a.Status {
				case Pass:
					c.Pass++
				case Fail:
					c.Fail++
				}
				checks[a.Type] = c
			}
		}
		if runs > 0 {
			add(spreadRuns, float64(passed)/float64(runs))
		}
		for t, c := range checks {
			if c.Pass+c.Fail > 0 {
				add(t, float64(c.Pass)/float64(c.Pass+c.Fail))
			}
		}
	}
	return out
}

func (rep *Report) summary(model string) (Summary, bool) {
	i := slices.IndexFunc(rep.Summary, func(s Summary) bool { return s.Model == model })
	if i < 0 {
		return Summary{}, false
	}
	return rep.Summary[i], true
}

// cell is a scenario's runs on a model: runs passed of runs, or why not.
func (rep *Report) cell(model, scenario string) string {
	pass, n, errs := 0, 0, 0
	skipped := ""
	for _, r := range rep.Runs {
		if r.Model != model || r.Scenario != scenario {
			continue
		}
		if r.Skipped != "" {
			skipped = r.Skipped
			continue
		}
		n++
		if r.Passed() {
			pass++
		}
		if r.Error != "" {
			errs++
		}
	}
	switch {
	case n == 0 && skipped != "":
		return "skip (" + skipped + ")"
	case n == 0:
		return ""
	case errs > 0:
		return fmt.Sprintf("%d/%d (%d broke)", pass, n, errs)
	}
	return fmt.Sprintf("%d/%d", pass, n)
}

func (rep *Report) scenarios() []string {
	var out []string
	for _, r := range rep.Runs {
		if !slices.Contains(out, r.Scenario) {
			out = append(out, r.Scenario)
		}
	}
	return out
}

func (rep *Report) checkTypes() []string {
	var out []string
	for _, s := range rep.Summary {
		for t := range s.Checks {
			if !slices.Contains(out, t) {
				out = append(out, t)
			}
		}
	}
	slices.Sort(out)
	return out
}

// ran is the scenarios a model ran, skipped ones left out, sorted.
func (rep *Report) ran(model string) []string {
	var out []string
	for _, r := range rep.Runs {
		if r.Model == model && r.Skipped == "" && !slices.Contains(out, r.Scenario) {
			out = append(out, r.Scenario)
		}
	}
	slices.Sort(out)
	return out
}

// Changes are what differs from an earlier report: each model's check
// rates, when it ran the same scenarios, and each scenario's cell.
func (rep *Report) Changes(prev *Report) []string {
	if prev == nil {
		return nil
	}
	var out []string
	for _, m := range rep.Models {
		now, _ := rep.summary(m)
		was, ok := prev.summary(m)
		if !ok {
			out = append(out, m+": new model")
			continue
		}
		// Check rates only compare when both ran the same scenarios.
		if !slices.Equal(prev.ran(m), rep.ran(m)) {
			out = append(out, m+": other scenarios ran than last time, so check rates are not compared")
		} else {
			for _, t := range rep.checkTypes() {
				if a, b := was.Checks[t].String(), now.Checks[t].String(); a != b {
					out = append(out, fmt.Sprintf("%s: %s %s → %s", m, t, a, b))
				}
			}
		}
		for _, sc := range rep.scenarios() {
			if a, b := prev.cell(m, sc), rep.cell(m, sc); a != b && a != "" {
				out = append(out, fmt.Sprintf("%s: %s %s → %s", m, sc, a, b))
			}
		}
	}
	return out
}

// Markdown is the report for reading.
func (rep *Report) Markdown(prev *Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Scenario report: %s\n\n", rep.Set)
	fmt.Fprintf(&b, "Started %s, took %.0f s. Reps %d, time scale %g.\n", rep.Started.Format(time.RFC3339), rep.Took, rep.Reps, rep.Scale)
	b.WriteString("\n## Models\n\n| Model | Runs passed | Broke | Skipped | Prompt tokens | Cache read | Output tokens | Time |\n|---|---|---|---|---|---|---|---|\n")
	for _, s := range rep.Summary {
		passed := fmt.Sprintf("%d/%d", s.Passed, s.Runs)
		if r, ok := s.Spread[spreadRuns]; ok {
			passed += " (" + r.String() + ")"
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %d | %.0f %% | %d | %.0f s |\n", s.Model, passed, s.Errors, s.Skipped, s.Prompt, s.CacheRead*100, s.Output, s.Seconds)
	}
	b.WriteString("\nTokens and time are averages per run. Cache read is the share of all prompt tokens read from the provider's cache.\n")
	if rep.Reps >= spreadFrom {
		b.WriteString("In brackets: the least and the most of a rep's pass share.\n")
	}
	if types := rep.checkTypes(); len(types) > 0 {
		b.WriteString("\n## Checks\n\nPassed of counted; skipped checks are left out.\n\n| Check |")
		for _, m := range rep.Models {
			b.WriteString(" " + m + " |")
		}
		b.WriteString("\n|---|" + strings.Repeat("---|", len(rep.Models)) + "\n")
		for _, t := range types {
			b.WriteString("| " + t + " |")
			for _, m := range rep.Models {
				s, _ := rep.summary(m)
				cell := s.Checks[t].String()
				if r, ok := s.Spread[t]; ok {
					cell += " (" + r.String() + ")"
				}
				b.WriteString(" " + cell + " |")
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n## Scenarios\n\nRuns passed of runs.\n\n| Scenario |")
	for _, m := range rep.Models {
		b.WriteString(" " + m + " |")
	}
	b.WriteString("\n|---|" + strings.Repeat("---|", len(rep.Models)) + "\n")
	for _, sc := range rep.scenarios() {
		b.WriteString("| " + sc + " |")
		for _, m := range rep.Models {
			b.WriteString(" " + rep.cell(m, sc) + " |")
		}
		b.WriteString("\n")
	}
	rep.groundMarkdown(&b)
	if prev != nil {
		fmt.Fprintf(&b, "\n## Changes since %s\n\n", prev.Started.Format(time.RFC3339))
		ch := rep.Changes(prev)
		if len(ch) == 0 {
			b.WriteString("None.\n")
		}
		for _, c := range ch {
			b.WriteString("- " + c + "\n")
		}
	}
	var fails []string
	for _, r := range rep.Runs {
		if r.Error != "" {
			fails = append(fails, fmt.Sprintf("- %s, %s r%d broke: %s", r.Model, r.Scenario, r.Rep, clip(r.Error, 300)))
		}
		for _, a := range r.Asserts {
			if a.Status == Fail {
				info := ""
				if a.Test == "info" {
					info = " (info)"
				}
				fails = append(fails, fmt.Sprintf("- %s, %s r%d: %s%s: %s", r.Model, r.Scenario, r.Rep, a.Type, info, clip(a.Why, 300)))
			}
		}
	}
	if len(fails) > 0 {
		b.WriteString("\n## Failures\n\n" + strings.Join(fails, "\n") + "\n")
	}
	return b.String()
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// Write saves the report in dir as <time>.json and <time>.md, and each
// run's transcript under <time>/<model>/. It returns the Markdown file.
func (rep *Report) Write(dir string, prev *Report) (string, error) {
	stamp := rep.Started.Format("20060102-150405")
	base := filepath.Join(dir, stamp)
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(base+".json", append(b, '\n'), 0o644); err != nil {
		return "", err
	}
	md := base + ".md"
	if err := os.WriteFile(md, []byte(rep.Markdown(prev)), 0o644); err != nil {
		return "", err
	}
	for _, r := range rep.Runs {
		if r.Transcript == "" {
			continue
		}
		d := filepath.Join(base, unsafeName.ReplaceAllString(r.Model, "_"))
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", err
		}
		f := filepath.Join(d, fmt.Sprintf("%s_r%d.md", r.Scenario, r.Rep))
		if err := os.WriteFile(f, []byte(r.Transcript), 0o644); err != nil {
			return "", err
		}
	}
	return md, nil
}

// Previous reads the newest report in dir, or nil if there is none.
func Previous(dir string) (*Report, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(files) == 0 {
		return nil, err
	}
	slices.Sort(files)
	return ReadReport(files[len(files)-1])
}

// ReadReport reads a report's JSON file, such as a baseline to compare
// with.
func ReadReport(path string) (*Report, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rep Report
	if err := json.Unmarshal(b, &rep); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &rep, nil
}
