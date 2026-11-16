package statetest

import (
	"context"
	"slices"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/chatexport"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
)

// ProjectAdmin is the Fake's part for the project screens (TASK-057):
// each project's private hosts, and the exports asked for. Set it up
// before the stores read it; the methods read and write it under the
// Fake's lock.
type ProjectAdmin struct {
	Hosts map[id.Project][]string
	// HostsErr fails SetPrivateHosts, as the Go side refuses a host.
	HostsErr error
	// CreateErr fails Create, as the Go side refuses a project's name.
	CreateErr error
	Estimate  project.ExportEstimate
	// Gate, when set, holds each project export until it is closed or
	// the export is stopped; a stopped export fails as Jenab's do.
	Gate        chan struct{}
	Exports     []app.ExportRequest
	ChatExports []app.ChatExportRequest
	stop        chan struct{}
}

// Admin returns a copy of the Fake's ProjectAdmin, for tests to read.
func (f *Fake) Admin() ProjectAdmin {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := f.admin
	a.Exports, a.ChatExports = slices.Clone(a.Exports), slices.Clone(a.ChatExports)
	return a
}

// SetAdmin sets up the Fake's ProjectAdmin.
func (f *Fake) SetAdmin(a ProjectAdmin) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.admin = a
}

func (p projects) PrivateHosts(_ context.Context, pid id.Project) ([]string, error) {
	p.f.mu.Lock()
	defer p.f.mu.Unlock()
	return slices.Clone(p.f.admin.Hosts[pid]), nil
}

func (p projects) SetPrivateHosts(_ context.Context, pid id.Project, hosts []string) ([]string, error) {
	p.f.mu.Lock()
	defer p.f.mu.Unlock()
	if err := p.f.admin.HostsErr; err != nil {
		return nil, err
	}
	if p.f.admin.Hosts == nil {
		p.f.admin.Hosts = map[id.Project][]string{}
	}
	p.f.admin.Hosts[pid] = slices.Clone(hosts)
	return slices.Clone(hosts), nil
}

func (p projects) ExportEstimate(context.Context, id.Project) (project.ExportEstimate, error) {
	p.f.mu.Lock()
	defer p.f.mu.Unlock()
	return p.f.admin.Estimate, nil
}

func (p projects) Export(ctx context.Context, pid id.Project, r app.ExportRequest) (project.ExportResult, error) {
	p.f.mu.Lock()
	p.f.admin.Exports = append(p.f.admin.Exports, r)
	gate := p.f.admin.Gate
	stop := make(chan struct{})
	p.f.admin.stop = stop
	p.f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-stop:
			return project.ExportResult{}, &app.UIError{Kind: app.KindClosing, Message: "stopped"}
		}
	}
	return project.ExportResult{Path: r.Target, Format: r.Format, ID: "copy", Size: 2_500_000, Objects: 3, Millis: 1500}, nil
}

func (p projects) StopExport(context.Context, id.Project) (bool, error) {
	p.f.mu.Lock()
	defer p.f.mu.Unlock()
	if p.f.admin.stop == nil {
		return false, nil
	}
	close(p.f.admin.stop)
	p.f.admin.stop = nil
	return true, nil
}

func (p projects) ExportStatus(context.Context, id.Project) (*project.ExportProgress, error) {
	return nil, nil
}

// Export is ChatExportAPI's: it records the request and says it wrote
// the chat's messages.
func (f *Fake) Export(_ context.Context, _ id.Project, c id.Chat, r app.ChatExportRequest) (chatexport.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.admin.ChatExports = append(f.admin.ChatExports, r)
	return chatexport.Result{Path: r.Target, Messages: len(f.Msgs[c]), Size: 1234}, nil
}
