package main

import (
	"fmt"
	"strings"
	"time"
)

func secs(d time.Duration) string {
	if d == 0 {
		return "–"
	}
	return fmt.Sprintf("%.2f s", d.Seconds())
}

func cell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.ReplaceAll(s, "\n", " ")
}

func statusLabel(s int) string {
	if s == 0 {
		return "transport/stream"
	}
	return fmt.Sprint(s)
}

func errLabel(r Result) string {
	switch {
	case strings.HasPrefix(r.ErrText, "skipped"):
		return "skipped (" + strings.TrimPrefix(r.ErrText, "skipped: ") + ")"
	case r.Status == 0 || r.Status == 200:
		return "error (transport/stream)"
	}
	return fmt.Sprintf("error %d", r.Status)
}

func report(all []*ModelResult, bursts []string) {
	w := out
	// 1
	fmt.Fprintf(w, "## 1. Basic streaming\n\nPrompt: reply \"pong\" and three colors; max tokens 400. TTFT = first streamed content, reasoning or tool delta.\n\n")
	fmt.Fprintf(w, "| Model | Result | finish_reason | Text | Usage in/out | Usage chunk | cached_tokens field | TTFT | Total | Reasoning chars | Notes |\n|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, r := range all {
		b := r.Basic
		res := "pass"
		if !r.BasicPass {
			res = "FAIL: " + r.BasicWhy
		}
		fmt.Fprintf(w, "| %s | %s | %s | %s | %d/%d | %s | %s | %s | %s | %d | %s |\n", r.M.Name(), cell(res), orDash(b.Finish), cell(short(b.Text, 50)),
			b.Prompt, b.Output, orDash(b.UsageWhere), yn(b.CachedSeen), secs(b.TTFT), secs(b.Total), len(b.Reasoning), cell(strings.Join(b.Notes, "; ")))
	}
	// 2
	fmt.Fprintf(w, "\n## 2. Tool calling (%d tools)\n\n", len(toolSpecs))
	for _, t := range tasks {
		fmt.Fprintf(w, "- %s (%s): %s\n", t.ID, t.Kind, t.Prompt)
	}
	fmt.Fprintf(w, "\nA task passes when every step calls exactly the expected tools with schema-valid and correct arguments, and the final answer (if any) has no tool call and contains the expected fact. Fake tool results are fed back between steps.\n\n")
	fmt.Fprintf(w, "| Model | Tasks passed | Calls schema-valid |")
	for _, t := range tasks {
		fmt.Fprintf(w, " %s |", t.ID)
	}
	fmt.Fprintf(w, "\n|---|---|---|%s\n", strings.Repeat("---|", len(tasks)))
	for _, r := range all {
		pass := 0
		byID := map[string]TaskResult{}
		for _, t := range r.Tasks {
			byID[t.ID] = t
			if t.Pass {
				pass++
			}
		}
		fmt.Fprintf(w, "| %s | %d/%d | %d/%d |", r.M.Name(), pass, len(r.Tasks), r.CallsValid, r.CallsTotal)
		for _, t := range tasks {
			tr, ok := byID[t.ID]
			switch {
			case !ok:
				fmt.Fprintf(w, " – |")
			case tr.Pass:
				fmt.Fprintf(w, " ok %.0fs |", tr.Time.Seconds())
			default:
				fmt.Fprintf(w, " x |")
			}
		}
		fmt.Fprintln(w)
	}
	var ne []string
	for _, r := range all {
		signed := false
		for _, t := range r.Tasks {
			signed = signed || t.Signed
		}
		if r.M.P.Name != "gemini" {
			continue
		}
		s := fmt.Sprintf("- %s: tool calls carried `extra_content` (thought signature): %s.", r.M.Name(), yn(signed))
		if r.NoEcho != nil {
			res := "passed"
			if !r.NoEcho.Pass {
				res = "failed: " + cell(r.NoEcho.Why)
			}
			s += " t9 again without sending it back: " + res
		}
		ne = append(ne, s)
	}
	if len(ne) > 0 {
		fmt.Fprintf(w, "\nGemini thought signatures (sent back unchanged in the runs above):\n\n%s\n", strings.Join(ne, "\n"))
	}
	fmt.Fprintf(w, "\nFailures:\n\n")
	for _, r := range all {
		for _, t := range r.Tasks {
			if !t.Pass {
				fmt.Fprintf(w, "- %s %s: %s\n", r.M.Name(), t.ID, cell(t.Why))
			}
		}
		if r.IndexReuse > 0 {
			fmt.Fprintf(w, "- %s: in %d response(s) parallel tool calls came with the same index and different ids (handled: a new id starts a new call)\n", r.M.Name(), r.IndexReuse)
		}
		for _, b := range r.BadArgs {
			fmt.Fprintf(w, "- %s invalid arguments: %s\n", r.M.Name(), cell(b))
		}
	}
	// 3
	fmt.Fprintf(w, "\n## 3. Context size\n\nOne Burrow-shaped request per size (SPEC 3.1 blocks in the system message, a history window of query turns with tool result previews, the current turn, all %d tools). Sizes are estimated from the chars per token measured in section 4.\n\n", len(toolSpecs))
	fmt.Fprintf(w, "| Model | Context limit | Target | Prompt tokens | TTFT | Total | Result |\n|---|---|---|---|---|---|---|\n")
	for _, r := range all {
		for _, c := range r.Ctx {
			res := "ok: " + short(c.R.Text, 60)
			if c.R.Err != nil {
				res = "ERROR " + short(c.R.ErrText, 220)
			} else if c.R.Text == "" {
				res = "ok, finish " + c.R.Finish + ", no text"
			}
			fmt.Fprintf(w, "| %s | %s | %s | %d | %s | %s | %s |\n", r.M.Name(), cell(r.M.Context), c.Label, c.R.Prompt, secs(c.R.TTFT), secs(c.R.Total), cell(res))
		}
	}
	// 4
	fmt.Fprintf(w, "\n## 4. Prompt caching\n\nTurn 1: a Burrow-shaped request of about 4k tokens with a fresh session nonce at the start (cold). Turn 2 and 3: the same messages plus the assistant answer and one short new user message (the request only grows at the end).\n\n")
	fmt.Fprintf(w, "| Model | Turn 1 in / cached / TTFT | Turn 2 in / cached / TTFT | Turn 3 in / cached / TTFT | Whole run: calls with cached_tokens field / with cached > 0 / max cached / successful calls |\n|---|---|---|---|---|\n")
	for _, r := range all {
		fmt.Fprintf(w, "| %s |", r.M.Name())
		seen := false
		for i := 0; i < 3; i++ {
			if i >= len(r.Cache) {
				fmt.Fprintf(w, " – |")
				continue
			}
			c := r.Cache[i]
			if c.Err != nil {
				fmt.Fprintf(w, " ERROR %s |", cell(short(c.ErrText, 80)))
				continue
			}
			seen = seen || c.CachedSeen
			fmt.Fprintf(w, " %d / %d / %s |", c.Prompt, c.Cached, secs(c.TTFT))
		}
		_ = seen
		r.M.mu.Lock()
		fmt.Fprintf(w, " %d / %d / %d / %d |\n", r.M.cachedSeen, r.M.cachedHits, r.M.cachedMax, r.M.calls)
		r.M.mu.Unlock()
	}
	// 5
	fmt.Fprintf(w, "\n## 5. Rate limits\n\nBurst tests (after the other tests of the provider): Groq sends 4 requests of about 3.5k tokens back to back (TPM limit test); Gemini sends up to 16 tiny requests back to back (RPM test); Ollama and Z.ai send 2 requests at the same time (Ollama free plan: 1 concurrent request).\n\n")
	fmt.Fprintf(w, "| Provider | Model | Result |\n|---|---|---|\n")
	for _, b := range bursts {
		fmt.Fprintln(w, b)
	}
	fmt.Fprintf(w, "\nEvery 429 and every other error status seen in the whole run (rate-limit header values only; no auth headers are read):\n\n")
	fmt.Fprintf(w, "| Model | Request | Attempt | Status | Rate-limit headers | Wait hint in body | Action | Body (short) |\n|---|---|---|---|---|---|---|---|\n")
	rlMu.Lock()
	for _, e := range rlLog {
		fmt.Fprintf(w, "| %s | %s | %d | %s | %s | %s | %s | %s |\n", e.Model, e.Label, e.Attempt, statusLabel(e.Status), cell(orDash(e.Headers)), orDash(e.Hint), cell(e.Action), cell(short(e.Msg, 700)))
	}
	if len(rlLog) == 0 {
		fmt.Fprintf(w, "| – | – | – | – | – | – | none | – |\n")
	}
	rlMu.Unlock()
	// sample headers of successful calls
	fmt.Fprintf(w, "\nRate-limit headers on the basic request (success):\n\n")
	for _, r := range all {
		fmt.Fprintf(w, "- %s: %s\n", r.M.Name(), cell(orDash(r.Basic.Headers)))
	}
	// summary
	fmt.Fprintf(w, "\n## Summary\n\n| Model | Basic | Tools | 8k TTFT | 32k TTFT | Cached tokens: cache turn 2; max in run | 429s / other errors |\n|---|---|---|---|---|---|---|\n")
	for _, r := range all {
		basic := "pass"
		if !r.BasicPass {
			basic = "fail"
		}
		pass := 0
		for _, t := range r.Tasks {
			if t.Pass {
				pass++
			}
		}
		ctx := map[string]string{}
		for _, c := range r.Ctx {
			k := strings.Fields(c.Label)[0]
			if c.R.Err != nil {
				ctx[k] = errLabel(c.R)
			} else {
				ctx[k] = secs(c.R.TTFT)
			}
		}
		cached := "–"
		if len(r.Cache) > 1 && r.Cache[1].Err == nil {
			if r.Cache[1].CachedSeen {
				cached = fmt.Sprintf("%d of %d", r.Cache[1].Cached, r.Cache[1].Prompt)
			} else {
				cached = "not reported"
			}
		}
		r.M.mu.Lock()
		cached += fmt.Sprintf("; max %d", r.M.cachedMax)
		r.M.mu.Unlock()
		n429, nerr := 0, 0
		rlMu.Lock()
		for _, e := range rlLog {
			if e.Model == r.M.Name() {
				if e.Status == 429 {
					n429++
				} else {
					nerr++
				}
			}
		}
		rlMu.Unlock()
		k8 := orDash(ctx["8k"])
		if v, ok := ctx["5.5k"]; ok {
			k8 += " (5.5k: " + v + ")"
		}
		fmt.Fprintf(w, "| %s | %s | %d/%d | %s | %s | %s | %d / %d |\n", r.M.Name(), basic, pass, len(r.Tasks), k8, orDash(ctx["32k"]), cached, n429, nerr)
	}
}
