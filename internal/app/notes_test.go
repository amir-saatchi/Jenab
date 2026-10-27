package app

import (
	"context"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
)

// The notes sheet reads a chat's session notes: none yet at revision 0,
// then the notes with their summary.
func TestChatNotes(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	op, err := e.svc.Project.Create(ctx, "Notes")
	if err != nil {
		t.Fatal(err)
	}
	if n, err := e.svc.Chat.Notes(ctx, op.ID, op.Mother); err != nil || n.Revision != 0 || n.Content != "" {
		t.Fatalf("no notes yet: %+v, %v", n, err)
	}
	err = open(ctx, e.pm, op.ID, func(p *project.Project) error {
		if _, err := p.Chats.SaveSummary(ctx, op.Mother, "Imported prices.", 0); err != nil {
			return err
		}
		_, err := p.Chats.SaveNotes(ctx, chat.SessionNote{Chat: op.Mother, Content: "برنامه: نمودار.", Revision: 1})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	n, err := e.svc.Chat.Notes(ctx, op.ID, op.Mother)
	if err != nil || n.Content != "برنامه: نمودار." || n.Summary != "Imported prices." || n.Revision != 2 || n.UpdatedAt.IsZero() {
		t.Errorf("notes: %+v, %v", n, err)
	}
	if _, err := e.svc.Chat.Notes(ctx, id.Project(id.New()), op.Mother); err == nil {
		t.Error("notes of no project")
	} else {
		uiErr(t, err, KindNotFound)
	}
}
