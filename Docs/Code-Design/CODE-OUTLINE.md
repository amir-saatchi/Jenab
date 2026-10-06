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
7. **`internal/sysmem`** (P1-14): free memory and its level (ok, low, critical) for the bottom bar (5.12), and later for the memory check before a command starts (8.5). Commit headroom out of the commit limit on Windows, MemAvailable out of MemTotal on Linux, the pressure level on macOS.

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
sqlguard, bucket, workspace, proc, skill
id, chat, config, limit, secret, logfile, sysmem
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
    lock, err := lockRoot(paths.Root)                           // app.lock: one copy of the app at a time
    settings, problems := config.LoadSettingsOrDefaults(paths.Settings) // a broken config.yaml gives Defaults and a Problem
    paths = paths.WithDataFolder(settings.DataFolder)           // projects/ (2.1)
    log, closeLog, err := logfile.Open(paths.Logs, settings.DevTools)
    defer closeLog()
    for _, p := range problems { log.Warn("setting ignored", "problem", p) }
    ctx, cancel := context.WithCancel(context.Background()) // the app context (Q20)
    defer cancel()

    live    := app.NewSettings(paths.Settings, settings, problems) // a save applies at once
    secrets := secret.New(secret.OSKeyring("jenab"))
    registry, err := store.OpenRegistry(ctx, paths.Registry)
    moved, err := project.MoveProjects(ctx, paths, registry, log) // the data folder changed (2.1); logged, not fatal
    calls := limit.NewGate(settings.LLM.MaxParallelCalls)
    runs  := limit.NewGate(settings.Scheduler.MaxParallelRuns)

    models   := provider.NewRegistry(provider.Deps{Settings: settings.LLM, Secrets: secrets, Gate: calls, Backends: backends.All(), Log: log})
    projects := project.NewManager(project.Deps{Paths: paths, Registry: registry, Log: log})
    webc     := web.New(web.Deps{Secrets: secrets, Searchers: web.Searchers(settings, secrets)})
    tools    := tool.NewRegistry()
    tool.AddBuiltins(tools, tool.Deps{Web: webc, Registry: registry, Secrets: secrets})

    wapp   := app.New(app.Deps{Name: appName, Assets: assets, WebviewData: filepath.Join(paths.Root, "webview"), Log: log}) // first, so events can be sent
    runner := pipeline.New(pipeline.Deps{Projects: projects, Models: models, Gate: runs, Web: webc, Events: wapp.Publisher(), Log: log})
    tools.Add(runner.Tools()...)
    var traces *agent.Traces // the turn inspector's record (8.4), only with DevTools on at the start
    if settings.DevTools { traces = agent.NewTraces() }
    orch   := agent.New(agent.Deps{Projects: projects, Models: models, Tools: tools, Runs: runner, Settings: live.Get, Events: wapp.Publisher(), Traces: traces, Log: log})
    sched  := schedule.New(schedule.Deps{Wake: schedule.OSWake(), Start: runner.StartScheduled, Log: log})
    upd    := update.New(update.Deps{Settings: settings.Updates, Busy: projects.Busy, Log: log})

    live.OnChange(func(s config.Settings) { calls.SetSize(s.LLM.MaxParallelCalls); models.Apply(s.LLM) })

    wapp.Bind(app.Services{Orchestrator: orch, Runner: runner, Projects: projects, Registry: registry, Models: models, Secrets: secrets, Settings: live, Traces: traces, Logs: paths.Logs, Updates: upd, Log: log})
    wapp.OnStart(func(ctx context.Context) { go sched.Run(ctx); go upd.Run(ctx) }) // owned by the app root (Q15)
    wapp.OnShutdown(app.Shutdown{Refuse: {sched.Stop, orch.Refuse}, Cancel: cancel, Wait: {orch.Wait, runner.Wait}, Close: projects.CloseAll, Log: log}.Run) // Q30
    return wapp.Run()
}
```

`wapp.OnShutdown` hides the window, then runs the function on Wails' `OnShutdown`. `wapp.OnStart` comes with the scheduler (Phase 3), over a Wails service startup hook.

`app.Shutdown.Run` runs the Q30 order within 10 s: the `Refuse` functions (no new work), `Cancel` the app context and wait up to 5 s for the `Wait` functions, then `Close` (drain writers, checkpoint, remove the lock files) with the rest of the time. Errors are joined and logged.

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
// Approval (8.8): ID, Kind ("host", "starlark" …), Target, Ask, Why, Options, then Answer, Note, By and AnsweredAt;
// Stopped when Stop closed it unanswered. Question: ask_user's items, then Answers, Note, AnsweredAt or Stopped.
type ApprovalOption struct { Label string; Grant Grant } // a card needs a Deny option and no grant twice
type Grant string // GrantOnce, GrantAlways, GrantDeny
// JSON names are snake_case (2.3). ToolCall.Extra and Thinking.Signature are strings, so they come back byte for byte.

type SessionNote struct { Chat id.Chat; Content string; Revision int; UpdatedAt time.Time }

// Event payloads (Q32); every one carries its IDs and the chat's sequence number.
type Delta   struct { Project id.Project; Chat id.Chat; Message id.Message; Seq uint64; Part int; Kind PartKind; Text string; Offset int } // Offset: where Text starts in the part, in UTF-16 units
type PartDone struct { Project id.Project; Chat id.Chat; Seq uint64; Message id.Message; Index int; Turn int; Part Part }
type Status  struct { Project id.Project; Chat id.Chat; Seq uint64; State State; Tasks int; Waiting *Waiting; Retry *Retry; Streaming id.Message }
// Streaming is the answer being streamed. The first PartDone of a message replaces the copy built from its deltas;
// deltas for a message neither stored nor Streaming are dropped (a failed try).
```

### `config`: settings and configs (1, 10)

```go
type Paths struct { Root, Settings, Registry, Logs, LastData, DataFolder, Projects string } // LastData: the folder used last time
func DefaultPaths(appDir string) (Paths, error)
func (p Paths) WithDataFolder(dir string) Paths

type Settings struct { DataFolder string; Context ContextSettings; LLM LLMSettings; Scheduler SchedulerSettings; Approvals ApprovalSettings; UI UISettings; Updates UpdateSettings; DevTools bool }
func Defaults() Settings                                       // from the embedded default.yaml
func LoadSettings(path string) (Settings, []Problem, error)    // problems: ignored keys and values, with lines
func LoadSettingsOrDefaults(path string) (Settings, []Problem) // at start: a file that doesn't parse gives Defaults and a Problem
func SaveSettings(path string, s Settings) error               // keeps comments, unknown keys and entries rejected at load; a broken file is copied to config.yaml.broken first

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

### `skill`: skills (8.9)

Imports only `chat`. A skill is a folder: `SKILL.md` (front matter and body) and extra `.md` files.

```go
type Skill struct { Name, Description string; LoadWith []string; Body string; Files map[string]string; Mother bool } // Mother: only Mother lists it; set by the app
func Parse(file string, src []byte) (Skill, error) // a SKILL.md; each bad field is its own *Error
func Read(fsys fs.FS, name string) (Skill, error)  // the folder name: SKILL.md, extra files, the name matches the folder
type Error struct { File, Field, Msg string }      // errors.Is(err, ErrInvalid)
func (s Skill) Text() string                       // "Skill loaded: <name>" and the body: load_skill's result and block 1's text
func Tokens(s string) int                          // 4 bytes a token, as requests are counted
const (MaxDescription = 200; MaxBodyTokens = 3000; MaxLoaded = 6; MaxLoadedTokens = 10000)

type Set struct { … } // by name; names are unique
func New(ss ...Skill) (*Set, error)
func Builtin() (*Set, error)                                 // embedded in the app; Phase 1 has config-guide
func (s *Set) For(k chat.Kind) []Skill                       // the skills a chat can use; a nil Set has none
func (s *Set) Get(k chat.Kind, name string) (Skill, bool)
func (s *Set) With(k chat.Kind, tool string) []Skill         // load_with
func (s *Set) Block(k chat.Kind, loaded []string) string     // the loaded skills' text, then the `## Skills` list
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

// Approvals (8.8): the newest decision for a (kind, target) counts, so a later Deny revokes an Always.
type Approval struct { ID id.Approval; Kind, Target string; Answer chat.Grant; Note string; Source id.Source; CreatedAt time.Time }
func (p *ProjectDB) RecordApproval(ctx context.Context, a Approval) error
func (p *ProjectDB) Approved(ctx context.Context, kind, target string) (bool, error) // the newest decision is Always
func (p *ProjectDB) Approvals(ctx context.Context) ([]Approval, error)              // newest first, for project settings

// ChatsDB is chats.db. Every write to a chat adds one to its seq and returns it (Q32).
type ChatsDB struct { *DB }
func OpenChats(ctx context.Context, dir string) (*ChatsDB, error)          // and OpenChatsReadOnly after damage
func (c *ChatsDB) EnsureMother(ctx context.Context) (chat.Chat, error)     // at create and at every open (8.6)
func (c *ChatsDB) CreateChat(ctx context.Context, src id.Source, ch chat.Chat) (chat.Chat, error) // model "default"
func (c *ChatsDB) Chats(ctx context.Context) ([]chat.Chat, error)          // Mother first
func (c *ChatsDB) Usage(ctx context.Context, from time.Time, offset time.Duration) ([]UsageRow, error) // tokens by local day, chat and model
func (c *ChatsDB) Messages(ctx context.Context, ch id.Chat, from, to int) ([]chat.Message, uint64, error) // turns, with the seq they are current at
func (c *ChatsDB) AppendMessage(ctx context.Context, m chat.Message) (chat.Message, uint64, error) // before any tool runs (2.3)
func (c *ChatsDB) AppendPart(ctx context.Context, m id.Message, p chat.Part) (int, uint64, error) // a finished part; its index
func (c *ChatsDB) SetSkills(ctx context.Context, ch id.Chat, skills []string) (uint64, error) // the chat's loaded skills (8.9)
func (c *ChatsDB) SetPart(ctx context.Context, m id.Message, i int, p chat.Part) (uint64, error) // an approval or question part, once answered or closed (8.8); ErrNotPending if it already was
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

Store-only types: `Run`, `RunStep`, `ChangeEntry`, `Memory`, `Object`, `Migration` and its steps (8.2).

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
func (m *Manager) Activities() []Activity                                  // the open projects' Activity by name, for the runtime panel; opens nothing

// MoveProjects runs at start, before any project opens: every registered project under the projects
// folder of an earlier data folder moves to the current one (a rename, or a copy only across drives:
// copy, rename into place, update the registry, then delete the old folder). The new folder is
// recorded only when all moved, so a failed one is tried again at the next start. A registered
// folder that is gone but sits in Projects/<id> is pointed there (also in Manager.load and scan).
// ErrNested: the new data folder and the old one are inside each other.
func MoveProjects(ctx context.Context, paths config.Paths, reg *store.Registry, log *slog.Logger) (*MoveResult, error)

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

// Settings in _jenab_meta. A new project's level comes from Deps.Level (approvals.default_level in config.yaml).
type Level string // Strict, Standard, Auto (8.8)
func ParseLevel(s string) (Level, error)
func (p *Project) Level(ctx context.Context) (Level, error)                 // Standard when none or an unknown one is stored
func (p *Project) SetLevel(ctx context.Context, l Level) error
func (p *Project) PrivateHosts(ctx context.Context) ([]string, error)       // private-network exceptions (6.7)
func (p *Project) SetPrivateHosts(ctx context.Context, hosts []string) error // lower case, punycode, sorted, no duplicates

type Reporter interface { Status() []Status }
type Status struct { ID string; Kind, State string; Title string; Started, LastActivity time.Time; Limit time.Duration; Progress string; Err string }
// Limit: how long the work may go without moving before the runtime panel flags it (8.4): the provider's
// first-event limit during a request, the tool's timeout during a tool call, 0 while it waits for the user or a retry.
type Publisher interface { Notice(Notice); Activity(Activity) }
type Notice struct { Project id.Project; Kind NoticeKind; Text string; Damage *Damage } // recovered, damaged
type Activity struct { Project id.Project; Name string; Open bool; Leases int; Work []Status; Writer store.WriterStats }
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
// Errors wrap ErrBadName, ErrBadBaseURL, ErrBadField, ErrNeedKey (only a local Ollama goes without) or
// ErrKeyTooLong; a URL in a message has no user info, query or fragment.
func (r *Registry) Models(ctx context.Context, provider string) ([]ModelInfo, error)
func (r *Registry) Status() []Status                               // pauses, limits, last problem
func (r *Registry) Catalog() *Catalog                              // the built-in catalog, for names and alias picks
func (r *Registry) Calls() limit.GateStats                         // the max_parallel_calls slots, for the runtime panel
func (r *Registry) FirstEvent(provider string) time.Duration       // the stall limit before the first event (3.8)
```

`Block.Name` names a system block for the turn inspector; backends don't send it.

- Connections are `llm.providers` in the settings: a name, a kind (`anthropic`, `openai`, `gemini`, `openai_compatible`, `ollama`), a base URL and the models that are on. The key is in the keychain as `provider:<name>`.
- A base URL may hold placeholders such as `{account_id}` (`Placeholders`); their values are in the keychain as `provider:<name>:<field>` (`FieldName`) and filled in when the backend is built.
- `Presets` lists well-known providers for the *Connect* form: a name, kind and base URL, never models.
- `Connect` needs a key for every kind but Ollama, as calls without a stored key are refused.
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
func (c *Client) Fetch(ctx context.Context, url string, r Rules) (Page, error) // readable text (3.7); 5 MB, 10 redirects
type Rules struct { Private HostCheck; Redirect RedirectCheck } // Redirect is asked before a redirect to another host
type RedirectCheck func(ctx context.Context, to *url.URL) error  // fetch_page: the host must be approved (6.7)
type RedirectError struct { URL string; Err error }              // a refused redirect; the model can fetch URL itself
func Host(u *url.URL) string // lower case, punycode, no trailing dot, IPs in standard form
type Page struct { URL, Title, Text, MIME string; Truncated, NeedsJavaScript bool } // URL after redirects; plain text and JSON as they are
func CheckURL(raw string) (*url.URL, error) // http and https, a host, no user name, no numeric host but dotted IPv4
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
type Needs struct { Effects Effects; Approvals []chat.Approval } // the agent asks for, or auto-approves, each approval before Run (8.8)

// Call carries the arguments and where the call runs.
type Call struct { ID string; Args json.RawMessage; Env *Env }
type Env struct { Project *project.Project; Chat id.Chat; Message id.Message; Source id.Source; Priority limit.Priority; PreviewTokens int; PrivateHosts web.HostCheck; ChatStatus func(id.Chat) string;
    Ask func(ctx context.Context, q chat.Question) (chat.Question, error);   // Ask: shows a question form and waits; nil for subagents
    Skill func(ctx context.Context, name, file string) (Result, error) }     // load_skill (8.9); set by the agent
// Workspace *workspace.Root joins Env with the workspace package.

type Result struct { Text string; Ref string; Images []chat.Image; Whole bool } // Whole: never stored with a preview (a skill)
type Error struct { Msg string; Err error } // the model can fix it (Q24)

func Func[A any](s Spec, fn func(ctx context.Context, env *Env, args A) (Result, error)) Tool // Q27; args checked by JSON Schema, then decoded
func Run(ctx context.Context, t Tool, c Call) (Result, error) // with the tool's timeout (none for AsksUser); running over it is an *Error, a cancel stays a cancel

// Previews and refs (3.7)
func Output(ctx context.Context, c Call, name string, n int, r Result, err error) (chat.ToolResult, error) // stores a large result under cache/tool/
func Stub(r chat.ToolResult) string // a result's header without its size, for previous turns
func HostApproval(host, by string) chat.Approval // the card for a new host (6.7, 8.8)
func Host(u *url.URL) string // web.Host, as approvals name hosts

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
    Skills   *skill.Set // the skills chats can load (8.9); nil has none
    Traces   *Traces    // the turn inspector's record (8.4); nil records nothing
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
func (o *Orchestrator) ChatStatus(c id.Chat) string                    // "waiting for the user", "in a turn" or "", for list_chats and Mother's chat list
func (o *Orchestrator) State(p id.Project, c id.Chat) chat.State       // idle, working (running or queued) or waiting, for the chat list
func (o *Orchestrator) Waits() []Wait                                  // every chat waiting for the user, by chat ID, for *Waiting* in the rail
func (o *Orchestrator) Answer(p id.Project, c id.Chat, a Answer) error // the card or form the turn waits for (8.8); ErrNotWaiting, ErrBadAnswer
func (o *Orchestrator) SetLevel(ctx context.Context, p id.Project, c id.Chat, l project.Level) error // from the next tool call; adds a notice to the chat
func (o *Orchestrator) Refuse()                                        // shutdown step 1: Send, Retry, Clear and Delete fail
func (o *Orchestrator) Wait(ctx context.Context) error

// Answer: the card's message and part index, then an approval's Grant, or a form's Answers by header
// (the picked labels or the Other text), and a Note. A message sent while waiting is a Deny with the message as the note.
type Answer struct { Message id.Message; Index int; Grant chat.Grant; Answers map[string][]string; Note string }
// Live also has Waiting, the card or form the turn waits for.
type Wait struct { Project id.Project; Chat id.Chat; Waiting chat.Waiting }

// Traces keeps what recent turns sent and got back, in memory only: the last 100 turns, and 32 MiB of request
// text across them. Over that, the oldest requests lose their texts (Dropped); their block sizes stay.
type Traces struct { … }
func NewTraces() *Traces
func (s *Traces) List() []TurnTrace                    // newest first, without the request texts
func (s *Traces) Turn(k TraceKey) (TurnTrace, bool)
type TurnTrace struct { Project; Chat; Turn int; Title, Model string; Started, Ended time.Time; Requests []RequestTrace; Tools []ToolTrace }
type RequestTrace struct { Started time.Time; Took time.Duration; Done, Last bool; Req provider.Request; Dropped bool;
    Blocks []TraceBlock; Message id.Message; Usage chat.Usage; Stop provider.StopReason; Parts []chat.PartKind; Err string }
type TraceBlock struct { Name string; Tokens int; Cache bool } // tokens estimated at 4 bytes; Cache: a cache point after it
type ToolTrace struct { Request int; CallID, Name string; Started time.Time; Took time.Duration; Bytes int; Ref string; Error bool }
func Blocks(req provider.Request, turn int) []TraceBlock // Tools, each system block, History window, This turn
func SplitTurn(ms []chat.Message, turn int) (before, now []chat.Message)
// The runner records one RequestTrace per try, around the stream, and one ToolTrace per tool call.

func Tools() []tool.Tool // ask_user (1–4 questions, 2–4 options each; Other is added by the UI); later the other agent tools

type Publisher interface {
    Delta(chat.Delta)
    Part(chat.PartDone)
    Status(chat.Status) // Retry while a request waits to be tried again; Streaming during a try
    // Called under the chat's lock, so it must not block; a panic is logged, not passed on.
    // OpenPage(OpenPage) comes with the pages (Phase 2)
}

// Tools() also has load_skill(name, file): it loads a skill into the chat (chats.skills, a skill_loaded notice
// in the tool message for the chip), or reads an extra file. At most skill.MaxLoaded skills or MaxLoadedTokens a chat.
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
func (r *runner) runTool(ctx context.Context, t *turn, tm *toolMsg, msg id.Message, n int, c chat.ToolCall) []chat.Part
    // preflight, approvals, private hosts, then the tool: recover, error mapping, cancelled on Stop;
    // after an Untrusted tool the chat is marked untrusted until the user's next message
type toolMsg struct { … } // an answer's tool message: its results and the cards and forms they wait for
func (r *runner) approve(ctx context.Context, n tool.Needs) (asks, autos []chat.Approval, err error)
    // 8.8 levels and the untrusted mark, in one place: approvals already given are dropped; autos are recorded with source auto
func (r *runner) await(ctx context.Context, tm *toolMsg, p chat.Part) (chat.Part, error)
    // writes the card or form, waits with no transaction held (status "waiting"), writes the answer into the same part;
    // Stop marks it stopped
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
```

IDs: each try makes its answer's ID and its tool message's ID when it starts, under the chat's lock like Send, and messages sort by (turn, ID). So a user message sent during the try sorts after the answer and its results, and a retried try leaves nothing behind.

A turn in one picture:

```
Send → AppendMessage → runner.turn → step ─┬─ models.Stream ──► coalescer ──► Publisher.Delta
                                           ├─ ChatsDB.AppendMessage ──► Publisher.Part
                                           └─ runTool ──► tool.Run ──► ProjectDB (writer)
```

Skills (8.9): block 1 has the chat's loaded skills after the role, then the skill list. A tool's first call in a chat also
loads the skills whose `load_with` names it, if they fit, and adds their text to its result; in-turn trimming leaves
that result whole. The text in results becomes a stub at the next cut, when it moves into block 1.

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
type Deps struct { Name string; Assets fs.FS; WebviewData string; Log *slog.Logger }
func New(d Deps) *App                     // Wails logs at Warn and above only
func (a *App) Publisher() *Publisher
func (a *App) Bind(s Services)            // the services and the objects route; DevService only with DevTools on at the start
func (a *App) OnShutdown(fn func())
func (a *App) Run() error

type Services struct { Orchestrator; Projects; Registry; Models; Secrets; Settings *Settings; Started Started; Traces *agent.Traces; Logs string; Log }
type Bound struct { Project *ProjectService; Chat *ChatService; Settings *SettingsService; Bucket *BucketService; System *SystemService; Dev *DevService }
func NewServices(s Services) Bound        // what Bind registers; tests call the services without Wails

// Settings holds the settings while the app runs (7.6).
func NewSettings(path string, s config.Settings, problems []config.Problem) *Settings
func (s *Settings) Get() config.Settings
func (s *Settings) OnChange(fn func(config.Settings)) // after each save

type ProjectService struct { … } // List, Create, Open → OpenedProject{ID, Name, Mother, Level, Damage}, Activity, FolderWarning
type ChatService struct { … }
func (s *ChatService) List(ctx context.Context, p id.Project) ([]ChatItem, error) // the chat, its State, last activity
func (s *ChatService) Waiting(ctx context.Context) ([]WaitingItem, error)        // every chat waiting, in every open project, with the project's title
func (s *ChatService) Snapshot(ctx context.Context, p id.Project, c id.Chat) (ChatSnapshot, error) // last 30 turns + Seq + Live (Q32)
func (s *ChatService) Messages(ctx context.Context, p id.Project, c id.Chat, from, to int) ([]chat.Message, error) // older turns
func (s *ChatService) Send(ctx context.Context, p id.Project, c id.Chat, text string) (id.Message, error)
func (s *ChatService) Answer(ctx context.Context, p id.Project, c id.Chat, a agent.Answer) error
// Also Create, Stop, Retry, Clear, Delete, Rename, SetRole, SetModel, Archive, SetLevel, Search.
// SettingsService: Get, Save → SettingsView{Settings, Problems}; Providers → []ProviderStatus;
//   Started → the data folder, dev tools and MoveResult of this start; CheckFolder → a FolderWarning;
//   Models → []ModelGroup (the models that are on, by provider, with their aliases);
//   Presets → []PresetItem (a running local Ollama first, with no key);
//   Connect(ConnectRequest) → ConnectResult: checks the key by listing models, stores it, saves the
//   provider with its catalog models on, and points default and fast at it when they point nowhere (3.9).
//   Connect again with the same name, kind and base URL replaces the key and keeps the models; another
//   kind or base URL under a taken name is invalid. Settings changes go through Settings.update, one at a time.
//   Remove(name): deletes the provider, its limit and its key; aliases move to another provider's models.
//   ProviderModels(name) → the catalog models it lists, *Other models* the catalog doesn't know, NoModelList.
//   Usage(days) → UsageReport: tokens by day, chat and model across all projects, with catalog prices.
// BucketService: List → ObjectPage (100 a page), Versions.
// DevService (8.4), read-only, every text redacted: Providers, Log;
//   Turns → []TurnItem (the recorded turns, newest first); Turn(p, c, n) → TurnView: requests with usage,
//   parts and context blocks (Tokens, Point, Cache hit, part or miss: the estimates scaled to the provider's
//   prompt count, compared with its cache read), and tool calls, times from the turn's start; not_found if not recorded;
//   Block(p, c, n, request, block) → the block's text, as BlockText renders it from what was sent;
//   Runtime → the LLM-call slots, providers, and per open project its work and writers, with Stuck flags
//   (work past its Limit, a write running over 30 s).
// SystemService: Memory → sysmem.Reading{Total, Free, Level} for the bottom bar (ok, low or critical);
//   Notify(Notification{Project, Chat, Title, Body}) → a desktop notification (Wails notifications);
//   a click shows the window and sends app:open with the chat.
// Later tickets add files to Send, UndoTurn, PageService and PipelineService in the same style.

func windowTheme(theme string, dark bool) (application.RGBA, application.Theme) // the window's background before the
    // first paint, from the saved theme (System reads Windows' setting); a save changes the background, not the frame

type UIError struct { Kind, Message, Details string } // MarshalJSON → the error's cause in JavaScript (Q25)
// Kinds: invalid, not_found, busy, read_only, closing, provider, internal.
func toUI(err error, log *slog.Logger, redact func(string) string, what string) error // known errors get a message; else logged, "Something went wrong."

type Publisher struct { … }               // implements agent and project Publisher with Event.Emit
func objectHandler(pm *project.Manager) http.Handler // bucket.Handler with an Opener that leases the project per request

type Shutdown struct { Refuse []func(); Cancel func(); Wait []func(context.Context) error; Close func(context.Context) error; Log *slog.Logger; Exit func() }
func (s Shutdown) Run() error             // the Q30 order within 10 s; past it plus 2 s, Exit (nil: os.Exit(1))
```

```go
// events.go: the only init() (see section 0)
func init() {
    application.RegisterEvent[chat.Delta]("chat:delta")
    application.RegisterEvent[chat.PartDone]("chat:part")
    application.RegisterEvent[chat.Status]("chat:status")
    application.RegisterEvent[project.Notice]("project:notice")
    application.RegisterEvent[project.Activity]("project:activity")
    application.RegisterEvent[Open]("app:open") // a desktop notification was clicked
    // run:status (pipeline.RunStatus) comes with pipelines.
}
```

Every service method starts with `defer s.guard("what", &err)` (Q26). It recovers a panic and turns the error into a `*UIError`, so errors are logged once, here. The config structs have `json` tags that mirror their `yaml` tags, so the frontend sees config.yaml's names.

The bindings are generated into `frontend/bindings` by `wails3 task bindings` and committed; CI fails if they are stale (DEVELOPMENT.md).

## 10. `frontend`

React 19, Vite, Tailwind v4 and shadcn/ui (radix-nova), with Zustand for state. Tests run with `bun test`.

```
src/
  main.tsx, App.tsx     start: settings, projects and *Waiting*, then the project opened last
  index.css             the tokens for both themes, the tones and the Mother gradient (from the mockups)
  typeset.css           Markdown styling (5.8), with the typeset-chat and typeset-page presets in index.css
  components/ui/        shadcn components, copied from the mockups; changed only where noted in the file
  lib/                  api.ts (bindings and enums), errors.ts (UIError, showError with Copy details), theme.ts,
                        blocks.ts (Markdown split into blocks), dir.ts (a text's direction, as dir="auto" finds it)
  state/                Zustand stores, one per concern
    nav.ts              history (back and forward), the most recently used chats (Ctrl+Tab), the sidebar
    projects.ts         the list, the opened projects (Mother, level, damage), the folder warning
    chats.ts            each project's chat list, and *Waiting*
    settings.ts         the settings view; edit, connect, removeProvider and setTheme run one at a time, each on the last saved settings; the view shows pending edits, a failed one is taken out
    ui.ts               dialogs and overlays
    thread.ts           each open chat: its messages from the snapshot, then chat:part and chat:status
    stream.ts           the answer streaming, outside React; deltas applied once per frame; a chat read mid-answer skips the text Live holds by Delta.Offset
    events.ts           the events into the stores; the desktop notification when a chat starts waiting off screen
  chat/                 the chat (5.8, 8.3, 8.8): thread view, rows, parts (thinking, tool chips, notices, skill chips),
                        markdown (lazy), cards (approval, question, waiting and retry bars), composer, provider card
  shell/                the window (5.12): rail, chat sidebar, main area, right sidebar, bottom bar,
                        Settings, project settings, the welcome screen
  settings/             the Settings sections (3.9): general (data folder, approval level), models (providers,
                        their models, aliases, limits), usage (recharts chart, lazy), developer
  dev/                  the developer tools (8.4), wide pages under Settings → Developer: the turn inspector
                        (timeline, requests, tool calls, context blocks and their text) and the runtime panel
  hooks/                use-poll.ts (a call every n ms while the window shows, never two at once; a failure clears the value), use-mobile.ts
```

- **Snapshots, then events:** a store loads with a service call, and `events.ts` keeps it up to date. A chat that goes idle, or one the store doesn't know, reloads the chat list.
- **A chat's thread:** the snapshot is read first and the events that come meanwhile are replayed after it. A part with a seq the thread already has is dropped; a gap reads the snapshot again, and so does the end of each turn.
- **Streaming (SPIKE-022):** only the streaming part re-renders, once per frame; rows are memoised, and Markdown is parsed block by block, so a finished block isn't parsed again. react-markdown and the highlighter load lazily.
- **Opening a chat (N-02):** the last rows, about two screens, render at once; the rest follow in a transition.
- **Theme:** `localStorage` holds a copy of the choice, read by a script in `index.html`, so the first paint has the right colours. The Go side sets the window background from the same setting.
- **Below 900 px** the rail and both sidebars are sheets, and the bottom bar is hidden.
- **Contrast (N-53):** `contrast.test.ts` reads the tokens from `index.css` and checks every text pair in both themes.

## 11. Open points

1. **Retry after text was shown.** Decided in P1-10: each try streams a new message (`Status.Streaming`), and the frontend drops the deltas of a try that failed, so a retry never shows text twice.
2. **The frontend folder.** Decided in P1-14: from `mockups/src` came the shadcn components, `index.css` (without `typeset.css`), `components.json`, the fonts (Geist, Geist Mono, Vazirmatn) and the settings layout. The screens and the fake data stay in `mockups/`; the chat, page and pipeline parts move over with their tickets. P1-15 brought `typeset.css` and the chat components (bubble, message, message-scroller, marker, questionnaire and others). P1-16 brought switch, select, table, alert-dialog and chart, with recharts 3.8.0.
