package dev

import (
	"slices"
	"testing"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/id"
)

// Ported from the React app's dev/format.test.ts.

func TestTimesAndSizesReadShort(t *testing.T) {
	got := []string{Duration(38), Duration(1240), Duration(4000), Duration(184_000), Duration(120_000)}
	if want := []string{"38 ms", "1.2 s", "4 s", "3 min 4 s", "2 min"}; !slices.Equal(got, want) {
		t.Errorf("durations %q, want %q", got, want)
	}
	got = []string{Size(64), Size(200), Size(20_480), Size(1_500_000)}
	if want := []string{"64 B", "0.2 KB", "20.5 KB", "1.5 MB"}; !slices.Equal(got, want) {
		t.Errorf("sizes %q, want %q", got, want)
	}
	if got := Share(6120, 7412); got != "6,120 cached (83 %)" {
		t.Errorf("share %q", got)
	}
	if got := Share(0, 0); got != "" {
		t.Errorf("share of nothing %q", got)
	}
	if got := []string{Num(0), Num(999), Num(7412), Num(1_234_567), Num(-4200)}; !slices.Equal(got, []string{"0", "999", "7,412", "1,234,567", "-4,200"}) {
		t.Errorf("numbers %q", got)
	}
	if got := Duration(-5); got != "0 ms" {
		t.Errorf("a negative span %q", got)
	}
}

func TestTheTimelineHasAtMostSixRoundTicks(t *testing.T) {
	s := NewScale(3600)
	if s.Span != 4000 || s.Step != 1000 || !slices.Equal(s.Ticks, []int64{0, 1000, 2000, 3000, 4000}) {
		t.Errorf("scale(3600) = %+v", s)
	}
	if got := NewScale(0).Span; got != 1 {
		t.Errorf("scale(0) spans %d", got)
	}
	s = NewScale(130)
	if s.Step != 50 || s.Span != 150 || Tick(s.Ticks[1], s.Step) != "50 ms" {
		t.Errorf("scale(130) = %+v, tick %q", s, Tick(s.Ticks[1], s.Step))
	}
	if got := Tick(1500, 500); got != "1500 ms" {
		t.Errorf("tick %q", got)
	}
	if got := Tick(2500, 2500); got != "2.5 s" {
		t.Errorf("tick %q", got)
	}
	for _, total := range []int64{1, 7, 99, 1234, 45_000, 600_001} {
		s := NewScale(total)
		if s.Span < total || len(s.Ticks) > 6 {
			t.Errorf("scale(%d) = %+v", total, s)
		}
	}
}

func TestEachChatIsPickedOnceNewestFirst(t *testing.T) {
	turn := func(p id.Project, c id.Chat, n int) app.TurnItem {
		title := ""
		if c == "c1" {
			title = "Mother"
		}
		return app.TurnItem{Project: p, Chat: c, Turn: n, ProjectName: "Prices", ChatTitle: title}
	}
	var labels []string
	for _, o := range ChatOptions([]app.TurnItem{turn("p", "c2", 3), turn("p", "c1", 2), turn("p", "c2", 1)}) {
		labels = append(labels, o.Label)
	}
	if want := []string{"Prices · Untitled chat", "Prices · Mother"}; !slices.Equal(labels, want) {
		t.Errorf("labels %q, want %q", labels, want)
	}
	if got := ChatOptions([]app.TurnItem{{Project: "q", Chat: "c"}})[0].Label; got != "Closed project · Untitled chat" {
		t.Errorf("a closed project's chat %q", got)
	}
	if got := Parts([]string{"thinking", "text", "tool_call", "tool_call"}); got != "thinking · answer · tool call" {
		t.Errorf("parts %q", got)
	}
	if got := Parts([]string{"web_result"}); got != "web result" {
		t.Errorf("an unknown part %q", got)
	}
}

func TestTheInspectorAndRuntimeLines(t *testing.T) {
	item := app.TurnItem{Turn: 3, Requests: 2}
	if got := TurnLabel("14:05", item); got != "14:05 · turn 3 · 2 requests" {
		t.Errorf("turn label %q", got)
	}
	item.Requests = 1
	if got := TurnLabel("14:05", item); got != "14:05 · turn 3 · 1 request" {
		t.Errorf("turn label %q", got)
	}
	item.Running = true
	if got := TurnLabel("14:05", item); got != "14:05 · turn 3 · running" {
		t.Errorf("turn label %q", got)
	}
	r := app.RequestView{}
	if got := Counted(r, Prompt(r)); got != "–" {
		t.Errorf("no counts yet %q", got)
	}
	r.Usage.Input, r.Usage.CacheRead, r.Usage.CacheWrite, r.Usage.Output = 1292, 6120, 0, 412
	if got := Counted(r, Prompt(r)); got != "7,412" {
		t.Errorf("prompt %q", got)
	}
	p := app.ProviderStatus{Limit: 4, Current: 4}
	if ProviderState(p) != "OK" || ProviderLimit(p) != "4" {
		t.Errorf("a calm provider: %q, %q", ProviderState(p), ProviderLimit(p))
	}
	p.Current, p.PausedMS, p.LastProblem = 2, 30_000, "Rate limited"
	if ProviderState(p) != "Paused for 30 s · Rate limited" || ProviderLimit(p) != "2 of 4" {
		t.Errorf("a paused provider: %q, %q", ProviderState(p), ProviderLimit(p))
	}
	if StuckTitle(1) != "1 thing hasn't moved for longer than its limit" || StuckTitle(2) != "2 things haven't moved for longer than their limit" {
		t.Error(StuckTitle(1), StuckTitle(2))
	}
	if ProjectLine(0, 1) != "Nothing running. Held open by 1 lease." || ProjectLine(2, 3) != "2 running. Held open by 3 leases." {
		t.Error(ProjectLine(0, 1), ProjectLine(2, 3))
	}
}
