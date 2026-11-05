package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// Approval kinds (8.8, 8.10). Picking the window and the first key press
// ask at every level; an action asks at Strict only.
const (
	KindWindow = "desktop_window"
	KindAction = "desktop_action"
	KindKey    = "desktop_key"
)

// Tools are desktop_start, desktop_snapshot, desktop_act, desktop_wait and
// desktop_end (8.10). They are offered only while m's switch is on, and
// only in turns the user started in a chat.
func (m *Manager) Tools() []tool.Tool {
	// Screen text is untrusted, like a web page (8.10): every tool that
	// returns some marks the turn.
	spec := func(s tool.Spec) tool.Spec {
		s.On, s.UserTurns = m.d.On, true
		if s.Name != "desktop_end" {
			s.Effects |= tool.Untrusted
		}
		return s
	}
	return []tool.Tool{
		tool.Func(spec(tool.Spec{Name: "desktop_start", Timeout: 2 * time.Minute,
			Description: "Ask the user for one window of another app on this computer for you to read and use, such as a browser or an editor. Only for what no other tool can do. Name the app you want; the user picks the window in a card, and you see only that one. Say why in one line.",
			Schema: json.RawMessage(`{"type":"object","properties":{` +
				`"app":{"type":"string","maxLength":100,"description":"The program you want, by its process name without .exe, such as chrome, msedge, firefox, notepad or calculatorapp. Not a window title"},` +
				`"why":{"type":"string","maxLength":300,"description":"Why you need a window, in one line the user reads in the card"}` +
				`},"required":["why"],"additionalProperties":false}`),
		}), m.start),
		tool.Func(spec(tool.Spec{Name: "desktop_snapshot", Timeout: 90 * time.Second,
			Description: "Read the picked window: its elements, one per line, each one you can act on with an id like [3:e12]. Interactive elements only, unless full is true (also static text, to read a page).",
			Schema:      json.RawMessage(`{"type":"object","properties":{"full":{"type":"boolean","description":"Also the static text"}},"additionalProperties":false}`),
		}), m.snapshotTool),
		tool.Func(spec(tool.Spec{Name: "desktop_act", Timeout: 2 * time.Minute,
			Description: "Do one action on one element of the picked window, by its id from your latest snapshot: click, set_value (replaces the element's text), toggle, expand, collapse, select (an option by its text), or press a key (return, escape, tab or backspace) in the element that has the focus. The result says what changed.",
			Schema: json.RawMessage(`{"type":"object","properties":{` +
				`"id":{"type":"string","description":"Element id from a snapshot, like 3:e12"},` +
				`"action":{"type":"string","enum":["click","set_value","toggle","expand","collapse","select","press"]},` +
				`"text":{"type":"string","maxLength":10000,"description":"For set_value: the new text. For select: the option"},` +
				`"key":{"type":"string","enum":["return","escape","tab","backspace"],"description":"For press"}` +
				`},"required":["id","action"],"additionalProperties":false}`),
		}), m.act),
		tool.Func(spec(tool.Spec{Name: "desktop_wait", Timeout: time.Minute,
			Description: "Wait until a text appears in the picked window, or with gone true until it disappears. Up to 20 seconds.",
			Schema:      json.RawMessage(`{"type":"object","properties":{"text":{"type":"string","minLength":1,"maxLength":200},"gone":{"type":"boolean"},"seconds":{"type":"number","minimum":1,"maximum":20}},"required":["text"],"additionalProperties":false}`),
		}), m.wait),
		tool.Func(spec(tool.Spec{Name: "desktop_end",
			Description: "Stop using the picked window, once the task is done.",
			Schema:      json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		}), m.endTool),
	}
}

type startArgs struct {
	App string `json:"app"`
	Why string `json:"why"`
}

// program is an app's name as the model and the card compare it: its
// process name in lower case, without ".exe".
func program(app string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(app)), ".exe")
}

func (m *Manager) start(ctx context.Context, env *tool.Env, a startArgs) (tool.Result, error) {
	if env.Approve == nil || env.AtTurnEnd == nil {
		return tool.Result{}, tool.Errorf("desktop control works only in a chat turn the user started")
	}
	k := key{env.Project.ID, env.Chat}
	m.mu.Lock()
	if s := m.sessions[k]; s != nil && s.turn == env.Turn {
		m.mu.Unlock()
		return tool.Result{}, tool.Errorf("the window %q is already in use; call desktop_end first to ask for another", title(s.win))
	}
	m.mu.Unlock()
	ws, err := listWindows(ctx, m.d.Runner, "")
	if err != nil {
		return tool.Result{}, (&Session{m: m}).fail(err)
	}
	card := chat.Approval{
		ID:     id.Approval(id.New()),
		Kind:   KindWindow,
		Target: "desktop",
		Ask:    "Pick a window for the agent to use",
		Why:    strings.TrimSpace(a.Why),
		Risk:   "The agent can read the window and click, set values and press keys in it, until the task is done, 10 minutes pass, you press Stop or the turn ends. Text in the window goes to the model.",
	}
	app := program(a.App)
	byGrant := map[chat.Grant]Window{}
	var match, other []chat.ApprovalOption
	for _, w := range ws {
		class := ""
		if m.d.Class != nil {
			class = m.d.Class(w.ID)
		}
		if ok, _ := pickable(w, m.d.Self, class); !ok {
			continue
		}
		g := chat.Grant(chat.GrantPick + w.ID)
		if _, dup := byGrant[g]; dup {
			continue
		}
		byGrant[g] = w
		o := chat.ApprovalOption{Label: title(w) + " — " + program(w.App), Grant: g}
		switch {
		case app == "":
			card.Options = append(card.Options, o)
		case program(w.App) == app:
			o.Group = chat.OptionMatch
			match = append(match, o)
		default:
			o.Group = chat.OptionOther
			other = append(other, o)
		}
	}
	// The named app's windows come first, the rest after them (8.10).
	card.Options = append(card.Options, match...)
	card.Options = append(card.Options, other...)
	if len(card.Options) == 0 {
		return tool.Result{}, tool.Errorf("no window can be picked: the user needs to open the app's window first")
	}
	switch {
	case app == "":
	case len(match) == 1:
		card.Ask = "Let the agent use " + match[0].Label + "?"
	case len(match) > 1:
		card.Ask = "Pick a window of " + app + " for the agent to use"
	default:
		card.Ask = "No window of " + app + " is open: pick another for the agent to use"
	}
	card.Options = append(card.Options, chat.ApprovalOption{Label: "Deny", Grant: chat.GrantDeny})
	got, err := env.Approve(ctx, card)
	if err != nil {
		return tool.Result{}, err
	}
	w, ok := byGrant[got.Answer]
	if !ok {
		msg := "the user didn't pick a window"
		if got.Note != "" {
			msg += "; their note: " + got.Note
		}
		return tool.Result{}, tool.Errorf("%s", msg)
	}
	s := &Session{m: m, key: k, turn: env.Turn, win: w, browser: isBrowser(w.App), started: m.d.Now(),
		ids: map[string]string{}, nodes: map[string]Node{}}
	if err := s.check(ctx); err != nil {
		return tool.Result{}, tool.Errorf("the picked window is gone; ask again if the user still wants it")
	}
	m.mu.Lock()
	m.sessions[k] = s
	delete(m.ended, k)
	m.mu.Unlock()
	env.AtTurnEnd(func() { m.end(s, "the turn ended") })
	m.d.Log.Info("desktop: session started", "app", w.App, "browser", s.browser)
	kind := "window"
	if s.browser {
		kind = "browser window; you see its page and the address bar's site, and the session stays on the first site it opens"
	}
	return tool.Result{Text: fmt.Sprintf("The user picked %q (%s). Take a snapshot to see it. The session ends after 10 minutes, at desktop_end, or when this turn ends.", title(w), kind)}, nil
}

type snapshotArgs struct {
	Full bool `json:"full"`
}

func (m *Manager) snapshotTool(ctx context.Context, env *tool.Env, a snapshotArgs) (tool.Result, error) {
	s, err := m.session(env)
	if err != nil {
		return tool.Result{}, err
	}
	start := m.d.Now()
	if err := s.check(ctx); err != nil {
		return tool.Result{}, err
	}
	n := s.snaps + 1
	v, snap, err := s.snapshot(ctx, a.Full, n)
	if err != nil {
		return tool.Result{}, err
	}
	s.snaps, s.full, s.last = n, a.Full, v.lines
	s.remember(v)
	var b strings.Builder
	if s.browser {
		site := s.origin
		if site == "" {
			site = "no site yet"
		}
		fmt.Fprintf(&b, "Browser window %q (snapshot %d), page: %s\n", title(s.win), n, site)
	} else {
		fmt.Fprintf(&b, "Window %q (snapshot %d)\n", title(s.win), n)
	}
	b.WriteString(v.text)
	if !snap.Complete {
		b.WriteString("[the tree is incomplete: the app didn't answer in time]\n")
	}
	m.d.Log.Info("desktop: snapshot", "app", s.win.App, "n", n, "full", a.Full, "lines", len(v.lines), "chars", b.Len(), "took", m.d.Now().Sub(start).Milliseconds())
	return tool.Result{Text: b.String(), Whole: true, Series: "desktop_snapshot",
		Stub: fmt.Sprintf("[snapshot %d of %q — replaced by a newer snapshot]", n, title(s.win))}, nil
}

type actArgs struct {
	ID     string `json:"id"`
	Action string `json:"action"`
	Text   string `json:"text"`
	Key    string `json:"key"`
}

func (m *Manager) act(ctx context.Context, env *tool.Env, a actArgs) (tool.Result, error) {
	s, err := m.session(env)
	if err != nil {
		return tool.Result{}, err
	}
	start := m.d.Now()
	ref, ok := s.ids[a.ID]
	if !ok {
		return tool.Result{}, tool.Errorf("%q is not an id from this session's snapshots. Take a snapshot and use an id from it.", a.ID)
	}
	if err := s.check(ctx); err != nil {
		return tool.Result{}, err
	}
	if s.browser {
		// The site is checked before each action (8.10).
		if _, _, err := s.snapshot(ctx, false, 0); err != nil {
			return tool.Result{}, err
		}
	}
	n := s.nodes[a.ID]
	var cmd []string
	var target *Node // where a key goes
	switch a.Action {
	case "click", "toggle", "expand", "collapse":
		cmd = []string{a.Action, ref}
	case "set_value":
		cmd = []string{"set-value", ref, a.Text}
	case "select":
		if a.Text == "" {
			return tool.Result{}, tool.Errorf("select needs text: the option to pick")
		}
		cmd = []string{"select", ref, a.Text}
	case "press":
		if !slices.Contains([]string{"return", "escape", "tab", "backspace"}, a.Key) {
			return tool.Result{}, tool.Errorf("press needs key: return, escape, tab or backspace")
		}
		// A key goes to the element that has the focus in the app, which
		// must be in the locked window (8.10).
		if target, err = s.focused(ctx); err != nil {
			return tool.Result{}, err
		}
		if target == nil {
			return tool.Result{}, tool.Errorf("no element of the window has the keyboard focus, so the key could go somewhere else. Click or set a value in the element first, or use another action.")
		}
		n = *target
		cmd = []string{"press", a.Key, "--window-id", s.win.ID}
	default:
		return tool.Result{}, tool.Errorf("unknown action %q", a.Action)
	}
	if a.Action != "set_value" && isConsent(n) {
		return tool.Result{}, tool.Errorf("refused: the user doesn't allow accepting cookies, terms or other consent. Decline instead, or stop and tell the user.")
	}
	if a.Action == "set_value" || a.Action == "select" || a.Action == "press" {
		if isPassword(n) {
			return tool.Result{}, tool.Errorf("refused: this is a password or code field. Ask the user to type it themselves.")
		}
		if a.Action != "press" {
			st, err := s.states(ctx, ref)
			if err != nil {
				return tool.Result{}, err
			}
			if slices.Contains(st, "secure") {
				return tool.Result{}, tool.Errorf("refused: this is a password field. Ask the user to type it themselves.")
			}
		}
	}
	if err := s.approve(ctx, env, a, n); err != nil {
		return tool.Result{}, err
	}
	data, err := m.d.Runner.Run(ctx, callTimeout, cmd...)
	if err != nil {
		m.d.Log.Info("desktop: act", "app", s.win.App, "action", a.Action, "text_len", len([]rune(a.Text)), "failed", true, "took", m.d.Now().Sub(start).Milliseconds())
		return tool.Result{}, s.fail(err)
	}
	out := delivery(data)
	if a.Action == "press" {
		out += fmt.Sprintf(" The key went to the focused element: %s.", head(*target))
	}
	// After each action, one line on what changed (8.10).
	v, snap, err := s.snapshot(ctx, s.full, 0)
	if err != nil {
		return tool.Result{}, err
	}
	if anySecure(&snap.Tree) {
		return tool.Result{}, s.endError("a password field is in the window now. The user has to fill it in themselves")
	}
	out += " " + changes(s.last, v.lines)
	s.last = v.lines
	m.d.Log.Info("desktop: act", "app", s.win.App, "action", a.Action, "text_len", len([]rune(a.Text)), "result_len", len(out), "took", m.d.Now().Sub(start).Milliseconds())
	return tool.Result{Text: out}, nil
}

// approve asks for the cards an action needs (8.10): the first key press
// of a session at every level, and any action at Strict. The level
// decides which ask; Approve returns an unasked one as granted.
func (s *Session) approve(ctx context.Context, env *tool.Env, a actArgs, n Node) error {
	if env.Approve == nil {
		return tool.Errorf("desktop control works only in a chat turn the user started")
	}
	where := fmt.Sprintf("Window: %s (%s)\nElement: %s", title(s.win), strings.ToLower(s.win.App), head(n))
	card := chat.Approval{ID: id.Approval(id.New()), Kind: KindAction, Target: "desktop:" + s.win.ID,
		Options: []chat.ApprovalOption{{Label: "Once", Grant: chat.GrantOnce}, {Label: "Deny", Grant: chat.GrantDeny}}}
	switch a.Action {
	case "press":
		if s.keyAsked {
			card.Kind = KindAction
		} else {
			card.Kind = KindKey
		}
		card.Ask = fmt.Sprintf("Press %s in %q?", keyName(a.Key), title(s.win))
		card.Risk = "A key goes to the element that has the focus in the app. Jenab checked it is in this window."
		card.Details = where + "\nKey: " + keyName(a.Key)
	case "set_value":
		card.Ask = fmt.Sprintf("Set the text of %s in %q?", head(n), title(s.win))
		card.Risk = "The element's text is replaced."
		card.Details = where + "\nText: " + a.Text
	default:
		card.Ask = fmt.Sprintf("%s %s in %q?", strings.ToUpper(a.Action[:1])+a.Action[1:], head(n), title(s.win))
		card.Risk = "The app does what this element does."
		card.Details = where + "\nAction: " + a.Action
		if a.Text != "" {
			card.Details += "\nOption: " + a.Text
		}
	}
	got, err := env.Approve(ctx, card)
	if err != nil {
		return err
	}
	if got.Answer == chat.GrantDeny {
		msg := "not done: the user denied it"
		if got.Note != "" {
			msg += "; their note: " + got.Note
		}
		return tool.Errorf("%s", msg)
	}
	if card.Kind == KindKey {
		s.keyAsked = true
	}
	return nil
}

func keyName(k string) string {
	return map[string]string{"return": "Return", "escape": "Escape", "tab": "Tab", "backspace": "Backspace"}[k]
}

// delivery says whether an action reached the app, from its disposition.
func delivery(data json.RawMessage) string {
	var d struct {
		Disposition struct {
			Delivery string `json:"delivery"`
		} `json:"disposition"`
	}
	_ = json.Unmarshal(data, &d)
	switch d.Disposition.Delivery {
	case "delivered_verified":
		return "Done, and the app confirmed it."
	case "delivered_unverified", "":
		return "Done; the app didn't confirm it."
	}
	return "Sent (" + d.Disposition.Delivery + ")."
}

type waitArgs struct {
	Text    string  `json:"text"`
	Gone    bool    `json:"gone"`
	Seconds float64 `json:"seconds"`
}

func (m *Manager) wait(ctx context.Context, env *tool.Env, a waitArgs) (tool.Result, error) {
	s, err := m.session(env)
	if err != nil {
		return tool.Result{}, err
	}
	if err := s.check(ctx); err != nil {
		return tool.Result{}, err
	}
	if a.Seconds <= 0 {
		a.Seconds = 5
	}
	limit := time.Duration(min(a.Seconds, 20) * float64(time.Second))
	waited := time.Duration(0)
	for {
		data, err := m.d.Runner.Run(ctx, callTimeout, "find", "--window-id", s.win.ID, "--text", a.Text, "--count")
		if err != nil {
			return tool.Result{}, s.fail(err)
		}
		var c struct {
			Count int `json:"count"`
		}
		_ = json.Unmarshal(data, &c)
		switch there := c.Count > 0; {
		case there && !a.Gone:
			return tool.Result{Text: "The text is in the window now."}, nil
		case !there && a.Gone:
			return tool.Result{Text: "The text is gone from the window."}, nil
		case waited >= limit:
			state := "still not there"
			if a.Gone {
				state = "still there"
			}
			return tool.Result{Text: fmt.Sprintf("Waited %.0f s: the text is %s.", limit.Seconds(), state)}, nil
		}
		if err := sleep(ctx, m.d.Poll); err != nil {
			return tool.Result{}, err
		}
		waited += m.d.Poll
	}
}

func (m *Manager) endTool(ctx context.Context, env *tool.Env, _ struct{}) (tool.Result, error) {
	s, err := m.session(env)
	if err != nil {
		return tool.Result{Text: "No window was in use."}, nil
	}
	m.end(s, "the agent ended it")
	return tool.Result{Text: fmt.Sprintf("Ended: %q is no longer in use.", title(s.win))}, nil
}
