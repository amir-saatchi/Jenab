package statetest

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/state"
)

// Dev is the developer tools' service in memory (state.DevAPI): the turns
// recorded, their views and block texts, and the runtime.
type Dev struct {
	mu      sync.Mutex
	List    []app.TurnItem
	Views   map[state.TurnRef]app.TurnView
	Texts   map[string]string // by BlockKey
	RT      app.Runtime
	RTError error
}

// NewDev makes a Dev with nothing recorded.
func NewDev() *Dev {
	return &Dev{Views: map[state.TurnRef]app.TurnView{}, Texts: map[string]string{}}
}

// BlockKey names a block's text in Texts.
func BlockKey(r state.TurnRef, request, block int) string {
	return fmt.Sprintf("%s/%d/%d", r.Key(), request, block)
}

// Add records a turn: its view, and the item listing it first.
func (d *Dev) Add(v app.TurnView) {
	d.mu.Lock()
	defer d.mu.Unlock()
	v.Requests = append([]app.RequestView{}, v.Requests...)
	v.TurnItem.Requests = len(v.Requests)
	d.Views[state.TurnRef{Project: v.Project, Chat: v.Chat, Turn: v.Turn}] = v
	d.List = append([]app.TurnItem{v.TurnItem}, d.List...)
}

// Turns is DevAPI's.
func (d *Dev) Turns(context.Context) ([]app.TurnItem, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]app.TurnItem{}, d.List...), nil
}

// Turn is DevAPI's.
func (d *Dev) Turn(_ context.Context, p id.Project, c id.Chat, n int) (app.TurnView, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	v, ok := d.Views[state.TurnRef{Project: p, Chat: c, Turn: n}]
	if !ok {
		return v, &app.UIError{Kind: app.KindNotFound, Message: "This turn isn't recorded. Turns are kept from the start of the app: the last 100."}
	}
	return v, nil
}

// Block is DevAPI's.
func (d *Dev) Block(_ context.Context, p id.Project, c id.Chat, n, request, block int) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	text, ok := d.Texts[BlockKey(state.TurnRef{Project: p, Chat: c, Turn: n}, request, block)]
	if !ok {
		return "", &app.UIError{Kind: app.KindNotFound, Message: "This block isn't recorded."}
	}
	return text, nil
}

// Runtime is DevAPI's.
func (d *Dev) Runtime(context.Context) (app.Runtime, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.RT, d.RTError
}

// SampleTurn is a finished turn with two requests and two tool calls, the
// second request reading most of its prompt from the cache.
func SampleTurn(p id.Project, projectName string, c id.Chat, title string, n int, started time.Time) app.TurnView {
	blocks := func(cache ...string) []app.BlockView {
		return []app.BlockView{
			{Name: "Tools", Tokens: 2400, Cache: cache[0]},
			{Name: "System prompt", Tokens: 1800, Point: true, Cache: cache[1]},
			{Name: "History", Tokens: 900, Cache: cache[2]},
			{Name: "This turn", Tokens: 320, Cache: cache[3]},
		}
	}
	return app.TurnView{
		TurnItem: app.TurnItem{Project: p, ProjectName: projectName, Chat: c, ChatTitle: title, Turn: n,
			Model: "fake/scripted", Started: started},
		TookMS: 3600,
		Requests: []app.RequestView{
			{StartMS: 0, TookMS: 1240, Usage: chat.Usage{Input: 5420, Output: 180}, Parts: []string{"thinking", "text", "tool_call", "tool_call"},
				Blocks: blocks("miss", "miss", "miss", "miss")},
			{StartMS: 2100, TookMS: 1500, Last: true, Usage: chat.Usage{Input: 1292, CacheRead: 6120, Output: 412}, Parts: []string{"text"},
				Blocks: blocks("hit", "hit", "part", "miss")},
		},
		Tools: []app.ToolView{
			{Request: 0, CallID: "call_1", Name: "fetch_url", StartMS: 1260, TookMS: 640, Bytes: 20_480, Ref: "cache/fetch/1.md"},
			{Request: 0, CallID: "call_2", Name: "query", StartMS: 1300, TookMS: 38, Bytes: 64, Error: true},
		},
	}
}

// devTools is the Fake's settings with the developer tools on at this
// start.
type devTools struct{ *Fake }

func (devTools) Started(context.Context) (app.Started, error) {
	return app.Started{DevTools: true}, nil
}

// WithDevTools is the stores' services, all of them f, with the developer
// tools on at this start.
func (f *Fake) WithDevTools() state.API {
	a := f.API()
	a.Settings = devTools{f}
	return a
}
