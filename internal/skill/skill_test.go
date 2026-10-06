package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/amir-saatchi/jenab/internal/chat"
)

const good = "---\nname: sql-queries\ndescription: \"SQL: the dialect and the guard. Load to write or debug SQL.\"\nload_with: [query, save_view]\n---\nUse named parameters.\n"

func TestParse(t *testing.T) {
	s, err := Parse("sql-queries/SKILL.md", []byte(good))
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "sql-queries" || s.Description != "SQL: the dialect and the guard. Load to write or debug SQL." ||
		strings.Join(s.LoadWith, ",") != "query,save_view" || s.Body != "Use named parameters." {
		t.Errorf("got %+v", s)
	}
	// CRLF, a byte order mark, no load_with and a closing line at the end
	// of the file are fine.
	crlf := string(rune(0xFEFF)) + strings.ReplaceAll("---\nname: a\ndescription: d\n---\nbody\n---", "\n", "\r\n")
	if s, err := Parse("a/SKILL.md", []byte(crlf)); err != nil || s.Body != "body\n---" || s.LoadWith != nil {
		t.Errorf("CRLF: %+v, %v", s, err)
	}
	if s, err := Parse("a/SKILL.md", []byte("---\nname: a\ndescription: d\n---")); err == nil || err.(*Error).Field != "body" {
		t.Errorf("no body: %+v, %v", s, err)
	}
	// Fence lines may end in spaces; a load_with tool name may be 64
	// characters, as the registry allows.
	long := "a" + strings.Repeat("b", 63)
	if s, err := Parse("a/SKILL.md", []byte("--- \nname: a\ndescription: d\nload_with: ["+long+"]\n---  \nbody")); err != nil || s.Body != "body" {
		t.Errorf("spaces after ---: %+v, %v", s, err)
	}
}

// Each bad field gives its own error.
func TestParseErrors(t *testing.T) {
	fm := func(lines ...string) string {
		return "---\n" + strings.Join(lines, "\n") + "\n---\nThe body.\n"
	}
	name, desc := "name: a", "description: d"
	cases := []struct {
		src, field, msg string
	}{
		{"no front matter", "front matter", "must start"},
		{"---\nname: a\n", "front matter", "closes"},
		{fm("name: [a"), "front matter", "yaml"},
		{fm("- a"), "front matter", "yaml"},
		{fm(name, desc, "loadwith: [x]"), "loadwith", "unknown field"},
		{fm(desc), "name", "missing"},
		{fm("name: ''", desc), "name", "missing"},
		{fm("name: {a: b}", desc), "name", "must be text"},
		{fm("name: Big", desc), "name", "lowercase"},
		{fm("name: a--b", desc), "name", "lowercase"},
		{fm("name: -a", desc), "name", "lowercase"},
		{fm("name: "+strings.Repeat("a", MaxName+1), desc), "name", "longer than 64"},
		{fm(name), "description", "missing"},
		{fm(name, "description: '  '"), "description", "missing"},
		{fm(name, "description: [a]"), "description", "must be text"},
		{fm(name, "description: \"two\\nlines\""), "description", "one line"},
		{fm(name, `description: "a\Lb"`), "description", "one line"},
		{fm(name, `description: "a\Pb"`), "description", "one line"},
		{fm(name, `description: "a\Nb"`), "description", "one line"},
		{fm(name, `description: "a\vb"`), "description", "one line"},
		{fm(name, `description: "a\0b"`), "description", "control characters"},
		{fm(name, `description: "a\eb"`), "description", "control characters"},
		{fm(name, "description: "+strings.Repeat("é", MaxDescription+1)), "description", "201 characters"},
		{fm(name, desc, "load_with: query"), "load_with", "list of tool names"},
		{fm(name, desc, "load_with: [Query]"), "load_with", "not a tool name"},
		{fm(name, desc, "load_with: [save-view]"), "load_with", "not a tool name"},
		{fm(name, desc, "load_with: [query, query]"), "load_with", "twice"},
		{fm(name, desc, "load_with: [load_skill]"), "load_with", "can't load a skill"},
		{fm(name, desc, "load_with: [a"+strings.Repeat("b", 64)+"]"), "load_with", "not a tool name"},
		{"---\n---\nThe body.\n", "front matter", "empty"},
		{"---\n  \n---\nThe body.\n", "front matter", "empty"},
		{"---\nname: a\ndescription: d\n---\n  \n", "body", "missing"},
		{"---\nname: a\ndescription: d\n---\n" + strings.Repeat("word ", MaxBodyTokens), "body", "at most 3000"},
		{"\xff\xfe", "front matter", "UTF-8"},
	}
	for _, c := range cases {
		_, err := Parse("a/SKILL.md", []byte(c.src))
		var e *Error
		if !errors.As(err, &e) || e.Field != c.field || !strings.Contains(err.Error(), c.msg) || !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: %v; want %s: …%s…", c.src, err, c.field, c.msg)
		}
	}
	// The longest allowed values pass.
	long := fm("name: "+strings.Repeat("a", MaxName), "description: "+strings.Repeat("é", MaxDescription))
	if _, err := Parse("a/SKILL.md", []byte(long)); err != nil {
		t.Error(err)
	}
	full := "---\nname: a\ndescription: d\n---\n" + strings.Repeat("a", 4*MaxBodyTokens)
	if _, err := Parse("a/SKILL.md", []byte(full)); err != nil {
		t.Error(err)
	}
	if _, err := Parse("a/SKILL.md", []byte(full+"a")); err == nil {
		t.Error("a body one byte over the limit passed")
	}
}

func TestRead(t *testing.T) {
	fsys := fstest.MapFS{
		"sql-queries/SKILL.md":    {Data: []byte(good)},
		"sql-queries/recipes.md":  {Data: []byte("\r\nRecipe one.\r\n")},
		"sql-queries/window-2.md": {Data: []byte("Window functions.")},
	}
	s, err := Read(fsys, "sql-queries")
	if err != nil {
		t.Fatal(err)
	}
	if s.Files["recipes.md"] != "Recipe one." || strings.Join(s.FileNames(), ",") != "recipes.md,window-2.md" {
		t.Errorf("files %q", s.Files)
	}
	want := "Skill loaded: sql-queries\n\nUse named parameters.\n\nExtra files, read with load_skill(\"sql-queries\", file): recipes.md, window-2.md"
	if s.Text() != want {
		t.Errorf("Text = %q", s.Text())
	}
	if s, _ := Parse("a/SKILL.md", []byte(good)); strings.Contains(s.Text(), "Extra files") {
		t.Errorf("no files: %q", s.Text())
	}

	bad := []struct {
		name  string
		fsys  fstest.MapFS
		file  string
		field string
	}{
		{"folder name", fstest.MapFS{"sql/SKILL.md": {Data: []byte(good)}}, "sql/SKILL.md", "name"},
		{"subfolder", fstest.MapFS{"sql-queries/SKILL.md": {Data: []byte(good)}, "sql-queries/more/a.md": {Data: []byte("a")}}, "sql-queries/more", "file"},
		{"folder.md", fstest.MapFS{"sql-queries/SKILL.md": {Data: []byte(good)}, "sql-queries/more.md/a.md": {Data: []byte("a")}}, "sql-queries/more.md", "file"},
		{"not .md", fstest.MapFS{"sql-queries/SKILL.md": {Data: []byte(good)}, "sql-queries/a.txt": {Data: []byte("a")}}, "sql-queries/a.txt", "file"},
		{"upper case", fstest.MapFS{"sql-queries/SKILL.md": {Data: []byte(good)}, "sql-queries/A.md": {Data: []byte("a")}}, "sql-queries/A.md", "file"},
		{"empty", fstest.MapFS{"sql-queries/SKILL.md": {Data: []byte(good)}, "sql-queries/a.md": {Data: []byte(" \r\n")}}, "sql-queries/a.md", "file"},
		{"not UTF-8", fstest.MapFS{"sql-queries/SKILL.md": {Data: []byte(good)}, "sql-queries/a.md": {Data: []byte("\xff")}}, "sql-queries/a.md", "file"},
		{"bad SKILL.md", fstest.MapFS{"sql-queries/SKILL.md": {Data: []byte("x")}}, "sql-queries/SKILL.md", "front matter"},
		{"link", fstest.MapFS{"sql-queries/SKILL.md": {Data: []byte(good)}, "sql-queries/a.md": {Data: []byte("../../secret"), Mode: fs.ModeSymlink}}, "sql-queries/a.md", "file"},
		{"SKILL.md link", fstest.MapFS{"sql-queries/SKILL.md": {Data: []byte("../x/SKILL.md"), Mode: fs.ModeSymlink}}, "sql-queries/SKILL.md", "file"},
		{"big file", fstest.MapFS{"sql-queries/SKILL.md": {Data: []byte(good)}, "sql-queries/a.md": {Data: big(MaxFileBytes + 1)}}, "sql-queries/a.md", "file"},
		{"big SKILL.md", fstest.MapFS{"sql-queries/SKILL.md": {Data: append([]byte(good), big(MaxFileBytes)...)}}, "sql-queries/SKILL.md", "file"},
		{"many files", many(MaxFiles+1, 1), "sql-queries/f32.md", "file"},
		{"big folder", many(9, 60<<10), "sql-queries/f08.md", "file"},
	}
	for _, c := range bad {
		dir := strings.SplitN(c.file, "/", 2)[0]
		_, err := Read(c.fsys, dir)
		var e *Error
		if !errors.As(err, &e) || e.File != c.file || e.Field != c.field {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	if _, err := Read(fstest.MapFS{}, "none"); err == nil || errors.Is(err, ErrInvalid) {
		t.Errorf("no skill: %v", err)
	}
	// The limits themselves pass.
	if s, err := Read(many(MaxFiles, 1), "sql-queries"); err != nil || len(s.Files) != MaxFiles {
		t.Errorf("%d files: %v", MaxFiles, err)
	}
	// On Windows SKILL.md may be skill.md; it isn't an extra file.
	lower := fstest.MapFS{"sql-queries/SKILL.md": {Data: []byte(good)}, "sql-queries/skill.md": {Data: []byte(good)}}
	if s, err := Read(lower, "sql-queries"); err != nil || len(s.Files) != 0 {
		t.Errorf("skill.md: %v, files %q", err, s.FileNames())
	}
}

// big is n bytes of text.
func big(n int) []byte { return []byte(strings.Repeat("a", n)) }

// many is a skill with n extra files of size bytes, f00.md and on.
func many(n, size int) fstest.MapFS {
	fsys := fstest.MapFS{"sql-queries/SKILL.md": {Data: []byte(good)}}
	for i := range n {
		fsys[fmt.Sprintf("sql-queries/f%02d.md", i)] = &fstest.MapFile{Data: big(size)}
	}
	return fsys
}

func TestSet(t *testing.T) {
	a := Skill{Name: "b-skill", Description: "B.", LoadWith: []string{"save_view"}, Body: "Body B."}
	b := Skill{Name: "a-skill", Description: "A.", LoadWith: []string{"save_view", "query"}, Body: "Body A."}
	m := Skill{Name: "delegation", Description: "Mother.", LoadWith: []string{"query"}, Body: "Body M.", Mother: true}
	if _, err := New(a, b, a); err == nil {
		t.Error("two skills with one name")
	}
	s, err := New(a, m, b)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(s.For(chat.KindChat)); got != "a-skill,b-skill" {
		t.Errorf("For(chat) = %s", got)
	}
	if got := names(s.For(chat.KindMother)); got != "a-skill,b-skill,delegation" {
		t.Errorf("For(mother) = %s", got)
	}
	if _, ok := s.Get(chat.KindChat, "delegation"); ok {
		t.Error("a chat got Mother's skill")
	}
	if sk, ok := s.Get(chat.KindMother, "delegation"); !ok || sk.Body != "Body M." {
		t.Errorf("Mother: %+v", sk)
	}
	if got := names(s.With(chat.KindChat, "save_view")); got != "a-skill,b-skill" {
		t.Errorf("With(save_view) = %s", got)
	}
	if got := names(s.With(chat.KindChat, "query")); got != "a-skill" {
		t.Errorf("With(query) = %s", got)
	}

	want := "Skill loaded: b-skill\n\nBody B.\n\n" + listHeader + "\n- a-skill: A.\n- b-skill: B."
	if got := s.Block(chat.KindChat, []string{"b-skill", "delegation", "gone"}); got != want {
		t.Errorf("Block = %q", got)
	}
	if got := s.Block(chat.KindMother, nil); !strings.HasSuffix(got, "\n- delegation: Mother.") || strings.Contains(got, "Skill loaded") {
		t.Errorf("Mother's block = %q", got)
	}
	var none *Set
	if none.Block(chat.KindChat, []string{"a-skill"}) != "" || none.With(chat.KindChat, "query") != nil {
		t.Error("a nil set has skills")
	}
	if empty, _ := New(); empty.Block(chat.KindChat, nil) != "" {
		t.Error("an empty set has a list")
	}
}

func TestBuiltin(t *testing.T) {
	s, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	cg, ok := s.Get(chat.KindChat, "config-guide")
	if !ok || strings.Join(cg.LoadWith, ",") != "save_view,save_page" || !strings.Contains(cg.Description, "Load before save_view or save_page") {
		t.Fatalf("config-guide: %+v", cg)
	}
}

func names(ss []Skill) string {
	var ns []string
	for _, s := range ss {
		ns = append(ns, s.Name)
	}
	return strings.Join(ns, ",")
}
