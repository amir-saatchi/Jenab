package tool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/web"
)

// testEnv is a project with its Mother chat and one more chat; calls run
// in the second chat.
func testEnv(t *testing.T) *Env {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.OpenProject(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(ctx) })
	chats, err := store.OpenChats(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { chats.Close(ctx) })
	if _, err := chats.EnsureMother(ctx); err != nil {
		t.Fatal(err)
	}
	c, err := chats.CreateChat(ctx, id.SourceUser, chat.Chat{Title: "Prices", Role: "Track coin prices.\nDaily."})
	if err != nil {
		t.Fatal(err)
	}
	m := id.Message(id.New())
	return &Env{
		Project:       &project.Project{ID: id.Project(id.New()), DB: db, Chats: chats},
		Chat:          c.ID,
		Message:       m,
		Source:        id.SourceOf("message", string(m)),
		Priority:      limit.Interactive,
		PreviewTokens: 25, // 100 bytes
	}
}

// call runs the tool named name from Builtin with JSON args.
func call(t *testing.T, env *Env, name, args string) (Result, error) {
	t.Helper()
	return callWith(t, Deps{}, env, name, args)
}

func callWith(t *testing.T, d Deps, env *Env, name, args string) (Result, error) {
	t.Helper()
	r := NewRegistry()
	r.Add(Builtin(d)...)
	tl, ok := r.Get(name)
	if !ok {
		t.Fatalf("no tool %s", name)
	}
	return Run(context.Background(), tl, Call{ID: "c1", Args: json.RawMessage(args), Env: env})
}

// mustText runs a call that should succeed and returns its text.
func mustText(t *testing.T, env *Env, name, args string) string {
	t.Helper()
	r, err := call(t, env, name, args)
	if err != nil {
		t.Fatalf("%s(%s): %v", name, args, err)
	}
	return r.Text
}

// toolError runs a call that should fail with an *Error and returns its
// message.
func toolError(t *testing.T, env *Env, name, args string) string {
	t.Helper()
	_, err := call(t, env, name, args)
	var te *Error
	if !errors.As(err, &te) {
		t.Fatalf("%s(%s): got %v, want a *tool.Error", name, args, err)
	}
	return te.Msg
}

func contains(t *testing.T, got string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
}

func TestBuiltinSpecs(t *testing.T) {
	r := NewRegistry()
	r.Add(Builtin(Deps{Web: web.NewClient(web.Options{})})...) // panics on a bad schema or name
	want := []string{"bucket_delete", "bucket_list", "bucket_put", "bucket_read", "fetch_page", "list_chats",
		"read_messages", "read_ref", "search_history", "search_ref"}
	var got []string
	for _, tl := range r.For(chat.KindChat) {
		s := tl.Spec()
		got = append(got, s.Name)
		if s.Description == "" || s.Effects == 0 {
			t.Errorf("%s: no description or effects", s.Name)
		}
		var schema map[string]any
		if err := json.Unmarshal(s.Schema, &schema); err != nil || schema["type"] != "object" || schema["additionalProperties"] != false {
			t.Errorf("%s: schema must be a closed object: %s", s.Name, s.Schema)
		}
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("tools %v, want %v", got, want)
	}
	for _, n := range []string{"fetch_page", "read_ref", "search_ref", "bucket_read", "read_messages"} {
		if tl, _ := r.Get(n); tl.Spec().Effects&Untrusted == 0 {
			t.Errorf("%s is not Untrusted", n)
		}
	}
	for _, n := range []string{"search_history", "list_chats", "bucket_list", "bucket_put", "bucket_delete"} {
		if tl, _ := r.Get(n); tl.Spec().Effects&Untrusted != 0 {
			t.Errorf("%s is Untrusted", n)
		}
	}
}

type echoArgs struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func echoTool() Tool {
	return Func(Spec{
		Name: "echo",
		Schema: json.RawMessage(`{"type": "object", "properties": {
			"name": {"type": "string", "minLength": 1},
			"count": {"type": "integer", "minimum": 0, "maximum": 5}
		}, "required": ["name"], "additionalProperties": false}`),
	}, func(ctx context.Context, env *Env, a echoArgs) (Result, error) {
		return Result{Text: strings.Repeat(a.Name, a.Count+1)}, nil
	})
}

func TestFuncArgs(t *testing.T) {
	tl := echoTool()
	for _, tc := range []struct {
		args, want, err string
	}{
		{`{"name": "ab", "count": 2}`, "ababab", ""},
		{`{"name": "ab"}`, "ab", ""},
		{` {"name":"x","count":0} `, "x", ""},
		{``, "", "/: missing property 'name'"},
		{`{}`, "", "missing property 'name'"},
		{`{"name": ""}`, "", "at /name"},
		{`{"name": 5}`, "", "at /name: got number, want string"},
		{`{"name": "a", "count": 1.5}`, "", "at /count"},
		{`{"name": "a", "count": 9}`, "", "at /count: maximum"},
		{`{"name": "a", "extra": 1}`, "", "additional properties 'extra' not allowed"},
		{`{"name": "a", "count": 1, "x": 1, "y": 2}`, "", "additional properties"},
		{`[1]`, "", "got array, want object"},
		{`{"name": "a"`, "", "not valid JSON"},
		{`null`, "", "got null, want object"},
	} {
		r, err := Run(context.Background(), tl, Call{Args: json.RawMessage(tc.args)})
		if tc.err == "" {
			if err != nil || r.Text != tc.want {
				t.Errorf("%s: got %q, %v; want %q", tc.args, r.Text, err, tc.want)
			}
			continue
		}
		var te *Error
		if !errors.As(err, &te) || !strings.Contains(te.Msg, tc.err) {
			t.Errorf("%s: got %v, want a *tool.Error with %q", tc.args, err, tc.err)
		}
		if strings.Contains(err.Error(), "tool:echo") {
			t.Errorf("%s: the message names the schema: %v", tc.args, err)
		}
	}
}

func TestFuncBadSchema(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("a bad schema didn't panic")
		}
	}()
	Func(Spec{Name: "bad", Schema: json.RawMessage(`{"type": "nope"}`)}, func(context.Context, *Env, struct{}) (Result, error) {
		return Result{}, nil
	})
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	mother := Func(Spec{Name: "create_chat", Mother: true, Schema: json.RawMessage(`{}`)}, func(context.Context, *Env, struct{}) (Result, error) {
		return Result{}, nil
	})
	r.Add(echoTool(), mother)
	names := func(ts []Tool) (out []string) {
		for _, tl := range ts {
			out = append(out, tl.Spec().Name)
		}
		return out
	}
	if got := names(r.For(chat.KindChat)); strings.Join(got, ",") != "echo" {
		t.Errorf("chat tools %v", got)
	}
	if got := names(r.For(chat.KindMother)); strings.Join(got, ",") != "create_chat,echo" {
		t.Errorf("Mother tools %v", got)
	}
	if _, ok := r.Get("echo"); !ok {
		t.Error("echo not found")
	}
	if _, ok := r.Get("nope"); ok {
		t.Error("nope found")
	}
	for _, bad := range []Tool{echoTool(), Func(Spec{Name: "Bad-Name", Schema: json.RawMessage(`{}`)}, func(context.Context, *Env, struct{}) (Result, error) {
		return Result{}, nil
	})} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("adding %s didn't panic", bad.Spec().Name)
				}
			}()
			r.Add(bad)
		}()
	}
}

type slowTool struct{ d time.Duration }

func (s slowTool) Spec() Spec { return Spec{Name: "slow", Timeout: s.d} }
func (s slowTool) Run(ctx context.Context, _ Call) (Result, error) {
	select {
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case <-time.After(5 * time.Second):
		return Result{Text: "done"}, nil
	}
}

func TestRunTimeout(t *testing.T) {
	_, err := Run(context.Background(), slowTool{10 * time.Millisecond}, Call{})
	var te *Error
	if !errors.As(err, &te) || !strings.Contains(te.Msg, "slow timed out after 10ms") {
		t.Errorf("own timeout: %v", err)
	}
	// The caller's cancel stays a cancel: Stop is not the model's mistake.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Run(ctx, slowTool{time.Second}, Call{}); !errors.Is(err, context.Canceled) || errors.As(err, &te) {
		t.Errorf("cancelled: %v", err)
	}
	// So does the caller's own deadline.
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := Run(ctx, slowTool{time.Second}, Call{}); !errors.Is(err, context.DeadlineExceeded) || errors.As(err, &te) {
		t.Errorf("caller's deadline: %v", err)
	}
}

func TestEffectsString(t *testing.T) {
	if got := (ReadsDB | Untrusted | NoUndo).String(); got != "reads_db|untrusted|no_undo" {
		t.Errorf("got %q", got)
	}
	if got := Effects(0).String(); got != "" {
		t.Errorf("got %q", got)
	}
}
