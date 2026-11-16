//go:build bench

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

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
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	up, err := b.st.Actions.Upload(ctx, p, r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	b.ui(func() { b.st.OpenFile(p, ch, up.Key) })
	json.NewEncoder(w).Encode(up)
}
