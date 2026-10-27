package app

import (
	"slices"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/project"
)

func TestPublisher(t *testing.T) {
	s := &sink{}
	p := &Publisher{emit: s.emit}
	p.Delta(chat.Delta{})
	p.Part(chat.PartDone{})
	p.Status(chat.Status{})
	p.Notice(project.Notice{})
	p.Activity(project.Activity{})
	want := []string{"chat:delta", "chat:part", "chat:status", "project:notice", "project:activity"}
	if got := s.names(); !slices.Equal(got, want) {
		t.Errorf("events %v", got)
	}
	// The payloads go out as the registered types, which Wails checks.
	if _, ok := s.events[0].data.(chat.Delta); !ok {
		t.Errorf("delta payload %T", s.events[0].data)
	}
}
