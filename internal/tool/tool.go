// Package tool is the tool layer (CODE-OUTLINE 6, SPEC 8.1): one Tool
// interface for every tool, the registry, previews and refs for large
// results (SPEC 3.7), and the Phase 1 tools.
package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/web"
)

// Tool is a built-in tool, and later an MCP tool or a subagent (Q11).
type Tool interface {
	Spec() Spec
	Run(ctx context.Context, call Call) (Result, error)
}

// Spec describes a tool to the model and to the orchestrator.
type Spec struct {
	Name        string
	Description string
	Schema      json.RawMessage // JSON Schema of the arguments
	Effects     Effects
	Timeout     time.Duration // 0 means DefaultTimeout
	Mother      bool          // only the Mother chat gets it (8.6)
}

// DefaultTimeout bounds a tool call whose Spec sets none.
const DefaultTimeout = time.Minute

// Effects are what a tool does, so the orchestrator decides approvals and
// undo in one place (8.8).
type Effects uint

const (
	ReadsDB   Effects = 1 << iota // reads the project's data or chats
	WritesDB                      // changes the project's data
	Network                       // calls the internet
	Bucket                        // changes the bucket
	Workspace                     // reads the linked folder
	Memory                        // changes memory or session notes
	Schema                        // changes the schema
	AsksUser                      // waits for the user
	// Untrusted: the result is outside data (web pages, files, MCP
	// results); after one, the turn is marked (8.5, 8.7).
	Untrusted
	// NoUndo: the change is outside the app (MCP), so Undo turn skips it.
	NoUndo
)

var effectNames = []string{"reads_db", "writes_db", "network", "bucket", "workspace", "memory", "schema", "asks_user", "untrusted", "no_undo"}

func (e Effects) String() string {
	var out []string
	for i, n := range effectNames {
		if e&(1<<i) != 0 {
			out = append(out, n)
		}
	}
	return strings.Join(out, "|")
}

// Preflighter is optional. A tool whose needs depend on its arguments
// reports them before Run, such as fetch_page for its host.
type Preflighter interface {
	Preflight(ctx context.Context, call Call) (Needs, error)
}

// Needs are a call's effects and the approvals it asks for. The
// orchestrator drops approvals already given and applies the approval
// level (P1-11).
type Needs struct {
	Effects   Effects
	Approvals []chat.Approval
}

// Call is one tool call and where it runs.
type Call struct {
	ID   string
	Args json.RawMessage
	Env  *Env
}

// Env is where a call runs.
type Env struct {
	Project  *project.Project
	Chat     id.Chat
	Message  id.Message // the assistant message with the call
	Source   id.Source  // "message:<id>", for created_by
	Priority limit.Priority
	// PreviewTokens is the largest result that enters the context whole
	// (tool_preview_tokens, SPEC 3.6).
	PreviewTokens int
	// PrivateHosts are the hosts that may reach private addresses: the
	// project's exceptions (6.7). Nil allows none.
	PrivateHosts web.HostCheck
}

// Result is what a tool returns. A tool that stores its own output, such
// as fetch_page, sets Ref and puts the preview in Text.
type Result struct {
	Text   string
	Ref    string
	Images []chat.Image
	Denied bool // the user said no; Text has their note
}

// Error is a mistake the model can fix, such as bad arguments, a missing
// object or a blocked URL (Q24). It goes back to the model as an error
// result; any other error is a system failure.
type Error struct {
	Msg string
	Err error
}

func (e *Error) Error() string { return e.Msg }
func (e *Error) Unwrap() error { return e.Err }

// Errorf returns an *Error.
func Errorf(format string, a ...any) *Error {
	err := fmt.Errorf(format, a...)
	return &Error{Msg: err.Error(), Err: errors.Unwrap(err)}
}

// Run runs t with its timeout. A timeout of the tool's own becomes an
// *Error the model sees; a cancelled ctx stays as it is.
func Run(ctx context.Context, t Tool, call Call) (Result, error) {
	d := t.Spec().Timeout
	if d <= 0 {
		d = DefaultTimeout
	}
	tctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	r, err := t.Run(tctx, call)
	if err != nil && ctx.Err() == nil && errors.Is(tctx.Err(), context.DeadlineExceeded) {
		return Result{}, Errorf("%s timed out after %s", t.Spec().Name, d)
	}
	return r, err
}

// Func makes a tool from a function (Q27). The arguments are checked
// against s.Schema and decoded into A; wrong ones are an *Error. It panics
// if the schema doesn't compile, which is a bug.
func Func[A any](s Spec, fn func(ctx context.Context, env *Env, args A) (Result, error)) Tool {
	return &funcTool[A]{spec: s, schema: compile(s), fn: fn}
}

type funcTool[A any] struct {
	spec   Spec
	schema *jsonschema.Schema
	fn     func(ctx context.Context, env *Env, args A) (Result, error)
}

func (t *funcTool[A]) Spec() Spec { return t.spec }

func (t *funcTool[A]) Run(ctx context.Context, call Call) (Result, error) {
	a, err := decodeArgs[A](t.schema, call.Args)
	if err != nil {
		return Result{}, err
	}
	return t.fn(ctx, call.Env, a)
}

func compile(s Spec) *jsonschema.Schema {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(s.Schema))
	if err != nil {
		panic(fmt.Sprintf("tool %s: schema: %v", s.Name, err))
	}
	c := jsonschema.NewCompiler()
	url := "tool:" + s.Name
	if err := c.AddResource(url, doc); err != nil {
		panic(fmt.Sprintf("tool %s: schema: %v", s.Name, err))
	}
	return c.MustCompile(url)
}

// decodeArgs checks raw against the schema and decodes it. No arguments
// are an empty object.
func decodeArgs[A any](sch *jsonschema.Schema, raw json.RawMessage) (A, error) {
	var a A
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = json.RawMessage("{}")
	}
	v, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return a, Errorf("the arguments are not valid JSON: %v", err)
	}
	if err := sch.Validate(v); err != nil {
		return a, &Error{Msg: "wrong arguments:" + schemaErrors(err), Err: err}
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return a, Errorf("wrong arguments: %v", err)
	}
	return a, nil
}

// schemaErrors lists a validation error's causes, one per line, without
// the schema's URL.
func schemaErrors(err error) string {
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return " " + err.Error()
	}
	var lines []string
	var walk func(*jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) == 0 {
			at := "/" + strings.Join(e.InstanceLocation, "/")
			lines = append(lines, fmt.Sprintf("\n- at %s: %s", at, e.ErrorKind.LocalizedString(printer)))
		}
		for _, c := range e.Causes {
			walk(c)
		}
	}
	walk(ve)
	return strings.Join(lines, "")
}

var printer = message.NewPrinter(language.English)

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// Registry holds the tools by name.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry { return &Registry{tools: map[string]Tool{}} }

// Add registers tools. It panics on a bad or repeated name, which is a bug.
func (r *Registry) Add(ts ...Tool) {
	for _, t := range ts {
		n := t.Spec().Name
		if !namePattern.MatchString(n) {
			panic("tool: bad name " + n)
		}
		if _, ok := r.tools[n]; ok {
			panic("tool: " + n + " is added twice")
		}
		r.tools[n] = t
	}
}

// Get returns the tool with that name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// For returns the tools a chat of kind k gets, by name. Only Mother gets
// the Mother tools.
func (r *Registry) For(k chat.Kind) []Tool {
	var out []Tool
	for _, n := range slices.Sorted(maps.Keys(r.tools)) {
		if t := r.tools[n]; !t.Spec().Mother || k == chat.KindMother {
			out = append(out, t)
		}
	}
	return out
}
