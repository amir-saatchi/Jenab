//go:build bench

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/egoist/mygo"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/secret"
	"github.com/amir-saatchi/jenab/internal/version"
)

// The bench build (go build -tags bench) lets cmd/jenab-bench -native
// drive and time the app (SPIKE-048 Part B), as the Wails app's bench
// build opens WebView2's DevTools port. With JENAB_BENCH_PORT set it
// serves a loopback control: open a chat, send a message, press keys,
// quit. It measures nothing itself: MyGo logs every frame with
// MYGO_FRAME_STATS=all, and the log's times have microseconds here, so
// the bench reads when each frame ended next to the marks below.
func init() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	benchHook = startBench
	newKeyring = benchKeyring
}

// benchKeyring keeps secrets in memory, never in the OS keychain: a
// provider's key comes from JENAB_BENCH_KEY_<PROVIDER>, as
// JENAB_BENCH_KEY_GEMINI for "provider:gemini".
func benchKeyring() secret.Keyring {
	k := &memKeyring{m: map[string]string{}}
	for _, kv := range os.Environ() {
		name, v, _ := strings.Cut(kv, "=")
		if p, ok := strings.CutPrefix(name, "JENAB_BENCH_KEY_"); ok && v != "" {
			k.m["provider:"+strings.ToLower(p)] = v
		}
	}
	return k
}

type memKeyring struct {
	mu sync.Mutex
	m  map[string]string
}

func (k *memKeyring) Get(name string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	v, ok := k.m[name]
	if !ok {
		return "", secret.ErrNotFound
	}
	return v, nil
}

func (k *memKeyring) Set(name, value string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.m[name] = value
	return nil
}

func (k *memKeyring) Delete(name string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if _, ok := k.m[name]; !ok {
		return secret.ErrNotFound
	}
	delete(k.m, name)
	return nil
}

type bench struct {
	st   *state.Stores
	post state.Post
	win  *mygo.Window
}

// mark logs what the bench waits for, with the time.
func mark(f string, a ...any) { log.Printf("bench: "+f, a...) }

func startBench(st *state.Stores, post state.Post, win *mygo.Window) {
	port, err := strconv.Atoi(os.Getenv("JENAB_BENCH_PORT"))
	if err != nil || port <= 0 {
		return
	}
	b := &bench{st: st, post: post, win: win}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		mark("no port: %v", err)
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /state", b.state)
	mux.HandleFunc("POST /open", b.open)
	mux.HandleFunc("POST /send", b.send)
	mux.HandleFunc("POST /key", b.key)
	mux.HandleFunc("POST /front", func(w http.ResponseWriter, r *http.Request) {
		b.ui(func() { show(win) })
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /size", func(w http.ResponseWriter, r *http.Request) {
		cw, _ := strconv.Atoi(r.URL.Query().Get("w"))
		ch, _ := strconv.Atoi(r.URL.Query().Get("h"))
		b.ui(func() { win.SetContentSize(cw, ch) })
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /shot", b.shot)
	mux.HandleFunc("POST /file", b.file)
	b.updateRoutes(mux)
	mux.HandleFunc("POST /quit", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		go b.post(mygo.App.Quit)
	})
	go http.Serve(ln, mux)
	// Mark when the projects are loaded: the first frame after it shows
	// the frame with the rail, as the Wails probe waits for the rail.
	go b.until(func() bool { return st.Projects.Loaded }, func() { mark("projects loaded") })
}

// ui runs fn on the UI thread and waits for it.
func (b *bench) ui(fn func()) {
	done := make(chan struct{})
	b.post(func() { fn(); close(done) })
	select {
	case <-done:
	case <-time.After(10 * time.Second):
	}
}

// until checks cond on the UI thread every millisecond, then runs then
// there.
func (b *bench) until(cond func() bool, then func()) {
	for {
		ok := false
		b.ui(func() {
			if ok = cond(); ok {
				then()
			}
		})
		if ok {
			return
		}
		time.Sleep(time.Millisecond)
	}
}

type benchState struct {
	Loaded    bool     `json:"loaded"`
	Chats     []string `json:"chats"`
	Open      string   `json:"open"`
	Messages  int      `json:"messages"`
	Running   bool     `json:"running"`
	Streaming bool     `json:"streaming"`
	Working   int      `json:"working"` // chats of the project not idle
	// TASK-059: the build, every project, whether the window is shown and
	// focused, and the updater.
	Version  string      `json:"version"`
	Projects []string    `json:"projects"`
	Shown    bool        `json:"shown"`
	Focused  bool        `json:"focused"`
	Update   benchUpdate `json:"update"`
}

func (b *bench) project() (id.Project, bool) {
	if len(b.st.Projects.List) == 0 {
		return "", false
	}
	return b.st.Projects.List[0].ID, true
}

func (b *bench) state(w http.ResponseWriter, r *http.Request) {
	s := benchState{Version: version.Get().Version, Update: updateState()}
	b.ui(func() {
		s.Loaded = b.st.Projects.Loaded
		for _, p := range b.st.Projects.List {
			s.Projects = append(s.Projects, p.Name)
		}
		s.Shown = b.win.IsVisible() && !b.win.IsMinimized()
		s.Focused = s.Shown && b.win.IsFocused()
		p, ok := b.project()
		if !ok {
			return
		}
		for _, it := range b.st.Chats.ByProject[p] {
			s.Chats = append(s.Chats, it.Chat.Title)
			if it.State != chat.StateIdle {
				s.Working++
			}
		}
		if pl := b.st.Nav.Current(); pl != nil {
			if t := b.st.Threads.Threads[pl.Chat]; t != nil {
				s.Open, s.Messages = t.Chat.Title, len(t.Messages)
				s.Running, s.Streaming = t.State != chat.StateIdle, t.Stream != ""
			}
		}
	})
	json.NewEncoder(w).Encode(s)
}

// open shows the chat of the first project titled title, and answers once
// its thread is read. The marks give the click and the read.
func (b *bench) open(w http.ResponseWriter, r *http.Request) {
	title, draft := r.URL.Query().Get("title"), r.URL.Query().Get("draft")
	var ch id.Chat
	var p id.Project
	b.ui(func() {
		pid, ok := b.project()
		if !ok {
			return
		}
		if _, ok := b.st.Chats.ByProject[pid]; !ok {
			b.st.OpenProject(pid, "")
			return
		}
		for _, it := range b.st.Chats.ByProject[pid] {
			if it.Chat.Title == title {
				p, ch = pid, it.Chat.ID
			}
		}
		if ch != "" {
			if draft != "" {
				b.st.Threads.Drafts[ch] = draft
			}
			mark("open %s", title)
			b.st.OpenChat(p, ch)
		}
	})
	if ch == "" {
		http.Error(w, "no chat "+title+" yet", http.StatusNotFound)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		b.until(func() bool { return ctx.Err() != nil || b.st.Threads.Threads[ch] != nil }, func() {
			if t := b.st.Threads.Threads[ch]; t != nil {
				mark("opened %s with %d messages", title, len(t.Messages))
			}
		})
		close(done)
	}()
	<-done
	w.WriteHeader(http.StatusNoContent)
}

// send sends text in the open chat, as Enter in the composer does.
func (b *bench) send(w http.ResponseWriter, r *http.Request) {
	text := r.URL.Query().Get("text")
	errc := make(chan error, 1)
	b.ui(func() {
		pl := b.st.Nav.Current()
		if pl == nil || pl.View != state.ViewChat {
			errc <- fmt.Errorf("no chat is open")
			return
		}
		b.st.Actions.Send(pl.Project, pl.Chat, text, nil, func(_ id.Message, err error) { errc <- err })
	})
	if err := <-errc; err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	procPostMessage  = user32.NewProc("PostMessageW")
	procFindWindowEx = user32.NewProc("FindWindowExW")
)

const (
	wmKeyDown = 0x0100
	wmKeyUp   = 0x0101
	wmChar    = 0x0102
)

// key posts a key press to the window's surface, as the keyboard would:
// key down, its character, key up. ch is a letter; vk a key without one,
// as 8 for Backspace. n repeats it. The mark is the time of the first.
func (b *bench) key(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	n, _ := strconv.Atoi(q.Get("n"))
	n = max(n, 1)
	vk, _ := strconv.Atoi(q.Get("vk"))
	ch := q.Get("ch")
	if ch != "" {
		vk = int(ch[0] &^ 0x20) // the letter's key
	}
	cls, _ := syscall.UTF16PtrFromString("MyGoSurface")
	surface, _, _ := procFindWindowEx.Call(b.win.NativeHandle(), 0, uintptr(unsafe.Pointer(cls)), 0)
	if surface == 0 {
		http.Error(w, "no surface", http.StatusInternalServerError)
		return
	}
	for i := range n {
		if i == 0 {
			mark("key %q", ch+strconv.Itoa(vk))
		}
		procPostMessage.Call(surface, wmKeyDown, uintptr(vk), 1)
		if ch != "" {
			procPostMessage.Call(surface, wmChar, uintptr(ch[0]), 1)
		}
		procPostMessage.Call(surface, wmKeyUp, uintptr(vk), 1|3<<30)
	}
	w.WriteHeader(http.StatusNoContent)
}
