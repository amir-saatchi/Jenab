package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// fake is agent-desktop for the tests: a few windows and a tree per
// window, changed by the actions it gets.
type fake struct {
	mu      sync.Mutex
	windows []Window
	trees   map[string]Node   // window id → tree
	focus   map[string]string // window id → the focused element's ref
	secure  map[string]bool   // refs read as secure by get
	errs    map[string]error  // command → its error
	calls   [][]string
	inputs  []string          // what RunInput got on standard input, in order
	partial bool              // snapshots are cut short: incomplete, refs unproven
	snaps   []func(Node) Node // applied one per snapshot, then the last tree stays
	// onAct changes the window after an action, like an app: the command
	// and the element's ref.
	onAct func(f *fake, cmd, ref string)
}

func (f *fake) Run(ctx context.Context, timeout time.Duration, args ...string) (json.RawMessage, error) {
	return f.run(nil, args)
}

func (f *fake) RunInput(ctx context.Context, timeout time.Duration, input string, args ...string) (json.RawMessage, error) {
	return f.run(&input, args)
}

func (f *fake) run(input *string, args []string) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, args)
	if input != nil {
		f.inputs = append(f.inputs, *input)
	}
	if err := f.errs[args[0]]; err != nil {
		return nil, err
	}
	switch args[0] {
	case "list-windows":
		app := flag(args, "--app")
		var ws []Window
		for _, w := range f.windows {
			if app == "" || strings.EqualFold(w.App, app) {
				ws = append(ws, w)
			}
		}
		return marshal(ws)
	case "snapshot":
		w := flag(args, "--window-id")
		if len(f.snaps) > 0 {
			f.trees[w] = f.snaps[0](f.trees[w])
			f.snaps = f.snaps[1:]
		}
		title := w
		for _, win := range f.windows {
			if win.ID == w {
				title = win.Title
			}
		}
		d := map[string]any{"complete": !f.partial, "identity_unproven": f.partial, "window": map[string]string{"id": w, "title": title}, "tree": f.trees[w]}
		return marshal(d)
	case "find":
		w := flag(args, "--window-id")
		if t := flag(args, "--text"); t != "" {
			n := 0
			if strings.Contains(allText(new(f.trees[w])), t) {
				n = 1
			}
			return marshal(map[string]int{"count": n})
		}
		if flag(args, "--role") != "" || flag(args, "--name") != "" {
			return marshal(map[string]any{"matches": f.matches(w, args)})
		}
		ref := f.focus[w]
		if ref == "" {
			return marshal(map[string]any{"match": nil})
		}
		return marshal(map[string]any{"match": find(new(f.trees[w]), func(n *Node) bool { return n.Ref == ref })})
	case "get":
		var st []string
		if f.secure[args[1]] {
			st = []string{"secure"}
		}
		return marshal(map[string]any{"value": st})
	case "set-value":
		// agent-desktop 1.2.0: --stdin reads the value from standard input
		// and refuses a value in the arguments as well.
		if len(args) != 3 || args[2] != "--stdin" || input == nil {
			return nil, &Error{Code: "INVALID_ARGS", Message: fmt.Sprintf("set-value %q", args[1:])}
		}
		for _, t := range f.trees {
			if n := find(&t, func(n *Node) bool { return n.Ref == args[1] }); n != nil {
				n.Value = *input
			}
		}
		if f.onAct != nil {
			f.onAct(f, args[0], args[1])
		}
		return marshal(map[string]any{"disposition": map[string]string{"delivery": "delivered_verified"}})
	case "click", "toggle", "select", "expand", "collapse":
		if f.onAct != nil {
			f.onAct(f, args[0], args[1])
		}
	}
	return marshal(map[string]any{})
}

// matches is find by role and name, as agent-desktop 1.2.0 answers it:
// --exact is the whole name without case, else a part of it; path lists
// the ancestors' roles.
func (f *fake) matches(w string, args []string) []map[string]any {
	role, name := flag(args, "--role"), strings.ToLower(flag(args, "--name"))
	exact := slices.Contains(args, "--exact")
	limit, _ := strconv.Atoi(flag(args, "--limit"))
	out := []map[string]any{}
	var walk func(n Node, path []string)
	walk = func(n Node, path []string) {
		nm := strings.ToLower(n.Name)
		if (role == "" || n.Role == role) && (name == "" || exact && nm == name || !exact && strings.Contains(nm, name)) {
			var v any
			if n.Value != "" {
				v = n.Value
			}
			out = append(out, map[string]any{"ref_id": n.Ref, "role": n.Role, "name": n.Name, "value": v, "states": n.States, "path": slices.Clone(path)})
		}
		for _, c := range n.Children {
			walk(c, append(path, n.Role))
		}
	}
	walk(f.trees[w], nil)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// node is the element with ref in window w, to change it.
func (f *fake) node(w, ref string) *Node {
	t := f.trees[w]
	n := find(&t, func(n *Node) bool { return n.Ref == ref })
	f.trees[w] = t
	return n
}

func (f *fake) ran(cmd string) [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out [][]string
	for _, c := range f.calls {
		if c[0] == cmd {
			out = append(out, c)
		}
	}
	return out
}

func flag(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func marshal(v any) (json.RawMessage, error) {
	b, err := json.Marshal(v)
	return b, err
}

const self = 4242

func editorTree() Node {
	return Node{Role: "window", Name: "Notes", Children: []Node{
		{Role: "textfield", Name: "Text", Ref: "@s:e1"},
		{Role: "button", Name: "Save", Ref: "@s:e2"},
		{Role: "textfield", Name: "Password", Ref: "@s:e3"},
		{Role: "button", Name: "Accept all cookies", Ref: "@s:e4"},
		{Role: "button", Name: "Reject all", Ref: "@s:e5"},
		{Role: "textfield", Name: "Code", Ref: "@s:e6"},
		{Role: "button", Name: "Alle akzeptieren", Ref: "@s:e7"},
		{Role: "button", Name: "پذیرش همه", Ref: "@s:e8"},
		{Role: "textfield", Name: "رمز عبور", Ref: "@s:e9"},
	}}
}

func browserTree(url string, page []Node) Node {
	return Node{Role: "window", Name: "Chrome", Children: []Node{
		{Role: "textfield", Name: "Address and search bar", Ref: "@b:e1", Value: url},
		{Role: "document", Name: "Page", Value: url, Children: page},
	}}
}

func newFake() *fake {
	return &fake{
		windows: []Window{
			{ID: "w-1", Title: "Notes", App: "notes.exe", PID: 10, Instance: "i10", Visible: true},
			{ID: "w-2", Title: "Search", App: "chrome.exe", PID: 20, Instance: "i20", Visible: true},
			{ID: "w-3", Title: "PowerShell", App: "WindowsTerminal.exe", PID: 30, Instance: "i30", Visible: true},
			{ID: "w-4", Title: "Jenab", App: "jenab.exe", PID: self, Instance: "i42", Visible: true},
			{ID: "w-5", Title: "Run", App: "explorer.exe", PID: 50, Instance: "i50", Visible: true},
			{ID: "w-6", Title: "Passwords", App: "KeePassXC.exe", PID: 60, Instance: "i60", Visible: true},
		},
		trees:  map[string]Node{"w-1": editorTree(), "w-2": browserTree("about:blank", nil)},
		focus:  map[string]string{},
		secure: map[string]bool{},
		errs:   map[string]error{},
	}
}

// harness is one chat turn with the tools: Approve answers with pick for
// the window card and with answer for the others, and records the cards.
type harness struct {
	t      *testing.T
	f      *fake
	m      *Manager
	tools  map[string]tool.Tool
	env    *tool.Env
	cards  []chat.Approval
	pick   string                 // the window id to pick, "" to deny
	answer chat.Grant             // for action and key cards
	level  func(kind string) bool // whether a card asks; nil asks all
	ends   []func()
	now    time.Time
}

func newHarness(t *testing.T) *harness {
	h := &harness{t: t, f: newFake(), pick: "w-1", answer: chat.GrantOnce, now: time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)}
	h.build(nil)
	h.env = &tool.Env{Project: &project.Project{ID: id.Project(id.New())}, Chat: id.Chat(id.New()), Turn: 1,
		Approve: func(_ context.Context, a chat.Approval) (chat.Approval, error) {
			if h.level != nil && !h.level(a.Kind) {
				a.Answer = chat.GrantOnce
				return a, nil
			}
			h.cards = append(h.cards, a)
			switch {
			case a.Kind != KindWindow:
				a.Answer = h.answer
			case h.pick == "":
				a.Answer, a.Note = chat.GrantDeny, "not now"
			default:
				a.Answer = chat.Grant(chat.GrantPick + h.pick)
			}
			return a, nil
		},
		AtTurnEnd: func(f func()) { h.ends = append(h.ends, f) },
	}
	return h
}

// build makes the manager and its tools, with st for macros and recipes.
func (h *harness) build(st Store) {
	h.m = NewManager(Deps{Runner: h.f, On: func() bool { return true }, Self: self, Poll: time.Millisecond, Store: st,
		Now: func() time.Time { return h.now },
		Class: func(w string) string {
			if w == "w-5" {
				return "#32770"
			}
			return ""
		}})
	h.tools = map[string]tool.Tool{}
	for _, tl := range h.m.Tools() {
		h.tools[tl.Spec().Name] = tl
	}
}

// call runs a tool and returns its text, or its error's text.
func (h *harness) call(name string, args any) (string, bool) {
	h.t.Helper()
	b, _ := json.Marshal(args)
	r, err := tool.Run(context.Background(), h.tools[name], tool.Call{ID: fmt.Sprint(len(h.f.calls)), Args: b, Env: h.env})
	if err != nil {
		var te *tool.Error
		if !errors.As(err, &te) {
			h.t.Fatalf("%s: %v", name, err)
		}
		return te.Msg, false
	}
	return r.Text, true
}

func (h *harness) ok(name string, args any) string {
	h.t.Helper()
	s, ok := h.call(name, args)
	if !ok {
		h.t.Fatalf("%s(%v): %s", name, args, s)
	}
	return s
}

func (h *harness) fails(name string, args any, want string) string {
	h.t.Helper()
	s, ok := h.call(name, args)
	if ok || !strings.Contains(s, want) {
		h.t.Fatalf("%s(%v) = %q (ok %v), want an error with %q", name, args, s, ok, want)
	}
	return s
}

func (h *harness) endTurn() {
	for _, f := range h.ends {
		f()
	}
	h.ends = nil
}
