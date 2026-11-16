package settings

import (
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/settings/logic"
	"github.com/amir-saatchi/jenab/internal/update"
)

// updates is the update rows of General (SPEC 2.8, TASK-059): the mode,
// the beta channel, and the status with what it offers. Nothing without
// an updater.
func (s *Screen) updates(c *kit.Context, set config.Settings) {
	u := s.st.Updates
	if u == nil || u.API == nil {
		return
	}
	kit.FactRow(c, "Updates", func() {
		mode := set.Updates.Mode
		choices := make([]kit.Choice, len(logic.UpdateModes))
		for i, m := range logic.UpdateModes {
			choices[i] = kit.Choice{Value: m.Value, Label: m.Label}
		}
		if kit.Segmented(c, &mode, choices, "Updates").Changed() {
			s.editUpdates(func(c *config.Settings) { c.Updates.Mode = mode })
		}
	}, nil)
	kit.Hint(c, "Automatic installs a new version and restarts when no chat is working and the window isn't in use. A check sends only the version, the platform and the processor type.")
	kit.SettingRow(c, "Beta versions", "Get prereleases too, such as 0.3.0-beta.1.", func() {
		beta := set.Updates.Channel == "beta"
		if kit.Switch(c, &beta, "Beta versions").Changed() {
			channel := "stable"
			if beta {
				channel = "beta"
			}
			s.editUpdates(func(c *config.Settings) { c.Updates.Channel = channel })
		}
	})
	line, action := logic.UpdateLine(u.Status, dateTime(c))
	kit.FactRow(c, "", func() {
		kit.Column(c).Gap(6).Shrink(1).MinWidth(0).Children(func() {
			kit.Muted(c, line)
			if u.Status.State == update.StateDownloading {
				kit.Progress(c, u.Status.Progress, "Download")
			}
		})
	}, func() {
		if action == logic.UpdateNone {
			return
		}
		v := kit.Outline
		if action != logic.UpdateCheck {
			v = kit.Default
		}
		if kit.Button(c, logic.UpdateButton(action), v).Height(32).PaddingX(12).Clicked() {
			switch action {
			case logic.UpdateCheck:
				u.API.CheckNow()
			case logic.UpdateInstall:
				u.API.Install()
			case logic.UpdateRestart:
				u.API.Restart()
			}
		}
	})
}

// editUpdates saves a change of the update settings, then tells the
// updater.
func (s *Screen) editUpdates(change func(*config.Settings)) {
	s.wait(s.st.Settings.Edit(change), func(err error) {
		if err != nil {
			s.st.Error(err)
			return
		}
		s.st.Updates.API.SettingsChanged()
	})
}
