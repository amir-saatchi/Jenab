//go:build bench

package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/update"
)

// The bench build tests updates against a local feed (TASK-059), never
// the public one: with JENAB_UPDATE_FEED set to a folder, or to the
// loopback URL of one, the updater reads update-<os>-<arch>.json and the
// files it names from there. The checks stay MyGo's: the archive must be
// signed with the key the build was made with.
func init() {
	if feed := os.Getenv("JENAB_UPDATE_FEED"); feed != "" {
		mygo.Updater.SetSource(localFeed(strings.TrimSuffix(feed, "/")))
	}
}

type localFeed string

func (f localFeed) Manifest(ctx context.Context, _ bool) (io.ReadCloser, error) {
	return f.Open(ctx, "update-"+runtime.GOOS+"-"+runtime.GOARCH+".json")
}

// Open serves the file the manifest names by its base name: the manifest
// names it at the release host.
func (f localFeed) Open(ctx context.Context, name string) (io.ReadCloser, error) {
	base := path.Base(name)
	if u, err := url.Parse(string(f)); err == nil && u.Scheme == "http" {
		if ip := net.ParseIP(u.Hostname()); u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return nil, fmt.Errorf("JENAB_UPDATE_FEED %s is not on the loopback interface", f)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, string(f)+"/"+url.PathEscape(base), nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("GET %s: %s", req.URL, resp.Status)
		}
		return resp.Body, nil
	}
	return os.Open(filepath.Join(string(f), base))
}

// updateRoutes are the bench's update and notification routes.
func (b *bench) updateRoutes(mux *http.ServeMux) {
	// POST /update?do=check|install|restart runs a step of the updater;
	// GET /state shows where it is.
	mux.HandleFunc("POST /update", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("do") {
		case "check":
			updateService.CheckNow()
		case "install":
			updateService.Install()
		case "restart":
			updateService.Restart()
		default:
			http.Error(w, "do is check, install or restart", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	// POST /panic?text= posts a closure to the UI thread that panics with
	// text, for the crash log's check (TASK-067). The app ends.
	mux.HandleFunc("POST /panic", func(w http.ResponseWriter, r *http.Request) {
		text := r.URL.Query().Get("text")
		mark("panic posted")
		w.WriteHeader(http.StatusNoContent)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		b.post(func() { panic("bench: " + text) })
	})
	// POST /hide hides the window, as closing it does.
	mux.HandleFunc("POST /hide", func(w http.ResponseWriter, r *http.Request) {
		b.ui(func() { b.win.Hide() })
		w.WriteHeader(http.StatusNoContent)
	})
	// POST /notify shows a notification of the first project's first
	// chat, as a chat that starts to wait does; a click shows the window.
	mux.HandleFunc("POST /notify", func(w http.ResponseWriter, r *http.Request) {
		var n app.Notification
		b.ui(func() {
			if p, ok := b.project(); ok {
				n.Project = p
				if items := b.st.Chats.ByProject[p]; len(items) > 0 {
					n.Chat = items[0].Chat.ID
				}
			}
		})
		n.Title, n.Body = "Jenab bench", r.URL.Query().Get("body")
		if n.Body == "" {
			n.Body = "A test notification. Click it to open Jenab."
		}
		if err := appNotify(n); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		mark("notification shown")
		w.WriteHeader(http.StatusNoContent)
	})
}

// benchUpdate is the updater's status in GET /state.
type benchUpdate struct {
	State   update.State `json:"state"`
	Current string       `json:"current"`
	Version string       `json:"version,omitempty"`
	Err     string       `json:"err,omitempty"`
}

func updateState() benchUpdate {
	if updateService == nil {
		return benchUpdate{}
	}
	st := updateService.Status()
	return benchUpdate{State: st.State, Current: st.Current, Version: st.Version, Err: st.Err}
}
