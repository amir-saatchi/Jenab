package skill

import (
	"embed"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/amir-saatchi/jenab/internal/chat"
)

// Set is the skills a chat can choose from, sorted by name. Names are
// unique. User and project skills join it in Phase 5.
type Set struct {
	list []Skill
}

// New makes a set; two skills with one name are an error.
func New(ss ...Skill) (*Set, error) {
	list := slices.Clone(ss)
	slices.SortFunc(list, func(a, b Skill) int { return strings.Compare(a.Name, b.Name) })
	for i := 1; i < len(list); i++ {
		if list[i].Name == list[i-1].Name {
			return nil, fmt.Errorf("skill: two skills are named %s", list[i].Name)
		}
	}
	return &Set{list: list}, nil
}

//go:embed builtin
var builtinFS embed.FS

// Builtin reads the skills embedded in the app (8.9). They are the same
// for every model (1). delegation, the one for Mother only, comes with
// Mother's guidance in Phase 5.
func Builtin() (*Set, error) {
	fsys, err := fs.Sub(builtinFS, "builtin")
	if err != nil {
		return nil, err
	}
	ents, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	var ss []Skill
	for _, e := range ents {
		s, err := Read(fsys, e.Name())
		if err != nil {
			return nil, err
		}
		ss = append(ss, s)
	}
	return New(ss...)
}

// For are the skills a chat of kind k can use. A nil set has none.
func (s *Set) For(k chat.Kind) []Skill {
	if s == nil {
		return nil
	}
	var out []Skill
	for _, sk := range s.list {
		if !sk.Mother || k == chat.KindMother {
			out = append(out, sk)
		}
	}
	return out
}

// Get finds a skill a chat of kind k can use.
func (s *Set) Get(k chat.Kind, name string) (Skill, bool) {
	for _, sk := range s.For(k) {
		if sk.Name == name {
			return sk, true
		}
	}
	return Skill{}, false
}

// With are the skills whose load_with names the tool.
func (s *Set) With(k chat.Kind, tool string) []Skill {
	var out []Skill
	for _, sk := range s.For(k) {
		if slices.Contains(sk.LoadWith, tool) {
			out = append(out, sk)
		}
	}
	return out
}

// listHeader starts the skill list. Its wording was tested in SPIKE-025.
const listHeader = "## Skills\nSkills hold the formats, rules and examples for one kind of work. " +
	"Before you do that kind of work, load its skill with load_skill(name); the text arrives as the tool result and stays in this chat. " +
	"Load only what the task needs."

// Block is block 1's part for skills (3.1, 8.9): the text of the chat's
// loaded skills, then the skill list, one line per skill. Loaded names
// the chat can't use, such as a removed skill, are skipped. It is "" when
// there are no skills.
func (s *Set) Block(k chat.Kind, loaded []string) string {
	var parts []string
	for _, n := range loaded {
		if sk, ok := s.Get(k, n); ok {
			parts = append(parts, sk.Text())
		}
	}
	if list := s.For(k); len(list) > 0 {
		var b strings.Builder
		b.WriteString(listHeader)
		for _, sk := range list {
			fmt.Fprintf(&b, "\n- %s: %s", sk.Name, sk.Description)
		}
		parts = append(parts, b.String())
	}
	return strings.Join(parts, "\n\n")
}
