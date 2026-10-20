package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// checked lists the numbers the check reads in s, as value/half; small
// ones are marked with "~".
func checked(s string) []string {
	var out []string
	for _, n := range numbers(s) {
		v := fmt.Sprintf("%g/%g", n.value, n.half)
		if n.small {
			v = "~" + v
		}
		out = append(out, v)
	}
	return out
}

func TestNumbers(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want []string
	}{
		{"BTC is at 61,240 USD", []string{"61240/5"}},
		{"61,240.00", []string{"61240/0.005"}},
		{"64,512", []string{"64512/0.5"}},
		{"61240", []string{"61240/5"}},
		{"about 61.2k", []string{"61200/50"}},
		{"62M USD and 410 million", []string{"6.2e+07/500000", "4.1e+08/5e+06"}},
		{"market cap 1.21T", []string{"1.21e+12/5e+09"}},
		{"$500 and €1,200.50", []string{"500/5", "1200.5/0.005"}},
		{"+1.2% in 24 hours", []string{"1.2/0.05", "~24/0.5"}},
		{"2026-09-23,62980.10\n09-24,63312.75", []string{"62980.1/0.005", "~9/0.5", "~24/0.5", "63312.75/0.005"}}, // CSV rows
		{"62980.10,63312.75 and 62,980.10,63,312.75", []string{"62980.1/0.005", "63312.75/0.005", "62980.1/0.005", "63312.75/0.005"}},
		{"64,000", []string{"64000/500"}},           // trailing zeros may be rounding
		{"60,000", []string{"60000/500"}},           // but two digits are kept
		{"64,500", []string{"64500/50"}},            //
		{"۶۱٬۲۴۰٫۵ دلار", []string{"61240.5/0.05"}}, // Persian digits and separators
		{"٦١٢٤٠", []string{"61240/5"}},              // Arabic-Indic
		{"۶۱,۲۴۰", []string{"61240/5"}},             // Persian digits with a Latin comma
		{"1,234,567,890 and 100,102,101,103,104", []string{"1.23456789e+09/5", "~100/5", "~102/0.5", "~101/0.5", "~103/0.5", "~104/0.5"}},
		{"$410 M in a day, 62 M USD, 3 Ms", []string{"4.1e+08/5e+06", "6.2e+07/500000", "~3/0.5"}},
		// A narrow or no-break space groups digits:
		{"62 980.10 | +1.08 %", []string{"62980.1/0.005", "1.08/0.005"}},
		{"1 234 567 and 7 days", []string{"1.234567e+06/0.5", "~7/0.5"}},
		// Left out or not checked:
		{"a 7-day average over 30 days, 4% of 200", []string{"~7/0.5", "~30/0.5", "~4/0.5", "~200/5"}},
		{"in 2026 and 1999", []string{"~2026/0.5", "~1999/0.5"}},
		{"on 2026-09-29 at 08:00", nil},
		{"on 29.09.2026, 9/29/26 and 2026-09", nil},
		{"تاریخ ۱۴۰۵/۰۷/۲۱", nil},
		{"in 1405 and ۱۴۰۵", []string{"~1405/0.5", "~1405/0.5"}}, // Solar Hijri years
		// After a letter, a number is part of a name; letters glued after it
		// are a unit, in any script, unless digits follow (a hash).
		{"v3, r_1, 0x1f, gpt4, 3a9c, قیمت۶۴۵۱۲, price64512", nil},
		{"24h, 3rd, 4x, 3600s", []string{"~24/0.5", "~3/0.5", "~4/0.5", "3600/50"}},
		{"64512USD and ۶۴٬۵۱۲دلار", []string{"64512/0.5", "64512/0.5"}},
		// Scale words, glued or after a space, in English and Persian.
		{"21 million, 21million, ۲۱ میلیون, ۲۱میلیون, ۳ هزار, ۲ میلیارد",
			[]string{"2.1e+07/500000", "2.1e+07/500000", "2.1e+07/500000", "2.1e+07/500000", "3000/500", "2e+09/5e+08"}},
		{"version 1.2.3", nil},
		{"1. first\n2) second\n10. tenth", []string{"~1/0.5", "~2/0.5", "~10/0.5"}},
		{"days 1,5,7", []string{"~1/0.5", "~5/0.5", "~7/0.5"}},
	} {
		if got := checked(tt.in); !slices.Equal(got, tt.want) {
			t.Errorf("%q: %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCheckNumbers(t *testing.T) {
	result := "BTC price today: 61,240 USD, +1.2% in 24 hours. Market cap 1.21T USD."
	rows := "date | close\n2026-09-26 | 65230.80\n2026-09-27 | 63912.40\n2026-09-28 | 64512.30"
	for _, tt := range []struct {
		name    string
		said    string
		saved   string
		sources []string
		want    []string
		checked int
	}{
		{"same number, other forms", "It is 61,240 USD, or 61.2k, or 61240.00, up 1.2%.", "", []string{result}, nil, 2},
		{"coarser source", "It is 61,240.", "", []string{"about 61.2k"}, nil, 1},
		{"rounded in the answer", "Yesterday it closed near 64,500, about 64.5k.", "", []string{rows}, nil, 1},
		{"made up", "Close: 58,234.12, 59,101.50 and 61,240.", "", []string{result}, []string{"58,234.12", "59,101.50"}, 3},
		{"repeats count once", "58,234.12 then 58,234.12 again", "", []string{result}, []string{"58,234.12"}, 1},
		// A rounding goes with the number it rounds; it never stands for it.
		{"a rounding doesn't hide a value", "About 64k; it closed at 63,870.40.", "", []string{"close: 64,100"}, []string{"63,870.40"}, 1},
		{"in either order", "It closed at 63,870.40, about 64k.", "", []string{"close: 64,100"}, []string{"63,870.40"}, 1},
		{"Persian scale word", "سقف عرضه ۲۱ میلیون است.", "", []string{"The cap is 21,000,000 BTC."}, nil, 1},
		{"each value once, however written", "Close 2.52k, or 2,520.50, or 2520.5.", "date,close\n2026-09-26,2520.50", nil, []string{"2,520.50"}, 1},
		// Computed values have no source: an average and a conversion.
		{"average of real rows", "Highest 65,230.80, lowest 63,912.40, average 64,551.83.", "", []string{rows}, []string{"64,551.83"}, 3},
		{"conversion to EUR", "0.35 BTC is 19,316.57 EUR at 55,190.20 EUR per BTC.", "", []string{`{"bitcoin":{"eur":55190.2}}`, "I hold 0.35 BTC"}, []string{"19,316.57"}, 3},
		{"the user's own numbers", "Saved: buy more under 60,000 USD.", "", []string{"Save a note: buy more if BTC drops under 60,000 USD."}, nil, 1},
		{"Persian answer, ASCII source", "قیمت بسته شدن دیروز ۶۴٬۵۱۲٫۳۰ دلار بود.", "", []string{rows}, nil, 1},
		{"ASCII answer, Persian source", "It closed at 64,512.30.", "", []string{"بسته شدن ۶۴٬۵۱۲٫۳۰"}, nil, 1},
		{"dates and small numbers", "Over the 7 days to 2026-09-28, 3 days rose.", "", []string{rows}, nil, 0},
		{"saved content", "Saved the table.", "date,close\n2026-09-27,63912.40\n2026-09-28,64512.30\n2026-09-29,64800.00", []string{rows}, []string{"64800.00"}, 3},
		// A percentage written the other way has a source.
		{"percent as a fraction", "Up 5.2% on the day, a change of 0.0310 the day before.", "", []string{"change: 0.052\nchange: 3.1 %"}, nil, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var saved []string
			if tt.saved != "" {
				saved = []string{tt.saved}
			}
			res := checkNumbers([]string{tt.said}, saved, nil, newSourceSet(tt.sources, nil), nil)
			var got []string
			for _, m := range res.missing {
				got = append(got, m.text)
			}
			if !slices.Equal(got, tt.want) || res.checked != tt.checked {
				t.Errorf("checked %d, unsourced %q; want %d, %q", res.checked, got, tt.checked, tt.want)
			}
			if tt.saved != "" && res.saved != len(tt.want) {
				t.Errorf("%d from saved, want %d", res.saved, len(tt.want))
			}
		})
	}
}

// The sources are what the model was given, without this turn's answers,
// the check's notice, or the answers of a turn whose check fired.
func TestSourceTexts(t *testing.T) {
	text := func(s string) chat.Part { return chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: s}} }
	notice := func(k chat.NoticeKind, s string) chat.Part {
		return chat.Part{Kind: chat.PartNotice, Notice: &chat.Notice{Kind: k, Text: s}}
	}
	req := provider.Request{
		System: []provider.Block{{Text: "Today is 2026-10-13."}},
		Tools:  []provider.ToolDef{{Name: "t", Description: "Returns up to 200 rows."}},
		Messages: []chat.Message{
			{Turn: 1, Role: chat.RoleUser, Parts: []chat.Part{text("u1")}},
			{Turn: 1, Role: chat.RoleAssistant, Parts: []chat.Part{text("made up 1")}},
			{Turn: 1, Role: chat.RoleUser, Parts: []chat.Part{notice(NoticeAppCheck, "[app check: 58.2]")}},
			{Turn: 2, Role: chat.RoleUser, Parts: []chat.Part{text("u2")}},
			{Turn: 2, Role: chat.RoleAssistant, Parts: []chat.Part{text("earlier 2"), {Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{ID: "c", Name: "t", Args: json.RawMessage(`{}`)}}}},
			{Turn: 2, Role: chat.RoleTool, Parts: []chat.Part{{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: "c", Text: "r2"}}}},
			{Turn: 3, Role: chat.RoleUser, Parts: []chat.Part{text("u3"), notice(chat.NoticeAnswerCut, "[cut]")}},
			{Turn: 3, Role: chat.RoleAssistant, Parts: []chat.Part{text("now")}},
		},
	}
	got, read := sourceTexts(req, 3, map[int]bool{1: true}, nil)
	want := []string{"Today is 2026-10-13.", "Returns up to 200 rows.", "u1", "u2", "earlier 2", "r2", "u3", "[cut]"}
	if !slices.Equal(got, want) || len(read) != 0 {
		t.Errorf("sources %q, read %q\nwant %q", got, read, want)
	}
	// The results of tools that read the project come apart.
	got, read = sourceTexts(req, 3, map[int]bool{1: true}, func(n string) bool { return n == "t" })
	if len(read) != 1 || read[0].text != "r2" || string(read[0].args) != "{}" || slices.Contains(got, "r2") {
		t.Errorf("sources %q, read %q", got, read)
	}
}

func TestCheckTexts(t *testing.T) {
	missing := []string{"58.2", "59.1", "60,400", "61,000.5"}
	if got, want := checkText(missing, false), "[app check: 4 numbers in your answer appear in no tool result or message, e.g. 58.2, 59.1, 60,400. If no tool gave them, say so or get them with a tool.]"; got != want {
		t.Errorf("notice %q", got)
	}
	if got := checkText(missing, true); !strings.Contains(got, "in your answer or in what you saved") || !strings.HasSuffix(got, "get them with a tool, and correct what you saved.]") {
		t.Errorf("notice %q", got)
	}
	if got, want := markText(missing), "[some numbers have no source: 58.2, 59.1, 60,400 and 1 more]"; got != want {
		t.Errorf("mark %q", got)
	}
}

// partial returns three of seven days of prices.
func partial() tool.Tool {
	return tool.Func(tool.Spec{Name: "prices", Description: "Daily closes.", Schema: json.RawMessage(`{"type": "object"}`)},
		func(context.Context, *tool.Env, struct{}) (tool.Result, error) {
			return tool.Result{Text: "2026-09-22: 62,410.50\n2026-09-25: 63,050.20\n2026-09-28: 64,512.30"}, nil
		})
}

// save is a write tool.
func save() tool.Tool {
	return tool.Func(tool.Spec{Name: "save", Description: "Save text.", Effects: tool.Bucket,
		Schema: json.RawMessage(`{"type": "object", "properties": {"content": {"type": "string"}}}`)},
		func(context.Context, *tool.Env, struct{ Content string }) (tool.Result, error) {
			return tool.Result{Text: "saved"}, nil
		})
}

const madeUp = "| day | close |\n|---|---|\n| 09-22 | 62,410.50 |\n| 09-23 | 62,980.10 |\n| 09-24 | 63,312.75 |\n| 09-25 | 63,050.20 |\n| 09-26 | 63,870.40 |\n| 09-27 | 64,101.90 |\n| 09-28 | 64,512.30 |"

func groundingHarness(t *testing.T, g Grounding) (*harness, chat.Chat) {
	h := newHarness(t, t.TempDir(), "", testSettings(), partial(), save())
	h.o.d.Grounding, h.o.d.Traces = g, NewTraces()
	return h, h.newChat(chat.Chat{Title: "Prices"})
}

func checksOf(h *harness) []GroundingCheck {
	var out []GroundingCheck
	for _, tt := range slices.Backward(h.o.d.Traces.List()) {
		out = append(out, tt.Checks...)
	}
	return out
}

func hasNotice(ms []chat.Message, k chat.NoticeKind) bool {
	return slices.ContainsFunc(ms, func(m chat.Message) bool {
		return slices.ContainsFunc(m.Parts, func(p chat.Part) bool { return p.Notice != nil && p.Notice.Kind == k })
	})
}

func TestGroundingOff(t *testing.T) {
	h, c := groundingHarness(t, Grounding{})
	defer h.stop()
	h.fp.Push(fake.ToolCall("c1", "prices", map[string]any{}), fake.Text(madeUp))
	h.turn(c.ID, "BTC closes for the last 7 days?")
	if n := len(checksOf(h)); n != 0 || hasNotice(h.messages(c.ID), NoticeUnsourced) {
		t.Errorf("%d checks with the check off", n)
	}
}

// Mark: the answer is marked, the model is never told, and no request is
// added (SPIKE-032).
func TestGroundingMark(t *testing.T) {
	h, c := groundingHarness(t, Grounding{Mode: GroundingMark, K: 3})
	defer h.stop()
	h.fp.Push(fake.ToolCall("c1", "prices", map[string]any{}), fake.Text(madeUp), fake.Text("ok"))
	h.turn(c.ID, "BTC closes for the last 7 days?")
	cs := checksOf(h)
	if len(cs) != 1 || !cs[0].Fired || cs[0].Told || cs[0].Checked != 7 ||
		!slices.Equal(cs[0].Unsourced, []string{"62,980.10", "63,312.75", "63,870.40", "64,101.90"}) {
		t.Fatalf("checks %+v", cs)
	}
	ms := h.messages(c.ID)
	if !hasNotice(ms, NoticeUnsourced) || hasNotice(ms, NoticeAppCheck) {
		t.Errorf("stored %q", texts(ms))
	}
	if n := len(h.fp.Calls()); n != 2 {
		t.Errorf("%d requests, want 2", n)
	}
	// The next turn's request doesn't carry the mark.
	h.turn(c.ID, "thanks")
	for _, s := range texts(h.fp.Calls()[2].Messages) {
		if strings.Contains(s, "no source") {
			t.Errorf("the model got the mark: %q", s)
		}
	}
	validHistory(t, h.messages(c.ID))
}

// Same turn: one more request with the notice; its answer is checked
// again, with no second notice.
func TestGroundingSameTurn(t *testing.T) {
	h, c := groundingHarness(t, Grounding{Mode: GroundingSameTurn, K: 3})
	defer h.stop()
	h.fp.Push(fake.ToolCall("c1", "prices", map[string]any{}), fake.Text(madeUp),
		fake.Text("Sorry: only 09-22 (62,410.50), 09-25 (63,050.20) and 09-28 (64,512.30) came from the tool. I made up 4 days: 62,980.10 and others."))
	h.turn(c.ID, "BTC closes for the last 7 days?")
	calls := h.fp.Calls()
	if len(calls) != 3 {
		t.Fatalf("%d requests, want 3", len(calls))
	}
	got := texts(calls[2].Messages)
	if last := got[len(got)-1]; !strings.HasPrefix(last, "user: [app check: 4 numbers in your answer appear in no tool result or message, e.g. 62,980.10, 63,312.75, 63,870.40.") {
		t.Errorf("the last request ends with %q", last)
	}
	if slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, "no source") }) {
		t.Error("the model got the mark")
	}
	cs := checksOf(h)
	if len(cs) != 2 || !cs[0].Fired || !cs[0].Told || cs[1].Told || !cs[1].After || len(cs[1].Unsourced) != 0 || cs[1].Quoted != 1 {
		t.Errorf("checks %+v", cs)
	}
	validHistory(t, h.messages(c.ID))
}

// Same turn at the request cap: the notice still gets its one request,
// without tools.
func TestGroundingSameTurnAtCap(t *testing.T) {
	set := testSettings()
	set.LLM.TurnMaxRequests = 2
	h := newHarness(t, t.TempDir(), "", set, partial())
	h.o.d.Grounding = Grounding{Mode: GroundingSameTurn, K: 3}
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Prices"})
	h.fp.Push(fake.ToolCall("c1", "prices", map[string]any{}), fake.Text(madeUp), fake.Text("Only three days came from the tool."))
	h.turn(c.ID, "BTC closes for the last 7 days?")
	calls := h.fp.Calls()
	if len(calls) != 3 || calls[2].ToolChoice != provider.ToolsNone {
		t.Fatalf("%d requests; want 3, the last without tools", len(calls))
	}
}

// Next turn: the notice is stored after the answer and comes before the
// next message, with no request added.
func TestGroundingNextTurn(t *testing.T) {
	h, c := groundingHarness(t, Grounding{Mode: GroundingNextTurn, K: 3})
	defer h.stop()
	h.fp.Push(fake.ToolCall("c1", "prices", map[string]any{}), fake.Text(madeUp), fake.Text("Only three days came from the tool."))
	h.turn(c.ID, "BTC closes for the last 7 days?")
	if n := len(h.fp.Calls()); n != 2 {
		t.Fatalf("%d requests, want 2", n)
	}
	h.turn(c.ID, "and save them")
	got := texts(h.fp.Calls()[2].Messages)
	n := len(got)
	if !strings.HasPrefix(got[n-2], "user: [app check: 4 numbers") || got[n-1] != "user: and save them" {
		t.Errorf("the next request ends with %q", got[n-2:])
	}
	cs := checksOf(h)
	if len(cs) != 2 || !cs[0].Told || cs[1].Fired {
		t.Errorf("checks %+v", cs)
	}
}

// What a write tool saves is checked like the answer, and the numbers of
// a turn whose check fired are no source for the next turn.
func TestGroundingSaved(t *testing.T) {
	h, c := groundingHarness(t, Grounding{Mode: GroundingMark, K: 3})
	defer h.stop()
	h.fp.Push(fake.ToolCall("c1", "prices", map[string]any{}), fake.Text(madeUp),
		fake.ToolCall("c2", "save", map[string]string{"content": "date,close\n2026-09-23,62980.10\n2026-09-24,63312.75\n2026-09-26,63870.40"}), fake.Text("Saved."))
	h.turn(c.ID, "BTC closes for the last 7 days?")
	h.turn(c.ID, "save them")
	cs := checksOf(h)
	if len(cs) != 2 || !cs[1].Fired || cs[1].Saved != 3 {
		t.Errorf("checks %+v", cs)
	}
}

// An honest answer with one computed value stays under K.
func TestGroundingHonest(t *testing.T) {
	h, c := groundingHarness(t, Grounding{Mode: GroundingSameTurn, K: 3})
	defer h.stop()
	h.fp.Push(fake.ToolCall("c1", "prices", map[string]any{}),
		fake.Text("The tool has 3 of the 7 days: 62,410.50, 63,050.20 and 64,512.30; their average is 63,324.33. The other 4 days are missing."))
	h.turn(c.ID, "BTC closes for the last 7 days?")
	cs := checksOf(h)
	if len(cs) != 1 || cs[0].Fired || len(cs[0].Unsourced) != 1 || len(h.fp.Calls()) != 2 {
		t.Errorf("checks %+v, %d requests", cs, len(h.fp.Calls()))
	}
	if hasNotice(h.messages(c.ID), NoticeUnsourced) {
		t.Error("an honest answer is marked")
	}
}

// The check on a long answer and a long window, for SPIKE-032's timing.
// The check at the end of a turn: a 40-row answer in a window of about
// 24,000 tokens, and a 300-row answer in one of about 250,000 (the long
// windows of 3.1).
func BenchmarkCheckNumbers(b *testing.B) {
	for _, c := range []struct {
		name         string
		answer, rows int
	}{{"typical", 40, 2000}, {"long", 300, 20000}} {
		var answer, window strings.Builder
		for i := range c.answer {
			fmt.Fprintf(&answer, "| 2026-%02d-%02d | %.2f | %.2f |\n", i%12+1, i%28+1, 60000+float64(i)*13.37, 1.5+math.Mod(float64(i), 7))
		}
		for i := range c.rows {
			fmt.Fprintf(&window, "row %d: %.2f USD, volume %d\n", i, 50000+float64(i)*0.77, 1000+i*3)
		}
		b.Run(c.name, func(b *testing.B) {
			b.SetBytes(int64(answer.Len() + window.Len()))
			for b.Loop() {
				checkNumbers([]string{answer.String()}, nil, nil, newSourceSet([]string{window.String()}, nil), nil)
			}
		})
	}
}

// keyStore is a write tool with a key, and fetch reads its keys back, as
// bucket_put and bucket_read do.
func keyStore() (tool.Tool, tool.Tool) {
	kept := map[string]string{}
	put := tool.Func(tool.Spec{Name: "put", Description: "Save text under a key.", Effects: tool.Bucket,
		Schema: json.RawMessage(`{"type": "object", "properties": {"key": {"type": "string"}, "content": {"type": "string"}}}`)},
		func(_ context.Context, _ *tool.Env, a struct{ Key, Content string }) (tool.Result, error) {
			kept[a.Key] = a.Content
			return tool.Result{Text: "stored " + a.Key}, nil
		})
	get := tool.Func(tool.Spec{Name: "get", Description: "Read a key.", Effects: tool.ReadsDB,
		Schema: json.RawMessage(`{"type": "object", "properties": {"key": {"type": "string"}}}`)},
		func(_ context.Context, _ *tool.Env, a struct{ Key string }) (tool.Result, error) {
			return tool.Result{Text: kept[a.Key]}, nil
		})
	return put, get
}

const madeCSV = "date,close\n2026-09-22,62410.50\n2026-09-23,62980.10\n2026-09-24,63312.75\n2026-09-26,63870.40"

func storeHarness(t *testing.T, g Grounding) (*harness, chat.Chat) {
	put, get := keyStore()
	h := newHarness(t, t.TempDir(), "", testSettings(), partial(), put, get)
	h.o.d.Grounding, h.o.d.Traces = g, NewTraces()
	return h, h.newChat(chat.Chat{Title: "Prices"})
}

// Numbers saved without a source are no source when a tool reads them
// back, in the same turn or a later one (round 1's G9 misses).
func TestGroundingReadBack(t *testing.T) {
	h, c := storeHarness(t, Grounding{Mode: GroundingMark, K: 3})
	defer h.stop()
	h.fp.Push(fake.ToolCall("c1", "prices", map[string]any{}),
		fake.ToolCall("c2", "put", map[string]string{"key": "w.csv", "content": madeCSV}),
		fake.ToolCall("c3", "get", map[string]string{"key": "w.csv"}),
		fake.Text("Saved w.csv: 62,980.10, 63,312.75 and 63,870.40 on the missing days."),
		fake.ToolCall("c4", "get", map[string]string{"key": "w.csv"}),
		fake.Text("Chart of 62,410.50, 62,980.10, 63,312.75, 63,870.40."))
	h.turn(c.ID, "Save the week as w.csv")
	h.turn(c.ID, "Chart w.csv")
	cs := checksOf(h)
	if len(cs) != 2 || !cs[0].Fired || cs[0].Saved != 3 || !cs[1].Fired || len(cs[1].Unsourced) != 3 {
		t.Errorf("checks %+v", cs)
	}
}

// After the notice, what was saved before it is checked again unless it
// was saved again: an admission in text leaves the made-up file marked.
func TestGroundingSavedLeft(t *testing.T) {
	for _, tt := range []struct {
		name  string
		fix   []fake.Reply
		left  int
		fired bool
	}{
		{"admitted in text only", []fake.Reply{fake.Text("Sorry, I made up 62,980.10 and two more.")}, 3, true},
		{"saved again", []fake.Reply{
			fake.ToolCall("c3", "put", map[string]string{"key": "w.csv", "content": "date,close\n2026-09-22,62410.50\n2026-09-23,unknown"}),
			fake.Text("Fixed: only the found close is in w.csv.")}, 0, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h, c := storeHarness(t, Grounding{Mode: GroundingSameTurn, K: 3})
			defer h.stop()
			h.fp.Push(append([]fake.Reply{fake.ToolCall("c1", "prices", map[string]any{}),
				fake.ToolCall("c2", "put", map[string]string{"key": "w.csv", "content": madeCSV}),
				fake.Text("Saved w.csv.")}, tt.fix...)...)
			h.turn(c.ID, "Save the week as w.csv")
			cs := checksOf(h)
			if len(cs) != 2 || !cs[0].Told || cs[0].Saved != 3 {
				t.Fatalf("checks %+v", cs)
			}
			if a := cs[1]; !a.After || a.SavedLeft != tt.left || a.Fired != tt.fired || a.Quoted != 1 && tt.left > 0 {
				t.Errorf("after the notice %+v", a)
			}
			got := texts(h.fp.Calls()[3].Messages)
			if last := got[len(got)-1]; !strings.HasSuffix(last, "get them with a tool, and correct what you saved.]") {
				t.Errorf("notice %q", last)
			}
		})
	}
}

// Two thresholds: the mark at K, the notice at TellK, or at any number
// without a source in what was saved.
func TestGroundingTellK(t *testing.T) {
	for _, tt := range []struct {
		name string
		g    Grounding
		save bool
		told bool
	}{
		{"marked, not told", Grounding{Mode: GroundingSameTurn, K: 3, TellK: 8}, false, false},
		{"told at TellK", Grounding{Mode: GroundingSameTurn, K: 3, TellK: 4}, false, true},
		{"told for what was saved", Grounding{Mode: GroundingSameTurn, K: 3, TellK: 8, TellSaved: true}, true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h, c := storeHarness(t, tt.g)
			defer h.stop()
			rs := []fake.Reply{fake.ToolCall("c1", "prices", map[string]any{})}
			if tt.save {
				rs = append(rs, fake.ToolCall("c2", "put", map[string]string{"key": "w.csv", "content": "2026-09-23,62980.10"}))
			}
			h.fp.Push(append(rs, fake.Text(madeUp), fake.Text("Only three days came from the tool."))...)
			h.turn(c.ID, "BTC closes for the last 7 days?")
			cs := checksOf(h)
			if len(cs) == 0 || !cs[0].Fired || cs[0].Told != tt.told || hasNotice(h.messages(c.ID), NoticeAppCheck) != tt.told {
				t.Errorf("checks %+v", cs)
			}
		})
	}
}

// Compute adds block 1's line only with a query tool.
func TestGroundingCompute(t *testing.T) {
	query := tool.Func(tool.Spec{Name: "query", Description: "SQL.", Schema: json.RawMessage(`{"type": "object"}`)},
		func(context.Context, *tool.Env, struct{}) (tool.Result, error) { return tool.Result{Text: "1"}, nil })
	for _, tt := range []struct {
		name  string
		tools []tool.Tool
		g     Grounding
		want  bool
	}{
		{"on", []tool.Tool{query}, Grounding{Mode: GroundingMark, Compute: true}, true},
		{"off", []tool.Tool{query}, Grounding{Mode: GroundingMark}, false},
		{"no query", []tool.Tool{partial()}, Grounding{Mode: GroundingMark, Compute: true}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, t.TempDir(), "", testSettings(), tt.tools...)
			h.o.d.Grounding = tt.g
			defer h.stop()
			c := h.newChat(chat.Chat{Title: "x"})
			h.fp.Push(fake.Text("ok"))
			h.turn(c.ID, "hi")
			if got := strings.Contains(h.fp.Calls()[0].System[0].Text, computeLine); got != tt.want {
				t.Errorf("compute line %v, want %v", got, tt.want)
			}
		})
	}
}

// A percentage read the other way is counted in pct.
func TestCheckNumbersPct(t *testing.T) {
	res := checkNumbers([]string{"Up 5.2% on the day, a change of 0.0310 the day before, and 64,512.30."}, nil, nil,
		newSourceSet([]string{"change: 0.052\nchange: 3.1 %\nclose: 64512.30"}, nil), nil)
	if res.checked != 3 || res.pct != 2 || len(res.missing) != 0 {
		t.Errorf("%+v", res)
	}
}

// A read is tied to a write by its key or ref, or its table; a query only
// when it selects the table's columns as they are.
func TestTainted(t *testing.T) {
	n := numbers("74222.23")[0]
	ts := []taint{{n: n, key: "alert.yaml"}, {n: n, table: "prices"}}
	for _, tt := range []struct {
		args string
		want int
	}{
		{`{"key": "alert.yaml"}`, 1},
		{`{"ref": "alert.yaml"}`, 1},
		{`{"key": "other.yaml"}`, 0},
		{`{"table": "Prices"}`, 1},
		{`{"sql": "SELECT date, close FROM prices WHERE coin = 'BTC'"}`, 1},
		{`{"sql": "select * from \"prices\" limit 5"}`, 1},
		{`{"sql": "SELECT close * 1.02 AS upper FROM prices"}`, 0},
		{`{"sql": "SELECT AVG(close) FROM prices"}`, 0},
		{`{"sql": "SELECT close FROM fx_rates JOIN prices USING (date)"}`, 0},
		{`{}`, 0},
	} {
		if got := tainted(ts, json.RawMessage(tt.args)); len(got) != tt.want {
			t.Errorf("%s: %d tainted, want %d", tt.args, len(got), tt.want)
		}
	}
	// A taint drops only the same number, written as finely: not a
	// computed 74222.2278, nor 69,800.00 for a saved 70,000.
	src := newSourceSet(nil, nil)
	src.add([]string{"74222.23 | 74222.2278 | 69,800.00"}, []number{n, numbers("70,000")[0]})
	src.sort()
	if len(src.fine) != 2 || !src.match(n) { // the computed one sources a rounded 74222.23
		t.Errorf("sources %+v", src.fine)
	}
}

// A computed query of a table the model wrote to sources its result
// (gemma's R7 r3 in round 2: close * 1.02 after a made-up 74222.23).
func TestGroundingComputedReadBack(t *testing.T) {
	put, _ := keyStore()
	query := tool.Func(tool.Spec{Name: "query", Description: "SQL.", Effects: tool.ReadsDB,
		Schema: json.RawMessage(`{"type": "object", "properties": {"sql": {"type": "string"}}}`)},
		func(context.Context, *tool.Env, struct{ SQL string }) (tool.Result, error) {
			return tool.Result{Text: "upper | lower\n74222.22780000001 | 71311.55219999999"}, nil
		})
	h := newHarness(t, t.TempDir(), "", testSettings(), put, query)
	h.o.d.Grounding, h.o.d.Traces = Grounding{Mode: GroundingMark, K: 3}, NewTraces()
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Alert"})
	h.fp.Push(fake.ToolCall("c1", "put", map[string]string{"key": "alert.yaml", "content": "upper: 74222.23\nlower: 71311.55\ncooldown: 3600"}),
		fake.ToolCall("c2", "query", map[string]string{"sql": "SELECT close * 1.02 AS upper, close * 0.98 AS lower FROM prices"}),
		fake.Text("Saved; the thresholds are 74,222.23 and 71,311.55."))
	h.turn(c.ID, "Save an alert config")
	cs := checksOf(h)
	if len(cs) != 1 || !slices.Equal(cs[0].Unsourced, []string{"3600"}) {
		t.Errorf("checks %+v", cs)
	}
}

// The notice's numbers are exempt only in the answer right after it, in
// the same turn, and only written the same.
func TestGroundingQuoted(t *testing.T) {
	t.Run("next turn", func(t *testing.T) {
		h, c := groundingHarness(t, Grounding{Mode: GroundingNextTurn, K: 3})
		defer h.stop()
		h.fp.Push(fake.ToolCall("c1", "prices", map[string]any{}), fake.Text(madeUp), fake.Text(madeUp))
		h.turn(c.ID, "BTC closes for the last 7 days?")
		h.turn(c.ID, "again, as a table")
		cs := checksOf(h)
		if len(cs) != 2 || !cs[0].Told || !cs[1].Fired || cs[1].Quoted != 0 || len(cs[1].Unsourced) != 4 {
			t.Errorf("checks %+v", cs)
		}
	})
	t.Run("same turn, written another way", func(t *testing.T) {
		h, c := groundingHarness(t, Grounding{Mode: GroundingSameTurn, K: 3})
		defer h.stop()
		h.fp.Push(fake.ToolCall("c1", "prices", map[string]any{}), fake.Text(madeUp),
			fake.Text("I made up 62,980.10, and 63,312.8 and 63,870 are rounded guesses too."))
		h.turn(c.ID, "BTC closes for the last 7 days?")
		cs := checksOf(h)
		if len(cs) != 2 || cs[1].Quoted != 1 || len(cs[1].Unsourced) != 2 {
			t.Errorf("checks %+v", cs)
		}
	})
}

// A tool that fetches from the network saves what it fetched: the numbers
// in its arguments (a URL) aren't saved numbers.
func TestGroundingNetworkTool(t *testing.T) {
	fetch := tool.Func(tool.Spec{Name: "fetch", Description: "Fetch a page.", Effects: tool.Network | tool.Bucket,
		Schema: json.RawMessage(`{"type": "object", "properties": {"url": {"type": "string"}}}`)},
		func(context.Context, *tool.Env, struct{ URL string }) (tool.Result, error) {
			return tool.Result{Text: "a page"}, nil
		})
	h := newHarness(t, t.TempDir(), "", testSettings(), fetch)
	h.o.d.Grounding, h.o.d.Traces = Grounding{Mode: GroundingMark, K: 1}, NewTraces()
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Page"})
	h.fp.Push(fake.ToolCall("c1", "fetch", map[string]string{"url": "https://example.com/item/48213/page/77120?id=99881"}), fake.Text("Fetched."))
	h.turn(c.ID, "Fetch it")
	if cs := checksOf(h); len(cs) != 1 || cs[0].Checked != 0 || cs[0].Fired {
		t.Errorf("checks %+v", cs)
	}
}
