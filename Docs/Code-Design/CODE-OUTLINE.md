# Code outline
**Status:** Draft, 2026-09-30

The high-level code: packages, main types, interfaces and functions, built from the answers in [QUESTIONS.md](QUESTIONS.md) (`Q<n>` below) and the SPEC (plain section numbers). Signatures are sketches: names and shapes, not final code. Error returns and `ctx` are shown where they matter.

## 0. Additions to the Q1 map

Writing the code out, and checking it against later phases, added these to the map:

1. **`internal/id`:** all typed IDs (`id.Project`, `id.Chat`, `id.Message`, `id.Run`, `id.Task`, `id.Change`), the ULID generator, and `id.Source`, the one format for who did something (SPEC 2.5). `ProjectID` must sit below `store`, and chat events need it, so the IDs can't live in `chat` as Q5 said. IDs come from `oklog/ulid/v2` with `crypto/rand` as the random source.
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
    paths, err := config.DefaultPaths(appDir)                   // <user data dir>/<app>/
    settings, problems, err := config.LoadSettings(paths.Settings) // config.yaml
    paths = paths.WithDataFolder(settings.DataFolder)           // projects/ (2.1)
    log, closeLog, err := logfile.Open(paths.Logs, settings.DevTools)
    defer closeLog()
    for _, p := range problems { log.Warn("setting ignored", "problem", p) }
    ctx, cancel := context.WithCancel(context.Background()) // the app context (Q20)
    defer cancel()

    secrets := secret.New(secret.OSKeyring("jenab"))
    registry, err := store.OpenRegistry(ctx, paths.Registry)
    calls := limit.NewGate(settings.LLM.MaxParallelCalls)
    runs  := limit.NewGate(settings.Scheduler.MaxParallelRuns)

    models   := provider.NewRegistry(provider.Deps{Settings: settings.LLM, Secrets: secrets, Gate: calls, Backends: backends.All(), Log: log})
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
func (p Part) Validate() error // exactly one field set, it matches Kind, and the kind's own rules
// JSON names are snake_case (2.3). ToolCall.Extra and Thinking.Signature are strings, so they come back byte for byte.

type SessionNote struct { Chat id.Chat; Content string; Revision int; UpdatedAt time.Time }

// Event payloads (Q32); every one carries its IDs and the chat's sequence number.
type Delta   struct { Project id.Project; Chat id.Chat; Message id.Message; Seq uint64; Part int; Kind PartKind; Text string }
type PartDone struct { Project id.Project; Chat id.Chat; Seq uint64; Message id.Message; Index int; Part Part }
type Status  struct { Project id.Project; Chat id.Chat; Seq uint64; State State; Tasks int; Waiting *Waiting; Retry *Retry; Streaming id.Message }
// Streaming is the answer being streamed. The first PartDone of a message replaces the copy built from its deltas;
// deltas for a message neither stored nor Streaming are dropped (a failed try).
```

### `config`: settings and configs (1, 10)

```go
type Paths struct { Root, Settings, Registry, Logs, DataFolder, Projects string }
func DefaultPaths(appDir string) (Paths, error)
func (p Paths) WithDataFolder(dir string) Paths

type Settings struct { DataFolder string; Context ContextSettings; LLM LLMSettings; Scheduler SchedulerSettings; Approvals ApprovalSettings; UI UISettings; Updates UpdateSettings; DevTools bool }
func Defaults() Settings                                       // from the embedded default.yaml
func LoadSettings(path string) (Settings, []Problem, error)    // problems: ignored keys and values, with lines
func SaveSettings(path string, s Settings) error               // keeps comments and unknown keys

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

// Fair is the 10:1 rule for the writer queues (7.3).
type Fair struct { streak int }
func (f *Fair) Next(interactive, background bool) Priority

// Gate: Interactive never waits but counts as in use; Background waits FIFO (7.6).
type Gate struct { … }
func NewGate(n int) *Gate
func (g *Gate) Acquire(ctx context.Context, p Priority) (release func(), err error)
func (g *Gate) SetSize(n int)    // a changed setting applies at once
func (g *Gate) Stats() GateStats // size, slots in use, waiters
```

### `secret`: keychain and redaction (6.7, Q36)

```go
type Value struct { name, v string }
func (s Value) Reveal() string          // only for the auth header
func (s Value) String() string          // "[secret:NAME]"; also GoString, LogValue, MarshalJSON

type Keyring interface { Get(name string) (string, error); Set(name, v string) error; Delete(name string) error }
func OSKeyring(service string) Keyring  // zalando/go-keyring; the app passes "jenab"

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
    Put(ctx context.Context, r io.Reader, max int64) (Blob, error) // tmp/ while hashing, fsync, rename; ErrTooLarge
    Open(ctx context.Context, hash string) (io.ReadSeekCloser, error)
    Delete(ctx context.Context, hash string) error
    All(ctx context.Context) iter.Seq2[Stored, error]               // for the sweep (4.4)
    CleanTmp(ctx context.Context, t time.Time) (int, error)
}
type Blob struct { Hash string; Size int64; Head []byte }          // Head: the first 512 bytes, for DetectMIME
type Folder struct { … }                                            // objects/ab/12/ab12f9…
func NewFolder(objects, tmp string) *Folder
func CheckKey(key string) error                                     // 4.1
func Link(p id.Project, key string) string                          // jenab://<project_id>/<key> (4.5)
func ParseLink(s string) (id.Project, string, error)
func DetectMIME(head []byte, key string) string                     // from the bytes, never the source (4.7)
func Handler(open Opener) http.Handler                              // /objects/<project_id>/<key>, the 4.7 rules
```

The index tables and the write order (4.3) are in `store`, which calls `bucket`. The handler is mounted on the Wails asset server in `app`.

### `store`: databases, writers, readers (2, 7)

```go
// DB is one database file: a writer goroutine and a reader pool (7.2).
type DB struct { … }
type Options struct { Format Format; Snapshots string; Readers int; WriterPragmas []string; ReadOnly bool } // ReadOnly: readers only, for a damaged file (2.7)
type Format struct { Name string; Steps []func(*sql.Tx) error }     // version = len(Steps), in user_version (2.8)
func Open(ctx context.Context, path string, o Options) (*DB, error) // pragmas, format version (2.8), starts the writer
func (db *DB) Close(ctx context.Context) error                     // drain, checkpoint(TRUNCATE) with 100 ms, close
func (db *DB) Checkpoint(ctx context.Context) (bool, error)         // at idle; false if a reader held it up
func (db *DB) Snapshot(ctx context.Context, target string) error    // VACUUM INTO with the safe-copy rule (7.5)
func (db *DB) Stats() WriterStats                                   // queue per priority, current request and age, last error
func QuickCheck(ctx context.Context, path string) (problem string, err error) // before open, after a crash (2.7); "" when sound

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
func (p *ProjectDB) PutObject(ctx context.Context, pr limit.Priority, src id.Source, key string, r io.Reader, o PutOptions) (Object, error) // bytes first, then the index (4.3)
func (p *ProjectDB) DeleteObject(ctx context.Context, pr limit.Priority, src id.Source, key string) error   // versions and bytes stay
func (p *ProjectDB) MoveObject(ctx context.Context, pr limit.Priority, src id.Source, from, to string) (Object, error)
func (p *ProjectDB) SweepObjects(ctx context.Context) (SweepReport, error) // bytes no version points to, old tmp/ files (4.4); after a crash (2.7)
// Object reads: HeadObject, OpenObject, ListObjects (prefix, "/" delimiter, pages), ObjectVersions

// Runs (6.2)
func (p *ProjectDB) StartRun(ctx context.Context, r Run) error
func (p *ProjectDB) FinishStep(ctx context.Context, s RunStep) error
func (p *ProjectDB) FinishRun(ctx context.Context, id id.Run, st RunState) error

// ChatsDB is chats.db. Every write to a chat adds one to its seq and returns it (Q32).
type ChatsDB struct { *DB }
func OpenChats(ctx context.Context, dir string) (*ChatsDB, error)          // and OpenChatsReadOnly after damage
func (c *ChatsDB) EnsureMother(ctx context.Context) (chat.Chat, error)     // at create and at every open (8.6)
func (c *ChatsDB) CreateChat(ctx context.Context, src id.Source, ch chat.Chat) (chat.Chat, error) // model "default"
func (c *ChatsDB) Chats(ctx context.Context) ([]chat.Chat, error)          // Mother first
func (c *ChatsDB) Messages(ctx context.Context, ch id.Chat, from, to int) ([]chat.Message, uint64, error) // turns, with the seq they are current at
func (c *ChatsDB) AppendMessage(ctx context.Context, m chat.Message) (chat.Message, uint64, error) // before any tool runs (2.3)
func (c *ChatsDB) AppendPart(ctx context.Context, m id.Message, p chat.Part) (int, uint64, error) // a finished part; its index
func (c *ChatsDB) SetTitle(ctx context.Context, ch id.Chat, title string, fixed bool) (string, uint64, error) // unique; generated ones get a number
func (c *ChatsDB) SetRole(ctx context.Context, ch id.Chat, role string, src id.Source) (uint64, error) // ≤ 500 tokens; recorded in role_changes
func (c *ChatsDB) Clear(ctx context.Context, ch id.Chat) (uint64, error)  // messages, notes, review results; any chat
func (c *ChatsDB) SaveNotes(ctx context.Context, n chat.SessionNote) (int, error) // revision rule as memory (3.3); ErrConflict
func (c *ChatsDB) Search(ctx context.Context, s SearchReq) (SearchResult, error) // whole words by FTS, or a 500 ms scan inside words (2.3)
func SearchText(text, query string, max int) ([]TextHit, int, error) // lines whose words start with the query's, as Search compares; search_ref
type SearchReq struct { Query string; Chat id.Chat; Substring bool; Limit int } // Chat "" is the whole project; Limit 20, at most 100
type Hit struct { Chat id.Chat; Message id.Message; Turn int; Role chat.Role; CreatedAt time.Time; Snippet string }
type SearchResult struct { Hits []Hit; Stopped bool }                     // Stopped: the scan ran out of time
func Normalize(s string) string                                            // for indexed text and queries (2.3)
// Also: Chat, Seq, SetModel, SetUsage, Archive and DeleteChat (not for Mother: ErrMother), Notes, RoleChanges.

// Registry is registry.db: one connection, no writer goroutine (2.4).
type Registry struct { … }
func OpenRegistry(ctx context.Context, path string) (*Registry, error)
// Projects, UserMemory, Connections, MCPServers: list, get, save with revision.

var (ErrNotFound, ErrConflict, ErrClosed, ErrNewerFormat, ErrReadOnly error) // ErrNewerFormat: the file is from a newer app

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
func (m *Manager) List(ctx context.Context) ([]store.ProjectEntry, error)  // adds folders the registry lost (2.1)
func (m *Manager) Busy() bool                                              // for the update restart
func (m *Manager) CloseAll(ctx context.Context) error                      // Q30 steps 3–4
func (m *Manager) FolderWarning() *FolderWarning                           // network drive or synced folder (2.1)

type Project struct {
    ID     id.Project
    Name   string
    Dir    string
    DB     *store.ProjectDB
    Chats  *store.ChatsDB // read-only with DB after damage
    Damage *Damage        // set when a quick_check failed; the project is then read-only (2.7)
    // unexported: lease count, idle timer, reporters
}
func (p *Project) Context() context.Context                  // cancelled at close; for background tasks and runs (Q22)
func (p *Project) Release()                                  // the last release starts the 10-minute idle timer
func (p *Project) Report(r Reporter) (unregister func())     // chat runners and runs register (Q19a)
func (p *Project) Activity() Activity                        // one snapshot of every Reporter plus writer Stats
func (p *Project) OnClose(fn func()) (remove func())         // for work that lives as long as the project, e.g. MCP servers

type Reporter interface { Status() []Status }
type Status struct { ID string; Kind, State string; Started, LastActivity time.Time; Progress string; Err string }
type Publisher interface { Notice(Notice); Activity(Activity) }
type Notice struct { Project id.Project; Kind NoticeKind; Text string; Damage *Damage } // recovered, damaged
type Activity struct { Project id.Project; Open bool; Leases int; Work []Status; Writer store.WriterStats }
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
    Model     string         // "<connection>/<modelID>" or an alias; the Registry resolves it
    System    []Block        // with cache points (3.1)
    Messages  []chat.Message
    Tools     []ToolDef      // name, description, JSON Schema; agent converts tool.Spec to this
    MaxTokens int
    Thinking  bool
    Known     *Model         // the catalog entry, set by the Registry: protocol facts such as ThinkingBudget
    Context   int            // the context window, set by the Registry; Ollama sends it as num_ctx
    Image     func(ctx context.Context, ref string) ([]byte, error) // reads image parts from the bucket
}
// Events: deltas (for the screen), finished parts (stored), then done with usage and the stop reason.
// EventWait comes from the Registry while the provider is paused.
type Event struct { Kind EventKind; PartKind chat.PartKind; Text string; Part *chat.Part; Usage *chat.Usage; Stop StopReason; Wait time.Duration }

type Error struct { Kind ErrorKind; Provider string; Status int; RetryAfter time.Duration; Message string; Err error }
// Kinds (3.8): RateLimited, Overloaded, Transport (cut-off and stalled streams too) are retried with the
// waits; Quota and Request (auth included) never are; TooLarge shrinks the context.

// Registry picks the backend for a model and adds the shared rules in one place: the global Gate
// and one Gate per provider (background calls take the provider's slot first), the pause per
// provider and its halved limit (3.8), the stall timeouts, secret redaction in errors.
type Registry struct { … }
func NewRegistry(d Deps) *Registry
func (r *Registry) Stream(ctx context.Context, p limit.Priority, req Request) iter.Seq2[Event, error]
func (r *Registry) Apply(s config.LLMSettings)                     // settings changed: limits, connections
func (r *Registry) Connect(ctx context.Context, name string, kind Kind, baseURL, key string, fields map[string]string) (Connected, error) // 3.9
// Connected: Settings to save, Other models, NoModelList (the user types the models)
func (r *Registry) Models(ctx context.Context, provider string) ([]ModelInfo, error)
func (r *Registry) Status() []Status                               // pauses, limits, last problem
```

- Connections are `llm.providers` in the settings: a name, a kind (`anthropic`, `openai`, `gemini`, `openai_compatible`, `ollama`), a base URL and the models that are on. The key is in the keychain as `provider:<name>`.
- A base URL may hold placeholders such as `{account_id}` (`Placeholders`); their values are in the keychain as `provider:<name>:<field>` (`FieldName`) and filled in when the backend is built.
- `Presets` lists well-known providers for the *Connect* form: a name, kind and base URL, never models.
- A local Ollama (by kind and a loopback base URL) gets a background limit of 1 unless `provider_max_parallel_calls` names it.
- `models.json` is the built-in catalog (3.9): facts only (limits, prices, `thinking_budget`), never prompts.
- Backends implement `Provider` and handle only their protocol. `backends.All()` maps each kind to one, apart from `provider` so there is no import cycle:
  - `provider/anthropic`: the Messages API.
  - `provider/openai`: the Responses API for OpenAI; Chat Completions for Gemini and OpenAI-compatible APIs.
  - `provider/ollama`: Ollama's native `/api/chat`, which takes `num_ctx` and `keep_alive`.
- `provider/fake`: `fake.New(replies ...fake.Reply)` plays scripted events, waits and errors.

### `web` (6.5, 6.7)

```go
type Searcher interface { Search(ctx context.Context, q SearchReq) ([]SearchResult, error) }
type Client struct { … }  // the address checks in the dialer, so they cover DNS answers and every redirect; no proxy
func NewClient(o Options) *Client // Options{Resolver, UserAgent}; tests give a fake resolver
func (c *Client) Search(ctx context.Context, q SearchReq) ([]SearchResult, error)          // Phase 4
func (c *Client) Fetch(ctx context.Context, url string, allow HostCheck) (Page, error) // readable text (3.7); 5 MB, 10 redirects
type Page struct { URL, Title, Text, MIME string; Truncated, NeedsJavaScript bool } // URL after redirects; plain text and JSON as they are
func CheckURL(raw string) (*url.URL, error) // http and https, a host, no user name
type BlockedError struct { URL, Reason string } // loopback, private, link-local, unspecified, multicast; a scheme
type StatusError struct { URL, Status string }
var ErrNotPage error // a PDF, an image, other binary
func (c *Client) Feed(ctx context.Context, urls []string, f FeedFilter) ([]FeedItem, error)
func (c *Client) CallAPI(ctx context.Context, conn config.Connection, key secret.Value, r APIReq) (Response, error) // 6.9
type HostCheck func(host string) bool // hosts allowed to be private (a NAS at home)
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
type Spec struct { Name, Description string; Schema json.RawMessage; Effects Effects; Timeout time.Duration; Mother bool } // Timeout 0 is 1 min; Mother: Mother chat only
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
type Env struct { Project *project.Project; Chat id.Chat; Message id.Message; Source id.Source; Priority limit.Priority; PreviewTokens int; PrivateHosts web.HostCheck; ChatStatus func(id.Chat) string }
// Workspace *workspace.Root joins Env with the workspace package.

type Result struct { Text string; Ref string; Images []chat.Image; Denied bool }
type Error struct { Msg string; Err error } // the model can fix it (Q24)

func Func[A any](s Spec, fn func(ctx context.Context, env *Env, args A) (Result, error)) Tool // Q27; args checked by JSON Schema, then decoded
func Run(ctx context.Context, t Tool, c Call) (Result, error) // with the tool's timeout; running over it is an *Error, a cancel stays a cancel

// Previews and refs (3.7)
func Output(ctx context.Context, c Call, name string, n int, r Result, err error) (chat.ToolResult, error) // stores a large result under cache/tool/
func Stub(r chat.ToolResult) string // a result's header without its size, for previous turns
func HostApproval(host, by string) chat.Approval // the card for a new host (6.7, 8.8)
func Host(u *url.URL) string // lower case and punycode, as approvals name hosts

type Deps struct { Web *web.Client }
func Builtin(d Deps) []Tool // the Phase 1 tools of this package

type Registry struct { … }
func NewRegistry() *Registry
func (r *Registry) Add(ts ...Tool)
func (r *Registry) Get(name string) (Tool, bool)
func (r *Registry) For(k chat.Kind) []Tool // by name; tools with Spec.Mother only for the Mother chat. Skill tools wait for SPIKE-028
```

Where each tool is written:

| Package | Tools |
|---|---|
| `tool` | `query`, `describe_table`, `insert`, `save_view`, `save_page`, `save_pipeline`, `read_config`, `validate`, `update_memory`, `update_session_notes`, `search_history`, `read_messages`, `read_ref`, `search_ref`, `web_search`, `read_feed`, `fetch_page`, `connect_api`, `call_api`, `list_files`, `read_file`, `search_code`, `bucket_*`, `save_link`, `list_chats` (the agent gives each chat's status); later `mcp_describe`, `mcp_call` (Phase 5), `edit_file`, `write_file`, `run_command` (Phase 6) |
| `pipeline` | `dry_run`, `run_pipeline`, `run_status` |
| `agent` | `subagent`, `task_status`, `ask_user`, `create_chat`, `send_to_chat`, `load_skill`, `request_schema_change`, `open_page` |

## 7. `agent`: the orchestrator (8.3, Q16)

```go
type Orchestrator struct { … }
func New(d Deps) *Orchestrator

type Deps struct {
    Context  context.Context // the app's; cancelling it ends every turn
    Projects *project.Manager
    Models   *provider.Registry
    Tools    *tool.Registry
    Settings func() config.Settings // read at the start of each turn
    Events   Publisher
    Skills   Skills // nil until P1-12
    Log      *slog.Logger
}

// Called by app.ChatService; each returns at once.
func (o *Orchestrator) Send(ctx context.Context, p id.Project, c id.Chat, m UserMessage) (id.Message, error) // stores the message, then joins the running turn or queues one; after Stop, a new turn
func (o *Orchestrator) Retry(ctx context.Context, p id.Project, c id.Chat) error // Retry now while waiting, or continue a failed turn (after a restart too); ErrNoRetry otherwise
func (o *Orchestrator) Stop(p id.Project, c id.Chat)                   // the running turn and a queued one
func (o *Orchestrator) Clear(ctx context.Context, p id.Project, c id.Chat) error  // ErrInTurn during a turn; resets the turn count and window
func (o *Orchestrator) Delete(ctx context.Context, p id.Project, c id.Chat) error // ErrInTurn during a turn; not Mother
func (o *Orchestrator) Wrote(p id.Project, c id.Chat, seq uint64)      // a chat write made elsewhere (title, role), so events carry its seq
func (o *Orchestrator) Live(p id.Project, c id.Chat) Live              // for a chat opened mid-turn: the streaming answer's parts and text so far
func (o *Orchestrator) ChatStatus(c id.Chat) string                    // "in a turn" or "", for list_chats and Mother's chat list
func (o *Orchestrator) Answer(p id.Project, c id.Chat, a Answer) error // later (P1-11): approval card or question form (8.8)
func (o *Orchestrator) Refuse()                                        // shutdown step 1: Send, Retry, Clear and Delete fail
func (o *Orchestrator) Wait(ctx context.Context) error

type Publisher interface {
    Delta(chat.Delta)
    Part(chat.PartDone)
    Status(chat.Status) // Retry while a request waits to be tried again; Streaming during a try
    // Called under the chat's lock, so it must not block; a panic is logged, not passed on.
    // OpenPage(OpenPage) comes with the pages (Phase 2)
}

type Skills interface { // block 1's skills (P1-12)
    Block(ctx context.Context, p *project.Project, c chat.Chat) (string, error)
}
```

Inside `agent` (unexported):

```go
type chatState struct { … }  // per chat, kept by the Orchestrator: turn number, seq, the newest user message,
                             // the queued and running turn, the retry wait, the streaming answer and the history window;
                             // its pub lock keeps a store write and its events in seq order
type runner struct { … }     // at most one goroutine per chat: runs the queued turn, then the next one a message queued
                             // meanwhile, and ends when none is left; holds a project lease and reports its Status;
                             // the project's close cancels it and waits up to 5 s for its last writes
func (r *runner) turn(ctx context.Context, n int) (*turn, bool) // steps until the model stops, the cap is hit, or Stop;
                                                                // at the end, a user message the last request didn't see means another step
func (r *runner) step(ctx context.Context, t *turn, last bool) (more bool, err error)
    // 1. request: shape the window (stubs, results for calls a crash left open), trim, tools (none on the last request)
    // 2. ask: stream into memory, deltas to the coalescer; retry with the wait shown, up to 10 minutes;
    //    a message sent during the wait goes into the next try
    // An answer without calls that hit the output limit, was refused or has no text adds an answer_cut notice
    // 3. write the answer in one AppendMessage, then run its calls and write their results in one tool message
func (r *runner) runTool(ctx context.Context, t *turn, msg id.Message, n int, c chat.ToolCall) []chat.Part // recover, error mapping, cancelled on Stop
func (r *runner) startTitle(ctx context.Context, wg *sync.WaitGroup, t *turn) // after the first turn, with the fast model, beside
    // the next turn; one at a time, dropped if the chat was cleared meanwhile; the runner waits for it

type window struct { start, stubBelow int; stubbed map[string]bool; chatList string; system []provider.Block; last time.Time } // 3.6;
    // the system blocks are built at a cut and kept until the next, so a role change waits for it
func (r *runner) cutIfDue(ctx context.Context, t *turn) error // past the max turns or tokens, idle over 5 min, or a new process
func shape(ms []chat.Message, w *window) []chat.Message
type coalescer struct { … }             // ≤ 1 Delta per 16 ms per chat (Q17); a new message, part or kind is sent at once

// Later:
type task struct { … }                  // a background subagent or a Mother chat task; its finish notice starts a turn
type subagent struct { … }              // not a chat: in-memory messages, no subagent tool of its own (one level);
                                        // writes use the parent's Source; at the end its transcript goes to the bucket
                                        // and the parent's tool_result gets the ref (8.6)
type schemaAgent struct { … }           // 8.2: steps, try, needs_approval
func approve(t *turn, s tool.Spec, n tool.Needs) []chat.Approval // 8.8 levels, the turn's Untrusted mark, in one place
```

IDs: each try makes its answer's ID and its tool message's ID when it starts, under the chat's lock like Send, and messages sort by (turn, ID). So a user message sent during the try sorts after the answer and its results, and a retried try leaves nothing behind.

A turn in one picture:

```
Send → AppendMessage → runner.turn → step ─┬─ models.Stream ──► coalescer ──► Publisher.Delta
                                           ├─ ChatsDB.AppendMessage ──► Publisher.Part
                                           └─ runTool ──► tool.Run ──► ProjectDB (writer)
```

Store and provider additions (P1-10): `ChatsDB.LastTurn`, `ChatsDB.LastActivity` (with the `messages_activity` index), `provider.Registry.Resume` (ends a provider's pause and wakes the calls waiting on it), `Event.Paused` and the `EventWait` with `Wait` 0 when the wait is over, `fake.Provider.Replace`.

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
func objectHandler(pm *project.Manager) http.Handler // bucket.Handler with an Opener over the open projects
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

1. **Retry after text was shown.** Decided in P1-10: each try streams a new message (`Status.Streaming`), and the frontend drops the deltas of a try that failed, so a retry never shows text twice.
2. **The frontend folder.** `frontend/src` starts from `mockups/src`; which parts carry over is decided when Phase 1 starts.
