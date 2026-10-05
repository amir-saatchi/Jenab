package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
)

// Approval is one decision on an approval card, as _jenab_approvals keeps
// it (SPEC 8.8).
type Approval struct {
	ID        id.Approval
	Kind      string // host, migration, starlark, …
	Target    string // what is approved: a host, a script's hash
	Answer    chat.Grant
	Note      string
	Source    id.Source // user, or auto for the Auto level
	CreatedAt time.Time
}

// RecordApproval stores a decision. The time is set if empty.
func (p *ProjectDB) RecordApproval(ctx context.Context, a Approval) error {
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now()
	}
	_, err := Do(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		_, err := tx.Exec(`INSERT INTO _jenab_approvals (id, kind, target, answer, note, source, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			a.ID, a.Kind, a.Target, a.Answer, a.Note, a.Source, formatTime(a.CreatedAt))
		return struct{}{}, err
	})
	return err
}

// Approved reports whether the newest decision on the target is
// GrantAlways, so it isn't asked for again.
func (p *ProjectDB) Approved(ctx context.Context, kind, target string) (bool, error) {
	rows, err := Query(ctx, p.DB, `SELECT answer FROM _jenab_approvals WHERE kind = ? AND target = ?
		ORDER BY created_at DESC, rowid DESC LIMIT 1`, []any{kind, target}, func(r *sql.Rows) (chat.Grant, error) {
		var g chat.Grant
		return g, r.Scan(&g)
	})
	if err != nil || len(rows) == 0 {
		return false, err
	}
	return rows[0] == chat.GrantAlways, nil
}

// Approvals returns every decision, the newest first.
func (p *ProjectDB) Approvals(ctx context.Context) ([]Approval, error) {
	return Query(ctx, p.DB, `SELECT id, kind, target, answer, note, source, created_at FROM _jenab_approvals
		ORDER BY created_at DESC, rowid DESC`, nil, func(r *sql.Rows) (Approval, error) {
		var a Approval
		var at string
		if err := r.Scan(&a.ID, &a.Kind, &a.Target, &a.Answer, &a.Note, &a.Source, &at); err != nil {
			return a, err
		}
		var err error
		a.CreatedAt, err = parseTime(at)
		return a, err
	})
}
