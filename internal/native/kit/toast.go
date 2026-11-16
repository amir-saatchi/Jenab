package kit

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// ToastKind is what a toast tells: news, a warning or an error.
type ToastKind string

const (
	ToastInfo    ToastKind = ""
	ToastWarning ToastKind = "warning"
	ToastError   ToastKind = "error"
)

// Toast shows text in a toast, as sonner's toast(): an error or a warning
// for longer, sticky until closed. The same text showing shows anew.
func Toast(c *Context, text string, kind ToastKind, sticky bool) {
	t := ui.Toast{ID: text, Title: text, Type: string(kind)}
	switch {
	case sticky:
		t.Timeout = -1
	case kind != ToastInfo:
		t.Timeout = 8 * time.Second
	}
	c.AddToast(t)
}

// Toasts builds the window's toasts, at the bottom on the right as Jenab's
// sonner Toaster: call it once in the view. Each has a close button.
func Toasts(c *Context) Element {
	k := TokensOf(c)
	return ui.ToastViewportBase(c, func(viewport ui.Element, toasts []ui.Toast) {
		viewport.Padding(16).AlignItems(ui.End).Justify(ui.End).Gap(8)
		for _, t := range toasts {
			toast := ui.ToastBase(c, t)
			toast.Root.Row().Gap(10).Padding(12, 14).Width(356).MaxWidthPercent(90).Radius(Radius).
				Background(k.Popover).Border(1, k.Border).TextColor(k.PopoverForeground).AlignItems(ui.Start).
				Shadow(0, 4, 12, 0, ui.Oklch(0, 0, 0).Alpha(0.1))
			toast.Root.Transition(ui.ElementTransition{Enter: &ui.Motion{Y: 8}, Exit: &ui.Motion{}})
			toast.Root.Children(func() {
				switch ToastKind(t.Type) {
				case ToastError:
					ui.Icon(c, IconError).Size(16, 16).TextColor(k.Tone(Red).Text).MarginY(2)
				case ToastWarning:
					ui.Icon(c, IconWarning).Size(16, 16).TextColor(k.Tone(Amber).Text).MarginY(2)
				}
				Text(c, t.Title).Grow(1).FontSize(13)
				if t.Action != "" {
					toast.ActionButton().Children(func() { ui.Text(c, t.Action) })
				}
				toast.CloseButton().Label("Close").Radius(4).Children(func() {
					ui.Icon(c, IconX).Size(14, 14).TextColor(k.MutedForeground)
				})
			})
		}
	})
}
