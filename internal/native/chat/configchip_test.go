package chat_test

import (
	"testing"

	jc "github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/native/state/statetest"
)

// persianPage is a page ID in Persian, right to left.
const persianPage = "قیمت_بیت‌کوین"

// A saved view's chip and a saved page's chip (P3-03): no undo of their
// own, Open on the page's alone, which the keyboard reaches; Undo turn
// marks both undone, and the page's Open goes.
func TestConfigChips(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		x := open(t, dark,
			statetest.Text(1, jc.RoleUser, "Make a Bitcoin page"),
			answer(1,
				notice(jc.Notice{Kind: jc.NoticeConfigSaved, Text: "View saved: btc_price_chart", Target: "view:btc_price_chart", Change: "21"}),
				notice(jc.Notice{Kind: jc.NoticeConfigSaved, Text: "Page saved: " + persianPage, Target: "page:" + persianPage, Change: "22"}),
				text("Done.")),
		)
		shows(t, x.tt, "View saved: btc_price_chart", "Page saved: "+persianPage, "Open")
		if x.tt.HasText("Undo") {
			t.Errorf("a config chip has its own Undo: %q", x.tt.Texts())
		}
		// Its accessible name is Open, and it takes input.
		if off(t, x, "Open") {
			t.Error("Open is off")
		}
		shot(t, x, "config-chips")
		// The keyboard reaches Open, and Enter opens the page.
		focus(t, x, "Open")
		x.tt.Key(0, kit.KeyEnter)
		x.draw()
		ref := state.ChatRef{Project: x.p, Chat: x.ch}
		if d := x.st.Shell.Open[ref]; d != (state.Docked{Kind: "page", Key: persianPage}) {
			t.Errorf("docked %+v", d)
		}
		if x.st.Shell.RightTab != state.TabPages {
			t.Errorf("right tab %q", x.st.Shell.RightTab)
		}
		// Undo turn 1: both chips say so, and the page has no Open.
		x.f.SetUndone("config:21", true)
		x.f.SetUndone("config:22", true)
		x.f.AddMessages(x.ch, turnUndone("undo1", "Turn 1 undone"))
		x.st.Threads.Reload(x.ch)
		x.until(t, "the chips undone", func() bool {
			return kit.Shows(x.tt, "View saved: btc_price_chart · undone") && kit.Shows(x.tt, "Page saved: "+persianPage+" · undone")
		})
		if x.tt.HasText("Open") {
			t.Errorf("an undone page still offers Open: %q", x.tt.Texts())
		}
		// Redo turn 1 brings Open back.
		x.f.SetUndone("config:21", false)
		x.f.SetUndone("config:22", false)
		x.f.AddMessages(x.ch, turnUndone("undo2", "Turn 1 back"))
		x.st.Threads.Reload(x.ch)
		x.until(t, "the chips back", func() bool { return kit.Shows(x.tt, "Open") })
	})
}

// A deleted page's chip has no Open; open_page's chip has, with its
// filter values.
func TestOpenPageChip(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		x := open(t, dark,
			statetest.Text(1, jc.RoleUser, "Show me the page"),
			answer(1,
				notice(jc.Notice{Kind: jc.NoticeConfigDeleted, Text: "Page deleted: old", Target: "page:old", Change: "30"}),
				notice(jc.Notice{Kind: jc.NoticeOpenPage, Text: "Page: bitcoin", Target: "page:bitcoin", Details: `{"from_date":"2026-01-01"}`})),
		)
		x.until(t, "the chips' state", func() bool { return kit.Shows(x.tt, "Page: bitcoin") })
		shows(t, x.tt, "Page deleted: old")
		click(t, x, "Open")
		ref := state.ChatRef{Project: x.p, Chat: x.ch}
		if d := x.st.Shell.Open[ref]; d != (state.Docked{Kind: "page", Key: "bitcoin", Filters: `{"from_date":"2026-01-01"}`}) {
			t.Errorf("docked %+v", d)
		}
	})
}
