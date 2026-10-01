package main

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// scheduler is the SPEC 6.2 wall-clock check in miniature: job times live in
// a file; a ticker compares the wall clock with them (every second here instead
// of every minute, to keep the run short). A job whose time passed while the
// app was not running (within 24 h) runs once at the next start: catch-up.
type scheduler struct {
	path string
	mu   sync.Mutex
	stopC chan struct{}
	done  chan struct{}
}

type job struct {
	ID        string `json:"id"`
	Due       string `json:"due"`
	LastRunAt string `json:"lastRunAt,omitempty"`
	RanBy     string `json:"ranBy,omitempty"`
	Trigger   string `json:"trigger,omitempty"`
}

const tick = time.Second

func newScheduler(path string) *scheduler { return &scheduler{path: path} }

func (s *scheduler) load() []job {
	var jobs []job
	b, err := os.ReadFile(s.path)
	if err == nil {
		_ = json.Unmarshal(b, &jobs)
	}
	return jobs
}

func (s *scheduler) save(jobs []job) {
	b, _ := json.MarshalIndent(jobs, "", "  ")
	tmp := s.path + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, s.path)
	}
}

func (s *scheduler) add(id string, due time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	jobs := s.load()
	jobs = append(jobs, job{ID: id, Due: due.Format(time.RFC3339Nano)})
	s.save(jobs)
}

func (s *scheduler) start() {
	s.stopC, s.done = make(chan struct{}), make(chan struct{})
	started := time.Now()
	go func() {
		defer close(s.done)
		s.check(started, true)
		t := time.NewTicker(tick)
		defer t.Stop()
		for {
			select {
			case <-s.stopC:
				return
			case now := <-t.C:
				s.check(now, false)
			}
		}
	}()
}

func (s *scheduler) stop() {
	if s.stopC == nil {
		return
	}
	select {
	case <-s.stopC:
	default:
		close(s.stopC)
	}
	<-s.done
}

func (s *scheduler) check(now time.Time, atStart bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	jobs := s.load()
	changed := false
	for i := range jobs {
		j := &jobs[i]
		if j.LastRunAt != "" {
			continue
		}
		due, err := time.Parse(time.RFC3339Nano, j.Due)
		if err != nil || due.After(now) || now.Sub(due) > 24*time.Hour {
			continue
		}
		trigger := "schedule"
		if atStart || now.Sub(due) > tick+500*time.Millisecond {
			trigger = "catch_up"
		}
		j.LastRunAt, j.RanBy, j.Trigger = now.Format(time.RFC3339Nano), version, trigger
		changed = true
		lg.Log("job-run", map[string]any{"id": j.ID, "due": j.Due, "lateMs": ms(now.Sub(due)), "trigger": trigger})
	}
	if changed {
		s.save(jobs)
	}
}
