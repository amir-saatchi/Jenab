package kit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSmallButtonAndDisclosure(t *testing.T) {
	open := false
	view := func(c *Context) {
		Column(c).Gap(8).Width(500).Children(func() {
			SmallButton(c, IconReply, "Reply", Ghost)
			Disclosure(c, "Thinking", &open, false, func() {
				Icon(c, IconBrain).Size(12, 12)
				Text(c, "Thinking")
			}, func() {
				Detail(c, func() { Text(c, persian) })
			})
			Disclosure(c, "Tool read_file", new(bool), true, func() { Mono(c, "read_file") }, func() {
				Panel(c, func() { Pre(c, "{\"path\": \"a.go\"}") })
			})
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		open = false
		tt.Frame()
		tt.Advance(1e9)
		shows(t, tt, "Reply", "Thinking", "read_file")
		if tt.HasText(persian) {
			t.Error("a closed disclosure shows its panel")
		}
		tabTo(t, tt, "Thinking")
		tt.Key(0, ui.KeyEnter)
		tt.Advance(1e9)
		if !open || !tt.HasText(persian) {
			t.Errorf("Enter did not open the disclosure: %v %q", open, tt.Texts())
		}
	})
}

func TestMessageBubbleAndFooter(t *testing.T) {
	view := func(c *Context) {
		Column(c).Gap(12).Width(700).Children(func() {
			between(c, func() {})
			Message(c, "You said", true, func() {
				Quote(c, "the quoted line")
				Bubble(c, persian, false)
				Bubble(c, "joins the turn", true)
				Footer(c, func() { SmallButton(c, IconCopy, "Copy", Ghost); Text(c, "10:42") })
			})
			Message(c, "Jenab said", false, func() { Text(c, "an answer") })
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		shows(t, tt, "You said", "Jenab said", persian, "joins the turn", "Copy", "10:42",
			"Go to the quoted text: the quoted line")
		user, _ := tt.Find(persian)
		agent, _ := tt.Find("an answer")
		if user.X <= agent.X {
			t.Errorf("the user's bubble is not on the end edge: %v vs %v", user, agent)
		}
		tabTo(t, tt, "Go to the quoted text: the quoted line")
		tabTo(t, tt, "Copy")
	})
}

func TestMarker(t *testing.T) {
	long := persian + " " + persian + " " + persian + " " + persian
	view := func(c *Context) {
		between(c, func() {
			Column(c).Width(500).Gap(8).Children(func() {
				Marker(c, "Model changed to Flash", false)
				Marker(c, long, true)
			})
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		shows(t, tt, "Model changed to Flash", long)
		passesOver(t, tt)
	})
}

func TestCardAndRecord(t *testing.T) {
	choice, other := "once", false
	view := func(c *Context) {
		Column(c).Gap(12).Width(600).Children(func() {
			Card(c, "Approval", func() {
				Kicker(c, "APPROVAL · SCHEMA CHANGE")
				CardTitle(c, persian)
				RadioGroup(c, "Allow", func() {
					Radio(c, &choice, "once", "Allow once", "")
					Radio(c, &choice, "always", "Always allow", "For this chat")
				})
				Checkbox(c, &other, "Something else")
				Row(c).Gap(8).Children(func() { Button(c, "Approve", Default); Button(c, "Deny", Outline) })
			})
			Record(c, IconShieldCheck, Green, func() { Text(c, "Approved"); Muted(c, persian) })
			Strip(c, nil, Neutral, "Working…", nil)
			Strip(c, IconWarning, Amber, "Rate limited", func() { SmallButton(c, IconRefresh, "Retry now", Outline) })
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		choice, other = "once", false
		shows(t, tt, "Approval", "APPROVAL · SCHEMA CHANGE", persian, "Approved", "Working…", "Rate limited", "For this chat")
		tabTo(t, tt, "Allow once")
		tt.Key(0, ui.KeyDown)
		tt.Frame()
		if choice != "always" {
			t.Errorf("Down chose %q", choice)
		}
		tabTo(t, tt, "Something else")
		tt.Key(0, ui.KeySpace)
		tt.Frame()
		if !other {
			t.Error("Space did not check the box")
		}
		tabTo(t, tt, "Retry now")
	})
}

func TestComposerPieces(t *testing.T) {
	text, removed, unquoted := "", 0, false
	view := func(c *Context) {
		Column(c).Width(600).Children(func() {
			InputGroup(c, func() {
				if QuoteChip(c, "the answer", persian) {
					unquoted = true
				}
				Row(c).Gap(6).Padding(6).Wrap().Children(func() {
					if Attachment(c, IconFileText, "notes.md", "2 KB", false, false) {
						removed++
					}
					Attachment(c, IconImage, "big.png", "Too large", true, false)
				})
				ComposerText(c, &text, "Message")
				Row(c).Padding(6).Children(func() {
					MenuButtonGhost(c, IconSparkles, "Flash", "Model: Flash", func(m *Menu) {
						m.Item("Flash")
						m.Item("Pro")
					})
					Spacer(c)
					IconButton(c, IconSend, "Send", Default)
				})
			})
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		text, removed, unquoted = "", 0, false
		shows(t, tt, "Replying to the answer", persian, "notes.md", "Too large", "Message", "Model: Flash", "Send")
		if err := tt.Click("Remove notes.md"); err != nil || removed != 1 {
			t.Errorf("remove: %v, %d", err, removed)
		}
		if err := tt.Click("Remove the quote"); err != nil || !unquoted {
			t.Errorf("unquote: %v", err)
		}
		tabTo(t, tt, "Message")
		tt.Type(persian)
		tt.Frame()
		if text != persian {
			t.Errorf("typed %q", text)
		}
		tabTo(t, tt, "Model: Flash")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		shows(t, tt, "Pro")
	})
}

// Text in a Selectable column copies across its texts.
func TestSelectable(t *testing.T) {
	view := func(c *Context) {
		Selectable(Column(c).Children(func() {
			Text(c, "first message")
			Text(c, persian)
		}))
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		a, _ := tt.Find("first message")
		b, _ := tt.Find(persian)
		tt.Press(a.X+1, a.Y+a.H/2)
		tt.Move(b.X+b.W/2, b.Y+b.H/2)
		tt.Move(b.X+b.W-1, b.Y+b.H-1)
		tt.Release(b.X+b.W-1, b.Y+b.H-1)
		tt.Key(ui.Ctrl, ui.KeyC)
		tt.Frame()
		if got := tt.Clipboard(); got == "" || got == "first message" {
			t.Errorf("copied %q, want both texts", got)
		}
	})
}

func TestComposerKeys(t *testing.T) {
	var text, sent string
	quote := true
	view := func(c *Context) {
		Column(c).Width(600).Children(func() {
			InputGroup(c, func() {
				ComposerKeys(ComposerText(c, &text, "Message"), func(s string) bool {
					if s == "" {
						return false
					}
					sent = s
					return true
				}, func() bool {
					if !quote {
						return false
					}
					quote = false
					return true
				})
			})
		})
	}
	themes(t, view, func(t *testing.T, tt *ui.Tester) {
		text, sent, quote = "", "", true
		tt.Frame()
		tabTo(t, tt, "Message")
		tt.Type(persian)
		tt.Key(ui.Shift, ui.KeyEnter)
		tt.Type("two")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if sent != persian+"\ntwo" {
			t.Errorf("Enter sent %q", sent)
		}
		if text != "" || tt.HasText(persian) {
			t.Errorf("the text stayed after sending: %q", text)
		}
		tt.Key(0, ui.KeyEnter)
		if sent != persian+"\ntwo" {
			t.Error("Enter on an empty composer sent")
		}
		tt.Key(0, ui.KeyEscape)
		if quote {
			t.Error("Escape did not take the quote off")
		}
	})
}
