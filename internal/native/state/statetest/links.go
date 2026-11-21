package statetest

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/id"
)

// FakeLinks is the saved links of the Fake's projects (P3-11), the newest
// first, as the Links tab reads them.
type FakeLinks struct {
	ByProject map[id.Project][]app.Link
	Deleted   map[id.Change]deletedLink
	n         int
}

type deletedLink struct {
	p    id.Project
	link app.Link
	at   int
}

// linkEpoch is when the Fake's links are saved: a fixed time, each one a
// minute after the one before.
var linkEpoch = time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)

// AddLink saves a link in p as by, as the agent's save_link or the user
// would, and returns it.
func (f *Fake) AddLink(p id.Project, by id.Source, url, title, note string, tags ...string) app.Link {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.addLink(p, by, app.LinkInput{URL: url, Title: title, Note: note, Tags: tags})
}

func (f *Fake) addLink(p id.Project, by id.Source, in app.LinkInput) app.Link {
	fl := &f.Links
	fl.n++
	if fl.ByProject == nil {
		fl.ByProject = map[id.Project][]app.Link{}
	}
	tags := in.Tags
	if tags == nil {
		tags = []string{}
	}
	l := app.Link{ID: fmt.Sprintf("lk%03d", fl.n), URL: in.URL, Title: in.Title, Note: in.Note, Tags: tags,
		CreatedBy: by, CreatedAt: linkEpoch.Add(time.Duration(fl.n) * time.Minute)}
	fl.ByProject[p] = slices.Insert(fl.ByProject[p], 0, l)
	return l
}

// links is the Fake as the Links tab's service.
type links struct{ f *Fake }

func (l links) List(_ context.Context, p id.Project) ([]app.Link, error) {
	l.f.mu.Lock()
	defer l.f.mu.Unlock()
	return append([]app.Link{}, l.f.Links.ByProject[p]...), nil
}

// check refuses what the store refuses that the tests use: no URL, a URL
// without its scheme, a tag with a comma, and a URL saved already other
// than by lid.
func (l links) check(p id.Project, lid string, in app.LinkInput) error {
	u := strings.TrimSpace(in.URL)
	switch {
	case u == "":
		return &app.UIError{Kind: app.KindInvalid, Message: "A link needs its URL."}
	case !strings.Contains(u, ":"):
		return &app.UIError{Kind: app.KindInvalid, Message: "A URL starts with its scheme, such as https://."}
	}
	for _, t := range in.Tags {
		if strings.ContainsAny(t, ",،") {
			return &app.UIError{Kind: app.KindInvalid, Message: "A tag has no comma: commas separate tags."}
		}
	}
	for _, o := range l.f.Links.ByProject[p] {
		if o.URL == u && o.ID != lid {
			return &app.UIError{Kind: app.KindInvalid, Message: "This URL is saved already."}
		}
	}
	return nil
}

func (l links) Add(_ context.Context, p id.Project, in app.LinkInput) (app.LinkSaved, error) {
	l.f.mu.Lock()
	defer l.f.mu.Unlock()
	if err := l.check(p, "", in); err != nil {
		return app.LinkSaved{}, err
	}
	in.URL = strings.TrimSpace(in.URL)
	return app.LinkSaved{Link: l.f.addLink(p, id.SourceUser, in), Change: "add"}, nil
}

func (l links) Edit(_ context.Context, p id.Project, lid string, in app.LinkInput) (app.LinkSaved, error) {
	l.f.mu.Lock()
	defer l.f.mu.Unlock()
	ls := l.f.Links.ByProject[p]
	i := slices.IndexFunc(ls, func(x app.Link) bool { return x.ID == lid })
	if i < 0 {
		return app.LinkSaved{}, notFound()
	}
	if err := l.check(p, lid, in); err != nil {
		return app.LinkSaved{}, err
	}
	tags := in.Tags
	if tags == nil {
		tags = []string{}
	}
	ls[i].URL, ls[i].Title, ls[i].Note, ls[i].Tags = strings.TrimSpace(in.URL), in.Title, in.Note, tags
	return app.LinkSaved{Link: ls[i], Change: "edit"}, nil
}

func (l links) Delete(_ context.Context, p id.Project, lid string) (app.Deleted, error) {
	l.f.mu.Lock()
	defer l.f.mu.Unlock()
	fl := &l.f.Links
	ls := fl.ByProject[p]
	i := slices.IndexFunc(ls, func(x app.Link) bool { return x.ID == lid })
	if i < 0 {
		return app.Deleted{}, notFound()
	}
	fl.n++
	ch := id.Change(fmt.Sprintf("lch%03d", fl.n))
	if fl.Deleted == nil {
		fl.Deleted = map[id.Change]deletedLink{}
	}
	fl.Deleted[ch] = deletedLink{p, ls[i], i}
	fl.ByProject[p] = slices.Delete(ls, i, i+1)
	return app.Deleted{Change: ch}, nil
}

func (l links) UndoDelete(_ context.Context, p id.Project, change id.Change) error {
	l.f.mu.Lock()
	defer l.f.mu.Unlock()
	fl := &l.f.Links
	d, ok := fl.Deleted[change]
	if !ok || d.p != p {
		return &app.UIError{Kind: app.KindBusy, Message: "The delete is undone already, or past the undo window."}
	}
	delete(fl.Deleted, change)
	at := min(d.at, len(fl.ByProject[p]))
	fl.ByProject[p] = slices.Insert(fl.ByProject[p], at, d.link)
	return nil
}
