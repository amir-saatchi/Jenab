package logic

import (
	"fmt"
	"time"

	"github.com/amir-saatchi/jenab/internal/update"
)

// UpdateModes are the choices of updates.mode (SPEC 2.8).
var UpdateModes = []Level{
	{Value: "automatic", Label: "Automatic"},
	{Value: "notify", Label: "Notify me"},
	{Value: "off", Label: "Off"},
}

// UpdateAction is the button an update status offers.
type UpdateAction int

const (
	UpdateNone UpdateAction = iota
	UpdateCheck
	UpdateInstall
	UpdateRestart
)

// UpdateLine is what Settings says about the updater, and the button it
// offers. format gives a time as the screen shows dates.
func UpdateLine(st update.Status, format func(time.Time) string) (string, UpdateAction) {
	checked := ""
	if !st.Checked.IsZero() {
		checked = " Checked " + format(st.Checked) + "."
	}
	switch st.State {
	case update.StateUnavailable:
		return "This build can't update itself. Builds made with task build:native and an update key, and installed with the installer, can.", UpdateNone
	case update.StateOff:
		return "Updates are off. Jenab doesn't check for new versions.", UpdateCheck
	case update.StateChecking:
		return "Checking for a new version…", UpdateNone
	case update.StateUpToDate:
		return "Jenab is up to date." + checked, UpdateCheck
	case update.StateAvailable:
		return fmt.Sprintf("Jenab %s is available.", st.Version) + checked, UpdateInstall
	case update.StateDownloading:
		return fmt.Sprintf("Downloading Jenab %s… %d%%", st.Version, int(st.Progress*100)), UpdateNone
	case update.StateReady:
		return fmt.Sprintf("Jenab %s is installed. It starts with the next restart.", st.Version), UpdateRestart
	case update.StateFailed:
		if st.Version != "" {
			return fmt.Sprintf("Jenab %s was not installed: %s. This version keeps running.", st.Version, st.Err), UpdateCheck
		}
		return "The check failed: " + st.Err, UpdateCheck
	}
	return "Not checked yet.", UpdateCheck
}

// UpdateButton is the label of an update action.
func UpdateButton(a UpdateAction) string {
	switch a {
	case UpdateCheck:
		return "Check now"
	case UpdateInstall:
		return "Install"
	case UpdateRestart:
		return "Restart now"
	}
	return ""
}
