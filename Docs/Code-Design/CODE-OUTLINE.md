# Code outline
**Status:** Draft, 2026-09-30

The high-level code: packages, main types, interfaces and functions, built from the answers in [QUESTIONS.md](QUESTIONS.md) (`Q<n>` below) and the SPEC (plain section numbers). Signatures are sketches: names and shapes, not final code. Error returns and `ctx` are shown where they matter.

## 0. Additions to the Q1 map

Writing the code out, and checking it against later phases, added these to the map:

1. **`internal/id`:** all typed IDs (`id.Project`, `id.Chat`, `id.Message`, `id.Run`, `id.Task`, `id.Change`), the ULID generator, and `id.Source`, the one format for who did something (SPEC 2.5). `ProjectID` must sit below `store`, and chat events need it, so the IDs can't live in `chat` as Q5 said. The generator is our own (about 40 lines), so no dependency.
2. **`internal/logfile`:** the rotating log writer and the `slog` setup (Q36).
3. **Publisher seams:** `agent`, `pipeline` and `project` each define a small `Publisher` interface for their events; `app` implements it with Wails events. This is the Q13 rule (the seam lives with its user), and it's how lower packages send events without importing `app`.
4. **`internal/workspace`:** the linked folder and its rules (8.5): paths stay inside, no links out, `.gitignore`, blocked credential files. The v1 read tools use it, and Phase 6's `edit_file` and `write_file` reuse the same checks.
5. **`internal/proc`:** child processes in a Job Object on Windows, with a memory cap, only the listed environment, and the whole tree killed on stop. Used by the Starlark worker (v1), MCP stdio servers (Phase 5) and `run_command` (Phase 6).
6. **`internal/mcp`** (Phase 5): the MCP client and one server process per project (8.7).

Two Wails facts found while writing this (beta.26 source):

- `RegisterEvent` must be called from an `init()` function, or the binding generator can't see the event. So `app/events.go` has the one `init()` in the code, and it only registers event names and types (an exception to Q28).
- A bound method's error reaches JavaScript as `{message, cause, kind}`, and `cause` is the error's own JSON. So `app.UIError` implements `MarshalJSON`, and React gets the kind and *Copy details* text without parsing strings.

## 1. Import graph

A package imports only from rows below its own.

```
cmd/desktop
app                                   Wails; nothing imports it
agent
pipeline, view
tool, schedule, update
project, provider, web, expr, script, mcp
store
sqlguard, bucket, workspace, proc
id, chat, config, limit, secret, logfile
```

- `agent` imports `pipeline`: Mother's chats and the finish notices need runs. `pipeline` never imports `agent`.
- `view` and `pipeline` share a row and don't import each other; a form that starts a run goes through a small `view.Runs` interface.
- `update` also imports Wails (`pkg/updater`), so a Wails upgrade touches `app` and `update`.

## 2. `cmd/desktop`: start

```go
func main() {
    if err := run(os.Args[1:]); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}

// run builds everything in import order and blocks until the app quits (Q28).
// Error checks are left out of this sketch.
func run(args []string) error {
    if slices.Contains(args, "--script-worker") {
        return script.Serve(os.Stdin, os.Stdout) // child process (Q4)
    }
    paths, err := config.DefaultPaths(appDir)            // <user data dir>/<app>/
    settings, err := config.LoadSettings(paths.Settings) // config.toml
    log, closeLog, err := logfile.Open(paths.Logs, settings.Debug)
    defer closeLog()
    ctx, cancel := context.WithCancel(context.Background()) // the app context (Q20)
    defer cancel()

    secrets := secret.New(secret.OSKeyring())
    registry, err := store.OpenRegistry(ctx, paths.Registry)
    calls := limit.NewGate(settings.LLM.MaxParallelCalls)
    runs  := limit.NewGate(settings.Scheduler.MaxParallelRuns)

    models   := provider.NewRegistry(provider.Deps{Settings: settings, Secrets: secrets, Gate: calls, Log: log})
    projects := project.NewManager(project.Deps{Paths: paths, Registry: registry, Log: log})
    webc     := web.New(web.Deps{Secrets: secrets, Searchers: web.Searchers(settings, secrets)})
    tools    := tool.NewRegistry()
    tool.AddBuiltins(tools, tool.Deps{Web: webc, Registry: registry, Secrets: secrets})

    wapp   := app.New(app.Deps{Log: log})         // Wails app first, so events can be sent
    runner := pipeline.New(pipeline.Deps{Projects: projects, Models: models, Gate: runs, Web: webc, Events: wapp.Publisher(), Log: log})
    tools.Add(runner.Tools()...)
    orch   := agent.New(agent.Deps{Projects: projects, Models: models, Tools: tools, Runs: runner, Settings: settings.LLM, Events: wapp.Publisher(), Log: log})
    sched  := schedule.New(schedule.Deps{Wake: schedule.OSWake(), Start: runner.StartScheduled, Log: log})
    upd    := update.New(update.Deps{Settings: settings.Updates, Busy: projects.Busy, Log: log})

    wapp.Bind(app.Services{Orchestrator: orch, Runner: runner, Projects: projects, Registry: registry, Models: models, Secrets: secrets, Updates: upd, Dev: settings.DevTools})
    wapp.OnStart(func(ctx context.Context) { go sched.Run(ctx); go upd.Run(ctx) }) // owned by the app root (Q15)
    wapp.OnShutdown(shutdown(cancel, sched, orch, runner, projects)) // Q30
    return wapp.Run()
}
```

`wapp.OnStart` and `wapp.OnShutdown` are our wrapper's methods over a Wails service startup hook and Wails' `OnShutdown`.

`shutdown` runs the Q30 order within 10 s: `sched.Stop()` and `orch.Refuse()` (no new work), cancel the app context and wait up to 5 s for `orch.Wait` and `runner.Wait`, then `projects.CloseAll(ctx)` (drain writers, checkpoint, remove the lock file) with the rest of the time.

## 3. Bottom packages

### `id`

```go
type Project string // also Chat, Message, Run, Task, Change, Approval
func New() string   // ULID, from time and crypto/rand

// Source says who did something (SPEC 2.5): "app", "user", "auto", "message:<id>", "run:<id>" …
// Used by the change log, chats.created_by, objects, links and approvals.
type Source string
func SourceOf(kind string, id string) Source // "message:01J…"; an empty id gives just the kind
func (s Source) Kind() string
func (s Source) ID() string                   // "" when there is none; "user" and "user:<id>" both parse
```

### `chat`: the types every layer shares (Q5, Q7)

```go
type Chat struct {
    ID          id.Chat
    Kind        Kind   // KindMother, KindChat
    Title       string
    TitleFixed  bool
    Role        string
    Skills      []string
    Model       string
    DefaultPage string
    CreatedBy   id.Source // "app" (the Mother chat), "user", or "message:<id>" of Mother's create_chat call
    CreatedAt   time.Time
    Archived    bool
}

type Message struct {
    ID        id.Message
    Chat      id.Chat
    Turn      int
    Role      Role // RoleUser, RoleAssistant, RoleTool
    Model     string
    Usage     Usage
    CreatedAt time.Time
    Parts     []Part
}

type Part struct {
    Kind       PartKind
    Text       *Text
    Thinking   *Thinking   // text plus signature, sent back unchanged (3.8)
    ToolCall   *ToolCall   // ID, name, JSON arguments, provider extras (Gemini's signature)
    ToolResult *ToolResult // call ID, text or ref, preview, IsError
    Image      *Image
    Notice     *Notice
    Approval   *Approval
    Question   *Question
}
func (p Part) Validate() error // exactly one field set, and it matches Kind

type SessionNote struct { Chat id.Chat; Content string; Revision int; UpdatedAt time.Time }

// Event payloads (Q32); every one carries its IDs and the chat's sequence number.
type Delta   struct { Project id.Project; Chat id.Chat; Message id.Message; Seq uint64; Part int; Kind PartKind; Text string }
type PartDone struct { Project id.Project; Chat id.Chat; Seq uint64; Message id.Message; Part Part }
type Status  struct { Project id.Project; Chat id.Chat; Seq uint64; State State; Tasks int; Waiting *Waiting }
```

### `config`: settings and configs (1, 10)

```go
type Paths struct { Root, Settings, Registry, Projects, Logs string }
func DefaultPaths(appDir string) (Paths, error)

type Settings struct { Scheduler SchedulerSettings; LLM LLMSettings; Approvals ApprovalSettings; Updates UpdateSettings; DevTools, Debug bool }
func LoadSettings(path string) (Settings, error)

type View struct { … }     // also Page, Form, Pipeline, Step, Connection, MCPServer
func ParseView(src []byte) (View, error) // YAML or JSON with the strict rules of 1; errors are Errors
func ParsePipeline(src []byte) (Pipeline, error)
func ToYAML(v any) ([]byte, error)       // display form (1)

// Schema is what the checks need from the database, as plain data; store fills it.
type Schema struct { Version int; Tables map[string]Table }
func CheckView(v View, s Schema) Errors  // section 10; also CheckPipeline, CheckPage, CheckForm

type Errors []FieldError
type FieldError struct { Path string; Line, Col int; Msg string } // "steps[2].with.table: …"
func (e Errors) Error() string
```

### `limit`: shared limits (Q19)

```go
type Priority int
const (Interactive Priority = iota; Background)

// Fair is the 10:1 rule, used by both the writer and the Gate.
type Fair struct { streak int }
func (f *Fair) Next(interactive, background bool) Priority

type Gate struct { … }
func NewGate(n int) *Gate
func (g *Gate) Acquire(ctx context.Context, p Priority) (release func(), err error) // FIFO per priority
func (g *Gate) Stats() GateStats                                                   // slots in use, waiters
```

### `secret`: keychain and redaction (6.7, Q36)

```go
type Value struct { name, v string }
func (s Value) Reveal() string          // only for the auth header
func (s Value) String() string          // "[secret:NAME]"; also GoString, LogValue, MarshalJSON

type Keyring interface { Get(name string) (string, error); Set(name, v string) error; Delete(name string) error }
func OSKeyring() Keyring                // zalando/go-keyring, service name from one constant

type Store struct { kr Keyring }
func New(kr Keyring) *Store
func (s *Store) Get(name string) (Value, error)
func (s *Store) Set(name, v string) error
func (s *Store) Redact(text string) string // replaces every known secret with [secret:NAME]
```

### `logfile`

```go
func Open(dir string, debug bool) (*slog.Logger, func() error, error) // text handler, 10 MB × 5 files
func Tail(dir string, match string) ([]string, error)                 // for *Copy details*
```

## 4. Data

### `sqlguard`: the three layers (2.2)

```go
func Check(query string) error // layer 1: the lexer; returns *RejectError

type Guard struct { … }         // caches the table map per schema_version
func New() *Guard
func (g *Guard) Explain(ctx context.Context, c *sql.Conn, query string) error // layer 2
func Prepare(ctx context.Context, c *sql.Conn) error                          // layer 3: query_only and limits on a reader

type RejectError struct { Layer int; Reason string; Pos int }
```

### `bucket`: bytes only (4.8)

```go
type ObjectStore interface {
    Put(ctx context.Context, hash string, r io.Reader) error // write .partial, fsync, rename
    Open(ctx context.Context, hash string) (io.ReadCloser, error)
    Delete(ctx context.Context, hash string) error
    All(ctx context.Context) iter.Seq2[string, error]        // for the sweep (2.7)
}
type Folder struct { root string }  // objects/ab/12/ab12f9…
func NewFolder(root string) *Folder
func Hash(r io.Reader) (string, error)
```

The index tables and the write order (4.3) are in `store`, which calls `bucket`.

### `store`: databases, writers, readers (2, 7)

```go
// DB is one database file: a writer goroutine and a reader pool (7.2).
type DB struct { … }
func Open(ctx context.Context, path string, o Options) (*DB, error) // pragmas, format version (2.8), starts the writer
func (db *DB) Close(ctx context.Context) error                     // drain, checkpoint(TRUNCATE) with 100 ms, close
func (db *DB) Stats() WriterStats                                   // queue per priority, current request and age, last error

func Do[T any](ctx context.Context, db *DB, p limit.Priority, fn func(*sql.Tx) (T, error)) (T, error) // Q27
func Query[T any](ctx context.Context, db *DB, q string, args []any, scan func(*sql.Rows) (T, error)) ([]T, error)
```

The writer loop, per request: pick the queue with `limit.Fair`, switch the request from waiting to taken (skip it if its caller gave up, Q21), run it in one transaction with `recover` (Q26), send the answer to the reply channel (room for one). Outside `store`, code uses the typed requests below, never `Do`.

```go
type ProjectDB struct { db *DB; guard *sqlguard.Guard; objects bucket.ObjectStore }

// Reads
func (p *ProjectDB) AgentQuery(ctx context.Context, q string, args []any, max int) (Rows, error) // guarded, 10 s
func (p *ProjectDB) ViewQuery(ctx context.Context, q string, args []any, page Page) (Rows, error)  // guarded, 2 s
func (p *ProjectDB) Schema(ctx context.Context) (config.Schema, error)
func (p *ProjectDB) Card(ctx context.Context) (Card, error)                                        // 3.2

// Writes: one request each, with a change-log entry (2.5); src is an id.Source
func (p *ProjectDB) Insert(ctx context.Context, pr limit.Priority, src id.Source, t string, rows []Row, c Conflict) (WriteResult, error) // 500-row chunks (7.4)
func (p *ProjectDB) Update(ctx context.Context, pr limit.Priority, src id.Source, u UpdateReq) (WriteResult, error)
func (p *ProjectDB) SaveConfig(ctx context.Context, src id.Source, c SavedConfig, rev int) (int, error)        // ErrConflict on a stale revision
func (p *ProjectDB) UpdateMemory(ctx context.Context, src id.Source, section, content string, rev int) (int, error)
func (p *ProjectDB) Migrate(ctx context.Context, src id.Source, m Migration, try bool) (MigrationReport, error) // 7.5, with the schema guard
func (p *ProjectDB) Undo(ctx context.Context, t UndoTarget, skip []Conflict, ext Restorers) (UndoReport, error) // 2.6
func (p *ProjectDB) PutObject(ctx context.Context, src id.Source, key string, r io.Reader) (Object, error)     // bytes first, then the index (4.3)

// Runs (6.2)
func (p *ProjectDB) StartRun(ctx context.Context, r Run) error
func (p *ProjectDB) FinishStep(ctx context.Context, s RunStep) error
func (p *ProjectDB) FinishRun(ctx context.Context, id id.Run, st RunState) error

type ChatsDB struct { db *DB }
func (c *ChatsDB) Chats(ctx context.Context) ([]chat.Chat, error)
func (c *ChatsDB) Messages(ctx context.Context, ch id.Chat, from, to int) ([]chat.Message, error)
func (c *ChatsDB) AppendMessage(ctx context.Context, m chat.Message) error  // before any tool runs (2.3)
func (c *ChatsDB) AppendPart(ctx context.Context, m id.Message, p chat.Part) error
func (c *ChatsDB) Search(ctx context.Context, s SearchReq) ([]Hit, error)   // FTS query builder, normalization (2.3)
func (c *ChatsDB) SaveNotes(ctx context.Context, n chat.SessionNote) (int, error)

// Registry is registry.db: one connection, no writer goroutine (2.4).
type Registry struct { … }
func OpenRegistry(ctx context.Context, path string) (*Registry, error)
// Projects, UserMemory, Connections, MCPServers: list, get, save with revision.

var (ErrNotFound = errors.New("not found"); ErrConflict = errors.New("revision conflict"))

// Restorer undoes a change outside the database, such as a workspace file (Phase 6),
// so store never writes into the user's folder itself. Keyed by target prefix ("file:").
type Restorer interface {
    Current(ctx context.Context, target string) (hash string, err error) // for the conflict check
    Restore(ctx context.Context, target string, before Object) error
}
type Restorers map[string]Restorer
```

Store-only types: `Run`, `RunStep`, `ChangeEntry`, `Memory`, `Approval`, `Object`, `Migration` and its steps (8.2).

### `project`: lifetime and leases (2.7, Q29)

```go
type Manager struct { … }
func NewManager(d Deps) *Manager
func (m *Manager) Open(ctx context.Context, p id.Project) (*Project, error) // takes a lease; opens and recovers if needed
func (m *Manager) Create(ctx context.Context, name string) (id.Project, error)
func (m *Manager) Busy() bool                                              // for the update restart
func (m *Manager) CloseAll(ctx context.Context) error                      // Q30 steps 3–4

type Project struct {
    ID     id.Project
    DB     *store.ProjectDB
    Chats  *store.ChatsDB
    // unexported: lease count, idle timer, reporters
}
func (p *Project) Release()                                  // the last release starts the 10-minute idle timer
func (p *Project) Report(r Reporter) (unregister func())     // chat runners and runs register (Q19a)
func (p *Project) Activity() Activity                        // one snapshot of every Reporter plus writer Stats
func (p *Project) OnClose(fn func()) (remove func())         // for work that lives as long as the project, e.g. MCP servers

type Reporter interface { Status() []Status }
type Status struct { ID string; Kind, State string; Started, LastActivity time.Time; Progress string; Err string }
type Publisher interface { Notice(Notice); Activity(Activity) }
```

Recovery on open (2.7) is unexported in `project`: `quick_check`, mark runs `interrupted`, empty `tmp/`, sweep `objects/`.

## 5. The outside world

### `provider` (3.8, Q10)

```go
type Provider interface {
    Stream(ctx context.Context, req Request) iter.Seq2[Event, error]
    Models(ctx context.Context) ([]ModelInfo, error)
}

type Request struct {
    Model     string
    System    []Block        // with cache points (3.1)
    Messages  []chat.Message
    Tools     []ToolDef      // name, description, JSON Schema; agent converts tool.Spec to this
    MaxTokens int
}
type Event struct { Kind EventKind; Text string; Call *chat.ToolCall; Usage *chat.Usage; Stop StopReason }

type Error struct { Kind ErrorKind; Status int; RetryAfter time.Duration; Provider string; Err error }
// Kinds: RateLimited, Transport, Server, CutOff (retried); Auth, Quota, Stalled (block); TooLarge (shrink); BadRequest.

// Registry picks the backend for a model and adds the shared rules in one place:
// a Gate slot, the stall timeouts, retries with capped waits, secret redaction in errors.
type Registry struct { … }
func NewRegistry(d Deps) *Registry
func (r *Registry) Stream(ctx context.Context, p limit.Priority, req Request) iter.Seq2[Event, error]
func (r *Registry) Models(ctx context.Context) ([]ModelInfo, error) // the built-in catalog plus each provider's list (3.9)
```

- `provider/anthropic`, `provider/openai` (OpenAI, Gemini, Ollama, compatible): `New(Options) provider.Provider`. Each maps its protocol and errors; nothing else.
- `provider/fake`: `fake.New(steps ...fake.Step) provider.Provider` plays scripted events, delays and errors.

### `web` (6.5, 6.7)

```go
type Searcher interface { Search(ctx context.Context, q SearchReq) ([]SearchResult, error) }
type Client struct { … }  // one http.Client with the host allow-list and size caps
func (c *Client) Search(ctx context.Context, q SearchReq) ([]SearchResult, error)
func (c *Client) Fetch(ctx context.Context, url string, allow HostCheck) (Page, error) // readable text (3.7)
func (c *Client) Feed(ctx context.Context, urls []string, f FeedFilter) ([]FeedItem, error)
func (c *Client) CallAPI(ctx context.Context, conn config.Connection, key secret.Value, r APIReq) (Response, error) // 6.9
type HostCheck func(host string) bool
```

### `workspace` (8.5)

```go
type Root struct { … }                                        // the linked folder, the blocked list, .gitignore
func Open(path string, blocked []string) (*Root, error)
func (r *Root) Resolve(rel string) (string, error)            // stays inside; links out are refused
func (r *Root) Walk(ctx context.Context, rel, pattern string) iter.Seq2[Entry, error]
func (r *Root) Read(ctx context.Context, rel string, from, to int) (Lines, error)
func (r *Root) Search(ctx context.Context, pattern, rel, glob string) ([]Match, error) // 500 matches or 2 s
// Phase 6: Write, Edit and Hash, used by edit_file, write_file and the file Restorer.
```

### `proc`

```go
type Spec struct { Path string; Args []string; Dir string; Env []string; MemoryMax int64; Stdin io.Reader; Stdout, Stderr io.Writer }
type Process struct { … }
func Start(ctx context.Context, s Spec) (*Process, error)     // in a Job Object on Windows
func (p *Process) Wait() error
func (p *Process) Kill() error                                // the whole tree; also on ctx cancel
```

### `mcp` (Phase 5, 8.7)

```go
type Manager struct { … }                                     // one server process per project, stopped by Project.OnClose or 10 min idle
func (m *Manager) Tools(ctx context.Context, p id.Project) ([]ToolInfo, error) // names, descriptions, readOnlyHint
func (m *Manager) Describe(ctx context.Context, p id.Project, server, tool string) (json.RawMessage, error)
func (m *Manager) Call(ctx context.Context, p id.Project, server, tool string, args json.RawMessage) (Result, error)
```

### `expr`, `script`, `schedule`

```go
// expr (6.4)
func Compile(src string, env Env) (*Program, error)          // only the listed syntax; size and nesting limits
func (p *Program) Eval(ctx context.Context, vars map[string]any) (any, error) // 1 s watchdog

// script (SPIKE-016)
func Serve(in io.Reader, out io.Writer) error                 // child side
func Run(ctx context.Context, code string, inputs map[string]any, q Querier) (any, error) // parent side; a proc child, killed on cancel
type Querier func(ctx context.Context, sql string, args []any) ([]map[string]any, error) // db.query through the parent

// schedule (6.2)
func Parse(expr string) (Cron, error)                         // strict 5 fields
func (c Cron) Next(after time.Time, loc *time.Location) time.Time // DST rule
func Due(c Cron, last, now time.Time, loc *time.Location) []time.Time // for catch-up
type Scheduler struct { … }
func New(d Deps) *Scheduler                                   // Deps.Start is called for each due job
func (s *Scheduler) Set(p id.Project, jobs []Job)
func (s *Scheduler) Run(ctx context.Context) error            // minute check plus wake events
type Wake interface { Resumed() <-chan time.Time }            // Q12; OSWake uses PowerRegisterSuspendResumeNotification
```

### `update` (2.8)

```go
func New(d Deps) *Updater
func (u *Updater) Run(ctx context.Context) error // check at start, then every 6 h
func (u *Updater) Apply(ctx context.Context) error // only when Deps.Busy() is false
```

## 6. Tools (8.1, Q11)

```go
type Tool interface {
    Spec() Spec
    Run(ctx context.Context, call Call) (Result, error)
}
type Spec struct { Name, Description string; Schema json.RawMessage; Effects Effects; Timeout time.Duration }
type Effects uint // ReadsDB, WritesDB, Network, Bucket, Workspace, Memory, Schema, AsksUser, Untrusted, NoUndo
// Untrusted: the result is outside data (web pages, files, MCP results); after one, the turn is marked (8.5, 8.7).
// NoUndo: the change is outside the app (MCP), so *Undo turn* skips it and the chip says so.

// Preflighter is optional. A tool whose needs depend on its arguments reports them before Run:
// fetch_page (a new host), mcp_call (readOnlyHint), run_command (the pattern), edit_file (the path).
type Preflighter interface {
    Preflight(ctx context.Context, call Call) (Needs, error)
}
type Needs struct { Effects Effects; Approvals []chat.Approval }

// Call carries the arguments and where the call runs.
type Call struct { ID string; Args json.RawMessage; Env *Env }
type Env struct { Project *project.Project; Workspace *workspace.Root; Chat id.Chat; Message id.Message; Source id.Source; Priority limit.Priority }

type Result struct { Text string; Ref string; Images []chat.Image; Denied bool }
type Error struct { Msg string; Err error } // the model can fix it (Q24)

func Func[A any](s Spec, fn func(ctx context.Context, env *Env, args A) (Result, error)) Tool // Q27

type Registry struct { … }
func NewRegistry() *Registry
func (r *Registry) Add(ts ...Tool)
func (r *Registry) Get(name string) (Tool, bool)
func (r *Registry) For(k chat.Kind, loaded []string) []Tool // Mother gets create_chat and send_to_chat
```

Where each tool is written:

| Package | Tools |
|---|---|
| `tool` | `query`, `describe_table`, `insert`, `save_view`, `save_page`, `save_pipeline`, `read_config`, `validate`, `update_memory`, `update_session_notes`, `search_history`, `read_messages`, `read_ref`, `search_ref`, `web_search`, `read_feed`, `fetch_page`, `connect_api`, `call_api`, `list_files`, `read_file`, `search_code`, `bucket_*`, `save_link`; later `mcp_describe`, `mcp_call` (Phase 5), `edit_file`, `write_file`, `run_command` (Phase 6) |
| `pipeline` | `dry_run`, `run_pipeline`, `run_status` |
| `agent` | `subagent`, `task_status`, `ask_user`, `list_chats`, `create_chat`, `send_to_chat`, `load_skill`, `request_schema_change`, `open_page` |

## 7. `agent`: the orchestrator (8.3, Q16)

```go
type Orchestrator struct { … }
func New(d Deps) *Orchestrator

// Called by app.ChatService; each returns at once.
func (o *Orchestrator) Send(ctx context.Context, p id.Project, c id.Chat, m UserMessage) (id.Message, error)
func (o *Orchestrator) Stop(p id.Project, c id.Chat)
func (o *Orchestrator) Answer(p id.Project, c id.Chat, a Answer) error // approval card or question form (8.8)
func (o *Orchestrator) Refuse()                                        // shutdown step 1
func (o *Orchestrator) Wait(ctx context.Context) error

type Publisher interface {
    Delta(chat.Delta)
    Part(chat.PartDone)
    Status(chat.Status)
    OpenPage(OpenPage)
}
```

Inside `agent` (unexported):

```go
type runner struct { inbox chan item; … } // one per chat with work; holds a project lease
func (r *runner) loop(ctx context.Context)          // runs turns one at a time; exits when the inbox is empty
func (r *runner) turn(ctx context.Context, in []item) error // a turn: steps until the model stops, the cap is hit, or Stop
func (r *runner) step(ctx context.Context, t *turn) (done bool, err error)
    // 1. buildContext → provider.Request (3.1)
    // 2. models.Stream: deltas to the coalescer, finished parts to chats.db
    // 3. write the assistant message and its tool calls (2.3), then run the calls in order
    // 4. take new user messages from the inbox (8.3)
func (r *runner) runTool(ctx context.Context, t *turn, c chat.ToolCall) chat.ToolResult // approval, timeout, recover, error mapping
func (r *runner) Status() []project.Status

type coalescer struct { … }             // ≤ 1 Delta per 16 ms per chat (Q17)
func buildContext(t *turn) provider.Request
type task struct { … }                  // a background subagent or a Mother chat task; sends its finish notice to the inbox
type subagent struct { … }              // not a chat: in-memory messages, no subagent tool of its own (one level);
                                        // writes use the parent's Source; at the end its transcript goes to the bucket
                                        // and the parent's tool_result gets the ref (8.6)
type schemaAgent struct { … }           // 8.2: steps, try, needs_approval
func approve(t *turn, s tool.Spec, n tool.Needs) []chat.Approval // 8.8 levels, the turn's Untrusted mark, in one place
```

A turn in one picture:

```
Send → inbox → runner.loop → turn → step ─┬─ models.Stream ──► coalescer ──► Publisher.Delta
                                          ├─ ChatsDB.AppendMessage / AppendPart ──► Publisher.Part
                                          └─ runTool ──► tool.Run ──► ProjectDB (writer)
```

## 8. `pipeline` and `view`

```go
// pipeline (6)
type Runner struct { … }
func New(d Deps) *Runner
func (r *Runner) Start(ctx context.Context, p id.Project, pl string, t Trigger, in map[string]any) (id.Run, error)
func (r *Runner) StartScheduled(ctx context.Context, j schedule.Job)  // catch-up and skip rules (6.2)
func (r *Runner) StartFromForm(ctx context.Context, p id.Project, pl string, in map[string]any) (id.Run, error) // for view.Runs
func (r *Runner) DryRun(ctx context.Context, p id.Project, cfg config.Pipeline, in map[string]any) (DryRunReport, error) // 6.8
func (r *Runner) Stop(p id.Project, run id.Run)
func (r *Runner) Wait(ctx context.Context) error
func (r *Runner) Tools() []tool.Tool

type Step interface { Run(ctx context.Context, in StepInput) (StepOutput, error) } // Q11
var catalog = map[string]func(config.Step) (Step, error){ "http.get": …, "db.insert_many": …, "llm.select": … } // 6.5
type Publisher interface { RunStatus(RunStatus) }

// view (5)
func Load(ctx context.Context, p *project.Project, viewID string, f Filters, pg store.Page) (Result, error)
func Submit(ctx context.Context, p *project.Project, runs Runs, formID string, values map[string]any) (SubmitResult, error)
func Action(ctx context.Context, p *project.Project, viewID, action string, keys [][]any) (ActionResult, error)
type Result struct { Columns []Column; Rows [][]any; Total int } // shaped (Q33)
type Runs interface { StartFromForm(ctx context.Context, p id.Project, pl string, in map[string]any) (id.Run, error) }
```

`view` gets a `Runs` value for forms that start a pipeline; `pipeline.Runner` has that method, so `app` passes the runner in and `view` never imports `pipeline`.

## 9. `app`: the Wails layer (Q31–33)

```go
type App struct { w *application.App; pub publisher }
func New(d Deps) *App
func (a *App) Publisher() *publisher
func (a *App) Bind(s Services)            // RegisterService for each service; DevService only with DevTools
func (a *App) Run() error

type ChatService struct { … }
func (s *ChatService) List(ctx context.Context, p id.Project) ([]chat.Chat, error)
func (s *ChatService) Snapshot(ctx context.Context, p id.Project, c id.Chat) (ChatSnapshot, error) // messages + Seq
func (s *ChatService) Send(ctx context.Context, p id.Project, c id.Chat, text string, files []string) (id.Message, error)
func (s *ChatService) Stop(ctx context.Context, p id.Project, c id.Chat) error
func (s *ChatService) Answer(ctx context.Context, p id.Project, c id.Chat, a agent.Answer) error
func (s *ChatService) UndoTurn(ctx context.Context, p id.Project, c id.Chat, turn int, skip []store.Conflict) (store.UndoReport, error)
// ProjectService, PageService, PipelineService, BucketService, SettingsService, DevService in the same style.

type UIError struct { Kind, Message, Details string } // MarshalJSON → cause in JavaScript (Q25)
func uiError(err error) *UIError       // errors.As on the known kinds; else "Something went wrong"

type publisher struct { w *application.App } // implements agent, pipeline and project Publisher with EmitEvent
func objectHandler(pm *project.Manager) http.Handler // /objects/<project_id>/<key> with the 4.7 headers
```

```go
// events.go: the only init() (see section 0)
func init() {
    application.RegisterEvent[chat.Delta]("chat:delta")
    application.RegisterEvent[chat.PartDone]("chat:part")
    application.RegisterEvent[chat.Status]("chat:status")
    application.RegisterEvent[pipeline.RunStatus]("run:status")
    application.RegisterEvent[project.Notice]("project:notice")
    application.RegisterEvent[project.Activity]("project:activity")
}
```

Every service method starts with `defer s.recover(&err)` (Q26) and ends with `return x, uiError(err)`, so errors are logged once, here.

## 10. Open points

1. **Retry after text was shown.** 3.8 retries a cut-off stream, but if deltas already reached the screen, a retry would show the text twice. *Lean:* retry only before the first event is passed on; after that, end the turn with the partial text kept, as for *Stop*.
2. **The frontend folder.** `frontend/src` starts from `mockups/src`; which parts carry over is decided when Phase 1 starts.
