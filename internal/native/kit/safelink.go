package kit

import "github.com/egoist/mygo/ui"

// SafeLink reports whether a link that came with content, as a model's
// answer, a user's message, a file or a page, may open in the system: an
// http, https or mailto address, as MyGo's ui.SafeToOpen says (TASK-064).
// Anything else, as file:, javascript:, a program's name, a relative path,
// a \\host or //host path, or a link with a control character, would open
// a file or run a program through the shell, so it opens nothing. Every
// place that opens a link from data in the system checks it first.
func SafeLink(url string) bool { return ui.SafeToOpen(url) }
