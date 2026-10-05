package store

import (
	"context"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
)

func TestApprovals(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	approved := func(kind, target string) bool {
		t.Helper()
		ok, err := p.Approved(ctx, kind, target)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	record := func(target string, g chat.Grant, at time.Time) id.Approval {
		t.Helper()
		a := Approval{ID: id.Approval(id.New()), Kind: "host", Target: target, Answer: g, Note: "n", Source: id.SourceUser, CreatedAt: at}
		if err := p.RecordApproval(ctx, a); err != nil {
			t.Fatal(err)
		}
		return a.ID
	}
	if approved("host", "example.com") {
		t.Fatal("approved before any decision")
	}
	at := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	record("example.com", chat.GrantDeny, at)
	if approved("host", "example.com") {
		t.Error("approved after a deny")
	}
	record("example.com", chat.GrantOnce, at.Add(time.Minute))
	if approved("host", "example.com") {
		t.Error("approved after a once")
	}
	always := record("example.com", chat.GrantAlways, at.Add(2*time.Minute))
	if !approved("host", "example.com") {
		t.Error("not approved after always")
	}
	if approved("host", "other.com") || approved("starlark", "example.com") {
		t.Error("an approval counts for another target or kind")
	}
	// The same time: the later row wins.
	last := record("example.com", chat.GrantDeny, at.Add(2*time.Minute))
	if approved("host", "example.com") {
		t.Error("an always still counts after a newer deny")
	}
	all, err := p.Approvals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 || all[0].ID != last || all[1].ID != always || !all[1].CreatedAt.Equal(at.Add(2*time.Minute)) {
		t.Errorf("Approvals = %+v", all)
	}
	if a := all[1]; a.Kind != "host" || a.Target != "example.com" || a.Answer != chat.GrantAlways || a.Note != "n" || a.Source != id.SourceUser {
		t.Errorf("read back %+v", a)
	}
	// No time set: now.
	if err := p.RecordApproval(ctx, Approval{ID: id.Approval(id.New()), Kind: "host", Target: "x.org", Answer: chat.GrantAlways, Source: id.SourceAuto}); err != nil {
		t.Fatal(err)
	}
	if all, _ = p.Approvals(ctx); time.Since(all[0].CreatedAt) > time.Minute {
		t.Errorf("created at %v", all[0].CreatedAt)
	}
}
