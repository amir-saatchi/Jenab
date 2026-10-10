package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// The comparisons: each pair is judged in both orders, so a judge that
// prefers the first (or second) answer shows up as a split.
var comparisons = [][2]string{
	{"index", "plain"},
	{"index", "thinking"},
	{"index", "path"},
	{"path", "plain"},
}

var judgeSchema = json.RawMessage(`{"type":"object","properties":{
"reason":{"type":"string","minLength":1,"description":"what decides it, in 2 to 4 sentences"},
"winner":{"type":"string","enum":["A","B","tie"]},
"score_a":{"type":"integer","minimum":1,"maximum":10},
"score_b":{"type":"integer","minimum":1,"maximum":10}},
"required":["reason","winner","score_a","score_b"]}`)

func judgeSystem() string {
	return `You compare two answers to a user's how-to task and say which one helps the user more.

The better answer is right; fits the facts and limits the task gives; covers what the user needs in order to act; is concrete about how; and names the risks that matter. Don't prefer an answer because it is longer, has more headings or lists, or sounds more sure. A shorter answer that does the job beats a longer one that pads. The task and answers may be in any language.

First give your reason, then the winner, and a score from 1 to 10 for each answer.

Answer with exactly one JSON object that matches this JSON Schema, and nothing else:
` + string(judgeSchema)
}

func judgeMsg(task, a, b string) string {
	return fmt.Sprintf("Task:\n%s\n\n=== Answer A ===\n%s\n\n=== Answer B ===\n%s\n\n=== End of the answers ===\n\nWhich answer helps the user more with this task?", task, a, b)
}

type verdict struct {
	Reason string `json:"reason"`
	Winner string `json:"winner"`
	ScoreA int    `json:"score_a"`
	ScoreB int    `json:"score_b"`
}

// judge has one judge compare the answers of every problem and model,
// skipping the verdicts it already gave.
func judge(ctx context.Context, st *store, c *client, maxTokens int) error {
	all, err := st.answers(ctx)
	if err != nil {
		return err
	}
	keys := sortedKeys(all)
	var errs []error
	for _, key := range keys {
		p, ok := findProblem(key[0])
		if !ok {
			continue
		}
		as := all[key]
		for _, cmp := range comparisons {
			x, y := as[cmp[0]], as[cmp[1]]
			if x == nil || y == nil || !x.Valid || !y.Valid {
				continue
			}
			for _, ab := range [][2]*answerRow{{x, y}, {y, x}} {
				a, b := ab[0], ab[1]
				var done bool
				st.db.QueryRowContext(ctx, `SELECT valid FROM judgments WHERE judge = ? AND a_id = ? AND b_id = ?`, c.model, a.ID, b.ID).Scan(&done)
				if done {
					continue
				}
				var v verdict
				rec := &callRec{Purpose: "judge"}
				err := c.askJSON(ctx, judgeSystem(), judgeMsg(p.Text, a.Text, b.Text), judgeSchema, true, maxTokens, func(raw json.RawMessage) error {
					v = verdict{}
					if err := json.Unmarshal(raw, &v); err != nil {
						return err
					}
					switch {
					case v.Winner == "A" && v.ScoreA < v.ScoreB, v.Winner == "B" && v.ScoreB < v.ScoreA:
						return fmt.Errorf("winner %s, but its score is lower", v.Winner)
					}
					return nil
				}, rec)
				call, serr := st.addCall(ctx, 0, c.model, rec)
				if serr != nil {
					return serr
				}
				if _, serr := st.db.ExecContext(ctx, `INSERT OR REPLACE INTO judgments (judge, a_id, b_id, winner, score_a, score_b, reason, valid, call_id, created_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, c.model, a.ID, b.ID, v.Winner, v.ScoreA, v.ScoreB, v.Reason, err == nil, call, now()); serr != nil {
					return serr
				}
				if err != nil {
					c.logf("%s %s: %s vs %s: %s", key[0], key[1], a.Kind, b.Kind, clip(err.Error()))
					errs = append(errs, err)
					if errors.Is(err, errQuota) || ctx.Err() != nil {
						return errors.Join(errs...)
					}
					continue
				}
				c.logf("%s %s: A %s %d, B %s %d → %s", key[0], key[1], a.Kind, v.ScoreA, b.Kind, v.ScoreB, v.Winner)
			}
		}
	}
	return errors.Join(errs...)
}

func sortedKeys[V any](m map[[2]string]V) [][2]string {
	var keys [][2]string
	for k := range m {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b [2]string) int { return strings.Compare(a[0]+" "+a[1], b[0]+" "+b[1]) })
	return keys
}

// outcome of one pair x against y, by one judge, over both orders.
type outcome struct {
	Judge, Problem, Model string
	X, Y                  string
	Result                string // x, y, tie, split; "" if a verdict is missing
	ScoreX, ScoreY        float64
	FirstWins             int // verdicts that picked answer A
}

func outcomes(ctx context.Context, st *store) ([]outcome, error) {
	all, err := st.answers(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := st.db.QueryContext(ctx, `SELECT judge, a_id, b_id, winner, score_a, score_b FROM judgments WHERE valid = 1 ORDER BY judge`)
	if err != nil {
		return nil, err
	}
	type vk struct {
		judge string
		a, b  int64
	}
	vs := map[vk]verdict{}
	judges := []string{}
	for rows.Next() {
		var k vk
		var v verdict
		if err := rows.Scan(&k.judge, &k.a, &k.b, &v.Winner, &v.ScoreA, &v.ScoreB); err != nil {
			rows.Close()
			return nil, err
		}
		vs[k] = v
		if !slices.Contains(judges, k.judge) {
			judges = append(judges, k.judge)
		}
	}
	rows.Close()
	var out []outcome
	for _, j := range judges {
		for _, key := range sortedKeys(all) {
			as := all[key]
			for _, cmp := range comparisons {
				x, y := as[cmp[0]], as[cmp[1]]
				if x == nil || y == nil {
					continue
				}
				o := outcome{Judge: j, Problem: key[0], Model: key[1], X: cmp[0], Y: cmp[1]}
				v1, ok1 := vs[vk{j, x.ID, y.ID}] // x shown first
				v2, ok2 := vs[vk{j, y.ID, x.ID}]
				if ok1 && ok2 {
					r1 := map[string]string{"A": "x", "B": "y", "tie": "tie"}[v1.Winner]
					r2 := map[string]string{"A": "y", "B": "x", "tie": "tie"}[v2.Winner]
					o.Result = r1
					if r1 != r2 {
						o.Result = "split"
					}
					o.ScoreX = float64(v1.ScoreA+v2.ScoreB) / 2
					o.ScoreY = float64(v1.ScoreB+v2.ScoreA) / 2
					for _, v := range []verdict{v1, v2} {
						if v.Winner == "A" {
							o.FirstWins++
						}
					}
				}
				out = append(out, o)
			}
		}
	}
	return out, nil
}

// writeJudges writes judges.md: per comparison and judge, how often each
// side won over both orders, and whether the two judges agree.
func writeJudges(ctx context.Context, st *store, dir string) (string, error) {
	outs, err := outcomes(ctx, st)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("# SPIKE-031 round 2: judges\n\nEach pair was judged twice, once in each order. *X wins* and *Y wins* count pairs where both verdicts agree; *split* is a pair where the order changed the verdict. *Scores* are the means, 1–10. *A picked* is the share of all verdicts that picked the answer shown first.\n\n")
	b.WriteString("| X vs Y | Builder | Judge | Pairs | X wins | Tie | Y wins | Split | Score X | Score Y | A picked |\n|---|---|---|---|---|---|---|---|---|---|---|\n")
	type gk struct{ cmp, model, judge string }
	type agg struct {
		n, x, y, tie, split, first int
		sx, sy                     float64
	}
	groups := map[gk]*agg{}
	var order []gk
	for _, o := range outs {
		if o.Result == "" {
			continue
		}
		for _, model := range []string{o.Model, "all"} {
			k := gk{o.X + " vs " + o.Y, model, o.Judge}
			if groups[k] == nil {
				groups[k] = &agg{}
				order = append(order, k)
			}
			a := groups[k]
			a.n++
			a.sx += o.ScoreX
			a.sy += o.ScoreY
			a.first += o.FirstWins
			switch o.Result {
			case "x":
				a.x++
			case "y":
				a.y++
			case "tie":
				a.tie++
			default:
				a.split++
			}
		}
	}
	slices.SortStableFunc(order, func(p, q gk) int {
		return strings.Compare(p.cmp+p.model+p.judge, q.cmp+q.model+q.judge)
	})
	for _, k := range order {
		a := groups[k]
		fmt.Fprintf(&b, "| %s | %s | %s | %d | %d | %d | %d | %d | %.1f | %.1f | %.0f%% |\n", k.cmp, k.model, k.judge, a.n, a.x, a.tie, a.y, a.split,
			a.sx/float64(a.n), a.sy/float64(a.n), 100*float64(a.first)/float64(2*a.n))
	}

	// Agreement: the pairs both judges resolved, and how often they agree.
	b.WriteString("\n## Do the judges agree?\n\n")
	byPair := map[string][]string{}
	var pairs []string
	for _, o := range outs {
		if o.Result == "" {
			continue
		}
		k := o.Problem + " · " + o.Model + " · " + o.X + " vs " + o.Y
		if byPair[k] == nil {
			pairs = append(pairs, k)
		}
		byPair[k] = append(byPair[k], o.Judge+": "+o.Result)
	}
	same, both := 0, 0
	for _, k := range pairs {
		if rs := byPair[k]; len(rs) == 2 {
			both++
			if strings.SplitN(rs[0], ": ", 2)[1] == strings.SplitN(rs[1], ": ", 2)[1] {
				same++
			}
		}
	}
	fmt.Fprintf(&b, "%d pairs judged by both; same result in %d.\n\n", both, same)
	for _, k := range pairs {
		fmt.Fprintf(&b, "- %s: %s\n", k, strings.Join(byPair[k], ", "))
	}
	path := filepath.Join(dir, "judges.md")
	return path, os.WriteFile(path, []byte(b.String()), 0o644)
}

// writeBlind writes up to n pairs for a person to judge, without saying
// which answer is which: the index graph's answer against the baseline
// the judges scored higher (thinking if there are no verdicts yet). The
// key is in key.json; don't open it before judging.
func writeBlind(ctx context.Context, st *store, dir string, n int) (string, error) {
	all, err := st.answers(ctx)
	if err != nil {
		return "", err
	}
	outs, err := outcomes(ctx, st)
	if err != nil {
		return "", err
	}
	score := map[string]float64{} // problem model kind → mean score against index
	count := map[string]int{}
	for _, o := range outs {
		if o.Result != "" && o.X == "index" {
			k := o.Problem + " " + o.Model + " " + o.Y
			score[k] += o.ScoreY
			count[k]++
		}
	}
	rng := rand.New(rand.NewPCG(31, 2))
	keys := sortedKeys(all)
	rng.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
	bdir := filepath.Join(dir, "blind")
	if err := os.MkdirAll(bdir, 0o755); err != nil {
		return "", err
	}
	type entry struct {
		Pair     string `json:"pair"`
		Problem  string `json:"problem"`
		Model    string `json:"model"`
		A        string `json:"a"`
		B        string `json:"b"`
		AID, BID int64
	}
	var key []entry
	for _, k := range keys {
		if len(key) == n {
			break
		}
		as := all[k]
		x := as["index"]
		if x == nil || !x.Valid {
			continue
		}
		var y *answerRow
		best := -1.0
		for _, kind := range []string{"thinking", "plain"} {
			a := as[kind]
			if a == nil || !a.Valid {
				continue
			}
			s := 0.0
			if c := count[k[0]+" "+k[1]+" "+kind]; c > 0 {
				s = score[k[0]+" "+k[1]+" "+kind] / float64(c)
			}
			if s > best {
				y, best = a, s
			}
		}
		if y == nil {
			continue
		}
		p, _ := findProblem(k[0])
		a, b := x, y
		if rng.IntN(2) == 1 {
			a, b = y, x
		}
		e := entry{Pair: fmt.Sprintf("pair-%02d", len(key)+1), Problem: k[0], Model: k[1], A: a.Kind, B: b.Kind, AID: a.ID, BID: b.ID}
		key = append(key, e)
		text := fmt.Sprintf("# %s\n\n**Task:**\n\n%s\n\n---\n\n## Answer A\n\n%s\n\n---\n\n## Answer B\n\n%s\n\n---\n\n## Your verdict\n\nWinner (A, B or tie): \nScore A (1–10): \nScore B (1–10): \nWhy: \n",
			e.Pair, p.Text, a.Text, b.Text)
		if err := os.WriteFile(filepath.Join(bdir, e.Pair+".md"), []byte(text), 0o644); err != nil {
			return "", err
		}
	}
	kb, _ := json.MarshalIndent(key, "", "  ")
	if err := os.WriteFile(filepath.Join(bdir, "key.json"), kb, 0o644); err != nil {
		return "", err
	}
	return bdir, nil
}
