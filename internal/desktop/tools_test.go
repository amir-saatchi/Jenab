package desktop

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/tool"
)

func TestToolsAreOffWithTheSwitch(t *testing.T) {
	on := false
	m := NewManager(Deps{Runner: newFake(), On: func() bool { return on }})
	for _, tl := range m.Tools() {
		s := tl.Spec()
		if s.IsOn() || !s.UserTurns {
			t.Fatalf("%s: on %v, user turns %v; want off and user turns only", s.Name, s.IsOn(), s.UserTurns)
		}
		if untrusted := s.Effects&tool.Untrusted != 0; untrusted != (s.Name != "desktop_end") {
			t.Errorf("%s: untrusted %v", s.Name, untrusted)
		}
	}
	on = true
	if !m.Tools()[0].Spec().IsOn() {
		t.Fatal("the switch is on, the tool is still off")
	}
	if NewManager(Deps{Runner: newFake()}).Tools()[0].Spec().IsOn() {
		t.Fatal("with no switch the tools must be off")
	}
}

func TestPickCardListsOnlyPickableWindows(t *testing.T) {
	h := newHarness(t)
	out := h.ok("desktop_start", map[string]string{"why": "to write your sentence in Notes"})
	if !strings.Contains(out, `"Notes"`) {
		t.Fatalf("start: %s", out)
	}
	c := h.cards[0]
	if c.Kind != KindWindow || c.Why != "to write your sentence in Notes" || c.Risk == "" {
		t.Fatalf("card: %+v", c)
	}
	var grants []chat.Grant
	for _, o := range c.Options {
		grants = append(grants, o.Grant)
	}
	want := []chat.Grant{chat.GrantPick + "w-1", chat.GrantPick + "w-2", chat.GrantDeny}
	if !slices.Equal(grants, want) {
		t.Fatalf("options %v, want %v: no terminal, Jenab, Run box or password manager", grants, want)
	}
	if c.Options[0].Label != "Notes — notes" {
		t.Errorf("label %q", c.Options[0].Label)
	}
	if len(h.ends) != 1 {
		t.Fatal("the session must end with the turn")
	}
}

func TestPickCardByApp(t *testing.T) {
	type opt struct {
		grant chat.Grant
		group string
	}
	cases := []struct {
		app  string
		ask  string
		want []opt
	}{
		// One window of the app: the card asks about it, the rest follow.
		{"Notes", "Let the agent use Notes — notes?",
			[]opt{{"pick:w-1", chat.OptionMatch}, {"pick:w-2", chat.OptionOther}, {"pick:w-7", chat.OptionOther}, {chat.GrantDeny, ""}}},
		// Several: they come first, in the order agent-desktop lists them.
		{"chrome.exe", "Pick a window of chrome for the agent to use",
			[]opt{{"pick:w-2", chat.OptionMatch}, {"pick:w-7", chat.OptionMatch}, {"pick:w-1", chat.OptionOther}, {chat.GrantDeny, ""}}},
		// None: the card says so and lists the others.
		{"calc", "No window of calc is open: pick another for the agent to use",
			[]opt{{"pick:w-1", chat.OptionOther}, {"pick:w-2", chat.OptionOther}, {"pick:w-7", chat.OptionOther}, {chat.GrantDeny, ""}}},
		// A denied app named is still never offered.
		{"WindowsTerminal", "No window of windowsterminal is open: pick another for the agent to use",
			[]opt{{"pick:w-1", chat.OptionOther}, {"pick:w-2", chat.OptionOther}, {"pick:w-7", chat.OptionOther}, {chat.GrantDeny, ""}}},
		// No app: the full list, as before.
		{"", "Pick a window for the agent to use",
			[]opt{{"pick:w-1", ""}, {"pick:w-2", ""}, {"pick:w-7", ""}, {chat.GrantDeny, ""}}},
	}
	for _, c := range cases {
		h := newHarness(t)
		h.f.windows = append(h.f.windows, Window{ID: "w-7", Title: "Mail", App: "Chrome.exe", PID: 70, Instance: "i70", Visible: true})
		h.ok("desktop_start", map[string]string{"app": c.app, "why": "x"})
		card := h.cards[0]
		var got []opt
		for _, o := range card.Options {
			got = append(got, opt{o.Grant, o.Group})
		}
		if card.Ask != c.ask || !slices.Equal(got, c.want) {
			t.Errorf("app %q: %q %v, want %q %v", c.app, card.Ask, got, c.ask, c.want)
		}
		if err := (chat.Part{Kind: chat.PartApproval, Approval: &card}).Validate(); err != nil {
			t.Errorf("app %q: %v", c.app, err)
		}
	}
}

func TestDenyList(t *testing.T) {
	cases := []struct {
		w     Window
		class string
		ok    bool
	}{
		{Window{App: "notepad.exe", PID: 1, Visible: true}, "", true},
		{Window{App: "msedge.exe", PID: 1, Visible: true}, "", true},
		{Window{App: "cmd.exe", PID: 1, Visible: true}, "", false},
		{Window{App: "powershell.exe", PID: 1, Visible: true}, "", false},
		{Window{App: "pwsh.exe", PID: 1, Visible: true}, "", false},
		{Window{App: "WindowsTerminal.exe", PID: 1, Visible: true}, "", false},
		{Window{App: "conhost.exe", PID: 1, Visible: true}, "", false},
		{Window{App: "regedit.exe", PID: 1, Visible: true}, "", false},
		{Window{App: "mmc.exe", PID: 1, Visible: true}, "", false},
		{Window{App: "SystemSettings.exe", PID: 1, Visible: true}, "", false},
		{Window{App: "1Password.exe", PID: 1, Visible: true}, "", false},
		{Window{App: "chrome.exe", Title: "Google Password Manager - Google Chrome", PID: 1, Visible: true}, "", false},
		{Window{App: "firefox.exe", Title: "Passwords — Mozilla Firefox", PID: 1, Visible: true}, "", false},
		{Window{App: "msedge.exe", Title: "Bitwarden Web vault - Microsoft Edge", PID: 1, Visible: true}, "", false},
		{Window{App: "chrome.exe", Title: "Search - Google Chrome", PID: 1, Visible: true}, "", true},
		{Window{App: "consent.exe", PID: 1, Visible: true}, "", false},
		{Window{App: "explorer.exe", PID: 1, Visible: true}, "#32770", false},
		{Window{App: "explorer.exe", PID: 1, Visible: true}, "CabinetWClass", true},
		{Window{App: "Jenab.exe", PID: 1, Visible: true}, "", false},
		{Window{App: "notepad.exe", PID: self, Visible: true}, "", false},
		{Window{App: "notepad.exe", PID: 1}, "", false},
	}
	for _, c := range cases {
		if ok, why := pickable(c.w, self, c.class); ok != c.ok || !ok && why == "" {
			t.Errorf("%s (pid %d, %q): %v %q, want %v", c.w.App, c.w.PID, c.class, ok, why, c.ok)
		}
	}
}

func TestDeniedPickStartsNothing(t *testing.T) {
	h := newHarness(t)
	h.pick = ""
	h.fails("desktop_start", map[string]string{"why": "x"}, "their note: not now")
	h.fails("desktop_snapshot", map[string]any{}, "call desktop_start first")
}

func TestSnapshotIsCompactWithIDs(t *testing.T) {
	h := newHarness(t)
	h.ok("desktop_start", map[string]string{"why": "x"})
	out := h.ok("desktop_snapshot", map[string]any{})
	for _, want := range []string{`Window "Notes" (snapshot 1)`, `- textfield "Text" [1:e1]`, `- button "Save" [1:e2]`} {
		if !strings.Contains(out, want) {
			t.Fatalf("snapshot lacks %q:\n%s", want, out)
		}
	}
	snaps := h.f.ran("snapshot")
	if !slices.Contains(snaps[0], "-i") {
		t.Errorf("interactive only by default: %v", snaps[0])
	}
	h.ok("desktop_snapshot", map[string]any{"full": true})
	if slices.Contains(h.f.ran("snapshot")[1], "-i") {
		t.Error("full is the whole tree")
	}
}

func TestSnapshotStopsAt400Lines(t *testing.T) {
	var kids []Node
	for i := range 450 {
		kids = append(kids, Node{Role: "button", Name: strings.Repeat("b", i%7+1), Ref: "@s:e" + string(rune('a'+i%26)) + strings.Repeat("x", i/26)})
	}
	v := compact([]Node{{Role: "window", Children: kids}}, 1)
	if len(v.lines) != maxLines || !strings.Contains(v.text, "[50 more elements left out") {
		t.Fatalf("%d lines; tail %q", len(v.lines), v.text[len(v.text)-80:])
	}
}

func TestIDsComeFromTheSession(t *testing.T) {
	h := newHarness(t)
	h.ok("desktop_start", map[string]string{"why": "x"})
	h.fails("desktop_act", map[string]string{"id": "1:e2", "action": "click"}, "not an id from this session's snapshots")
	h.ok("desktop_snapshot", map[string]any{})
	h.ok("desktop_act", map[string]string{"id": "1:e2", "action": "click"})
	if c := h.f.ran("click"); len(c) != 1 || c[0][1] != "@s:e2" {
		t.Fatalf("click: %v", c)
	}
}

func TestPasswordFieldsAreRefused(t *testing.T) {
	h := newHarness(t)
	h.f.secure["@s:e6"] = true
	h.ok("desktop_start", map[string]string{"why": "x"})
	h.ok("desktop_snapshot", map[string]any{})
	h.fails("desktop_act", map[string]string{"id": "1:e3", "action": "set_value", "text": "hunter2"}, "password")
	h.fails("desktop_act", map[string]string{"id": "1:e9", "action": "set_value", "text": "x"}, "password")
	h.fails("desktop_act", map[string]string{"id": "1:e6", "action": "set_value", "text": "123456"}, "password field")
	if len(h.f.ran("set-value")) != 0 {
		t.Fatal("a refused value was sent")
	}
	h.f.focus["w-1"] = "@s:e3"
	h.fails("desktop_act", map[string]string{"id": "1:e1", "action": "press", "key": "return"}, "password")
	if len(h.f.ran("press")) != 0 {
		t.Fatal("a key went to a password field")
	}
}

func TestSecureFieldAfterAnActionEndsTheSession(t *testing.T) {
	h := newHarness(t)
	h.ok("desktop_start", map[string]string{"why": "x"})
	h.ok("desktop_snapshot", map[string]any{})
	h.f.snaps = []func(Node) Node{func(n Node) Node {
		n.Children = append(n.Children, Node{Role: "textfield", Name: "Secret", States: []string{"secure"}, Ref: "@s:e99"})
		return n
	}}
	h.fails("desktop_act", map[string]string{"id": "1:e2", "action": "click"}, "password field is in the window")
	h.fails("desktop_snapshot", map[string]any{}, "the desktop session ended")
}

func TestConsentIsRefused(t *testing.T) {
	h := newHarness(t)
	h.ok("desktop_start", map[string]string{"why": "x"})
	h.ok("desktop_snapshot", map[string]any{})
	for _, id := range []string{"1:e4", "1:e7", "1:e8"} { // English, German, Persian
		h.fails("desktop_act", map[string]string{"id": id, "action": "click"}, "refused: the user doesn't allow accepting")
	}
	h.ok("desktop_act", map[string]string{"id": "1:e5", "action": "click"}) // Reject all
	for name, want := range map[string]bool{
		"Accept": true, "I agree": true, "Allow all": true, "Akzeptieren": true, "Ich stimme zu": true, "موافقم": true,
		"Reject all": false, "Nicht akzeptieren": false, "Manage consent": false, "رد کردن": false, "Save": false,
		"Accept and continue reading the full article about cookies on this site, which is long": false,
	} {
		if got := isConsent(Node{Role: "button", Name: name}); got != want {
			t.Errorf("isConsent(%q) = %v", name, got)
		}
	}
}

func TestKeyGoesOnlyToTheFocusedElement(t *testing.T) {
	h := newHarness(t)
	h.ok("desktop_start", map[string]string{"why": "x"})
	h.ok("desktop_snapshot", map[string]any{})
	h.fails("desktop_act", map[string]string{"id": "1:e1", "action": "press", "key": "return"}, "no element of the window has the keyboard focus")
	h.fails("desktop_act", map[string]string{"id": "1:e1", "action": "press", "key": "f4"}, "must be one of")
	h.f.focus["w-1"] = "@s:e1"
	out := h.ok("desktop_act", map[string]string{"id": "1:e2", "action": "press", "key": "return"})
	if !strings.Contains(out, `The key went to the focused element: textfield "Text"`) {
		t.Fatalf("press: %s", out)
	}
	p := h.f.ran("press")
	if len(p) != 1 || !slices.Equal(p[0], []string{"press", "return", "--window-id", "w-1"}) {
		t.Fatalf("press: %v", p)
	}
}

// The approval rows (8.8, 8.10): the window card and the first key press
// ask at every level, other actions only at Strict.
func TestApprovalRows(t *testing.T) {
	h := newHarness(t)
	h.level = func(kind string) bool { return kind != KindAction } // Standard
	h.f.focus["w-1"] = "@s:e1"
	h.ok("desktop_start", map[string]string{"why": "x"})
	h.ok("desktop_snapshot", map[string]any{})
	h.ok("desktop_act", map[string]string{"id": "1:e1", "action": "set_value", "text": "Hello"})
	h.ok("desktop_act", map[string]string{"id": "1:e1", "action": "press", "key": "return"})
	h.ok("desktop_act", map[string]string{"id": "1:e1", "action": "press", "key": "tab"})
	var kinds []string
	for _, c := range h.cards {
		kinds = append(kinds, c.Kind)
	}
	if !slices.Equal(kinds, []string{KindWindow, KindKey}) {
		t.Fatalf("cards %v: the window, then the first key only", kinds)
	}
	k := h.cards[1]
	if !strings.Contains(k.Details, "Element: textfield \"Text\"") || !strings.Contains(k.Details, "Key: Return") || k.Target != "desktop:w-1" {
		t.Errorf("key card: %+v", k)
	}
	for _, o := range k.Options {
		if o.Grant == chat.GrantAlways {
			t.Error("a desktop card never has Always")
		}
	}

	// At Strict every action asks, and Deny stops it with the note.
	h2 := newHarness(t)
	h2.ok("desktop_start", map[string]string{"why": "x"})
	h2.ok("desktop_snapshot", map[string]any{})
	h2.answer = chat.GrantDeny
	h2.fails("desktop_act", map[string]string{"id": "1:e1", "action": "set_value", "text": "Hello"}, "the user denied it")
	if len(h2.f.ran("set-value")) != 0 || h2.cards[1].Kind != KindAction || !strings.Contains(h2.cards[1].Details, "Text: Hello") {
		t.Fatalf("strict: %+v", h2.cards)
	}
}

func TestChangeLineAfterAction(t *testing.T) {
	h := newHarness(t)
	h.ok("desktop_start", map[string]string{"why": "x"})
	h.ok("desktop_snapshot", map[string]any{})
	out := h.ok("desktop_act", map[string]string{"id": "1:e1", "action": "set_value", "text": "Hello"})
	if !strings.Contains(out, "Done, and the app confirmed it.") || !strings.Contains(out, `Changed: textfield "Text" = "Hello"`) {
		t.Fatalf("act: %s", out)
	}
	out = h.ok("desktop_act", map[string]string{"id": "1:e2", "action": "click"})
	if !strings.Contains(out, "Nothing in the window changed.") {
		t.Fatalf("act: %s", out)
	}
}

func TestLockOnWindowAndInstance(t *testing.T) {
	h := newHarness(t)
	h.ok("desktop_start", map[string]string{"why": "x"})
	h.ok("desktop_snapshot", map[string]any{})
	h.f.windows[0].Instance = "i11" // the app restarted with the same window id
	h.fails("desktop_snapshot", map[string]any{}, "the app restarted")
	h.fails("desktop_snapshot", map[string]any{}, "the desktop session ended: the app restarted")

	h = newHarness(t)
	h.ok("desktop_start", map[string]string{"why": "x"})
	h.f.windows = h.f.windows[1:] // closed, or unlisted after "Keep on top"
	h.fails("desktop_snapshot", map[string]any{}, "is gone")
	if n := len(h.f.ran("snapshot")); n != 0 {
		t.Fatalf("%d snapshots of a window that is gone", n)
	}
}

func TestBrowserSiteLock(t *testing.T) {
	h := newHarness(t)
	h.pick = "w-2"
	h.ok("desktop_start", map[string]string{"why": "x"})
	out := h.ok("desktop_snapshot", map[string]any{})
	if !strings.Contains(out, "page: no site yet") {
		t.Fatalf("blank: %s", out)
	}
	// The blank page takes the first site it opens.
	h.f.trees["w-2"] = browserTree("https://example.org/search?q=jenab", []Node{{Role: "button", Name: "Search", Ref: "@b:e2"}})
	out = h.ok("desktop_snapshot", map[string]any{})
	if !strings.Contains(out, "page: https://example.org") || strings.Contains(out, "q=jenab") || !strings.Contains(out, `= "https://example.org"`) {
		t.Fatalf("site: %s", out)
	}
	h.ok("desktop_act", map[string]string{"id": "2:e2", "action": "click"})
	// Another site ends the session, checked before the next action.
	h.f.trees["w-2"] = browserTree("https://other.example/", []Node{{Role: "button", Name: "Search", Ref: "@b:e2"}})
	h.fails("desktop_act", map[string]string{"id": "2:e2", "action": "click"}, "left the site")
	if n := len(h.f.ran("click")); n != 1 {
		t.Fatalf("%d clicks; the one on the other site must not run", n)
	}
}

func TestBrowserWaitsForThePage(t *testing.T) {
	h := newHarness(t)
	h.pick = "w-2"
	h.f.trees["w-2"] = browserTree("https://example.org/", nil) // a cold browser: complete, but empty
	h.f.snaps = []func(Node) Node{
		func(n Node) Node { return n },
		func(n Node) Node { return n },
		func(Node) Node {
			return browserTree("https://example.org/", []Node{{Role: "link", Name: "Home", Ref: "@b:e3"}})
		},
	}
	h.ok("desktop_start", map[string]string{"why": "x"})
	out := h.ok("desktop_snapshot", map[string]any{})
	if !strings.Contains(out, `- link "Home"`) || len(h.f.ran("snapshot")) != 3 {
		t.Fatalf("%d snapshots: %s", len(h.f.ran("snapshot")), out)
	}
	if s := h.f.ran("snapshot")[0]; !slices.Contains(s, "--max-depth") || !slices.Contains(s, "40") {
		t.Errorf("a browser's page to depth 40: %v", s)
	}
}

func TestFileAddressShowsNoPath(t *testing.T) {
	h := newHarness(t)
	h.pick = "w-2"
	h.f.trees["w-2"] = browserTree("file:///C:/Users/someone/Desktop/page.html", []Node{{Role: "button", Name: "Go", Ref: "@b:e2"}})
	h.ok("desktop_start", map[string]string{"why": "x"})
	out := h.ok("desktop_snapshot", map[string]any{"full": true})
	if strings.Contains(out, "someone") || strings.Contains(out, "Desktop") {
		t.Fatalf("the path leaked: %s", out)
	}
}

func TestCaptchaEndsTheSession(t *testing.T) {
	h := newHarness(t)
	h.pick = "w-2"
	h.f.trees["w-2"] = browserTree("https://www.google.com/sorry/index", []Node{{Role: "text", Name: "Our systems have detected unusual traffic"}})
	h.ok("desktop_start", map[string]string{"why": "x"})
	h.fails("desktop_snapshot", map[string]any{}, "CAPTCHA")
	h.fails("desktop_snapshot", map[string]any{}, "the desktop session ended")
	if !isCaptcha(&Node{Role: "document", Children: []Node{{Role: "checkbox", Name: "I'm not a robot"}}}) {
		t.Error("the checkbox is a CAPTCHA")
	}
}

func TestPasswordManagerTabEndsTheSession(t *testing.T) {
	// The browser's title follows its tab: one that turns to a password
	// manager after the pick ends the session at the next snapshot.
	h := newHarness(t)
	h.pick = "w-2"
	h.ok("desktop_start", map[string]string{"why": "x"})
	h.ok("desktop_snapshot", map[string]any{})
	h.f.mu.Lock()
	h.f.windows[1].Title = "Google Password Manager - Google Chrome"
	h.f.mu.Unlock()
	h.fails("desktop_snapshot", map[string]any{}, "password manager")
	h.fails("desktop_snapshot", map[string]any{}, "the desktop session ended")
}

func TestSessionEnds(t *testing.T) {
	// At the turn's end, and the next turn can't use it.
	h := newHarness(t)
	h.ok("desktop_start", map[string]string{"why": "x"})
	h.endTurn()
	h.fails("desktop_snapshot", map[string]any{}, "the turn ended")

	// A session from an earlier turn is gone even before its hook ran.
	h = newHarness(t)
	h.ok("desktop_start", map[string]string{"why": "x"})
	h.env.Turn = 2
	h.fails("desktop_snapshot", map[string]any{}, "the turn it was started in ended")

	// After 10 minutes.
	h = newHarness(t)
	h.ok("desktop_start", map[string]string{"why": "x"})
	h.ok("desktop_snapshot", map[string]any{})
	h.now = h.now.Add(maxSession)
	h.fails("desktop_act", map[string]string{"id": "1:e2", "action": "click"}, "10-minute limit")
	if len(h.f.ran("click")) != 0 {
		t.Fatal("an action ran after the limit")
	}

	// At desktop_end; a second start in the same turn needs it.
	h = newHarness(t)
	h.ok("desktop_start", map[string]string{"why": "x"})
	h.fails("desktop_start", map[string]string{"why": "x"}, "already in use")
	h.ok("desktop_end", map[string]any{})
	h.fails("desktop_snapshot", map[string]any{}, "the agent ended it")
	h.ok("desktop_start", map[string]string{"why": "again"})
}

func TestErrorsReachTheModel(t *testing.T) {
	h := newHarness(t)
	h.ok("desktop_start", map[string]string{"why": "x"})
	h.ok("desktop_snapshot", map[string]any{})
	h.f.errs["click"] = &Error{Code: CodeStaleRef, Message: "gone"}
	h.fails("desktop_act", map[string]string{"id": "1:e2", "action": "click"}, "Take a new snapshot")
	h.f.errs["click"] = ErrKilled
	h.fails("desktop_act", map[string]string{"id": "1:e2", "action": "click"}, "didn't answer in time")
	h.f.errs["click"] = &Error{Code: "ACTION_FAILED", Message: "not foreground", Delivery: "unknown"}
	h.fails("desktop_act", map[string]string{"id": "1:e2", "action": "click"}, "it may have reached the app")
	h.f.errs["snapshot"] = &Error{Code: CodeWindowNotFound, Message: "no window"}
	h.fails("desktop_snapshot", map[string]any{}, "is gone")
	h.fails("desktop_snapshot", map[string]any{}, "the desktop session ended")
}

func TestWait(t *testing.T) {
	h := newHarness(t)
	h.ok("desktop_start", map[string]string{"why": "x"})
	if out := h.ok("desktop_wait", map[string]any{"text": "Save"}); !strings.Contains(out, "in the window now") {
		t.Fatal(out)
	}
	start := time.Now()
	if out := h.ok("desktop_wait", map[string]any{"text": "Saved!", "seconds": 1}); !strings.Contains(out, "still not there") {
		t.Fatal(out)
	}
	if time.Since(start) > 30*time.Second {
		t.Fatal("waited too long")
	}
}

func TestSnapshotStubs(t *testing.T) {
	h := newHarness(t)
	h.ok("desktop_start", map[string]string{"why": "x"})
	r, err := tool.Run(t.Context(), h.tools["desktop_snapshot"], tool.Call{ID: "s1", Args: []byte(`{}`), Env: h.env})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Whole || r.Series != "desktop_snapshot" || r.Stub != `[snapshot 1 of "Notes" — replaced by a newer snapshot]` {
		t.Fatalf("result: whole %v, series %q, stub %q", r.Whole, r.Series, r.Stub)
	}
}

func TestOrigin(t *testing.T) {
	for in, want := range map[string]string{
		"https://Example.org/a?b=c":          "https://example.org",
		"http://localhost:8080/x":            "http://localhost:8080",
		"file:///C:/Users/someone/page.html": "file://",
		"about:blank":                        "about:blank",
		"example.org/path":                   "example.org",
		"some words":                         "",
		"":                                   "",
	} {
		if got := origin(in); got != want {
			t.Errorf("origin(%q) = %q, want %q", in, got, want)
		}
	}
	for _, o := range []string{"", "about:blank", "chrome://newtab", "edge://newtab"} {
		if !blank(origin(o)) {
			t.Errorf("%q is blank", o)
		}
	}
}
