package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// cdp is a minimal Chrome DevTools Protocol client for the app's WebView2
// page: commands and their answers, and the events someone waits for.
type cdp struct {
	conn    *websocket.Conn
	next    atomic.Int64
	mu      sync.Mutex
	waiting map[int64]chan cdpAnswer
	events  map[string]chan json.RawMessage
	done    chan struct{}
}

// on returns a channel that gets the params of the next event of method.
func (c *cdp) on(method string) <-chan json.RawMessage {
	ch := make(chan json.RawMessage, 1)
	c.mu.Lock()
	c.events[method] = ch
	c.mu.Unlock()
	return ch
}

type cdpAnswer struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// dialPage waits until the DevTools port lists a page, then connects.
func dialPage(ctx context.Context, port int) (*cdp, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/json", port)
	var last error
	for {
		ws, err := pageURL(ctx, url)
		if err != nil {
			last = err
		}
		if err == nil && ws != "" {
			conn, _, err := websocket.Dial(ctx, ws, nil)
			if err != nil {
				return nil, err
			}
			conn.SetReadLimit(64 << 20)
			c := &cdp{conn: conn, waiting: map[int64]chan cdpAnswer{}, events: map[string]chan json.RawMessage{}, done: make(chan struct{})}
			go c.read()
			return c, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("the app's DevTools port never listed a page; is it a build with the bench tag? %w (last: %v)", ctx.Err(), last)
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func pageURL(ctx context.Context, url string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var ts []struct {
		Type string `json:"type"`
		WS   string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ts); err != nil {
		return "", err
	}
	for _, t := range ts {
		if t.Type == "page" {
			return t.WS, nil
		}
	}
	return "", nil
}

func (c *cdp) read() {
	defer close(c.done)
	for {
		_, b, err := c.conn.Read(context.Background())
		if err != nil {
			return
		}
		var m struct {
			ID     int64           `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			cdpAnswer
		}
		if json.Unmarshal(b, &m) != nil {
			continue
		}
		if m.ID == 0 {
			c.mu.Lock()
			ch := c.events[m.Method]
			delete(c.events, m.Method)
			c.mu.Unlock()
			if ch != nil {
				ch <- m.Params
			}
			continue
		}
		c.mu.Lock()
		ch := c.waiting[m.ID]
		delete(c.waiting, m.ID)
		c.mu.Unlock()
		if ch != nil {
			ch <- m.cdpAnswer
		}
	}
}

func (c *cdp) close() { c.conn.Close(websocket.StatusNormalClosure, "") }

// call sends one command and waits for its answer.
func (c *cdp) call(ctx context.Context, method string, params any, out any) error {
	id := c.next.Add(1)
	ch := make(chan cdpAnswer, 1)
	c.mu.Lock()
	c.waiting[id] = ch
	c.mu.Unlock()
	b, err := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err != nil {
		return err
	}
	if err := c.conn.Write(ctx, websocket.MessageText, b); err != nil {
		return err
	}
	select {
	case a := <-ch:
		if a.Error != nil {
			return fmt.Errorf("%s: %s", method, a.Error.Message)
		}
		if out != nil {
			return json.Unmarshal(a.Result, out)
		}
		return nil
	case <-c.done:
		return errors.New("the DevTools connection closed")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// eval runs an expression in the page, awaits a promise it returns, and
// decodes the value into out.
func (c *cdp) eval(ctx context.Context, expr string, out any) error {
	var r struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		Exception *struct {
			Text      string `json:"text"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	if err := c.call(ctx, "Runtime.evaluate", map[string]any{"expression": expr, "awaitPromise": true, "returnByValue": true}, &r); err != nil {
		return err
	}
	if e := r.Exception; e != nil {
		return fmt.Errorf("page: %s %s", e.Text, e.Exception.Description)
	}
	if out != nil && len(r.Result.Value) > 0 {
		return json.Unmarshal(r.Result.Value, out)
	}
	return nil
}

// key presses and releases a key in the focused element, as user input.
func (c *cdp) key(ctx context.Context, key, code string, vk int, text string) error {
	down := map[string]any{"type": "keyDown", "key": key, "code": code, "windowsVirtualKeyCode": vk}
	if text != "" {
		down["text"] = text
	}
	if err := c.call(ctx, "Input.dispatchKeyEvent", down, nil); err != nil {
		return err
	}
	return c.call(ctx, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": key, "code": code, "windowsVirtualKeyCode": vk}, nil)
}

// insertText types text into the focused element.
func (c *cdp) insertText(ctx context.Context, text string) error {
	return c.call(ctx, "Input.insertText", map[string]any{"text": text}, nil)
}
