package main

// Offline checks of the orchestrator and the assertions with a scripted model (no network).

import (
	"context"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
)

type scripted struct {
	steps []Result
	i     int
	seen  [][]openai.ChatCompletionMessageParamUnion
}

func (s *scripted) Call(ctx context.Context, r Req) Result {
	s.seen = append(s.seen, r.Messages)
	if s.i >= len(s.steps) {
		return Result{Text: "(end of script)", Finish: "stop", Prompt: 100}
	}
	x := s.steps[s.i]
	s.i++
	x.Finish = "stop"
	x.Prompt = 1000
	return x
}

func setup(t *testing.T) map[string]*Scenario {
	t.Helper()
	loadPrompts()
	initTools()
	sc, err := loadScenarios("scenarios")
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]*Scenario{}
	for _, s := range sc {
		m[s.ID] = s
	}
	return m
}

func play(t *testing.T, sc *Scenario, steps []Result) (*RunRec, *scripted) {
	t.Helper()
	db, err := openFixtureDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	llm := &scripted{steps: steps}
	o := newOrch(sc, llm, systemPrompt(sc.Context, "rule"), db)
	o.Run(context.Background())
	rec := &RunRec{Scenario: sc.ID}
	finishRec(rec, o, sc)
	return rec, llm
}

func call(name, args string) Call { return Call{ID: "c_" + name, Name: name, Args: args} }

func statusOfTest(rec *RunRec, test string) string { return rec.TestStatus[test] }

func dump(t *testing.T, rec *RunRec) {
	for _, a := range rec.Asserts {
		t.Logf("%s %s %s: %s", a.Status, a.Test, a.Type, a.Why)
	}
}

func TestGoodMultiPart(t *testing.T) {
	sc := setup(t)
	rec, _ := play(t, sc["t12_a_moving_average_btc"], []Result{
		{Text: "A moving average is the average of the last N closes; it smooths out noise.", Calls: []Call{call("run_pipeline", `{"id":"daily_prices","inputs":{"coin":"BTC"}}`)}},
		{Text: "The daily_prices run is running in the background; I'll post the price and news when it finishes."},
		{Text: "BTC closed today at 64,210.50 USD (+1.8%). News: ETF inflows of 410M USD; SEC delays staking ETFs; miner reserves keep falling."},
	})
	dump(t, rec)
	if len(rec.Turns) != 2 || rec.Turns[1].Kind != "system" {
		t.Fatalf("want a user turn and a system turn, got %d", len(rec.Turns))
	}
	for _, k := range []string{"1", "2"} {
		if statusOfTest(rec, k) != "pass" {
			t.Errorf("test %s: %s", k, statusOfTest(rec, k))
		}
	}
	if !strings.Contains(rec.Turns[1].Parts[0].Text, "[run r_1 finished") {
		t.Errorf("notice: %q", rec.Turns[1].Parts[0].Text)
	}
}

func TestRepeatAndRestart(t *testing.T) {
	sc := setup(t)
	rec, _ := play(t, sc["t12_a_moving_average_btc"], []Result{
		{Calls: []Call{call("run_pipeline", `{"id":"daily_prices"}`)}},
		{Text: "Started."},
		{Text: "A moving average is the average of the last N closes. BTC is 64,210.50 USD.", Calls: []Call{call("run_pipeline", `{"id":"daily_prices"}`)}},
		{Text: "ok"},
		{Text: "done"},
	})
	dump(t, rec)
	if statusOfTest(rec, "1") != "fail" {
		t.Errorf("test 1 should fail (quick part only in the finish turn): %s", statusOfTest(rec, "1"))
	}
	if statusOfTest(rec, "2") != "fail" {
		t.Errorf("test 2 should fail: %s", statusOfTest(rec, "2"))
	}
}

func TestMidTurnMessage(t *testing.T) {
	sc := setup(t)
	rec, llm := play(t, sc["t3_a_news_add_eth"], []Result{
		{Calls: []Call{call("web_search", `{"query":"bitcoin news"}`)}},
		{Calls: []Call{call("web_search", `{"query":"ethereum news"}`)}},
		{Text: "BTC: ETF inflows, hashrate record, SEC delay. ETH: gas fees low, staking queue shorter, ETF inflows."},
	})
	dump(t, rec)
	if len(rec.Turns) != 1 {
		t.Fatalf("message should join the running turn, got %d turns", len(rec.Turns))
	}
	// the second request must contain the user message after the tool result
	msgs := llm.seen[1]
	last := msgs[len(msgs)-1]
	if last.OfUser == nil || !strings.Contains(last.OfUser.Content.OfString.Value, "also add ETH") {
		t.Errorf("second request should end with the mid-turn message")
	}
	if statusOfTest(rec, "3") != "pass" {
		t.Errorf("test 3: %s", statusOfTest(rec, "3"))
	}
}

func TestMessageWhileBackground(t *testing.T) {
	sc := setup(t)
	rec, _ := play(t, sc["t3_c_run_then_add_eth"], []Result{
		{Calls: []Call{call("run_pipeline", `{"id":"daily_prices"}`)}},
		{Text: "Started run r_1."},
		{Calls: []Call{call("run_pipeline", `{"id":"daily_prices","inputs":{"coin":"ETH"}}`)}},
		{Text: "Started run r_2 for ETH."},
		{Text: "BTC: 64,210.50 USD."},
		{Text: "ETH: 2,587.40 USD."},
	})
	dump(t, rec)
	kinds := []string{}
	for _, t := range rec.Turns {
		kinds = append(kinds, t.Kind)
	}
	if strings.Join(kinds, ",") != "user,user,system,system" {
		t.Errorf("turn kinds %v", kinds)
	}
	if statusOfTest(rec, "3") != "pass" {
		t.Errorf("test 3: %s", statusOfTest(rec, "3"))
	}
}

func TestPollSuppressesNotice(t *testing.T) {
	sc := setup(t)
	steps := []Result{{Calls: []Call{call("run_pipeline", `{"id":"daily_prices"}`)}}}
	for i := 0; i < 3; i++ {
		steps = append(steps, Result{Calls: []Call{call("run_status", `{"run_id":"r_1"}`)}})
	}
	steps = append(steps, Result{Text: "A moving average smooths the last N days. BTC: 64,210.50."})
	s := *sc["t12_a_moving_average_btc"]
	s.Delays = map[string]string{"run_pipeline": "10s"}
	rec, _ := play(t, &s, steps)
	dump(t, rec)
	if len(rec.Turns) != 1 || rec.Tasks[0].Delivered != "poll" {
		t.Errorf("turns %d delivered %s", len(rec.Turns), rec.Tasks[0].Delivered)
	}
	if statusOfTest(rec, "2") != "na" {
		t.Errorf("test 2 should be n/a: %s", statusOfTest(rec, "2"))
	}
}

func TestMother(t *testing.T) {
	sc := setup(t)
	rec, _ := play(t, sc["t6_g_schedule_and_review"], []Result{
		{Text: "fear_greed runs daily at 09:00. I'm asking the Reviewer to check it for cost.", Calls: []Call{call("send_to_chat", `{"chat_id":"c_reviewer","message":"Please review the new fear_greed pipeline for cost and report back."}`)}},
		{Text: "Sent; I'll pass on the reply."},
		{Text: "The Reviewer says the llm.summarize step is not needed; dropping it saves about 1,200 tokens a day. The http.get step also needs on_error."},
	})
	dump(t, rec)
	if statusOfTest(rec, "6") != "pass" {
		t.Errorf("test 6: %s", statusOfTest(rec, "6"))
	}
	rec, _ = play(t, sc["t6_d_watch_gold_daily"], []Result{
		{Calls: []Call{call("create_chat", `{"title":"Gold watcher","role":"Watch the gold price daily and report big moves.","message":"Set up a daily gold price pipeline."}`)}},
		{Text: "Created a Gold watcher chat."},
		{Text: "Gold watcher is ready: gold_daily runs at 08:00."},
	})
	dump(t, rec)
	if statusOfTest(rec, "6") != "pass" {
		t.Errorf("test 6 (gold): %s", statusOfTest(rec, "6"))
	}
}

func TestOverlap(t *testing.T) {
	a := "A moving average is the mean of the last n values and smooths noise"
	if ov := overlap(a, a); ov != 1 {
		t.Errorf("self overlap %v", ov)
	}
	if ov := overlap("BTC closed at 64,210.50 USD today, up 1.8 percent.", a); ov != 0 {
		t.Errorf("overlap %v", ov)
	}
}
