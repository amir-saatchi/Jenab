package state

import "github.com/amir-saatchi/jenab/internal/update"

// UpdatesAPI runs the updater's steps (TASK-059); *update.Service has
// them. Each returns at once: the status comes back through
// Stores.UpdateStatus.
type UpdatesAPI interface {
	CheckNow()
	Install()
	Restart()
	SettingsChanged()
}

// Updates is the app's update status, for Settings › General (SPEC 2.8).
type Updates struct {
	Status update.Status
	// API is nil in builds and tests without an updater; Settings then
	// shows no update row.
	API UpdatesAPI

	told string // the state and version the last toast was about
}

// UpdateStatus takes a new status from the updater. A new version that
// waits for the user, or is ready, also shows a toast once.
func (s *Stores) UpdateStatus(st update.Status) {
	u := s.Updates
	u.Status = st
	var text string
	switch st.State {
	case update.StateAvailable:
		text = "Jenab " + st.Version + " is available. Install it in Settings › General."
	case update.StateReady:
		text = "Jenab " + st.Version + " is installed. It starts with the next restart."
	default:
		return
	}
	if key := string(st.State) + " " + st.Version; key != u.told {
		u.told = key
		s.Toasts = append(s.Toasts, Toast{Text: text})
	}
}

// Busy reports whether a chat is working or waiting, which an automatic
// restart must not cut off.
func (s *Stores) Busy() bool { return len(s.states) > 0 }
