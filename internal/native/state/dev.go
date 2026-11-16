package state

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/id"
)

// DevAPI is what the developer tools read (SPEC 8.4); *app.DevService has
// it.
type DevAPI interface {
	Turns(ctx context.Context) ([]app.TurnItem, error)
	Turn(ctx context.Context, p id.Project, c id.Chat, n int) (app.TurnView, error)
	Block(ctx context.Context, p id.Project, c id.Chat, n, request, block int) (string, error)
	Runtime(ctx context.Context) (app.Runtime, error)
}

var _ DevAPI = (*app.DevService)(nil)

// TurnRef names a recorded turn.
type TurnRef struct {
	Project id.Project
	Chat    id.Chat
	Turn    int
}

// Key is the turn's key among the turns.
func (r TurnRef) Key() string { return fmt.Sprintf("%s/%s#%d", r.Project, r.Chat, r.Turn) }

// Dev is the developer tools' store (frontend/src/dev/inspector.tsx and
// runtime.tsx): the turns recorded, the turn the inspector shows, the
// runtime, and a context block's text.
//
// The reads repeat on a timer, as the React views did: the turn every
// second while it runs, the runtime every second. They are driven by the
// screen, which calls PollTurn or PollRuntime as it builds; each is a
// no-op until a read is due. So a hidden window, or another screen, reads
// nothing, and the next frame shown reads again. A read that is due wakes
// the window with an empty Post.
type Dev struct {
	// Turns is the recorded turns, newest first; nil until read.
	Turns []app.TurnItem
	// Pick is the turn the inspector shows; nil until there is one.
	Pick *TurnRef
	// Runtime is the runtime last read, nil until one is; RuntimeError is
	// the last read's failure, "" after a read that worked.
	Runtime      *app.Runtime
	RuntimeError string
	// Block is the context block whose text is open, or nil.
	Block *BlockText

	// Now is the clock and After the timer; tests set them.
	Now   func() time.Time
	After func(d time.Duration, fn func())

	api       DevAPI
	post      Post
	listError string
	shown     shownTurn
	turn      poll
	runtime   poll
}

// BlockText is a context block's text as the provider got it, with its
// secrets redacted. Lines is nil while it loads; Err says why it didn't.
type BlockText struct {
	Turn    TurnRef
	Request int
	Block   int
	Name    string
	Tokens  int
	Lines   []string
	Err     string
}

// shownTurn is the turn read for the pick under key, or its error; for
// another key it is left out, so a new pick never shows the old turn.
type shownTurn struct {
	key  string
	view *app.TurnView
	err  string
}

// poll is the state of a read that repeats.
type poll struct {
	key      string // what it reads
	busy     bool
	done     bool // a finished turn isn't read again
	loaded   bool
	failures int
	next     time.Time // not before
}

// NewDev makes the store.
func NewDev(api DevAPI, post Post) *Dev {
	return &Dev{api: api, post: post, Now: time.Now,
		After: func(d time.Duration, fn func()) { time.AfterFunc(d, fn) }}
}

// OpenInspector opens the turn inspector at a turn, or at the newest turn
// recorded for nil, and reads the turns.
func (d *Dev) OpenInspector(at *TurnRef) {
	d.Pick = nil
	if at != nil {
		r := *at
		d.Pick = &r
	}
	d.shown, d.turn, d.listError = shownTurn{}, poll{}, ""
	d.LoadTurns()
}

// LoadTurns reads the turns recorded. The first turn becomes the pick if
// there is none.
func (d *Dev) LoadTurns() {
	go func() {
		list, err := d.api.Turns(background)
		d.post(func() {
			if err != nil {
				// A list on screen stays there.
				if d.Turns == nil {
					d.listError = errorText(err)
				}
				return
			}
			if list == nil {
				list = []app.TurnItem{}
			}
			d.Turns = list
			if d.Pick == nil && len(list) > 0 {
				d.Pick = &TurnRef{Project: list[0].Project, Chat: list[0].Chat, Turn: list[0].Turn}
			}
		})
	}()
}

// Choose shows another turn.
func (d *Dev) Choose(r TurnRef) { d.Pick = &r }

// Shown is the turn read for the pick, or why it can't be shown: the
// turn's error, or the list's. Both are empty while it loads.
func (d *Dev) Shown() (*app.TurnView, string) {
	if d.Pick != nil && d.shown.key == d.Pick.Key() {
		return d.shown.view, d.shown.err
	}
	return nil, d.listError
}

// PollTurn reads the pick when a read is due: at once, again every second
// while the turn runs, and after a failure twice as long each time, up to
// 30 s. A turn on screen stays there while a failed read is tried again.
func (d *Dev) PollTurn() {
	if d.Pick == nil {
		return
	}
	ref := *d.Pick
	key := ref.Key()
	if d.turn.key != key {
		d.turn = poll{key: key}
	}
	if p := &d.turn; p.busy || p.done || d.Now().Before(p.next) {
		return
	}
	d.turn.busy = true
	go func() {
		v, err := d.api.Turn(background, ref.Project, ref.Chat, ref.Turn)
		d.post(func() {
			p := &d.turn
			if p.key != key {
				return // another pick, read on its own
			}
			p.busy = false
			if err != nil {
				p.failures++
				if !p.loaded {
					d.shown = shownTurn{key: key, err: errorText(err)}
				}
				d.wait(p, min(time.Second<<min(p.failures, 5), 30*time.Second))
				return
			}
			p.loaded, p.failures = true, 0
			d.shown = shownTurn{key: key, view: &v}
			if v.Running {
				d.wait(p, time.Second)
				return
			}
			p.done = true
			d.LoadTurns() // the list says it ended
		})
	}()
}

// OpenRuntime starts the runtime panel afresh.
func (d *Dev) OpenRuntime() {
	d.Runtime, d.RuntimeError, d.runtime = nil, "", poll{}
}

// PollRuntime reads the runtime when a read is due: at once, then a second
// after each read ends.
func (d *Dev) PollRuntime() {
	p := &d.runtime
	if p.busy || d.Now().Before(p.next) {
		return
	}
	p.busy = true
	go func() {
		rt, err := d.api.Runtime(background)
		d.post(func() {
			p.busy = false
			if err != nil {
				d.RuntimeError = errorText(err)
			} else {
				d.Runtime, d.RuntimeError = &rt, ""
			}
			d.wait(p, time.Second)
		})
	}()
}

// wait makes p's next read due after by, and wakes the window then.
func (d *Dev) wait(p *poll, by time.Duration) {
	p.next = d.Now().Add(by)
	d.After(by, func() { d.post(func() {}) })
}

// OpenBlock shows the text of block of request in turn v.
func (d *Dev) OpenBlock(v *app.TurnView, request, block int) {
	b := &BlockText{Turn: TurnRef{Project: v.Project, Chat: v.Chat, Turn: v.Turn}, Request: request, Block: block}
	if request >= 0 && request < len(v.Requests) && block >= 0 && block < len(v.Requests[request].Blocks) {
		bv := v.Requests[request].Blocks[block]
		b.Name, b.Tokens = bv.Name, bv.Tokens
	}
	d.Block = b
	go func() {
		text, err := d.api.Block(background, b.Turn.Project, b.Turn.Chat, b.Turn.Turn, request, block)
		d.post(func() {
			if err != nil {
				b.Err = errorText(err)
				return
			}
			b.Lines = Lines(text)
		})
	}()
}

// CloseBlock closes the block's text.
func (d *Dev) CloseBlock() { d.Block = nil }

// Lines splits a text into its lines, for a virtual list: "" is one empty
// line, and a closing newline makes no line of its own.
func Lines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}
