package kit

// CopyText puts s on the system clipboard, as a file's Copy link does.
func CopyText(c *Context, s string) { c.WriteClipboard(s) }
