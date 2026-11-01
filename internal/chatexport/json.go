package chatexport

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
)

// header is the JSON export without its messages, which are streamed
// after it.
type header struct {
	Format     string           `json:"format"`
	Version    int              `json:"version"`
	ExportedAt time.Time        `json:"exported_at"`
	Project    projectRef       `json:"project"`
	Chat       chat.Chat        `json:"chat"`
	Notes      chat.SessionNote `json:"notes"`
}

type projectRef struct {
	ID   id.Project `json:"id"`
	Name string     `json:"name"`
}

// writeJSON writes {header fields…, "messages": [one message per line]}.
// Every part is as stored (SPEC 2.3), thinking included. Each piece is
// encoded, then redacted as text, so a key is caught in any field, raw or
// JSON-escaped.
func writeJSON(ctx context.Context, w io.Writer, s Source) (int, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	h := header{Format: FormatName, Version: Version, ExportedAt: s.Now.In(s.Loc), Project: projectRef{s.Project, s.ProjectName}, Chat: s.Chat, Notes: s.Notes}
	if err := enc.Encode(h); err != nil {
		return 0, err
	}
	head := bytes.TrimSuffix(bytes.TrimSpace(buf.Bytes()), []byte("}"))
	if _, err := io.WriteString(w, s.Redact(string(head))+`,"messages":[`); err != nil {
		return 0, err
	}
	n := 0
	err := eachBatch(ctx, s, func(ms []chat.Message) error {
		for _, m := range ms {
			buf.Reset()
			if n > 0 {
				buf.WriteByte(',')
			}
			buf.WriteByte('\n')
			if err := enc.Encode(m); err != nil {
				return err
			}
			buf.Truncate(buf.Len() - 1) // Encode's newline
			if _, err := io.WriteString(w, s.Redact(buf.String())); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	if err != nil {
		return n, err
	}
	_, err = io.WriteString(w, "\n]}\n")
	return n, err
}
