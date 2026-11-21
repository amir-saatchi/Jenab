package logic

import (
	"fmt"
	"strings"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/store"
)

// The internal targets of an undo, as store names them.
const (
	objectItem        = "_jenab_objects"     // key object:<key>
	roleItem          = "_jenab_chat_role"   // store.RoleItem; key the chat ID
	projectMemoryItem = "_jenab_memory"      // key memory:project/<section>
	userMemoryItem    = "_jenab_user_memory" // store.UserMemoryItem; key the section
	viewItem          = "_jenab_views"       // key view:<id>
	pageItem          = "_jenab_pages"       // key page:<id>
	linkItem          = "_jenab_links"       // key link:<id>; the conflict's Name names it
)

// Isolate keeps a name's direction from reordering the sentence around it,
// as a Persian table name in English text would (U+2068 FSI ... U+2069 PDI).
func Isolate(s string) string {
	return fsi + s + pdi
}

// The isolate's marks: First Strong Isolate and Pop Directional Isolate.
const (
	fsi = string(rune(0x2068))
	pdi = string(rune(0x2069))
)

// keyText is a row's primary key, stored in SQLite's json_array form, as
// the user reads it: "[2]" is 2, "[1,\"a\"]" is 1, a. Each part is isolated.
func keyText(key string) string {
	if v, err := parseJSON([]byte(key)); err == nil && v.kind == jsArray {
		parts := make([]string, len(v.items))
		for i, p := range v.items {
			if p.kind == jsString {
				parts[i] = Isolate(p.s)
			} else {
				parts[i] = Isolate(p.stringify(""))
			}
		}
		return strings.Join(parts, ", ")
	}
	// shown as stored
	return Isolate(key)
}

// ItemName names what an undo would revert, for the conflict dialog. A
// key of "" is the whole table, as a later migration's conflict has it.
func ItemName(it store.UndoItem) string {
	switch {
	case it.Table == objectItem:
		return "the file " + Isolate(strings.TrimPrefix(it.Key, "object:"))
	case it.Table == roleItem:
		return "the chat's role"
	case it.Table == projectMemoryItem:
		return "the project memory section " + Isolate(strings.TrimPrefix(it.Key, "memory:project/"))
	case it.Table == userMemoryItem:
		return "the user memory section " + Isolate(it.Key)
	case it.Table == viewItem:
		return "the view " + Isolate(strings.TrimPrefix(it.Key, "view:"))
	case it.Table == pageItem:
		return "the page " + Isolate(strings.TrimPrefix(it.Key, "page:"))
	case it.Table == linkItem:
		return "a saved link"
	case it.Key == "":
		return "the table " + Isolate(it.Table)
	}
	return fmt.Sprintf("row %s of %s", keyText(it.Key), Isolate(it.Table))
}

// changedBy says who made a later change. A message whose chat isn't in
// this project is, for user memory, a chat of another project.
func changedBy(c agent.TurnConflict) string {
	src := string(c.Conflict.Source)
	switch {
	case c.ChatTitle != "":
		return " in the chat “" + Isolate(c.ChatTitle) + "”"
	case src == "user":
		return " by you"
	case strings.HasPrefix(src, "message:"):
		if c.Conflict.Item.Table == userMemoryItem {
			return " by a chat in another project"
		}
		return " by another chat"
	case strings.HasPrefix(src, "undo:"):
		return " by an undo"
	case src != "":
		return " by " + src
	}
	return ""
}

// conflictItem names a conflict's item: a saved link by the title or host
// the store gave as its Name, as its key is only an ID; else ItemName.
func conflictItem(x store.UndoConflict) string {
	if x.Item.Table == linkItem && x.Name != "" {
		return "the link " + Isolate(x.Name)
	}
	return ItemName(x.Item)
}

// ConflictText says why an item stands in the undo's way.
func ConflictText(c agent.TurnConflict) string {
	x := c.Conflict
	detail := ""
	if x.Detail != "" {
		detail = ": " + x.Detail
	}
	switch x.Reason {
	case store.ConflictChanged:
		return conflictItem(x) + " changed since" + changedBy(c)
	case store.ConflictCascade:
		if x.Reached != nil {
			return "undoing " + conflictItem(x) + " would also change " + ItemName(*x.Reached)
		}
		return "undoing " + conflictItem(x) + " would change other rows"
	case store.ConflictReferenced:
		if x.Reached != nil {
			return ItemName(*x.Reached) + " refers to " + conflictItem(x)
		}
		return conflictItem(x) + " refers to a row that is gone"
	case store.ConflictSchema:
		if x.Item.Key == "" {
			return "a later migration changed " + conflictItem(x)
		}
		return "the table or a column of " + conflictItem(x) + " is gone"
	case store.ConflictConstraint:
		return "putting back " + conflictItem(x) + " breaks a rule of its table" + detail
	}
	return conflictItem(x) + " can't be undone" + detail
}

// SkipAll adds the conflicts' items to skip, each once.
func SkipAll(skip []store.UndoItem, cs []agent.TurnConflict) []store.UndoItem {
	out := append([]store.UndoItem(nil), skip...)
	has := map[store.UndoItem]bool{}
	for _, it := range skip {
		has[it] = true
	}
	for _, c := range cs {
		if it := c.Conflict.Item; !has[it] {
			has[it] = true
			out = append(out, it)
		}
	}
	return out
}

// Rest is the way a partly undone turn still has to go.
type Rest string

const (
	RestUndo Rest = "undo"
	RestRedo Rest = "redo"
	RestBoth Rest = "" // it can't be told; both ways are the rest
)

// Partly is a turn partly undone, for the footer: its chip text, and
// which way the rest is still to go. That is the way of the turn's last
// undo or redo, whose items left are the other side's (TurnPlan.Left):
// "undo" after an undo, "redo" after a redo. It is "" when that can't be
// told; then both ways are the rest.
type Partly struct {
	Text string
	Rest Rest
}

// PartlyOf reads the turn's state: "partly undone: 2 items not undone"
// after an undo that skipped two, or that undid one of the turn's changes
// alone; "partly redone: 1 item not redone" after such a redo. It is nil
// when the turn is all in effect or all undone, or past the undo window,
// where it is history.
func PartlyOf(st *agent.TurnUndoState) *Partly {
	if st == nil || st.Undo.Changes == 0 || st.Redo.Changes == 0 || st.Undo.Past || st.Redo.Past {
		return nil
	}
	items := func(n int) string {
		if n == 1 {
			return "1 item"
		}
		return fmt.Sprintf("%d items", n)
	}
	if st.Undo.Left > 0 {
		return &Partly{Text: "partly undone: " + items(st.Undo.Left) + " not undone", Rest: RestUndo}
	}
	if st.Redo.Left > 0 {
		return &Partly{Text: "partly redone: " + items(st.Redo.Left) + " not redone", Rest: RestRedo}
	}
	return &Partly{Text: "partly undone", Rest: RestBoth}
}

// IsRest says whether an undo (redo false) or a redo of a partly undone
// turn is its rest: the way it still has to go.
func IsRest(p *Partly, redo bool) bool {
	if p == nil {
		return false
	}
	way := RestUndo
	if redo {
		way = RestRedo
	}
	return p.Rest == RestBoth || p.Rest == way
}
