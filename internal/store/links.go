package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
)

// Saved links (SPEC 4.6): the sources the agent saves with save_link and
// the links the user adds, edits and deletes in the Links tab (P3-11).
// Each save and delete is one change, target link:<id>, with the link
// before and after it, so Undo turn, a delete's Undo and the conflict
// check cover links as they do views and memory (SPEC 2.5, 2.6). A
// deleted link's row goes; the change log keeps it for undo.

// linksTable is the saved links' table, and its UndoItem table.
const linksTable = "_jenab_links"

// linkTables is project.db format step 7: saved links (SPEC 4.6, P3-11).
// tags is a JSON array of strings.
var linkTables = execAll(
	`CREATE TABLE _jenab_links (
		id         TEXT PRIMARY KEY,
		url        TEXT NOT NULL UNIQUE,
		title      TEXT NOT NULL DEFAULT '',
		note       TEXT NOT NULL DEFAULT '',
		tags       TEXT NOT NULL DEFAULT '[]',
		created_by TEXT NOT NULL,
		created_at TEXT NOT NULL
	) WITHOUT ROWID`,
	`CREATE INDEX _jenab_links_created ON _jenab_links (created_at)`,
)

// The limits of a link's fields, in characters.
const (
	MaxLinkURL   = 2048
	MaxLinkTitle = 300
	MaxLinkNote  = 2000
	MaxLinkTags  = 20
	MaxLinkTag   = 50
)

// Link is a saved link.
type Link struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	Note      string    `json:"note"`
	Tags      []string  `json:"tags"`
	CreatedBy id.Source `json:"created_by"` // who saved it first
	CreatedAt time.Time `json:"created_at"`
}

// Target is the link's change-log target, link:<id>.
func (l Link) Target() string { return LinkTarget(l.ID) }

// LinkTarget is a link's change-log target (SPEC 2.5).
func LinkTarget(lid string) string { return "link:" + lid }

// LinkSave is a save of one link. With ID "" it adds a link; a URL saved
// already is ErrLinkTaken, unless Merge, as save_link does, which then
// updates that link's title, note and tags. With an ID it edits that link,
// its URL too.
type LinkSave struct {
	ID     string
	URL    string
	Title  string
	Note   string
	Tags   []string
	Merge  bool
	Source id.Source
}

// LinkWrite says what a save or a delete did.
type LinkWrite struct {
	Link   Link      `json:"link"` // after a save, before a delete
	Op     string    `json:"op"`   // save or delete; "" when it changed nothing
	Change id.Change `json:"change,omitempty"`
}

// ErrLinkTaken is a save of a URL that another saved link has. It matches
// ErrInvalid.
var ErrLinkTaken = errors.New("store: this URL is saved already")

// LinkError is a link that can't be saved as it is, with a message for
// the user. It matches ErrInvalid.
type LinkError struct{ Msg string }

func (e *LinkError) Error() string        { return e.Msg }
func (e *LinkError) Is(target error) bool { return target == ErrInvalid }

func linkErr(msg string) error { return &LinkError{Msg: msg} }

// CleanLink trims a link's fields and checks them: the URL is an absolute
// URL with a scheme, on one line, without spaces, control or format
// characters such as direction overrides, and is normalized (NormalLinkURL);
// the title and the tags are on one line, without direction overrides or
// isolates, which could reorder what is shown around them; a tag has no
// comma, since the Links tab separates tags with commas; tags are kept
// once each, folded as search folds them (FoldText), in their order.
func CleanLink(s LinkSave) (LinkSave, error) {
	s.URL = strings.TrimSpace(s.URL)
	s.Title = strings.Join(strings.Fields(dropBidi(s.Title)), " ")
	s.Note = strings.TrimSpace(dropBidi(s.Note))
	switch {
	case s.URL == "":
		return s, linkErr("A link needs its URL.")
	case !utf8.ValidString(s.URL) || !utf8.ValidString(s.Title) || !utf8.ValidString(s.Note):
		return s, linkErr("A link is UTF-8 text.")
	case utf8.RuneCountInString(s.URL) > MaxLinkURL:
		return s, linkErr(fmt.Sprintf("A URL is at most %d characters.", MaxLinkURL))
	case strings.IndexFunc(s.URL, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) >= 0:
		return s, linkErr("A URL has no spaces, control characters or direction marks.")
	case utf8.RuneCountInString(s.Title) > MaxLinkTitle:
		return s, linkErr(fmt.Sprintf("A title is at most %d characters.", MaxLinkTitle))
	case utf8.RuneCountInString(s.Note) > MaxLinkNote:
		return s, linkErr(fmt.Sprintf("A note is at most %d characters.", MaxLinkNote))
	}
	u, err := NormalLinkURL(s.URL)
	if err != nil {
		return s, err
	}
	s.URL = u
	var tags []string
	seen := map[string]bool{}
	for _, t := range s.Tags {
		t = strings.Join(strings.Fields(dropBidi(t)), " ")
		if t == "" {
			continue
		}
		if strings.ContainsAny(t, ",،") {
			return s, linkErr("A tag has no comma: commas separate tags.")
		}
		if !utf8.ValidString(t) || utf8.RuneCountInString(t) > MaxLinkTag {
			return s, linkErr(fmt.Sprintf("A tag is at most %d characters.", MaxLinkTag))
		}
		if f := FoldText(t); !seen[f] {
			seen[f] = true
			tags = append(tags, t)
		}
	}
	if len(tags) > MaxLinkTags {
		return s, linkErr(fmt.Sprintf("A link has at most %d tags.", MaxLinkTags))
	}
	s.Tags = tags
	return s, nil
}

// NormalLinkURL is how a URL is saved, so one address is one link: the
// scheme and the host in lower case, and an http or https URL with no
// path gets "/", as https://Example.com is https://example.com/. The rest
// stays as written. A URL that doesn't parse, or has no scheme, is a
// *LinkError.
func NormalLinkURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	switch {
	case err != nil:
		return raw, linkErr("This URL can't be read. Check its host and its characters.")
	case u.Scheme == "":
		return raw, linkErr("A URL starts with its scheme, such as https://.")
	case u.Host == "" && u.Opaque == "" && u.Path == "":
		return raw, linkErr("A URL has an address after its scheme, such as https://example.com.")
	}
	scheme := strings.ToLower(u.Scheme)
	rest := raw[len(u.Scheme)+1:] // after "scheme:"
	if !strings.HasPrefix(rest, "//") {
		return scheme + ":" + rest, nil
	}
	rest = rest[2:]
	end := strings.IndexAny(rest, "/?#")
	if end < 0 {
		end = len(rest)
	}
	auth, tail := rest[:end], rest[end:]
	user, host := "", auth
	if i := strings.LastIndex(auth, "@"); i >= 0 {
		user, host = auth[:i+1], auth[i+1:]
	}
	if (scheme == "http" || scheme == "https") && !strings.HasPrefix(tail, "/") {
		tail = "/" + tail
	}
	return scheme + "://" + user + strings.ToLower(host) + tail, nil
}

// isBidi tells the direction embeddings, overrides and isolates (U+202A
// to U+202E, U+2066 to U+2069): in a title or a tag they could reorder
// the text shown after them. The marks LRM, RLM and ALM, and ZWNJ, stay.
func isBidi(r rune) bool { return r >= 0x202A && r <= 0x202E || r >= 0x2066 && r <= 0x2069 }

func dropBidi(s string) string {
	if strings.IndexFunc(s, isBidi) < 0 {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isBidi(r) {
			return -1
		}
		return r
	}, s)
}

// linkName is what the user calls a link: its title, or else its URL's
// host, or the URL itself.
func linkName(l *Link) string {
	if l == nil {
		return ""
	}
	if l.Title != "" {
		return l.Title
	}
	if u, err := url.Parse(l.URL); err == nil && u.Hostname() != "" {
		return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	}
	return l.URL
}

const linkCols = "id, url, title, note, tags, created_by, created_at"

func scanLink(r *sql.Rows) (Link, error) {
	var l Link
	var tags, at string
	if err := r.Scan(&l.ID, &l.URL, &l.Title, &l.Note, &tags, &l.CreatedBy, &at); err != nil {
		return l, err
	}
	if err := json.Unmarshal([]byte(tags), &l.Tags); err != nil {
		return l, fmt.Errorf("store: link %s's tags: %w", l.ID, err)
	}
	var err error
	l.CreatedAt, err = parseTime(at)
	return l, err
}

func linkWhere(tx *sql.Tx, where string, arg any) (*Link, error) {
	ls, err := queryTx(tx, "SELECT "+linkCols+" FROM "+linksTable+" WHERE "+where, []any{arg}, scanLink)
	if err != nil || len(ls) == 0 {
		return nil, err
	}
	return &ls[0], nil
}

func putLink(tx *sql.Tx, l Link) error {
	tags := l.Tags
	if tags == nil {
		tags = []string{}
	}
	tj, _ := json.Marshal(tags)
	_, err := tx.Exec(`INSERT INTO `+linksTable+` (`+linkCols+`) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET url = excluded.url, title = excluded.title, note = excluded.note,
			tags = excluded.tags, created_by = excluded.created_by, created_at = excluded.created_at`,
		l.ID, l.URL, l.Title, l.Note, string(tj), string(l.CreatedBy), formatTime(l.CreatedAt))
	if isUnique(err) {
		return fault("%w: %s", ErrLinkTaken, l.URL)
	}
	return err
}

// linkRev is a link as the change log keeps it in before and after: NULL
// for a link that isn't there.
func linkRev(l *Link) sql.NullString {
	if l == nil {
		return sql.NullString{}
	}
	b, _ := json.Marshal(l)
	return sql.NullString{String: string(b), Valid: true}
}

func parseLinkRev(s sql.NullString) (*Link, error) {
	if !s.Valid {
		return nil, nil
	}
	var l Link
	if err := json.Unmarshal([]byte(s.String), &l); err != nil {
		return nil, fmt.Errorf("store: a link revision: %w", err)
	}
	return &l, nil
}

// sameLink reports whether a link is as a revision left it; nil is a link
// that isn't there.
func sameLink(a, b *Link) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.URL == b.URL && a.Title == b.Title && a.Note == b.Note && strings.Join(a.Tags, "\x00") == strings.Join(b.Tags, "\x00") &&
		a.CreatedBy == b.CreatedBy && a.CreatedAt.Equal(b.CreatedAt)
}

func linkKind(before, after *Link) string {
	switch {
	case before == nil:
		return rowInserted
	case after == nil:
		return rowDeleted
	}
	return rowUpdated
}

// logLink writes the change-log entry of a link from cur to next, nil for
// a deleted one.
func logLink(tx *sql.Tx, src id.Source, lid string, cur, next *Link, op string) (id.Change, error) {
	c := newChange(src, LinkTarget(lid), op)
	c.before, c.after = linkRev(cur), linkRev(next)
	row := rowEntry{table: linksTable, key: LinkTarget(lid), kind: linkKind(cur, next), before: c.before}
	return c.id, writeChange(tx, c, []rowEntry{row})
}

// SaveLinkTx saves s in tx with its change-log entry: target link:<id>,
// op save. A save of what is there already writes nothing.
func SaveLinkTx(tx *sql.Tx, s LinkSave) (LinkWrite, error) {
	if err := checkSource(s.Source); err != nil {
		return LinkWrite{}, err
	}
	s, err := CleanLink(s)
	if err != nil {
		return LinkWrite{}, err
	}
	var cur *Link
	if s.ID != "" {
		if cur, err = linkWhere(tx, "id = ?", s.ID); err != nil {
			return LinkWrite{}, err
		}
		if cur == nil {
			return LinkWrite{}, fmt.Errorf("%w: no saved link %q", ErrNotFound, s.ID)
		}
	}
	same, err := linkWhere(tx, "url = ?", s.URL)
	if err != nil {
		return LinkWrite{}, err
	}
	switch {
	case same != nil && cur != nil && same.ID != cur.ID, same != nil && cur == nil && !s.Merge:
		return LinkWrite{}, fault("%w: %s", ErrLinkTaken, s.URL)
	case same != nil && cur == nil:
		// Merge, as save_link: what the save leaves empty stays as it is,
		// so the agent saving a source again keeps the user's title, note
		// and tags.
		cur = same
		if s.Title == "" {
			s.Title = cur.Title
		}
		if s.Note == "" {
			s.Note = cur.Note
		}
		if len(s.Tags) == 0 {
			s.Tags = cur.Tags
		}
	}
	next := Link{ID: id.New(), URL: s.URL, Title: s.Title, Note: s.Note, Tags: s.Tags, CreatedBy: s.Source, CreatedAt: now()}
	if cur != nil {
		next.ID, next.CreatedBy, next.CreatedAt = cur.ID, cur.CreatedBy, cur.CreatedAt
		if sameLink(cur, &next) {
			return LinkWrite{Link: *cur}, nil
		}
	}
	if err := putLink(tx, next); err != nil {
		return LinkWrite{}, err
	}
	cid, err := logLink(tx, s.Source, next.ID, cur, &next, "save")
	if err != nil {
		return LinkWrite{}, err
	}
	return LinkWrite{Link: next, Op: "save", Change: cid}, nil
}

// DeleteLinkTx deletes a saved link in tx with its change-log entry, op
// delete. One that isn't there is ErrNotFound.
func DeleteLinkTx(tx *sql.Tx, src id.Source, lid string) (LinkWrite, error) {
	if err := checkSource(src); err != nil {
		return LinkWrite{}, err
	}
	cur, err := linkWhere(tx, "id = ?", lid)
	if err != nil {
		return LinkWrite{}, err
	}
	if cur == nil {
		return LinkWrite{}, fmt.Errorf("%w: no saved link %q", ErrNotFound, lid)
	}
	if _, err := tx.Exec(`DELETE FROM `+linksTable+` WHERE id = ?`, lid); err != nil {
		return LinkWrite{}, err
	}
	cid, err := logLink(tx, src, lid, cur, nil, "delete")
	if err != nil {
		return LinkWrite{}, err
	}
	return LinkWrite{Link: *cur, Op: "delete", Change: cid}, nil
}

// SaveLink saves a link as an interactive write with its change-log entry
// (SPEC 2.5, 4.6).
func (p *ProjectDB) SaveLink(ctx context.Context, s LinkSave) (LinkWrite, error) {
	return doThen(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (LinkWrite, error) { return SaveLinkTx(tx, s) }, p.linkWritten)
}

// DeleteLink deletes a saved link, as SaveLink saves one, with op delete.
// UndoChange of its change puts it back.
func (p *ProjectDB) DeleteLink(ctx context.Context, src id.Source, lid string) (LinkWrite, error) {
	return doThen(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (LinkWrite, error) { return DeleteLinkTx(tx, src, lid) }, p.linkWritten)
}

func (p *ProjectDB) linkWritten(w LinkWrite) {
	if w.Op != "" {
		p.linksChanged(true)
	}
}

// Links lists the saved links, the newest first.
func (p *ProjectDB) Links(ctx context.Context) ([]Link, error) {
	return Query(ctx, p.DB, "SELECT "+linkCols+" FROM "+linksTable+" ORDER BY created_at DESC, id DESC", nil, scanLink)
}

// LinkByID reads one saved link; one that isn't there is ErrNotFound.
func (p *ProjectDB) LinkByID(ctx context.Context, lid string) (Link, error) {
	return readTx(ctx, p.DB, func(tx *sql.Tx) (Link, error) {
		l, err := linkWhere(tx, "id = ?", lid)
		if err != nil {
			return Link{}, err
		}
		if l == nil {
			return Link{}, fmt.Errorf("%w: no saved link %q", ErrNotFound, lid)
		}
		return *l, nil
	})
}

// linkHook is the function a project's link changes go to.
type linkHook struct {
	mu sync.Mutex
	fn func()
}

// OnLinks sets fn to be called once a change to the saved links is
// committed: saves, deletes, and the undos and redos that revert them. It
// runs on the writer right after the commit and must not block. The
// project sends it as the project:links event (P3-11).
func (p *ProjectDB) OnLinks(fn func()) {
	p.links.mu.Lock()
	defer p.links.mu.Unlock()
	p.links.fn = fn
}

func (p *ProjectDB) linksChanged(changed bool) {
	if !changed {
		return
	}
	p.links.mu.Lock()
	fn := p.links.fn
	p.links.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// linkName names the link of target link:<id> for a conflict: its title
// or host now, or as its latest change left it when it is gone; "" when
// neither can be read.
func (u *undoRun) linkName(target string) string {
	lid, _ := strings.CutPrefix(target, "link:")
	if l, err := linkWhere(u.tx, "id = ?", lid); err == nil && l != nil {
		return linkName(l)
	}
	var before, after sql.NullString
	err := u.tx.QueryRow(`SELECT before, after FROM _jenab_changes WHERE target = ? ORDER BY at DESC, id DESC LIMIT 1`, target).Scan(&before, &after)
	if err != nil {
		return ""
	}
	for _, rev := range []sql.NullString{after, before} {
		if l, err := parseLinkRev(rev); err == nil && l != nil {
			return linkName(l)
		}
	}
	return ""
}

// revertLink puts a saved link back as it was before the change c: a new
// link goes, a deleted one comes back, an edit is taken back. A link
// changed since is a conflict; so is a URL another link has now.
func (u *undoRun) revertLink(c *undoChange) error {
	lid, ok := strings.CutPrefix(c.target, "link:")
	if !ok || lid == "" {
		return fmt.Errorf("store: %q is no link target", c.target)
	}
	now, err := parseLinkRev(c.after)
	if err != nil {
		return err
	}
	want, err := parseLinkRev(c.before)
	if err != nil {
		return err
	}
	cur, err := linkWhere(u.tx, "id = ?", lid)
	if err != nil {
		return err
	}
	item := UndoItem{linksTable, c.target}
	if !sameLink(cur, now) {
		if !u.has(item) {
			name := linkName(cur)
			if name == "" {
				name = linkName(now)
			}
			u.conflict(UndoConflict{Item: item, Reason: ConflictChanged, Name: name, Detail: "the link changed since"})
		}
		return nil
	}
	if want != nil {
		other, err := linkWhere(u.tx, "url = ?", want.URL)
		if err != nil {
			return err
		}
		if other != nil && other.ID != lid {
			u.conflict(UndoConflict{Item: item, Reason: ConflictConstraint, Reached: &UndoItem{linksTable, other.Target()},
				Name: linkName(want), Detail: "another saved link has this URL now"})
			return nil
		}
		if err := putLink(u.tx, *want); err != nil {
			return err
		}
	} else if _, err := u.tx.Exec(`DELETE FROM `+linksTable+` WHERE id = ?`, lid); err != nil {
		return err
	}
	before := linkRev(cur)
	if err := u.log(c.id, []rowEntry{{table: linksTable, key: c.target, kind: linkKind(cur, want), before: before}}); err != nil {
		return err
	}
	w := u.entry(c.id)
	w.before, w.after = before, linkRev(want)
	u.links = true
	return nil
}
