package state

import (
	"context"
	"net/url"
	"strings"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
)

// The Links tab's saved links (SPEC 4.6, 5.12, P3-11): each project's
// list, its search, the add and edit dialog, and deletes with Undo.

// LinksAPI is what the Links tab reads and writes; *app.LinkService has
// it.
type LinksAPI interface {
	List(ctx context.Context, p id.Project) ([]app.Link, error)
	Add(ctx context.Context, p id.Project, in app.LinkInput) (app.LinkSaved, error)
	Edit(ctx context.Context, p id.Project, lid string, in app.LinkInput) (app.LinkSaved, error)
	Delete(ctx context.Context, p id.Project, lid string) (app.Deleted, error)
	UndoDelete(ctx context.Context, p id.Project, change id.Change) error
}

var _ LinksAPI = (*app.LinkService)(nil)

// LinkList is a project's saved links as the tab shows them, the newest
// first.
type LinkList struct {
	Links  []app.Link
	Loaded bool // the first read is in
	Err    string

	rev int
	seq int
}

// LinkEditor is the dialog that adds a link, or with an ID edits one.
// Tags is the tags as typed, split at commas.
type LinkEditor struct {
	Project id.Project
	ID      string // "" adds
	URL     string
	Title   string
	Note    string
	Tags    string
	Busy    bool   // the save runs
	Problem string // why the last save was refused
}

// Links is the saved links' store. Lists are read when the tab first
// shows a project, and again after each project:links event (Changed).
// Only the open project's are kept (Keep).
type Links struct {
	Lists map[id.Project]*LinkList
	// Query is each project's search, as typed.
	Query map[id.Project]string
	// Editor is the add or edit dialog; nil when it is closed. It belongs
	// to the Links tab of Editor.Project: the screen detaches it when that
	// tab isn't shown (Detach).
	Editor *LinkEditor

	api     LinksAPI
	post    Post
	onError func(error)
	revs    map[id.Project]int
	seq     int
}

// NewLinks makes the store; api may be nil, as in screens' tests that
// show no links. onError shows the errors no dialog takes.
func NewLinks(api LinksAPI, post Post, onError func(error)) *Links {
	return &Links{Lists: map[id.Project]*LinkList{}, Query: map[id.Project]string{}, api: api, post: post, onError: onError,
		revs: map[id.Project]int{}}
}

// Keep drops what the store holds for projects other than p, as another
// project opens: their lists, searches and counts of changes. Their tab
// reads them again if it shows them.
func (l *Links) Keep(p id.Project) {
	for q := range l.Lists {
		if q != p {
			delete(l.Lists, q)
		}
	}
	for q := range l.Query {
		if q != p {
			delete(l.Query, q)
		}
	}
	for q := range l.revs {
		if q != p {
			delete(l.revs, q)
		}
	}
}

// Changed marks p's list as stale: the tab reads it again as it shows it.
func (l *Links) Changed(p id.Project) { l.revs[p]++ }

// List is p's list: it is read when it isn't yet, or it changed since.
// What it shows stays until the new read is in.
func (l *Links) List(p id.Project) *LinkList {
	v := l.Lists[p]
	rev := l.revs[p]
	if v != nil && v.rev == rev {
		return v
	}
	l.seq++
	nv := &LinkList{rev: rev, seq: l.seq}
	if v != nil && v.Err == "" { // after an error, the read again shows its spinner
		nv.Links, nv.Loaded = v.Links, v.Loaded
	}
	l.Lists[p] = nv
	if l.api == nil {
		nv.Err, nv.Loaded = "Links can't be read here.", true
		return nv
	}
	seq := nv.seq
	go func() {
		links, err := l.api.List(background, p)
		l.post(func() {
			v := l.Lists[p]
			if v == nil || v.seq != seq {
				return
			}
			v.Loaded = true
			if err != nil {
				v.Err = errorText(err)
				return
			}
			v.Err, v.Links = "", links
		})
	}()
	return nv
}

// StartAdd opens the dialog to add a link to p, and reports whether it
// did: not while a save runs, whose answer the dialog waits for.
func (l *Links) StartAdd(p id.Project) bool {
	if l.Editor != nil && l.Editor.Busy {
		return false
	}
	l.Editor = &LinkEditor{Project: p}
	return true
}

// StartEdit opens the dialog to edit lk, as StartAdd.
func (l *Links) StartEdit(p id.Project, lk app.Link) bool {
	if l.Editor != nil && l.Editor.Busy {
		return false
	}
	l.Editor = &LinkEditor{Project: p, ID: lk.ID, URL: lk.URL, Title: lk.Title, Note: lk.Note, Tags: JoinTags(lk.Tags)}
	return true
}

// CloseEditor closes the dialog, unless its save runs.
func (l *Links) CloseEditor() {
	if l.Editor != nil && !l.Editor.Busy {
		l.Editor = nil
	}
}

// Detach closes the dialog as its tab goes away: another tab, the sidebar
// hidden, or another project. A save that runs goes on; a refusal then
// shows as a toast, as no dialog is left to show it.
func (l *Links) Detach() { l.Editor = nil }

// Save saves the dialog's link as the user. A refusal stays in the dialog
// as its Problem; a save closes it and reads the list again.
func (l *Links) Save() {
	e := l.Editor
	if e == nil || e.Busy {
		return
	}
	if l.api == nil {
		e.Problem = "Links can't be saved here."
		return
	}
	e.Busy, e.Problem = true, ""
	p, lid := e.Project, e.ID
	in := app.LinkInput{URL: strings.TrimSpace(e.URL), Title: e.Title, Note: e.Note, Tags: SplitTags(e.Tags)}
	go func() {
		var err error
		if lid == "" {
			_, err = l.api.Add(background, p, in)
		} else {
			_, err = l.api.Edit(background, p, lid, in)
		}
		l.post(func() {
			e.Busy = false
			if l.Editor != e { // detached meanwhile
				if err != nil {
					l.onError(err)
				} else {
					l.Changed(p)
				}
				return
			}
			if err != nil {
				e.Problem = errorText(err)
				return
			}
			l.Editor = nil
			l.Changed(p)
		})
	}()
}

// DeleteLink deletes a saved link as the user. The toast's Undo puts it
// back while the change is in the undo window, as a file's delete does.
func (s *Stores) DeleteLink(p id.Project, lk app.Link) {
	if s.links == nil {
		return
	}
	name := LinkTitle(lk)
	go func() {
		d, err := s.links.Delete(background, p, lk.ID)
		s.post(func() {
			if err != nil {
				s.Error(err)
				return
			}
			s.Links.Changed(p)
			if e := s.Links.Editor; e != nil && e.Project == p && e.ID == lk.ID && !e.Busy {
				s.Links.Editor = nil
			}
			s.Toasts = append(s.Toasts, Toast{Text: "Deleted " + name, Action: "Undo", OnAction: func() { s.undoDeleteLink(p, name, d.Change) }})
		})
	}()
}

func (s *Stores) undoDeleteLink(p id.Project, name string, change id.Change) {
	go func() {
		err := s.links.UndoDelete(background, p, change)
		s.post(func() {
			if err != nil {
				s.Error(err)
				return
			}
			s.Links.Changed(p)
			s.Toasts = append(s.Toasts, Toast{Text: name + " is back"})
		})
	}()
}

// LinkHost is what a row shows of a link's URL: an http or https link's
// host without www., a mailto link's address, or else the scheme, as
// "javascript:".
func LinkHost(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" {
		return raw
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if h := strings.ToLower(u.Hostname()); h != "" {
			return strings.TrimPrefix(h, "www.")
		}
	case "mailto":
		addr := u.Opaque
		if addr == "" {
			addr = u.Path
		}
		if a, err := url.PathUnescape(addr); err == nil && a != "" {
			return a
		}
	}
	return strings.ToLower(u.Scheme) + ":"
}

// LinkTitle is a link's title, or its host when it has none.
func LinkTitle(lk app.Link) string {
	if lk.Title != "" {
		return lk.Title
	}
	return LinkHost(lk.URL)
}

// SplitTags splits tags as typed at commas, Latin or Persian, and drops
// the empty ones. The store drops repeats.
func SplitTags(s string) []string {
	out := []string{}
	for _, t := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '،' }) {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// JoinTags is tags as the dialog shows them.
func JoinTags(tags []string) string { return strings.Join(tags, ", ") }

// foldMarks drops the direction marks, which store.FoldText keeps.
var foldMarks = strings.NewReplacer("\u200e", "", "\u200f", "", "\u061c", "")

// foldSearch is s as search compares it: store.FoldText, as the chat
// search folds text (SPEC 2.3): lower case, NFKC presentation forms as
// plain letters, Arabic yeh and kaf as the Persian ones, Persian and
// Arabic digits as Latin, harakat, tatweel and ZWNJ dropped, so
// "می‌خواهم" finds "میخواهم"; direction marks dropped too.
func foldSearch(s string) string { return store.FoldText(foldMarks.Replace(s)) }

// holds tells whether text holds the folded query q. A ZWNJ in text also
// matches a space in q, so "می خواهم" finds "می‌خواهم".
func holds(text, q string) bool {
	if strings.Contains(foldSearch(text), q) {
		return true
	}
	return strings.ContainsRune(text, '\u200c') && strings.Contains(foldSearch(strings.ReplaceAll(text, "\u200c", " ")), q)
}

// FilterLinks is the links whose title, host or a tag holds q; all of
// them when q is blank.
func FilterLinks(links []app.Link, q string) []app.Link {
	q = foldSearch(strings.Join(strings.Fields(q), " "))
	if q == "" {
		return links
	}
	var out []app.Link
	for _, lk := range links {
		hit := holds(lk.Title, q) || holds(LinkHost(lk.URL), q)
		for _, t := range lk.Tags {
			hit = hit || holds(t, q)
		}
		if hit {
			out = append(out, lk)
		}
	}
	return out
}
