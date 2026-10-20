package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Replay: a run's transcript becomes a script for the scenario runner
// (-model scripted:<file>), so a later version of the check can score the
// same answers offline, with the real tools and no model. One script is
// written per model and rep of a report folder, named <prefix>_<model>_r<rep>.
//
// The transcripts clip tool calls at 600 bytes. A clipped call's JSON is
// closed where it was cut, so a long saved file loses its tail; the
// replay counts these.

type scriptReply struct {
	Text  string       `json:"text,omitempty"`
	Calls []scriptCall `json:"calls,omitempty"`
}

type scriptCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

var reTranscript = regexp.MustCompile(`^(.+)_r(\d+)\.md$`)

// writeScripts reads dir/<model>/<scenario>_r<rep>.md and writes the
// scripts to out.
func writeScripts(dir, out, prefix string) {
	models, err := os.ReadDir(dir)
	if err != nil {
		fail(err)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		fail(err)
	}
	clipped, broken := 0, 0
	for _, m := range models {
		if !m.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(dir, m.Name()))
		if err != nil {
			fail(err)
		}
		byRep := map[string]map[string][]scriptReply{}
		for _, f := range files {
			sm := reTranscript.FindStringSubmatch(f.Name())
			if sm == nil {
				continue
			}
			rs, c, b := readTranscript(filepath.Join(dir, m.Name(), f.Name()))
			clipped, broken = clipped+c, broken+b
			if byRep[sm[2]] == nil {
				byRep[sm[2]] = map[string][]scriptReply{}
			}
			byRep[sm[2]][sm[1]] = rs
		}
		_, model, ok := strings.Cut(m.Name(), "_") // <provider>_<model>
		if !ok {
			model = m.Name()
		}
		var reps []string
		for r := range byRep {
			reps = append(reps, r)
		}
		sort.Strings(reps)
		for _, r := range reps {
			b, err := json.MarshalIndent(map[string]any{"scenarios": byRep[r]}, "", " ")
			if err != nil {
				fail(err)
			}
			name := fmt.Sprintf("%s_%s_r%s.yaml", prefix, model, r)
			if err := os.WriteFile(filepath.Join(out, name), b, 0o644); err != nil {
				fail(err)
			}
			fmt.Println(filepath.Join(out, name))
		}
	}
	fmt.Fprintf(os.Stderr, "%d clipped calls closed, %d calls dropped\n", clipped, broken)
}

// readTranscript reads a transcript's model replies: a reply is the
// answer text and the calls before the next tool result, notice or user
// message.
func readTranscript(path string) (out []scriptReply, clipped, broken int) {
	f, err := os.Open(path)
	if err != nil {
		fail(err)
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		lines = append(lines, strings.TrimSuffix(sc.Text(), "\r"))
	}
	var cur *scriptReply
	end := func() {
		if cur != nil && (strings.TrimSpace(cur.Text) != "" || len(cur.Calls) > 0) {
			cur.Text = strings.TrimSpace(cur.Text)
			out = append(out, *cur)
		}
		cur = nil
	}
	started := false
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		switch {
		case strings.HasPrefix(l, "## Turn "):
			started = true
			end()
		case !started:
		case l == "## Number check" || l == "## Requests":
			end()
			return out, clipped, broken
		case strings.HasPrefix(l, "*[the model sent an empty answer"):
			end()
			out = append(out, scriptReply{Text: "(empty)"}) // a script reply needs text; this one has no number
		case strings.HasPrefix(l, "**User:**"), strings.HasPrefix(l, "*["), strings.HasPrefix(l, "> result "):
			end()
			for i+1 < len(lines) && strings.HasPrefix(lines[i+1], "> ") && !strings.HasPrefix(lines[i+1], "> call ") {
				i++ // the result's lines
			}
		case strings.HasPrefix(l, "**Agent:** "):
			if cur != nil && len(cur.Calls) > 0 {
				end()
			}
			if cur == nil {
				cur = &scriptReply{}
			}
			text := []string{strings.TrimPrefix(l, "**Agent:** ")}
			for i+1 < len(lines) && !blockStart(lines[i+1]) {
				i++
				text = append(text, lines[i])
			}
			if cur.Text != "" {
				cur.Text += "\n\n"
			}
			cur.Text += strings.Join(text, "\n")
		case strings.HasPrefix(l, "> call "):
			if cur == nil {
				cur = &scriptReply{}
			}
			name, raw, _ := strings.Cut(strings.TrimPrefix(l, "> call "), " ")
			args, c := parseArgs(raw)
			if args == nil {
				broken++
				continue
			}
			if c {
				clipped++
			}
			cur.Calls = append(cur.Calls, scriptCall{Name: name, Args: args})
		}
	}
	end()
	return out, clipped, broken
}

func blockStart(l string) bool {
	for _, p := range []string{"> call ", "> result ", "**User:**", "**Agent:** ", "*[some numbers", "*[app check", "*[the ", "## Turn ", "## Number check"} {
		if strings.HasPrefix(l, p) {
			return true
		}
	}
	return false
}

// parseArgs reads a call's JSON arguments, closing one the transcript
// clipped.
func parseArgs(raw string) (map[string]any, bool) {
	var a map[string]any
	if json.Unmarshal([]byte(raw), &a) == nil {
		return a, false
	}
	cut, ok := strings.CutSuffix(raw, "…")
	if !ok {
		return nil, false
	}
	for drop := 0; drop < 8 && drop < len(cut); drop++ {
		s := cut[:len(cut)-drop]
		for _, tail := range []string{`"}`, `"}]}`, `"}}`, `}`, `]}`, `"]}`} {
			if json.Unmarshal([]byte(s+tail), &a) == nil {
				return a, true
			}
		}
	}
	return nil, false
}
