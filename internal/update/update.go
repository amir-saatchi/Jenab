// Package update keeps the app up to date (SPEC 2.8). It checks for a
// newer version at start and every 6 hours, as the user chose (automatic,
// notify only, off), installs each version once, and restarts the app
// when nothing is in progress. The feed, the signature check and the swap
// of the app's files are the Updater's: cmd/desktop gives MyGo's
// mygo.Updater (TASK-059). This package has no UI and no MyGo.
package update

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/config"
)

// Updater finds and installs new versions. Its Check compares with the
// running version and returns nil when the app is up to date.
type Updater interface {
	// Enabled reports whether this build can update itself: it was built
	// with an update key and feed, and it can write where it is installed.
	Enabled() bool
	Check(ctx context.Context, prereleases bool) (Available, error)
}

// Available is a newer version that Check found.
type Available interface {
	Version() string
	Notes() string
	// Install downloads the version, checks its signature and puts its
	// files in place of the app's. The running app is not affected; the
	// new version runs after a restart.
	Install(ctx context.Context, progress func(done, total int64)) error
}

// State is where the updater is.
type State string

const (
	StateOff         State = "off"         // the user turned updates off
	StateUnavailable State = "unavailable" // this build cannot update itself
	StateIdle        State = "idle"        // nothing checked yet
	StateChecking    State = "checking"
	StateUpToDate    State = "up-to-date"
	StateAvailable   State = "available"   // notify only: a newer version waits for the user
	StateDownloading State = "downloading" // downloading and installing
	StateReady       State = "ready"       // installed; runs after a restart
	StateFailed      State = "failed"      // the check or the install failed; the app is unchanged
)

// Status is what Settings shows.
type Status struct {
	State   State
	Current string // the running version
	Version string // the newer version, while available, downloading or ready
	Notes   string // its release notes, in Markdown
	// Progress is how much of the download arrived, from 0 to 1.
	Progress float64
	Err      string
	Checked  time.Time // the last check that got an answer
}

// Deps are what the service needs.
type Deps struct {
	Updater Updater
	Current string // the running version
	// Settings returns the user's choice now.
	Settings func() config.UpdateSettings
	// CanRestart reports whether an automatic restart would interrupt
	// nothing: no work going on, and the window not in use.
	CanRestart func() bool
	// Relaunch quits the app the normal way and starts it again.
	Relaunch func()
	// OnStatus gets every change of the status, from the service's
	// goroutines, one at a time.
	OnStatus func(Status)
	Log      *slog.Logger
	// Every is how often it checks; 6 hours when 0. Poll is how often a
	// ready update looks for a moment to restart; 1 minute when 0.
	Every, Poll time.Duration
}

type request int

const (
	reqCheck request = iota
	reqInstall
	reqRestart
	reqSettings
)

// Service checks, installs and restarts. Its methods may be called from
// any goroutine; the work runs one step at a time in Run.
type Service struct {
	d    Deps
	reqs chan request

	mu     sync.Mutex
	status Status

	// Only Run's goroutine uses these.
	found     Available
	installed string          // the version whose files are in place
	notes     string          // its release notes
	refused   map[string]bool // versions whose install failed, not tried again on their own
	mode      string          // the mode of the last step
}

// New makes the service; Run starts it.
func New(d Deps) *Service {
	if d.Every <= 0 {
		d.Every = 6 * time.Hour
	}
	if d.Poll <= 0 {
		d.Poll = time.Minute
	}
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.CanRestart == nil {
		d.CanRestart = func() bool { return false }
	}
	s := &Service{d: d, reqs: make(chan request, 8), refused: map[string]bool{}}
	s.status = Status{State: StateIdle, Current: d.Current}
	return s
}

// Status returns the status now.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// CheckNow checks at once, even with updates off: the user asked.
func (s *Service) CheckNow() { s.send(reqCheck) }

// Install installs the version a check found (notify only).
func (s *Service) Install() { s.send(reqInstall) }

// Restart starts the installed version now.
func (s *Service) Restart() { s.send(reqRestart) }

// SettingsChanged applies a new choice: off stops the checks, and turning
// them on checks at once.
func (s *Service) SettingsChanged() { s.send(reqSettings) }

func (s *Service) send(r request) {
	select {
	case s.reqs <- r:
	default: // a burst; the steps already asked for cover it
	}
}

// Run checks at start, then every Deps.Every, until ctx ends.
func (s *Service) Run(ctx context.Context) {
	s.mode = s.settings().Mode
	check := time.NewTimer(0)
	defer check.Stop()
	restart := time.NewTicker(s.d.Poll)
	defer restart.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-check.C:
			s.check(ctx, false)
			check.Reset(s.d.Every)
		case <-restart.C:
			if s.installed != "" && s.settings().Mode == "automatic" && s.d.CanRestart() {
				s.d.Log.Info("update: restarting into the new version", "version", s.installed)
				s.d.Relaunch()
			}
		case r := <-s.reqs:
			switch r {
			case reqCheck:
				s.check(ctx, true)
			case reqInstall:
				if s.found != nil {
					s.install(ctx, s.found)
				}
			case reqRestart:
				if s.installed != "" {
					s.d.Log.Info("update: restarting into the new version", "version", s.installed)
					s.d.Relaunch()
				}
			case reqSettings:
				was := s.mode
				s.mode = s.settings().Mode
				switch {
				case s.mode == "off" && s.installed == "":
					s.found = nil
					s.set(Status{State: StateOff})
				case was == "off" && s.mode != "off":
					s.check(ctx, false)
					check.Reset(s.d.Every)
				}
			}
		}
	}
}

func (s *Service) settings() config.UpdateSettings {
	if s.d.Settings == nil {
		return config.UpdateSettings{Mode: "notify", Channel: "stable"}
	}
	return s.d.Settings()
}

// check asks for a newer version; manual is a check the user asked for,
// which runs with updates off too and tries a refused version again.
func (s *Service) check(ctx context.Context, manual bool) {
	set := s.settings()
	if set.Mode == "off" && !manual {
		if s.installed == "" {
			s.set(Status{State: StateOff})
		}
		return
	}
	if !s.d.Updater.Enabled() {
		s.set(Status{State: StateUnavailable})
		return
	}
	beta := set.Channel == "beta"
	// With a version installed, Settings keeps offering the restart while
	// it checks for a newer one.
	if s.installed == "" {
		s.set(Status{State: StateChecking, Checked: s.Status().Checked})
	}
	found, err := s.d.Updater.Check(ctx, beta)
	if err != nil {
		s.d.Log.Warn("update: the check failed", "err", err)
		if s.installed != "" {
			s.set(s.ready(s.Status().Checked))
			return
		}
		s.set(Status{State: StateFailed, Err: err.Error(), Checked: s.Status().Checked})
		return
	}
	now := time.Now()
	// A prerelease only reaches the beta channel (SPEC 2.8), whatever the
	// feed offers.
	if found != nil && !beta && Prerelease(found.Version()) {
		s.d.Log.Info("update: a prerelease is ignored on the stable channel", "version", found.Version())
		found = nil
	}
	if found == nil || found.Version() == s.installed {
		s.found = nil
		if s.installed != "" {
			s.set(s.ready(now))
		} else {
			s.set(Status{State: StateUpToDate, Checked: now})
		}
		return
	}
	s.found = found
	s.d.Log.Info("update: a new version is available", "version", found.Version())
	if set.Mode == "automatic" && (manual || !s.refused[found.Version()]) {
		s.install(ctx, found)
		return
	}
	s.set(Status{State: StateAvailable, Version: found.Version(), Notes: found.Notes(), Checked: now})
}

// install downloads and installs found, once. Switching updates off while
// it downloads stops the download (TASK-067); other requests wait for it.
func (s *Service) install(ctx context.Context, found Available) {
	v := found.Version()
	checked := s.Status().Checked
	s.set(Status{State: StateDownloading, Version: v, Notes: found.Notes(), Checked: checked})
	dctx, cancel := context.WithCancel(ctx)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		last := time.Time{}
		result <- found.Install(dctx, func(done, total int64) {
			if total <= 0 || time.Since(last) < 250*time.Millisecond && done < total {
				return
			}
			last = time.Now()
			s.set(Status{State: StateDownloading, Version: v, Notes: found.Notes(), Progress: float64(done) / float64(total), Checked: checked})
		})
	}()
	stopped := false
	var err error
wait:
	for {
		select {
		case err = <-result:
			break wait
		case r := <-s.reqs:
			// A check, an install or a restart asked for now is answered
			// by the download's end; only off changes it.
			if r != reqSettings {
				continue
			}
			s.mode = s.settings().Mode
			if s.mode == "off" && !stopped {
				stopped = true
				s.d.Log.Info("update: updates are off; the download stops", "version", v)
				cancel()
			}
		}
	}
	if err != nil && (stopped || ctx.Err() != nil) {
		// Stopped, not failed: the version is not refused, and the app is
		// unchanged.
		s.found = nil
		if s.installed != "" {
			s.set(s.ready(checked))
		} else {
			s.set(Status{State: StateOff})
		}
		return
	}
	if err != nil {
		// The Updater leaves the app as it was: a download that fails its
		// signature or size is never unpacked, and a swap that fails puts
		// the old files back.
		s.refused[v] = true
		s.d.Log.Warn("update: the install failed; the app is unchanged", "version", v, "err", err)
		if s.installed != "" {
			// An earlier version is still in place and still runs after a
			// restart.
			s.set(s.ready(checked))
			return
		}
		s.set(Status{State: StateFailed, Version: v, Err: err.Error(), Checked: checked})
		return
	}
	// Done, even if updates went off at the last moment: the files are in
	// place, so the restart is offered.
	s.installed, s.notes, s.found = v, found.Notes(), nil
	s.d.Log.Info("update: installed; it runs after a restart", "version", v)
	s.set(s.ready(checked))
}

// ready is the status while the installed version waits for a restart.
func (s *Service) ready(checked time.Time) Status {
	return Status{State: StateReady, Version: s.installed, Notes: s.notes, Checked: checked}
}

func (s *Service) set(st Status) {
	st.Current = s.d.Current
	s.mu.Lock()
	s.status = st
	s.mu.Unlock()
	if s.d.OnStatus != nil {
		s.d.OnStatus(st)
	}
}

// Prerelease reports whether v, a semantic version, is a prerelease such
// as 1.3.0-beta.1.
func Prerelease(v string) bool {
	core, _, _ := strings.Cut(v, "+")
	return strings.Contains(core, "-")
}
