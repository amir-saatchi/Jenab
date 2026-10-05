// Package skill reads skills (SPEC 8.9): Markdown instructions for one
// kind of work, which a chat loads only when it needs them, so requests
// stay short and provider caches stay valid.
//
// A skill is a folder: SKILL.md with front matter and the body, and
// optional extra .md files next to it, read with load_skill(name, file).
package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

const (
	MaxName         = 64   // characters
	MaxDescription  = 200  // characters, on one line
	MaxBodyTokens   = 3000 // longer material goes in extra files
	MaxLoaded       = 6    // skills loaded in one chat
	MaxLoadedTokens = 10000

	// Main is the skill's own file in its folder.
	Main = "SKILL.md"
)

// Skill is one skill.
type Skill struct {
	Name        string
	Description string
	LoadWith    []string // tools whose first call in a chat also loads the skill
	Body        string
	Files       map[string]string // extra files by name, e.g. "recipes.md"
	// Mother: only the Mother chat lists it, as delegation (8.6). Set by
	// the app for built-in skills, never by a file.
	Mother bool
}

// Error is a problem with one field of a skill, so each one can be shown
// where it is.
type Error struct {
	File  string // e.g. "config-guide/SKILL.md"
	Field string // front matter, name, description, load_with, body, or file
	Msg   string
}

func (e *Error) Error() string { return fmt.Sprintf("skill %s: %s: %s", e.File, e.Field, e.Msg) }

// ErrInvalid matches every *Error.
var ErrInvalid = errors.New("skill: invalid")

func (e *Error) Is(target error) bool { return target == ErrInvalid }

// Tokens is a rough count, 4 bytes a token, as the agent counts requests.
func Tokens(s string) int { return (len(s) + 3) / 4 }

var (
	nameRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	toolRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	fileRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*\.md$`)
	bom    = string(rune(0xFEFF))
)

// clean is a file's text with LF line ends and no byte order mark.
func clean(b []byte) string {
	return strings.TrimPrefix(strings.ReplaceAll(string(b), "\r\n", "\n"), bom)
}

// Parse reads a SKILL.md; file names it in errors.
func Parse(file string, src []byte) (Skill, error) {
	bad := func(field, format string, a ...any) (Skill, error) {
		return Skill{}, &Error{File: file, Field: field, Msg: fmt.Sprintf(format, a...)}
	}
	if !utf8.Valid(src) {
		return bad("front matter", "the file is not UTF-8")
	}
	s := clean(src)
	if !strings.HasPrefix(s, "---\n") {
		return bad("front matter", "the file must start with a --- line")
	}
	fm, body, ok := strings.Cut(s[4:], "\n---\n")
	if !ok {
		if fm, ok = strings.CutSuffix(s[4:], "\n---"); !ok {
			return bad("front matter", "no --- line closes it")
		}
	}
	var keys map[string]yaml.Node
	if err := yaml.Unmarshal([]byte(fm), &keys); err != nil {
		return bad("front matter", "%v", err)
	}
	for _, k := range slices.Sorted(maps.Keys(keys)) {
		if k != "name" && k != "description" && k != "load_with" {
			return bad(k, "unknown field; the fields are name, description and load_with")
		}
	}
	var sk Skill
	text := func(field string) (string, error) {
		n, ok := keys[field]
		if !ok {
			return "", nil
		}
		var v string
		if err := n.Decode(&v); err != nil {
			return "", &Error{File: file, Field: field, Msg: "must be text"}
		}
		return strings.TrimSpace(v), nil
	}

	name, err := text("name")
	switch {
	case err != nil:
		return Skill{}, err
	case name == "":
		return bad("name", "missing")
	case utf8.RuneCountInString(name) > MaxName:
		return bad("name", "longer than %d characters", MaxName)
	case !nameRE.MatchString(name):
		return bad("name", "%q: use lowercase letters, digits and -", name)
	}
	sk.Name = name

	d, err := text("description")
	switch {
	case err != nil:
		return Skill{}, err
	case d == "":
		return bad("description", "missing")
	case strings.ContainsAny(d, "\r\n"):
		return bad("description", "must be one line")
	case utf8.RuneCountInString(d) > MaxDescription:
		return bad("description", "%d characters; at most %d", utf8.RuneCountInString(d), MaxDescription)
	}
	sk.Description = d

	if n, ok := keys["load_with"]; ok {
		var with []string
		if err := n.Decode(&with); err != nil {
			return bad("load_with", "must be a list of tool names")
		}
		for _, t := range with {
			switch {
			case !toolRE.MatchString(t):
				return bad("load_with", "%q is not a tool name", t)
			case slices.Contains(sk.LoadWith, t):
				return bad("load_with", "%q is listed twice", t)
			}
			sk.LoadWith = append(sk.LoadWith, t)
		}
	}

	body = strings.TrimSpace(body)
	switch {
	case body == "":
		return bad("body", "missing")
	case Tokens(body) > MaxBodyTokens:
		return bad("body", "about %d tokens; at most %d, so put longer material in extra files", Tokens(body), MaxBodyTokens)
	}
	sk.Body = body
	return sk, nil
}

// Read reads the skill in folder name of fsys: its SKILL.md and extra
// files. The skill's name must be the folder's.
func Read(fsys fs.FS, name string) (Skill, error) {
	file := path.Join(name, Main)
	src, err := fs.ReadFile(fsys, file)
	if err != nil {
		return Skill{}, err
	}
	s, err := Parse(file, src)
	if err != nil {
		return Skill{}, err
	}
	if s.Name != name {
		return Skill{}, &Error{File: file, Field: "name", Msg: fmt.Sprintf("%q differs from its folder %q", s.Name, name)}
	}
	ents, err := fs.ReadDir(fsys, name)
	if err != nil {
		return Skill{}, err
	}
	for _, e := range ents {
		n := e.Name()
		if n == Main {
			continue
		}
		f := path.Join(name, n)
		bad := func(msg string) (Skill, error) {
			return Skill{}, &Error{File: f, Field: "file", Msg: msg}
		}
		if e.IsDir() {
			return bad("a skill has no folders")
		}
		if !fileRE.MatchString(n) {
			return bad("use a .md name of lowercase letters, digits and -")
		}
		b, err := fs.ReadFile(fsys, f)
		if err != nil {
			return Skill{}, err
		}
		if !utf8.Valid(b) {
			return bad("the file is not UTF-8")
		}
		text := strings.TrimSpace(clean(b))
		if text == "" {
			return bad("the file is empty")
		}
		if s.Files == nil {
			s.Files = map[string]string{}
		}
		s.Files[n] = text
	}
	return s, nil
}

// FileNames are the skill's extra files, sorted.
func (s Skill) FileNames() []string {
	ns := make([]string, 0, len(s.Files))
	for n := range s.Files {
		ns = append(ns, n)
	}
	slices.Sort(ns)
	return ns
}

// Text is a loaded skill as the chat sees it: as load_skill's result,
// with a load_with tool's result, and in block 1 after a cut.
func (s Skill) Text() string {
	t := "Skill loaded: " + s.Name + "\n\n" + s.Body
	if len(s.Files) > 0 {
		t += "\n\nExtra files, read with load_skill(\"" + s.Name + "\", file): " + strings.Join(s.FileNames(), ", ")
	}
	return t
}
