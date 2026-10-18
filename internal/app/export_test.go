package app

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
)

func TestExport(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	e.svc.Project.emit = e.sink.emit
	op, err := e.svc.Project.Create(ctx, "Export me")
	if err != nil {
		t.Fatal(err)
	}
	proj, err := e.pm.Open(ctx, op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := proj.DB.PutObject(ctx, limit.Interactive, id.SourceUser, "a.txt", strings.NewReader(strings.Repeat("a", 5000)), store.PutOptions{Source: "upload"}); err != nil {
		t.Fatal(err)
	}
	proj.Release()

	est, err := e.svc.Project.ExportEstimate(ctx, op.ID)
	if err != nil || est.Objects != 1 || est.Bytes < 5000 || est.Databases <= 0 {
		t.Fatalf("estimate = %+v, %v", est, err)
	}
	target := filepath.Join(t.TempDir(), "out.zip")
	res, err := e.svc.Project.Export(ctx, op.ID, ExportRequest{Target: target, Format: project.ExportZip})
	if err != nil || res.Path != target || res.Objects != 1 || res.ID == op.ID {
		t.Fatalf("export = %+v, %v", res, err)
	}
	if !slices.Contains(e.sink.names(), EventExport) {
		t.Errorf("no %s event: %v", EventExport, e.sink.names())
	}
	_, err = e.svc.Project.Export(ctx, op.ID, ExportRequest{Target: target, Format: project.ExportZip})
	uiErr(t, err, KindInvalid)
	if _, err := e.svc.Project.Export(ctx, op.ID, ExportRequest{Target: target, Format: project.ExportZip, Replace: true}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if e.pm.Busy() {
		t.Error("the export kept a lease")
	}

	if last := exportEvents(e.sink); len(last) == 0 || last[len(last)-1].Step != project.StepDone || last[len(last)-1].Result == nil {
		t.Errorf("the last event = %+v", last)
	}

	// While one runs: ExportStatus finds it, a second is refused without
	// an event of its own, and Stop makes the call fail as cancelled once
	// the export has cleaned up.
	if pr, err := e.svc.Project.ExportStatus(ctx, op.ID); err != nil || pr != nil {
		t.Errorf("status with no export = %+v, %v", pr, err)
	}
	if stopped, err := e.svc.Project.StopExport(ctx, op.ID); err != nil || stopped {
		t.Errorf("stop with no export = %v, %v", stopped, err)
	}
	other := filepath.Join(t.TempDir(), "stopped.zip")
	var once sync.Once
	sink := &sink{}
	e.svc.Project.emit = func(name string, data any) {
		sink.emit(name, data)
		pr, ok := data.(project.ExportProgress)
		if !ok || pr.Step != project.StepObjects {
			return
		}
		once.Do(func() {
			if st, err := e.svc.Project.ExportStatus(ctx, op.ID); err != nil || st == nil || st.Target != other {
				t.Errorf("status = %+v, %v", st, err)
			}
			_, err := e.svc.Project.Export(ctx, op.ID, ExportRequest{Target: filepath.Join(t.TempDir(), "second.zip"), Format: project.ExportZip})
			uiErr(t, err, KindBusy)
			if stopped, err := e.svc.Project.StopExport(ctx, op.ID); err != nil || !stopped {
				t.Errorf("stop = %v, %v", stopped, err)
			}
		})
	}
	_, err = e.svc.Project.Export(ctx, op.ID, ExportRequest{Target: other, Format: project.ExportZip})
	uiErr(t, err, KindClosing)
	if left, _ := filepath.Glob(other + "*"); len(left) != 0 {
		t.Errorf("a stopped export left %v", left)
	}
	evs := exportEvents(sink)
	failed := 0
	for _, pr := range evs {
		if pr.Step == project.StepFailed {
			failed++
		}
	}
	if last := evs[len(evs)-1]; failed != 1 || last.Step != project.StepFailed || !last.Stopped || last.Error == "" {
		t.Errorf("%d failed events; the last = %+v", failed, last)
	}
	if pr, _ := e.svc.Project.ExportStatus(ctx, op.ID); pr != nil {
		t.Errorf("status after the end = %+v", pr)
	}
}

func exportEvents(s *sink) []project.ExportProgress {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []project.ExportProgress
	for _, e := range s.events {
		if pr, ok := e.data.(project.ExportProgress); ok {
			out = append(out, pr)
		}
	}
	return out
}
