package state

// shell/export-chat.test.ts and export-project.test.ts, ported (the
// names and paths; the texts are the screen's), and the dialogs' store.

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/chatexport"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
)

func TestExportFileName(t *testing.T) {
	same(t, ExportFileName("BTC prices", project.ExportZip), "BTC prices.zip")
	same(t, ExportFileName("قیمت‌ها", project.ExportFolder), "قیمت‌ها")
	same(t, ExportFileName(`a/b:c*"d"?`, project.ExportFolder), "a-b-c-d")
	same(t, ExportFileName(" ..", project.ExportZip), "project.zip")
}

func TestExportPath(t *testing.T) {
	same(t, ExportPath(`C:\Users\me\BTC`, project.ExportZip), `C:\Users\me\BTC.zip`)
	same(t, ExportPath(`C:\Users\me\BTC.ZIP`, project.ExportZip), `C:\Users\me\BTC.ZIP`)
	same(t, ExportPath("/home/me/BTC", project.ExportFolder), "/home/me/BTC")
}

func TestChatFileName(t *testing.T) {
	same(t, ChatFileName("BTC prices", chatexport.FormatMarkdown), "BTC prices.md")
	same(t, ChatFileName("قیمت بیت‌کوین", chatexport.FormatJSON), "قیمت بیت‌کوین.json")
	same(t, ChatFileName(`a/b:c*"d"?`, chatexport.FormatMarkdown), "a-b-c-d.md")
	same(t, ChatFileName("", chatexport.FormatMarkdown), "chat.md")
}

func TestChatExportPath(t *testing.T) {
	same(t, ChatExportPath(`C:\Users\me\chat`, chatexport.FormatMarkdown), `C:\Users\me\chat.md`)
	same(t, ChatExportPath(`C:\Users\me\chat.MD`, chatexport.FormatMarkdown), `C:\Users\me\chat.MD`)
	same(t, ChatExportPath("/home/me/chat.md", chatexport.FormatJSON), "/home/me/chat.md.json")
}

type fakeExports struct {
	mu      sync.Mutex
	asked   []app.ExportRequest
	chats   []app.ChatExportRequest
	gate    chan struct{}
	stopped chan struct{}
	status  *project.ExportProgress
	err     error
}

func (f *fakeExports) ExportEstimate(context.Context, id.Project) (project.ExportEstimate, error) {
	return project.ExportEstimate{Bytes: 2_000_000, Objects: 3}, nil
}

func (f *fakeExports) Export(_ context.Context, _ id.Project, r app.ExportRequest) (project.ExportResult, error) {
	f.mu.Lock()
	f.asked = append(f.asked, r)
	gate, stopped, err := f.gate, f.stopped, f.err
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-stopped:
			return project.ExportResult{}, &app.UIError{Kind: app.KindClosing, Message: "closing"}
		}
	}
	if err != nil {
		return project.ExportResult{}, err
	}
	return project.ExportResult{Path: r.Target, Size: 10}, nil
}

func (f *fakeExports) StopExport(context.Context, id.Project) (bool, error) {
	close(f.stopped)
	return true, nil
}

func (f *fakeExports) ExportStatus(context.Context, id.Project) (*project.ExportProgress, error) {
	return f.status, nil
}

func (f *fakeExports) ChatExport() ChatExportAPI { return chatExporter{f} }

type chatExporter struct{ f *fakeExports }

func (c chatExporter) Export(_ context.Context, _ id.Project, _ id.Chat, r app.ChatExportRequest) (chatexport.Result, error) {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	c.f.chats = append(c.f.chats, r)
	if c.f.err != nil {
		return chatexport.Result{}, c.f.err
	}
	return chatexport.Result{Path: r.Target, Messages: 2}, nil
}

func newExports(t *testing.T, f *fakeExports, picked string) (*Exports, *loop, *[]error, *[]SaveRequest) {
	l := newLoop()
	var errs []error
	e := NewExports(f, f.ChatExport(), l.post, func(err error) { errs = append(errs, err) })
	var saves []SaveRequest
	var mu sync.Mutex
	e.Save = func(r SaveRequest) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		saves = append(saves, r)
		return picked, nil
	}
	return e, l, &errs, &saves
}

func TestAProjectExportAsksWhereThenRuns(t *testing.T) {
	f := &fakeExports{gate: make(chan struct{}), stopped: make(chan struct{})}
	e, l, errs, saves := newExports(t, f, `C:\Backups\BTC`)
	x := e.Open("p")
	until(t, l, func() bool { return x.Estimate != nil })
	e.Start("p", "BTC: prices", project.ExportZip, true)
	until(t, l, func() bool { return x.Running })
	same(t, (*saves)[0], SaveRequest{Title: "Export project", Name: "BTC- prices.zip", FilterName: "Zip file", Ext: "zip"})

	// Progress events show; the call's own end decides.
	e.Event(project.ExportProgress{Project: "p", Step: project.StepChats, Done: 5, Total: 10})
	same(t, x.Progress.Step, project.StepChats)
	e.Event(project.ExportProgress{Project: "other", Step: project.StepObjects})
	same(t, x.Progress.Step, project.StepChats)
	close(f.gate)
	until(t, l, func() bool { return x.Result != nil })
	same(t, x.Running, false)
	f.mu.Lock()
	same(t, f.asked[0], app.ExportRequest{Target: `C:\Backups\BTC.zip`, Format: project.ExportZip, IncludeApprovals: true})
	f.mu.Unlock()
	same(t, len(*errs), 0)
}

func TestAZipPickedWithItsEndingReplaces(t *testing.T) {
	f := &fakeExports{}
	e, l, _, _ := newExports(t, f, `C:\Backups\BTC.zip`)
	x := e.Open("p")
	e.Linux = false
	e.Start("p", "BTC", project.ExportZip, false)
	until(t, l, func() bool { return x.Result != nil })
	same(t, f.asked[0].Replace, true)
}

func TestStoppingAProjectExport(t *testing.T) {
	f := &fakeExports{gate: make(chan struct{}), stopped: make(chan struct{})}
	e, l, errs, _ := newExports(t, f, `C:\Backups\BTC`)
	x := e.Open("p")
	e.Start("p", "BTC", project.ExportFolder, false)
	until(t, l, func() bool { return x.Running })
	e.Stop("p")
	same(t, x.Stopping, true)
	// The failed event of its own export doesn't decide; the call does.
	e.Event(project.ExportProgress{Project: "p", Step: project.StepFailed, Error: "x"})
	same(t, x.Error, "")
	until(t, l, func() bool { return !x.Running })
	same(t, x.Error, StoppedText)
	same(t, x.Stopping, false)
	same(t, len(*errs), 0)
}

func TestAnExportFromBeforeIsFollowed(t *testing.T) {
	f := &fakeExports{status: &project.ExportProgress{Project: "p", Step: project.StepObjects, Done: 1, Total: 2}}
	e, l, _, _ := newExports(t, f, "")
	x := e.Open("p")
	until(t, l, func() bool { return x.Running })
	same(t, x.Progress.Step, project.StepObjects)
	e.Event(project.ExportProgress{Project: "p", Step: project.StepFailed, Error: "disk full"})
	same(t, x.Error, "The export failed: disk full")
	same(t, x.Running, false)
	// A closed dialog takes no events.
	e.Close("p")
	e.Event(project.ExportProgress{Project: "p", Step: project.StepChats})
	same(t, x.Progress.Step, project.StepObjects)
}

func TestACancelledSaveDialogDoesNothing(t *testing.T) {
	f := &fakeExports{}
	e, l, errs, saves := newExports(t, f, "")
	x := e.OpenChat("c")
	e.StartChat("p", "c", "BTC", chatexport.FormatJSON)
	until(t, l, func() bool { return len(*saves) == 1 })
	l.drain()
	same(t, x.Running, false)
	same(t, len(f.chats), 0)
	same(t, len(*errs), 0)
	same(t, (*saves)[0], SaveRequest{Title: "Export chat", Name: "BTC.json", FilterName: "JSON", Ext: "json"})
}

func TestAChatExport(t *testing.T) {
	f := &fakeExports{}
	e, l, _, _ := newExports(t, f, `C:\Backups\notes`)
	x := e.OpenChat("c")
	e.StartChat("p", "c", "notes", chatexport.FormatMarkdown)
	until(t, l, func() bool { return x.Result != nil })
	same(t, f.chats[0], app.ChatExportRequest{Target: `C:\Backups\notes.md`, Format: chatexport.FormatMarkdown})

	// An invalid place shows in the dialog; other errors in a toast.
	f.err = &app.UIError{Kind: app.KindInvalid, Message: "That folder can't be written."}
	x = e.OpenChat("c")
	e.StartChat("p", "c", "notes", chatexport.FormatMarkdown)
	until(t, l, func() bool { return x.Error != "" })
	same(t, x.Error, "That folder can't be written.")
}

func TestExportErrorsNoDialogTakesGoToTheToasts(t *testing.T) {
	f := &fakeExports{err: errors.New("disk on fire")}
	e, l, errs, _ := newExports(t, f, `C:\x`)
	x := e.Open("p")
	e.Start("p", "BTC", project.ExportFolder, false)
	until(t, l, func() bool { return len(*errs) == 1 })
	same(t, x.Error, "")
}
