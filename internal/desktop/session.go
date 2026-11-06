package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// maxSession is how long a session lasts (8.10).
const maxSession = 10 * time.Minute

// Deps are what the desktop tools need.
type Deps struct {
	// Runner runs agent-desktop: the CLI in the app, a fake in tests.
	Runner Runner
	// On is the Settings switch "Let agents use other apps' windows".
	On func() bool
	// Self is Jenab's process id, whose windows can't be picked.
	Self int
	// Class gives a window's class name, which tells system dialogs such
	// as the Run box; nil gives none.
	Class func(windowID string) string
	Log   *slog.Logger
	Now   func() time.Time // nil is time.Now
	// Poll is how long desktop_wait and a browser's first snapshot wait
	// between tries; 0 is a second.
	Poll time.Duration
	// Store keeps macros and recipes (8.11): registry.db in the app. Nil
	// leaves the macro and recipe tools without effect.
	Store Store
}

// Manager holds each chat's session: one window, in one turn.
type Manager struct {
	d        Deps
	mu       sync.Mutex
	sessions map[key]*Session
	ended    map[key]string // why a chat's last session ended, for the next call
	// last is a chat's session after it ended, for the saves of the rest
	// of its turn: a model often ends the session, then saves.
	last    map[key]*Session
	shipped sync.Once // the recipes that come with Jenab are in the store
}

type key struct {
	p id.Project
	c id.Chat
}

// NewManager returns a Manager with no sessions.
func NewManager(d Deps) *Manager {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Poll <= 0 {
		d.Poll = time.Second
	}
	if d.On == nil {
		d.On = func() bool { return false }
	}
	return &Manager{d: d, sessions: map[key]*Session{}, ended: map[key]string{}, last: map[key]*Session{}}
}

// Session is one locked window: its id and agent-desktop's process
// instance, and for a browser the site's origin (8.10). Element ids come
// only from its own snapshots.
type Session struct {
	m       *Manager
	key     key
	turn    int
	win     Window
	browser bool
	origin  string // the locked site; "" until a blank window opens one
	started time.Time

	snaps    int // snapshots the model got
	full     bool
	ids      map[string]string // Jenab's id → agent-desktop's ref
	nodes    map[string]Node
	last     []string // the latest tree, for the change line
	lastAll  []Node   // its elements, for the log check
	keyAsked bool     // the first key press was approved

	// The recorder (8.11): the steps that worked, in memory for this
	// session only.
	done      []Step
	byHand    int             // the model's own actions that worked
	saved     bool            // a macro or recipe was saved
	reminded  bool            // the reminder to save was given
	failed    map[string]bool // macros whose replay failed in this session
	changedBy []Node          // the elements the last action changed
	before    []Node          // the elements before it
}

// session returns the chat's open session for this turn.
func (m *Manager) session(env *tool.Env) (*Session, error) {
	k := key{env.Project.ID, env.Chat}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[k]
	if s != nil && s.turn != env.Turn {
		delete(m.sessions, k)
		s, m.ended[k] = nil, "the turn it was started in ended"
	}
	if s == nil {
		if why := m.ended[k]; why != "" {
			return nil, tool.Errorf("the desktop session ended: %s. If the user still wants it, call desktop_start to ask for a window again.", why)
		}
		return nil, tool.Errorf("no window is in use: call desktop_start first")
	}
	return s, nil
}

// end ends the chat's session, if it is still s.
func (m *Manager) end(s *Session, why string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions[s.key] == s {
		delete(m.sessions, s.key)
		m.ended[s.key] = why
		m.last[s.key] = s
		m.d.Log.Info("desktop: session ended", "app", s.win.App, "why", why, "snapshots", s.snaps, "took", m.d.Now().Sub(s.started).Round(time.Second))
	}
}

// EndChat drops all a chat keeps here: its session, the session kept for
// the saves of its turn and why the last one ended. The orchestrator calls
// it when the chat's turn numbers start again (a Clear, a rewind), so a
// new turn with an old number can't reach an old turn's window or steps.
func (m *Manager) EndChat(p id.Project, c id.Chat) {
	k := key{p, c}
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.sessions[k]; s != nil {
		m.d.Log.Info("desktop: session ended", "app", s.win.App, "why", "the chat was reset", "snapshots", s.snaps)
	}
	delete(m.sessions, k)
	delete(m.last, k)
	delete(m.ended, k)
}

// endError ends the session and is the error the model gets.
func (s *Session) endError(why string) error {
	s.m.end(s, why)
	return tool.Errorf("the desktop session ended: %s. Tell the user; if they still want it, call desktop_start to ask for a window again.", why)
}

// check runs before every command: the time limit, and the lock on the
// window and agent-desktop's process instance. A window agent-desktop
// stops listing ends the session.
func (s *Session) check(ctx context.Context) error {
	if s.m.d.Now().Sub(s.started) >= maxSession {
		return s.endError("it reached its 10-minute limit")
	}
	ws, err := listWindows(ctx, s.m.d.Runner, s.win.App)
	if err != nil {
		var de *Error
		if errors.As(err, &de) && de.Code == CodeWindowNotFound {
			return s.endError(fmt.Sprintf("the window %q is gone", s.win.Title))
		}
		return s.fail(err)
	}
	for _, w := range ws {
		if w.ID == s.win.ID {
			if w.Instance != s.win.Instance {
				return s.endError("the app restarted")
			}
			return nil
		}
	}
	return s.endError(fmt.Sprintf("the window %q is gone: closed, or no longer listed (an action such as \"Keep on top\" can do that)", s.win.Title))
}

// fail turns a command's error into what the model gets.
func (s *Session) fail(err error) error {
	var de *Error
	switch {
	case errors.Is(err, ErrKilled):
		return tool.Errorf("the app didn't answer in time, and the call was stopped. Wait a moment and try again, or stop and tell the user.")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case !errors.As(err, &de):
		return err // the runner: a missing program, the state folder
	}
	msg := clip(de.Message, 300)
	switch de.Code {
	case CodeWindowNotFound:
		return s.endError(fmt.Sprintf("the window %q is gone", s.win.Title))
	case CodeAppUnresponsive:
		return tool.Errorf("the app is not responding. Wait and try again, or stop and tell the user.")
	case CodeTimeout:
		return tool.Errorf("the app didn't answer in time (%s). Try again, or take a new snapshot.", msg)
	case CodeStaleRef, CodeElementNotFound, CodeSnapshotMissing:
		return tool.Errorf("that element changed or is gone (%s). Take a new snapshot and use an id from it.", msg)
	case CodeAmbiguous:
		// Nothing was sent. The ref wasn't proven unique (a snapshot cut
		// short, or an identical element elsewhere in the window), or a
		// second one like it appeared.
		return tool.Errorf("not done: agent-desktop can't tell that element apart from another just like it (%s). Take a new snapshot and use an id from it; if the same element is refused again, use another one.", msg)
	case CodePolicyDenied:
		return tool.Errorf("not possible without taking the pointer or focus, which isn't allowed: %s. Try another element or action.", msg)
	}
	text := de.Code + ": " + msg
	if de.Delivery != "" && de.Delivery != "not_delivered" {
		text += " (it may have reached the app: take a snapshot before trying again)"
	}
	return tool.Errorf("%s", text)
}

// snapshot reads the locked window for the model or for the change line.
// It checks the window, the site lock and CAPTCHAs, and returns the view.
func (s *Session) snapshot(ctx context.Context, full bool, n int) (view, *snapData, error) {
	args := []string{"snapshot", "--window-id", s.win.ID}
	if !full {
		args = append(args, "-i")
	}
	timeout := callTimeout
	if s.snaps == 0 {
		timeout = firstTimeout
	}
	args = append(args, "--timeout-ms", fmt.Sprint((timeout - 3*time.Second).Milliseconds()))
	if s.browser {
		args = append(args, browserArgs...)
	}
	var snap *snapData
	waited := time.Duration(0)
	for {
		data, err := s.m.d.Runner.Run(ctx, timeout, args...)
		if err != nil {
			return view{}, nil, s.fail(err)
		}
		if snap, err = parseSnap(data); err != nil {
			return view{}, nil, err
		}
		// A cold browser reports an empty page as complete (SPIKE-039):
		// wait until the page has children.
		p := pageNode(&snap.Tree)
		if !s.browser || p == nil || blank(origin(p.Value)) || len(p.Children) > 0 || waited >= 8*time.Second {
			break
		}
		if err := sleep(ctx, s.m.d.Poll); err != nil {
			return view{}, nil, err
		}
		waited += s.m.d.Poll
	}
	if snap.Window.ID != s.win.ID {
		return view{}, nil, s.endError("agent-desktop read another window than the one picked")
	}
	if isVault(snap.Window.Title) {
		return view{}, nil, s.endError("the window now shows a password manager, which Jenab never reads")
	}
	roots := []Node{snap.Tree}
	if s.browser {
		page := pageNode(&snap.Tree)
		o := ""
		if page != nil {
			o = origin(page.Value)
		}
		if err := s.lockSite(o); err != nil {
			return view{}, nil, err
		}
		if page != nil && isCaptcha(page) {
			return view{}, nil, s.endError("the page asks to prove the user is not a robot (a CAPTCHA). Jenab never solves one: the user has to handle it")
		}
		roots = roots[:0]
		if bar := addressBar(&snap.Tree, page); bar != nil {
			b := *bar
			b.Children, b.Value = nil, origin(b.Value)
			roots = append(roots, b)
		}
		if page != nil {
			roots = append(roots, *page)
		}
	}
	for i := range roots {
		scrub(&roots[i])
	}
	return compact(roots, n), snap, nil
}

// lockSite checks a browser page's origin against the lock. A window on a
// blank page takes the origin of the first site it opens.
func (s *Session) lockSite(o string) error {
	switch {
	case blank(o) && s.origin == "":
		return nil
	case s.origin == "":
		s.origin = o
		s.m.d.Log.Info("desktop: site locked", "app", s.win.App)
		return nil
	case o == "" || o == s.origin:
		return nil
	}
	return s.endError(fmt.Sprintf("the page left the site the session is locked to (%s) for %s", s.origin, o))
}

// scrub keeps only the origin of every document's address: a file://
// address has the Windows user name in it.
func scrub(n *Node) {
	if n.Role == "document" && n.Value != "" {
		n.Value = origin(n.Value)
	}
	for i := range n.Children {
		scrub(&n.Children[i])
	}
}

// focused is the element of the locked window with the keyboard focus, or
// nil when no element of it has the focus.
func (s *Session) focused(ctx context.Context) (*Node, error) {
	data, err := s.m.d.Runner.Run(ctx, callTimeout, "find", "--window-id", s.win.ID, "--state", "focused", "--last")
	if err != nil {
		return nil, s.fail(err)
	}
	var r struct {
		Match *Node `json:"match"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("desktop: reading find: %w", err)
	}
	return r.Match, nil
}

// states reads an element's states again, just before an action.
func (s *Session) states(ctx context.Context, ref string) ([]string, error) {
	data, err := s.m.d.Runner.Run(ctx, callTimeout, "get", ref, "--property", "states")
	if err != nil {
		return nil, s.fail(err)
	}
	var r struct {
		Value []string `json:"value"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("desktop: reading get: %w", err)
	}
	return r.Value, nil
}

// remember keeps a view's ids for later actions.
func (s *Session) remember(v view) {
	maps.Copy(s.ids, v.ids)
	maps.Copy(s.nodes, v.nodes)
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-t.C:
		return nil
	}
}

func title(w Window) string {
	t := clip(strings.TrimSpace(w.Title), 80)
	if t == "" {
		t = "(no title)"
	}
	return t
}
