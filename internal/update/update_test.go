package update

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/config"
)

type fakeUpdater struct {
	mu       sync.Mutex
	enabled  bool
	next     *fakeVersion // what Check finds; nil when up to date
	checks   int
	betas    []bool
	checkErr error
}

func (f *fakeUpdater) Enabled() bool { return f.enabled }

func (f *fakeUpdater) Check(ctx context.Context, pre bool) (Available, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checks++
	f.betas = append(f.betas, pre)
	if f.checkErr != nil {
		return nil, f.checkErr
	}
	if f.next == nil {
		return nil, nil
	}
	return f.next, nil
}

func (f *fakeUpdater) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.checks
}

type fakeVersion struct {
	v        string
	err      error
	hang     bool // Install downloads until its context ends
	installs atomic.Int32
	stopped  atomic.Bool
}

func (v *fakeVersion) Version() string { return v.v }
func (v *fakeVersion) Notes() string   { return "notes of " + v.v }
func (v *fakeVersion) Install(ctx context.Context, progress func(done, total int64)) error {
	v.installs.Add(1)
	if v.hang {
		progress(1, 10)
		<-ctx.Done()
		v.stopped.Store(true)
		return ctx.Err()
	}
	progress(10, 10)
	return v.err
}

// harness runs a service and collects its statuses.
type harness struct {
	s        *Service
	u        *fakeUpdater
	set      atomic.Pointer[config.UpdateSettings]
	can      atomic.Bool
	relaunch atomic.Int32
	statuses chan Status
}

func start(t *testing.T, u *fakeUpdater, mode, channel string) *harness {
	t.Helper()
	return startEvery(t, u, mode, channel, 0)
}

func startEvery(t *testing.T, u *fakeUpdater, mode, channel string, every time.Duration) *harness {
	t.Helper()
	h := &harness{u: u, statuses: make(chan Status, 100)}
	h.set.Store(&config.UpdateSettings{Mode: mode, Channel: channel})
	h.s = New(Deps{
		Updater:    u,
		Current:    "1.0.0",
		Settings:   func() config.UpdateSettings { return *h.set.Load() },
		CanRestart: h.can.Load,
		Relaunch:   func() { h.relaunch.Add(1) },
		OnStatus:   func(st Status) { h.statuses <- st },
		Poll:       5 * time.Millisecond,
		Every:      every,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { h.s.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return h
}

// wait returns the first status in state, failing after a second.
func (h *harness) wait(t *testing.T, want State) Status {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case st := <-h.statuses:
			if st.State == want {
				return st
			}
		case <-deadline:
			t.Fatalf("no %s status; now %+v", want, h.s.Status())
		}
	}
}

func TestOffSendsNothing(t *testing.T) {
	u := &fakeUpdater{enabled: true}
	h := start(t, u, "off", "stable")
	h.wait(t, StateOff)
	if n := u.count(); n != 0 {
		t.Fatalf("off made %d checks", n)
	}
	// The user's own check still runs.
	h.s.CheckNow()
	h.wait(t, StateUpToDate)
	if n := u.count(); n != 1 {
		t.Fatalf("CheckNow made %d checks, want 1", n)
	}
}

func TestDisabledBuild(t *testing.T) {
	u := &fakeUpdater{enabled: false, next: &fakeVersion{v: "1.1.0"}}
	h := start(t, u, "automatic", "stable")
	h.wait(t, StateUnavailable)
	if u.count() != 0 {
		t.Fatal("a build without updates checked")
	}
}

func TestNotifyWaitsForTheUser(t *testing.T) {
	v := &fakeVersion{v: "1.1.0"}
	h := start(t, &fakeUpdater{enabled: true, next: v}, "notify", "stable")
	st := h.wait(t, StateAvailable)
	if st.Version != "1.1.0" || st.Current != "1.0.0" || st.Notes == "" {
		t.Fatalf("status %+v", st)
	}
	if v.installs.Load() != 0 {
		t.Fatal("notify only installed on its own")
	}
	h.s.Install()
	h.wait(t, StateReady)
	h.can.Store(true)
	time.Sleep(30 * time.Millisecond)
	if h.relaunch.Load() != 0 {
		t.Fatal("notify only restarted on its own")
	}
	h.s.Restart()
	deadline := time.Now().Add(time.Second)
	for h.relaunch.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if h.relaunch.Load() != 1 {
		t.Fatal("Restart did not relaunch")
	}
}

func TestAutomaticInstallsOnceAndRestartsWhenIdle(t *testing.T) {
	v := &fakeVersion{v: "1.1.0"}
	u := &fakeUpdater{enabled: true, next: v}
	h := start(t, u, "automatic", "stable")
	h.wait(t, StateReady)
	time.Sleep(30 * time.Millisecond)
	if h.relaunch.Load() != 0 {
		t.Fatal("restarted while busy")
	}
	// A later check finds the same version: it is not downloaded again.
	h.s.CheckNow()
	h.wait(t, StateReady)
	if n := v.installs.Load(); n != 1 {
		t.Fatalf("installed %d times, want once", n)
	}
	h.can.Store(true)
	deadline := time.Now().Add(time.Second)
	for h.relaunch.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if h.relaunch.Load() == 0 {
		t.Fatal("no restart once idle")
	}
}

func TestRefusedInstallIsNotRetriedOnItsOwn(t *testing.T) {
	v := &fakeVersion{v: "1.1.0", err: errors.New("ed25519 signature did not verify")}
	u := &fakeUpdater{enabled: true, next: v}
	h := startEvery(t, u, "automatic", "stable", 10*time.Millisecond)
	st := h.wait(t, StateFailed)
	if st.Err == "" || st.Version != "1.1.0" {
		t.Fatalf("status %+v", st)
	}
	// The timer's next checks offer it without downloading it again.
	h.wait(t, StateAvailable)
	h.wait(t, StateAvailable)
	if n := v.installs.Load(); n != 1 {
		t.Fatalf("installed %d times, want 1", n)
	}
}

func TestPrereleasesOnlyOnBeta(t *testing.T) {
	u := &fakeUpdater{enabled: true, next: &fakeVersion{v: "1.1.0-beta.1"}}
	h := start(t, u, "notify", "stable")
	h.wait(t, StateUpToDate)
	h.set.Store(&config.UpdateSettings{Mode: "notify", Channel: "beta"})
	h.s.CheckNow()
	h.wait(t, StateAvailable)
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.betas) != 2 || u.betas[0] || !u.betas[1] {
		t.Fatalf("prereleases asked %v, want [false true]", u.betas)
	}
}

func TestTurningOnChecks(t *testing.T) {
	u := &fakeUpdater{enabled: true}
	h := start(t, u, "off", "stable")
	h.wait(t, StateOff)
	h.set.Store(&config.UpdateSettings{Mode: "notify", Channel: "stable"})
	h.s.SettingsChanged()
	h.wait(t, StateUpToDate)
	h.set.Store(&config.UpdateSettings{Mode: "off", Channel: "stable"})
	h.s.SettingsChanged()
	h.wait(t, StateOff)
}

func TestCheckFails(t *testing.T) {
	h := start(t, &fakeUpdater{enabled: true, checkErr: errors.New("offline")}, "notify", "stable")
	if st := h.wait(t, StateFailed); st.Err != "offline" {
		t.Fatalf("status %+v", st)
	}
}

// A check that fails after a version is installed keeps offering the
// restart into it (TASK-067).
func TestFailedRecheckKeepsTheInstalledVersion(t *testing.T) {
	u := &fakeUpdater{enabled: true, next: &fakeVersion{v: "1.1.0"}}
	h := start(t, u, "automatic", "stable")
	h.wait(t, StateReady)
	u.mu.Lock()
	u.checkErr = errors.New("offline")
	u.mu.Unlock()
	h.s.CheckNow()
	select {
	case st := <-h.statuses:
		if st.State != StateReady || st.Version != "1.1.0" || st.Notes == "" {
			t.Fatalf("after a failed re-check: %+v; want ready with 1.1.0", st)
		}
	case <-time.After(time.Second):
		t.Fatal("no status after the re-check")
	}
	if n := u.count(); n != 2 {
		t.Fatalf("%d checks, want 2", n)
	}
	h.can.Store(true)
	deadline := time.Now().Add(time.Second)
	for h.relaunch.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if h.relaunch.Load() == 0 {
		t.Fatal("no restart into the installed version")
	}
}

// Switching updates off stops a download, and no restart is offered
// (TASK-067).
func TestSwitchingOffStopsTheDownload(t *testing.T) {
	v := &fakeVersion{v: "1.1.0", hang: true}
	u := &fakeUpdater{enabled: true, next: v}
	h := start(t, u, "automatic", "stable")
	h.wait(t, StateDownloading)
	h.set.Store(&config.UpdateSettings{Mode: "off", Channel: "stable"})
	h.s.SettingsChanged()
	h.wait(t, StateOff)
	if !v.stopped.Load() {
		t.Fatal("the download was not cancelled")
	}
	h.can.Store(true)
	time.Sleep(30 * time.Millisecond)
	if st := h.s.Status(); st.State != StateOff {
		t.Fatalf("status %+v; want off", st)
	}
	if h.relaunch.Load() != 0 {
		t.Fatal("restarted after the download stopped")
	}
	// Turned on again, the version is offered again: it was stopped, not
	// refused.
	h.set.Store(&config.UpdateSettings{Mode: "notify", Channel: "stable"})
	h.s.SettingsChanged()
	h.wait(t, StateAvailable)
}

func TestPrerelease(t *testing.T) {
	for v, want := range map[string]bool{"1.0.0": false, "1.0.0-rc.1": true, "1.0.0+build-7": false, "0.1.0-dev": true} {
		if got := Prerelease(v); got != want {
			t.Errorf("Prerelease(%q) = %v", v, got)
		}
	}
}
