//go:build bench

package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/id"
)

// file stores the file at ?path in the open chat's project, through the
// upload route a dropped attachment takes, and opens it next to the chat
// (TASK-062): the live check docks a file with no system dialog. It
// answers with what was stored.
func (b *bench) file(w http.ResponseWriter, r *http.Request) {
	var p id.Project
	var ch id.Chat
	b.ui(func() {
		if pl := b.st.Nav.Current(); pl != nil {
			p, ch = pl.Project, pl.Chat
		}
	})
	if ch == "" || b.st.Actions.Upload == nil {
		http.Error(w, "no chat open", http.StatusConflict)
		return
	}
	// Attach, as the composer does, so the Files panel hears of the file.
	type result struct {
		up  app.Uploaded
		err error
	}
	done := make(chan result, 1)
	path := r.URL.Query().Get("path")
	b.ui(func() {
		b.st.Actions.Attach(p, path, func(up app.Uploaded, err error) { done <- result{up, err} })
	})
	var res result
	select {
	case res = <-done:
	case <-time.After(30 * time.Second):
		res.err = errors.New("the upload took longer than 30 s")
	}
	if res.err != nil {
		http.Error(w, res.err.Error(), http.StatusInternalServerError)
		return
	}
	b.ui(func() { b.st.OpenFile(p, ch, res.up.Key) })
	json.NewEncoder(w).Encode(res.up)
}
