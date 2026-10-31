package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
)

func TestCardWords(t *testing.T) {
	many := make([]string, 40)
	for i := range many {
		many[i] = fmt.Sprintf("w%d", i)
	}
	for _, tc := range []struct {
		text string
		want []string
	}{
		{"Make that 14-day average chart, for ETH too! make", []string{"make", "that", "14", "day", "average", "chart", "for", "eth", "too"}},
		{"همین جدول اجاره‌ی ۲۰ روزه", []string{"همین", "جدول", "اجارهی", "20", "روزه"}},
		{"قيمت كتاب", []string{"قیمت", "کتاب"}},
		{"a I x", nil},
		{strings.Join(many, " "), many[:32]},
	} {
		if got := cardWords(tc.text); !slices.Equal(got, tc.want) {
			t.Errorf("cardWords(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

func TestPickCards(t *testing.T) {
	for _, tc := range []struct {
		name   string
		scores []CardScore
		want   []int
	}{
		{"nothing matches", nil, nil},
		{"one", []CardScore{{Turn: 4, Score: 9, Words: 3}}, []int{4}},
		{"one word only", []CardScore{{Turn: 4, Score: 9, Words: 1}}, nil},
		{"too low", []CardScore{{Turn: 4, Score: 4.9, Words: 3}}, nil},
		{"a window turn matches better", []CardScore{{Turn: 4, Score: 9, Words: 3}, {Turn: 21, Score: 9.5, Words: 3}}, nil},
		{"a window turn matches as well", []CardScore{{Turn: 4, Score: 9, Words: 3}, {Turn: 21, Score: 9, Words: 3}}, nil},
		{"within the margin", []CardScore{{Turn: 4, Score: 9, Words: 3}, {Turn: 21, Score: 7.2, Words: 3}}, nil},
		{"past the margin", []CardScore{{Turn: 4, Score: 9.1, Words: 3}, {Turn: 21, Score: 7.2, Words: 3}}, []int{4}},
		{"a window turn with one word", []CardScore{{Turn: 4, Score: 9, Words: 3}, {Turn: 21, Score: 20, Words: 1}}, []int{4}},
		{"a second at half", []CardScore{{Turn: 9, Score: 5, Words: 2}, {Turn: 4, Score: 10, Words: 3}}, []int{4, 9}},
		{"a second below half", []CardScore{{Turn: 9, Score: 5.9, Words: 2}, {Turn: 4, Score: 12, Words: 3}}, []int{4}},
		{"at most two, the newer on a tie", []CardScore{{Turn: 2, Score: 8, Words: 2}, {Turn: 4, Score: 8, Words: 2}, {Turn: 6, Score: 8, Words: 2}}, []int{4, 6}},
	} {
		if got := PickCards(tc.scores, 20); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestHistoryCardCap: a card keeps to its room, clips each piece with
// "…", leaves out the history tools' calls, and says when calls are left
// out.
func TestHistoryCardCap(t *testing.T) {
	long := func(c string, n int) string { return strings.Repeat(c, n) }
	call := func(id, name, arg string) chat.Part {
		raw, _ := json.Marshal(map[string]string{"yaml": arg})
		return chat.Part{Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{ID: id, Name: name, Args: raw}}
	}
	ms := []chat.Message{
		{Turn: 4, Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: long("ب", 1500)}}}},
		{Turn: 4, Role: chat.RoleAssistant, Parts: []chat.Part{call("1", "search_history", "x"), call("2", "save_view", long("y", 2500)), call("3", "save_view", long("z", 2500)), call("4", "query", long("q", 2500))}},
		{Turn: 4, Role: chat.RoleAssistant, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: long("a", 3000)}}}},
	}
	first := historyCard("01CHAT", 4, ms, cardClip)
	if len(first) > cardClip || !utf8.ValidString(first) || !strings.HasPrefix(first, "[Turn 4 of this chat") || !strings.HasSuffix(first, "…]") {
		t.Fatalf("card of %d bytes:\n%.300s…%s", len(first), first, first[max(0, len(first)-100):])
	}
	if strings.Contains(first, "search_history") || !strings.Contains(first, "Call save_view") || !strings.Contains(first, "(more calls: read_messages has them)") {
		t.Errorf("calls in the card:\n%s", first)
	}
	user := first[strings.Index(first, "User: ")+len("User: "):]
	user = user[:strings.Index(user, "\n")]
	if len(user) > cardTextClip || !strings.HasSuffix(user, "…") || !strings.HasPrefix(user, "ب") {
		t.Errorf("user text of %d bytes: %.40s…", len(user), user)
	}
	room := min(cardClip, cardsClip-len(first)-2)
	second := historyCard("01CHAT", 9, ms, room)
	if len(second) > room || len(first)+2+len(second) > cardsClip || !strings.HasSuffix(second, "]") || !utf8.ValidString(second) {
		t.Errorf("second card of %d bytes in a room of %d", len(second), room)
	}
	short := historyCard("01CHAT", 5, []chat.Message{
		{Turn: 5, Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: "Make it."}}}},
		{Turn: 5, Role: chat.RoleAssistant, Parts: []chat.Part{call("5", "save_view", "v")}},
		{Turn: 5, Role: chat.RoleAssistant, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: "Saved."}}}},
	}, cardClip)
	want := "[Turn 5 of this chat, before the recent turns. read_messages(chat_id: 01CHAT, from: 5) has its tool results.\nUser: Make it.\nCall save_view {\"yaml\":\"v\"}\nAnswer: Saved.]"
	if short != want {
		t.Errorf("card:\n%s\nwant\n%s", short, want)
	}
	// Each piece is cut at a character, odd room or not, and fits even a
	// room too small for "…".
	for n := range len("بببب") {
		got := clipBytes(strings.Repeat("ب", 4), n)
		if len(got) > n || !utf8.ValidString(got) || (n >= len("…") && !strings.HasSuffix(got, "…")) {
			t.Errorf("clipBytes(n=%d) = %q", n, got)
		}
	}
}

// oldTurn is one earlier turn written into a chat: the user's text, a
// call with its result if tool is set, history cards' mark of outside data
// if mark is set, and the answer.
type oldTurn struct {
	user, tool, result, answer string
	args                       map[string]any
	mark                       bool
}

// filler is an earlier turn whose words every filler turn has.
func filler(n int) oldTurn {
	return oldTurn{user: fmt.Sprintf("Turn %d: what is the colour of item %d?", n, n), answer: fmt.Sprintf("Item %d is blue.", n)}
}

// longChat is 22 earlier turns: fillers, and the turns given.
func longChat(own map[int]oldTurn) []oldTurn {
	out := make([]oldTurn, 22)
	for i := range out {
		if o, ok := own[i+1]; ok {
			out[i] = o
		} else {
			out[i] = filler(i + 1)
		}
	}
	return out
}

func seedChat(h *harness, c id.Chat, turns []oldTurn) {
	h.t.Helper()
	text := func(s string) []chat.Part { return []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: s}}} }
	h.open(func(p *project.Project) {
		for i, o := range turns {
			n := i + 1
			ms := []chat.Message{{Chat: c, Turn: n, Role: chat.RoleUser, Parts: text(o.user)}}
			if o.mark {
				ms = append(ms, chat.Message{Chat: c, Turn: n, Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartNotice,
					Notice: &chat.Notice{Kind: chat.NoticeCardOutside, Text: fmt.Sprintf("[turn %d's history cards show turns that read outside data]", n)}}}})
			}
			if o.tool != "" {
				raw, err := json.Marshal(o.args)
				if err != nil {
					h.t.Fatal(err)
				}
				cid := fmt.Sprintf("old-%d", n)
				ms = append(ms,
					chat.Message{Chat: c, Turn: n, Role: chat.RoleAssistant, Parts: []chat.Part{{Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{ID: cid, Name: o.tool, Args: raw}}}},
					chat.Message{Chat: c, Turn: n, Role: chat.RoleTool, Parts: []chat.Part{{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: cid, Text: o.result}}}})
			}
			ms = append(ms, chat.Message{Chat: c, Turn: n, Role: chat.RoleAssistant, Parts: text(o.answer)})
			for _, m := range ms {
				if _, _, err := p.Chats.AppendMessage(context.Background(), m); err != nil {
					h.t.Fatal(err)
				}
			}
		}
	})
}

var volTurn = oldTurn{
	user: "Build a weekly volatility table for BTC over the last 26 weeks.",
	tool: "echo", args: map[string]any{"text": "btc_weekly_vol"}, result: "saved view (revision 1).",
	answer: "Saved btc_weekly_vol, the weekly volatility of BTC.",
}

// cardsIn is the cards in a request: the text parts that start one.
func cardsIn(msgs []chat.Message) []string {
	var out []string
	for _, m := range msgs {
		for _, p := range m.Parts {
			if p.Text != nil && strings.HasPrefix(p.Text.Text, "[Turn ") {
				out = append(out, fmt.Sprintf("%d: %s", m.Turn, p.Text.Text))
			}
		}
	}
	return out
}

// TestHistoryCards: after a cut, the message that names an older turn
// gets it as a card, which points to the turn with the chat's id. The turn's later requests and the next turn send the
// same bytes, so requests stay append-only, and the store is not changed.
func TestHistoryCards(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings(), echo())
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "Coins"})
	seedChat(h, c.ID, longChat(map[int]oldTurn{4: volTurn}))
	h.fp.Push(fake.ToolCall("c1", "echo", map[string]any{"text": "sol"}), fake.Text("Saved it for SOL."), fake.Text("You're welcome."))
	h.turn(c.ID, "Make that weekly volatility table for SOL too.")
	h.turn(c.ID, "Thanks!")
	calls := h.fp.Calls()
	if len(calls) != 3 {
		t.Fatalf("%d requests, want 3", len(calls))
	}
	cards := cardsIn(calls[0].Messages)
	if len(cards) != 1 || !strings.HasPrefix(cards[0], "23: [Turn 4 of this chat") || !strings.Contains(cards[0], "Call echo") || !strings.Contains(cards[0], "Answer: Saved btc_weekly_vol") {
		t.Fatalf("cards: %q", cards)
	}
	// The card points to the turn with read_messages' exact arguments, so
	// no model has to guess the chat's id.
	if ptr := "read_messages(chat_id: " + string(c.ID) + ", from: 4) has its tool results."; !strings.Contains(cards[0], ptr) {
		t.Errorf("card without %q: %q", ptr, cards[0])
	}
	for i := 1; i < len(calls); i++ {
		prev, got := texts(calls[i-1].Messages), texts(calls[i].Messages)
		if len(got) < len(prev) || !slices.Equal(got[:len(prev)], prev) {
			t.Errorf("request %d is not request %d with more at the end:\n%q\n%q", i+1, i, prev, got)
		}
		if c := cardsIn(calls[i].Messages); !slices.Equal(c, cards) {
			t.Errorf("request %d cards: %q", i+1, c)
		}
	}
	for _, s := range texts(h.messages(c.ID)) {
		if strings.Contains(s, "[Turn 4") {
			t.Errorf("a card was stored: %q", s)
		}
	}
	// A cut that drops turn 23 drops its card too.
	cs := h.o.find(h.pid, c.ID)
	cs.win.start = 24
	cs.win.dropCards()
	if _, ok := cs.win.cards[23]; ok {
		t.Error("turn 23's card outlived the cut")
	}
}

// TestHistoryCardsNone: no card when nothing matches, when a turn in the
// window matches better, in the first window, and with NoHistoryCards.
func TestHistoryCardsNone(t *testing.T) {
	for _, tc := range []struct {
		name  string
		turns []oldTurn
		text  string
		off   bool
	}{
		{"nothing matches", longChat(map[int]oldTurn{4: volTurn}), "Which planets have rings?", false},
		{"a window turn matches better", longChat(map[int]oldTurn{4: volTurn, 21: {
			user:   "Now the weekly volatility table for ETH, from the same 26 weeks.",
			answer: "Saved eth_weekly_vol, the weekly volatility of ETH.",
		}}), "Make that weekly volatility table for SOL too.", false},
		{"switched off", longChat(map[int]oldTurn{4: volTurn}), "Make that weekly volatility table for SOL too.", true},
		{"no cut yet", longChat(map[int]oldTurn{4: volTurn})[:2], "Make that weekly volatility table for SOL too.", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, t.TempDir(), "", testSettings(), echo())
			defer h.stop()
			h.o.d.NoHistoryCards = tc.off
			c := h.newChat(chat.Chat{Title: "Coins"})
			seedChat(h, c.ID, tc.turns)
			h.fp.Push(fake.Text("ok"))
			h.turn(c.ID, tc.text)
			if cards := cardsIn(h.fp.Calls()[0].Messages); len(cards) > 0 {
				t.Errorf("cards: %q", cards)
			}
		})
	}
}

// TestHistoryCardsPersian: a Persian message finds its turn through ZWNJ,
// Persian digits and Arabic letter forms, past Persian small words.
func TestHistoryCardsPersian(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings(), echo())
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "اجاره"})
	turns := make([]oldTurn, 22)
	for i := range turns {
		turns[i] = oldTurn{user: fmt.Sprintf("نوبت %d: رنگ مورد %d را بگو.", i+1, i+1), answer: fmt.Sprintf("مورد %d آبی است.", i+1)}
	}
	turns[2] = oldTurn{
		user: "یک جدول از اجاره‌ی هر متر مربع در سعادت‌آباد بساز، برای آپارتمان‌های 80 تا 120 متر.",
		tool: "echo", args: map[string]any{"text": "saadat_rent_per_m2"}, result: "saved view (revision 1).",
		answer: "جدول saadat_rent_per_m2 ذخیره شد.",
	}
	seedChat(h, c.ID, turns)
	h.fp.Push(fake.Text("باشد."))
	h.turn(c.ID, "همين جدول اجاره‌ی هر متر مربع را برای پونک هم بساز، آپارتمان‌های ۸۰ تا ۱۲۰ متر.")
	cards := cardsIn(h.fp.Calls()[0].Messages)
	if len(cards) != 1 || !strings.HasPrefix(cards[0], "23: [Turn 3 of this chat") || !strings.Contains(cards[0], "سعادت‌آباد") {
		t.Errorf("cards: %q", cards)
	}
}

// TestHistoryCardsUntrusted: a card of a turn that read outside data marks
// the turn as if it read it (8.8): Auto asks, and a memory chip notes it
// while the turn is in the window (3.3). A card of a turn without it
// leaves the mark off.
func TestHistoryCardsUntrusted(t *testing.T) {
	for _, tc := range []struct {
		tool string
		want bool
	}{{"outside", true}, {"echo", false}} {
		t.Run(tc.tool, func(t *testing.T) {
			h := newHarness(t, t.TempDir(), "", testSettings(), echo(), outside())
			defer h.stop()
			c := h.newChat(chat.Chat{Title: "Coins"})
			old := volTurn
			old.tool, old.args = tc.tool, map[string]any{}
			seedChat(h, c.ID, longChat(map[int]oldTurn{4: old}))
			h.fp.Push(fake.Text("ok"))
			h.turn(c.ID, "Make that weekly volatility table for SOL too.")
			if cards := cardsIn(h.fp.Calls()[0].Messages); len(cards) != 1 {
				t.Fatalf("cards: %q", cards)
			}
			cs := h.o.find(h.pid, c.ID)
			cs.mu.Lock()
			untrusted, outside := cs.untrusted, cs.outside
			cs.mu.Unlock()
			if untrusted != tc.want || (outside == 23) != tc.want {
				t.Errorf("untrusted %v, outside %d", untrusted, outside)
			}
		})
	}
}

// TestCardSmallWords: words common in the project's chats, or in more than
// a quarter of the chat's earlier turns, count for nothing, in every
// language. Each old turn here would get a card if they counted.
func TestCardSmallWords(t *testing.T) {
	sixTurns := func(text string) map[int]oldTurn {
		own := map[int]oldTurn{}
		for _, n := range []int{2, 4, 6, 8, 10, 12} {
			own[n] = oldTurn{user: fmt.Sprintf(text, n), answer: "OK."}
		}
		return own
	}
	other := make([]oldTurn, 30)
	for i := range other {
		other[i] = oldTurn{user: fmt.Sprintf("Please show the list %d.", i+1), answer: "Here it is."}
	}
	for _, tc := range []struct {
		name  string
		turns []oldTurn
		other []oldTurn // another chat of the project
		text  string
	}{
		{"common in the project", longChat(map[int]oldTurn{4: {user: "Please show grapes.", answer: "Grapes: 12."}}), other, "Please show me."},
		{"common in the chat", longChat(sixTurns("Make the chart pretty and bigger, item %d.")), nil, "Make chart pretty bigger."},
		{"Persian, common in the chat", longChat(sixTurns("برای این هم یک نمودار بساز، مورد %d.")), nil, "برای اون هم نمودار بساز."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, t.TempDir(), "", testSettings())
			defer h.stop()
			c := h.newChat(chat.Chat{Title: "Small"})
			seedChat(h, c.ID, tc.turns)
			if tc.other != nil {
				seedChat(h, h.newChat(chat.Chat{Title: "Other"}).ID, tc.other)
			}
			h.open(func(p *project.Project) {
				scores, err := CardScores(context.Background(), p.Chats, c.ID, tc.text, 23)
				if err != nil {
					t.Fatal(err)
				}
				if len(scores) > 0 {
					t.Errorf("scores: %+v", scores)
				}
			})
		})
	}
}
