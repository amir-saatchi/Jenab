package state

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/amir-saatchi/jenab/internal/id"
)

// HostsAPI is what Hosts reads and writes; *app.ProjectService has it.
type HostsAPI interface {
	PrivateHosts(ctx context.Context, p id.Project) ([]string, error)
	SetPrivateHosts(ctx context.Context, p id.Project, hosts []string) ([]string, error)
}

// Hosts is each project's private-network exceptions (SPEC 5.12, 6.7),
// as project settings shows them (shell/private-hosts.tsx): the hosts
// whose requests may reach this computer or the local network without a
// card.
type Hosts struct {
	// ByProject is each project's list as last read or saved; a project
	// not read yet is missing.
	ByProject map[id.Project][]string
	// Busy is set while a project's change runs or waits.
	Busy map[id.Project]bool

	api     HostsAPI
	post    Post
	onError func(error)
	// loads counts each project's reads started, and changes its changes
	// started. A read's list shows only if it is the last read started
	// and no change ran while it read: a read made during a save may
	// predate it, and the save's answer is the list. A project's changes
	// run one after another (TASK-078): each reads the list, changes it
	// and saves it, so two at once could lose one's edit. waiting holds
	// those still to run; running is set while one does.
	loads   map[id.Project]int
	changes map[id.Project]int
	running map[id.Project]bool
	waiting map[id.Project][]func()
}

// NewHosts makes the store. onError shows the errors no caller takes.
func NewHosts(api HostsAPI, post Post, onError func(error)) *Hosts {
	return &Hosts{ByProject: map[id.Project][]string{}, Busy: map[id.Project]bool{}, api: api, post: post, onError: onError,
		loads: map[id.Project]int{}, changes: map[id.Project]int{}, running: map[id.Project]bool{}, waiting: map[id.Project][]func(){}}
}

// Load reads the project's list.
func (h *Hosts) Load(p id.Project) {
	h.loads[p]++
	seq, changes, clear := h.loads[p], h.changes[p], !h.running[p]
	go func() {
		hosts, err := h.api.PrivateHosts(background, p)
		h.post(func() {
			if err != nil {
				h.onError(err)
				return
			}
			if seq == h.loads[p] && clear && changes == h.changes[p] {
				h.ByProject[p] = orEmpty(hosts)
			}
		})
	}()
}

// Change saves next of the stored list, which a card may have changed
// since the page read it, and gives then the error, if any. It runs once
// the project's earlier changes have ended. A failed change reads the
// list again, as a read made while it ran was dropped.
func (h *Hosts) Change(p id.Project, next func([]string) []string, then func(error)) {
	h.Busy[p] = true
	h.changes[p]++
	h.waiting[p] = append(h.waiting[p], func() { h.change(p, next, then) })
	if !h.running[p] {
		h.runNext(p)
	}
}

// runNext starts the project's next waiting change, or ends Busy.
func (h *Hosts) runNext(p id.Project) {
	q := h.waiting[p]
	if len(q) == 0 {
		delete(h.waiting, p)
		delete(h.running, p)
		delete(h.Busy, p)
		return
	}
	h.waiting[p] = q[1:]
	h.running[p] = true
	q[0]()
}

func (h *Hosts) change(p id.Project, next func([]string) []string, then func(error)) {
	go func() {
		stored, err := h.api.PrivateHosts(background, p)
		var saved []string
		if err == nil {
			saved, err = h.api.SetPrivateHosts(background, p, next(orEmpty(stored)))
		}
		h.post(func() {
			if err == nil {
				h.ByProject[p] = orEmpty(saved)
			}
			h.runNext(p)
			if err != nil && !h.running[p] {
				h.Load(p)
			}
			then(err)
		})
	}()
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

var (
	hostPort  = regexp.MustCompile(`^[^:\[\]]+:\d*$`)
	bracketed = regexp.MustCompile(`^\[.*\]:\d*$`)
)

// HostAdvice is the line under the field that adds a host.
const HostAdvice = "A name or an address, without http://, a path or a port."

// HostProblem catches the common slips before the call: a URL, a path or
// a port typed with the host. The Go side checks the rest (SPEC 6.7). ""
// is no problem.
func HostProblem(text string) string {
	t := strings.TrimSpace(text)
	switch {
	case t == "":
		return "Type a host."
	case strings.ContainsFunc(t, unicode.IsSpace):
		return "Add one host at a time."
	case strings.Contains(t, "://") || strings.Contains(t, "/"):
		return "Give the host alone, without http:// or a path."
	case hostPort.MatchString(t) || bracketed.MatchString(t):
		return "Give the host without a port."
	}
	return ""
}

// WithHost is the list after adding host. The hosts are compared as the
// user typed them; the Go side stores them in one spelling and returns
// the list.
func WithHost(hosts []string, host string) []string {
	h := strings.TrimSpace(host)
	if slices.Contains(hosts, h) {
		return hosts
	}
	return append(slices.Clone(hosts), h)
}

// WithoutHost is the list after removing host.
func WithoutHost(hosts []string, host string) []string {
	return slices.DeleteFunc(slices.Clone(hosts), func(h string) bool { return h == host })
}
