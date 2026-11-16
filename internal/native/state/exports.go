package state

import (
	"context"
	"errors"
	"regexp"
	"runtime"
	"strings"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/chatexport"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
)

// ExportsAPI is what Exports asks the services; *app.ProjectService has
// the project's half and *app.ChatService the chat's (ChatExportAPI).
type ExportsAPI interface {
	ExportEstimate(ctx context.Context, p id.Project) (project.ExportEstimate, error)
	Export(ctx context.Context, p id.Project, r app.ExportRequest) (project.ExportResult, error)
	StopExport(ctx context.Context, p id.Project) (bool, error)
	ExportStatus(ctx context.Context, p id.Project) (*project.ExportProgress, error)
}

// ChatExportAPI exports a chat; *app.ChatService has it.
type ChatExportAPI interface {
	Export(ctx context.Context, p id.Project, c id.Chat, r app.ChatExportRequest) (chatexport.Result, error)
}

// SaveRequest is what the save dialog shows: its title, the file name it
// suggests, and the one kind of file it lists (Ext "" for a folder).
type SaveRequest struct {
	Title      string
	Name       string
	FilterName string
	Ext        string
}

// ProjectExport is a project's export as its dialog shows it
// (shell/export-project.tsx): the size first, then the progress, then how
// it ended.
type ProjectExport struct {
	Estimate *project.ExportEstimate
	Progress *project.ExportProgress
	Result   *project.ExportResult
	Error    string
	Running  bool
	Stopping bool

	live  bool // the dialog is open: events and reads apply
	ended bool // an event said how the export ended
	own   bool // this dialog's own Export call runs; its failure shows from its error
}

// ChatExport is a chat's export as its dialog shows it (export-chat.tsx).
type ChatExport struct {
	Result  *chatexport.Result
	Error   string
	Running bool
}

// Exports runs the exports of projects (SPEC 2.1) and of chats (2.3):
// the save dialog, the call, and the project:export events.
type Exports struct {
	Projects map[id.Project]*ProjectExport
	Chats    map[id.Chat]*ChatExport
	// Save shows the save dialog and returns the path picked, "" when the
	// user cancels it. It blocks, so it runs off the UI thread; nil makes
	// every export fail.
	Save func(SaveRequest) (string, error)
	// Linux is set on Linux, where the save dialog may not ask before it
	// picks a file that is there, so a zip export never replaces one.
	Linux bool

	api     ExportsAPI
	chats   ChatExportAPI
	post    Post
	onError func(error)
}

// NewExports makes the store. onError shows the errors no dialog takes.
func NewExports(api ExportsAPI, chats ChatExportAPI, post Post, onError func(error)) *Exports {
	return &Exports{Projects: map[id.Project]*ProjectExport{}, Chats: map[id.Chat]*ChatExport{},
		Linux: runtime.GOOS == "linux", api: api, chats: chats, post: post, onError: onError}
}

// StoppedText is what the dialog says after Stop.
const StoppedText = "The export was stopped. Nothing was left behind."

// Open readies the project's export dialog as it opens: what an export
// would copy, and an export still running from before, e.g. if the
// dialog went away with the sidebar, to follow.
func (e *Exports) Open(p id.Project) *ProjectExport {
	x := &ProjectExport{live: true}
	e.Projects[p] = x
	go func() {
		pr, err := e.api.ExportStatus(background, p)
		e.post(func() {
			switch {
			case !x.live:
			case err != nil:
				e.onError(err)
			case pr != nil && !x.ended && !x.own:
				x.Progress, x.Running = pr, true
			}
		})
	}()
	go func() {
		est, err := e.api.ExportEstimate(background, p)
		e.post(func() {
			switch {
			case !x.live:
			case err != nil:
				e.onError(err)
			default:
				x.Estimate = &est
			}
		})
	}()
	return x
}

// Close is the dialog closing: its export, if one runs, goes on.
func (e *Exports) Close(p id.Project) {
	if x := e.Projects[p]; x != nil {
		x.live = false
	}
	delete(e.Projects, p)
}

// Event applies a project:export event to the project's open dialog. One
// export runs per project at a time, so the project tells its events; the
// last one says how it ended.
func (e *Exports) Event(pr project.ExportProgress) {
	x := e.Projects[pr.Project]
	if x == nil || !x.live {
		return
	}
	switch {
	case pr.Step == project.StepDone && pr.Result != nil:
		x.ended = true
		x.Result, x.Running, x.Stopping = pr.Result, false, false
	case pr.Step == project.StepFailed:
		x.ended = true
		if x.own {
			return
		}
		x.Error = "The export failed: " + pr.Error
		if pr.Stopped {
			x.Error = StoppedText
		}
		x.Running, x.Stopping = false, false
	default:
		x.Progress = &pr
	}
}

// Start asks where to save the project (name is its name) and exports it
// there, as a zip file or a folder, with its approvals when approvals.
func (e *Exports) Start(p id.Project, name string, format project.ExportFormat, approvals bool) {
	x := e.Projects[p]
	if x == nil {
		return
	}
	x.Error = ""
	req := SaveRequest{Title: "Export project", Name: ExportFileName(name, format)}
	zip := format == project.ExportZip
	if zip {
		req.FilterName, req.Ext = "Zip file", "zip"
	}
	e.save(req, func(picked string) {
		target := ExportPath(picked, format)
		replace := zip && target == picked && !e.Linux
		x.Running, x.Progress, x.own = true, nil, true
		r := app.ExportRequest{Target: target, Format: format, Replace: replace, IncludeApprovals: approvals}
		go func() {
			res, err := e.api.Export(background, p, r)
			e.post(func() {
				x.own, x.Running, x.Stopping = false, false, false
				if err == nil {
					x.Result = &res
					return
				}
				// Stop makes the call fail as cancelled, after the backend
				// has removed what it wrote; so does Jenab shutting down.
				switch uiKind(err) {
				case app.KindClosing:
					x.Error = StoppedText
				case app.KindInvalid, app.KindBusy:
					x.Error = errorText(err)
				default:
					e.onError(err)
				}
			})
		}()
	})
}

// Stop stops the project's running export.
func (e *Exports) Stop(p id.Project) {
	x := e.Projects[p]
	if x == nil {
		return
	}
	x.Stopping = true
	go func() {
		_, err := e.api.StopExport(background, p)
		if err != nil {
			e.post(func() {
				x.Stopping = false
				e.onError(err)
			})
		}
	}()
}

// OpenChat readies the chat's export dialog as it opens.
func (e *Exports) OpenChat(c id.Chat) *ChatExport {
	x := &ChatExport{}
	e.Chats[c] = x
	return x
}

// StartChat asks where to save the chat (title is its title) and exports
// it there as Markdown or JSON.
func (e *Exports) StartChat(p id.Project, c id.Chat, title string, format chatexport.Format) {
	x := e.Chats[c]
	if x == nil {
		return
	}
	x.Error = ""
	req := SaveRequest{Title: "Export chat", Name: ChatFileName(title, format), FilterName: "Markdown", Ext: "md"}
	if format == chatexport.FormatJSON {
		req.FilterName, req.Ext = "JSON", "json"
	}
	e.save(req, func(picked string) {
		x.Running = true
		r := app.ChatExportRequest{Target: ChatExportPath(picked, format), Format: format}
		go func() {
			res, err := e.chats.Export(background, p, c, r)
			e.post(func() {
				x.Running = false
				switch k := uiKind(err); {
				case err == nil:
					x.Result = &res
				case k == app.KindInvalid || k == app.KindNotFound:
					x.Error = errorText(err)
				default:
					e.onError(err)
				}
			})
		}()
	})
}

// save shows the save dialog off the UI thread and gives then the path
// picked, on it; a cancel gives nothing.
func (e *Exports) save(req SaveRequest, then func(string)) {
	if e.Save == nil {
		e.onError(errors.New("this app has no save dialog"))
		return
	}
	go func() {
		picked, err := e.Save(req)
		e.post(func() {
			switch {
			case err != nil:
				e.onError(err)
			case picked != "":
				then(picked)
			}
		})
	}()
}

// uiKind is err's UIError kind, or "".
func uiKind(err error) string {
	if ue, ok := errors.AsType[*app.UIError](err); ok {
		return ue.Kind
	}
	return ""
}

var (
	unsafeChars = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]+`)
	unsafeEnds  = regexp.MustCompile(`^[\s.-]+|[\s.-]+$`)
)

// SafeName is name without the characters a file name can't hold, or
// fallback when nothing is left.
func SafeName(name, fallback string) string {
	s := unsafeEnds.ReplaceAllString(unsafeChars.ReplaceAllString(name, "-"), "")
	if s == "" {
		return fallback
	}
	return s
}

// ExportFileName is the save dialog's suggested name for a project's
// export: the project's name as a file name, with .zip for a zip file.
func ExportFileName(name string, format project.ExportFormat) string {
	base := SafeName(name, "project")
	if format == project.ExportZip {
		return base + ".zip"
	}
	return base
}

// ExportPath is the path a project's export writes: a zip file gets .zip
// when the name has no such ending.
func ExportPath(picked string, format project.ExportFormat) string {
	if format == project.ExportZip && !strings.HasSuffix(strings.ToLower(picked), ".zip") {
		return picked + ".zip"
	}
	return picked
}

// ChatFileName is the save dialog's suggested name for a chat's export:
// its title as a file name, with the format's ending.
func ChatFileName(title string, format chatexport.Format) string {
	return SafeName(title, "chat") + chatEnding(format)
}

// ChatExportPath is the path a chat's export writes: the format's ending
// is added when the name has none.
func ChatExportPath(picked string, format chatexport.Format) string {
	end := chatEnding(format)
	if strings.HasSuffix(strings.ToLower(picked), end) {
		return picked
	}
	return picked + end
}

func chatEnding(format chatexport.Format) string {
	if format == chatexport.FormatJSON {
		return ".json"
	}
	return ".md"
}
