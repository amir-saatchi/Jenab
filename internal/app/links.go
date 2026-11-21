package app

import (
	"context"
	"errors"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
)

// LinkService is a project's saved links for the Links tab (SPEC 4.6,
// P3-11): the list, and the user's adds, edits and deletes. Each write is
// a change by user in the change log, so a delete's Undo, Undo turn and
// the conflict check cover it. Changes, the agent's save_link too, come as
// the project:links event (EventLinks).
type LinkService struct {
	base
	projects *project.Manager
}

// Link is a saved link.
type Link struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	Note      string    `json:"note"`
	Tags      []string  `json:"tags"`
	CreatedBy id.Source `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

func link(l store.Link) Link {
	tags := l.Tags
	if tags == nil {
		tags = []string{}
	}
	return Link{ID: l.ID, URL: l.URL, Title: l.Title, Note: l.Note, Tags: tags, CreatedBy: l.CreatedBy, CreatedAt: l.CreatedAt}
}

// LinkInput is a link as the user adds or edits it.
type LinkInput struct {
	URL   string   `json:"url"`
	Title string   `json:"title"`
	Note  string   `json:"note"`
	Tags  []string `json:"tags"`
}

// LinkSaved is a save from the Links tab: Change undoes it.
type LinkSaved struct {
	Link   Link      `json:"link"`
	Change id.Change `json:"change,omitempty"` // "" when the save changed nothing
}

// linkUI turns the store's link errors into messages for the user.
func linkUI(err error) error {
	var le *store.LinkError
	switch {
	case errors.As(err, &le):
		return &UIError{Kind: KindInvalid, Message: le.Msg, Details: err.Error()}
	case errors.Is(err, store.ErrLinkTaken):
		return &UIError{Kind: KindInvalid, Message: "This URL is saved already.", Details: err.Error()}
	case errors.Is(err, store.ErrNotFound):
		return &UIError{Kind: KindNotFound, Message: "This link no longer exists. It was deleted.", Details: err.Error()}
	}
	return err
}

// List lists the project's saved links, the newest first.
func (s *LinkService) List(ctx context.Context, p id.Project) (out []Link, err error) {
	defer s.guard("links.list", &err)
	out = []Link{}
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		ls, err := proj.DB.Links(ctx)
		for _, l := range ls {
			out = append(out, link(l))
		}
		return err
	})
	return out, err
}

// Add saves a new link as the user. A URL saved already is refused.
func (s *LinkService) Add(ctx context.Context, p id.Project, in LinkInput) (out LinkSaved, err error) {
	defer s.guard("links.add", &err)
	return s.save(ctx, p, store.LinkSave{URL: in.URL, Title: in.Title, Note: in.Note, Tags: in.Tags, Source: id.SourceUser})
}

// Edit changes a saved link as the user, its URL too.
func (s *LinkService) Edit(ctx context.Context, p id.Project, lid string, in LinkInput) (out LinkSaved, err error) {
	defer s.guard("links.edit", &err)
	if lid == "" {
		return out, &UIError{Kind: KindInvalid, Message: "Choose a link to edit."}
	}
	return s.save(ctx, p, store.LinkSave{ID: lid, URL: in.URL, Title: in.Title, Note: in.Note, Tags: in.Tags, Source: id.SourceUser})
}

func (s *LinkService) save(ctx context.Context, p id.Project, ls store.LinkSave) (out LinkSaved, err error) {
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		w, err := proj.DB.SaveLink(ctx, ls)
		if err != nil {
			return linkUI(err)
		}
		out = LinkSaved{Link: link(w.Link), Change: w.Change}
		return nil
	})
	return out, err
}

// Delete deletes a saved link as the user. UndoDelete puts it back while
// the change is in the undo window (SPEC 2.6).
func (s *LinkService) Delete(ctx context.Context, p id.Project, lid string) (d Deleted, err error) {
	defer s.guard("links.delete", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		w, err := proj.DB.DeleteLink(ctx, id.SourceUser, lid)
		if err != nil {
			return linkUI(err)
		}
		d.Change = w.Change
		return nil
	})
	return d, err
}

// UndoDelete puts back a link that Delete removed. A link saved since
// with the same URL, or a later change to it, stops it.
func (s *LinkService) UndoDelete(ctx context.Context, p id.Project, change id.Change) (err error) {
	defer s.guard("links.undo_delete", &err)
	return open(ctx, s.projects, p, func(proj *project.Project) error {
		r, err := proj.DB.UndoChange(ctx, change, store.UndoOptions{})
		switch {
		case errors.Is(err, store.ErrUndoConflict):
			return &UIError{Kind: KindBusy, Message: undoDeleteConflict(r.Conflicts), Details: err.Error()}
		case errors.Is(err, store.ErrNothingToUndo):
			return &UIError{Kind: KindBusy, Message: "The delete is undone already, or past the undo window.", Details: err.Error()}
		}
		return err
	})
}

// undoDeleteConflict says why a link's delete can't be undone: another
// link has its URL now, or the link changed since the delete.
func undoDeleteConflict(cs []store.UndoConflict) string {
	for _, c := range cs {
		if c.Reason == store.ConflictConstraint {
			return "A link with this URL was saved since, so the delete can't be undone."
		}
	}
	return "The link changed after it was deleted, so the delete can't be undone."
}
