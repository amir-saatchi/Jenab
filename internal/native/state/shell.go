package state

import (
	"math"
	"strings"

	"github.com/amir-saatchi/jenab/internal/id"
)

// Docked is what the main area shows next to the chat. Files come first;
// pages and pipelines add their kinds (SPEC 5.9).
type Docked struct {
	Kind string // "file"
	Key  string
}

// DockSide is the side the dock is on.
type DockSide string

const (
	DockLeft  DockSide = "left"
	DockRight DockSide = "right"
)

// MinDock is the narrowest the dock gets, and the narrowest the main area
// next to it gets (SPEC 5.12).
const MinDock = 320

// ClampWidth keeps the dock at least MinDock wide and leaves the main area
// as much; a box too narrow for both gives the dock MinDock.
func ClampWidth(px, box float64) float64 {
	return math.Max(MinDock, math.Min(px, box-MinDock))
}

// RightTab is the right sidebar's tab (SPEC 5.9).
type RightTab string

const (
	TabPages     RightTab = "pages"
	TabPipelines RightTab = "pipelines"
	TabFiles     RightTab = "files"
	TabLinks     RightTab = "links"
)

// MemoryFocus is a memory section to open for editing in the settings, the
// [edit] of a chat's chip (SPEC 3.3): its project and target,
// memory:<scope>/<section>. The settings page that shows it clears it.
type MemoryFocus struct {
	Project id.Project
	Target  string
}

// Shell is what is open in the shell besides the main area: the dock
// (state/dock.ts), the overlays and tabs (ui.ts) and the Files panel's
// place (files.ts). What is docked, and whether the chat is hidden,
// belong to each chat; the dock's side and width to the project, its last
// choice (SPEC 5.9, 5.12). It lives for the session.
type Shell struct {
	Open       map[ChatRef]Docked
	ChatHidden map[ChatRef]bool
	Drawer     map[ChatRef]bool // the chat's drawer below 900 px
	Side       map[id.Project]DockSide
	Width      map[id.Project]int // px; none is a third
	// ListShown is the user's choice to show the chat list while something
	// is docked; it hides by default then (SPEC 5.12).
	ListShown bool

	NewProject bool
	// The overlays below 900 px (SPEC 5.12): the rail with the chats, and
	// the right sidebar.
	LeftOverlay, RightOverlay bool
	MemoryFocus               *MemoryFocus
	RightTab                  RightTab

	// Folder is the Files panel's folder by project: "" for the top, else
	// ending in "/". Rev counts the changes made from the app, by project,
	// so the panel lists the folder again.
	Folder map[id.Project]string
	Rev    map[id.Project]int
}

// NewShell is the shell as the app starts.
func NewShell() *Shell {
	return &Shell{
		Open: map[ChatRef]Docked{}, ChatHidden: map[ChatRef]bool{}, Drawer: map[ChatRef]bool{},
		Side: map[id.Project]DockSide{}, Width: map[id.Project]int{},
		RightTab: TabPages, Folder: map[id.Project]string{}, Rev: map[id.Project]int{},
	}
}

// Dock shows d next to the chat. Docking into a chat with nothing docked
// hides the chat list again.
func (s *Shell) Dock(c ChatRef, d Docked) {
	if _, ok := s.Open[c]; !ok {
		s.ListShown = false
	}
	s.Open[c] = d
	s.Drawer[c] = false
}

// Close closes what is docked next to the chat.
func (s *Shell) Close(c ChatRef) {
	delete(s.Open, c)
	s.ChatHidden[c] = false
	s.Drawer[c] = false
}

// CloseFile closes key wherever it is open in project, as after a delete.
func (s *Shell) CloseFile(p id.Project, key string) {
	for c, d := range s.Open {
		if d.Kind == "file" && d.Key == key && c.Project == p {
			delete(s.Open, c)
		}
	}
}

// Swap moves the project's dock to the other side.
func (s *Shell) Swap(p id.Project) {
	if s.Side[p] == DockLeft {
		s.Side[p] = DockRight
	} else {
		s.Side[p] = DockLeft
	}
}

// SetWidth sets the project's dock width, in whole pixels.
func (s *Shell) SetWidth(p id.Project, px float64) { s.Width[p] = int(math.Round(px)) }

// SetFolder sets the Files panel's folder.
func (s *Shell) SetFolder(p id.Project, prefix string) {
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	s.Folder[p] = prefix
}

// Changed tells the Files panel the project's bucket changed from the app.
func (s *Shell) Changed(p id.Project) { s.Rev[p]++ }
