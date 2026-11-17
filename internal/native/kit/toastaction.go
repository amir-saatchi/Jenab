package kit

import "github.com/egoist/mygo/ui"

// ToastAction shows text in a toast with detail under it, if not "", and
// a button showing action that runs do, as sonner's toast with an action:
// a file deleted with its Undo. It closes as the button is clicked.
func ToastAction(c *Context, text, detail, action string, do func()) {
	c.AddToast(ui.Toast{ID: text, Title: text, Description: detail, Action: action, OnAction: do})
}

// StickyToastAction is ToastAction with kind's icon that stays until it is
// closed or its button is clicked: the notice that the app closed
// unexpectedly, with Open the log (TASK-067).
func StickyToastAction(c *Context, text, detail, action string, kind ToastKind, do func()) {
	c.AddToast(ui.Toast{ID: text, Title: text, Description: detail, Type: string(kind), Action: action, OnAction: do, Timeout: -1})
}
