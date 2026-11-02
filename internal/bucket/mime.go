package bucket

import (
	"bytes"
	"mime"
	"net/http"
	"path"
	"strings"
)

// DetectMIME finds an object's type from its first bytes when it is
// stored, never from what its source claimed (SPEC 4.7). The key's
// extension only refines plain text into another text type that is safe to
// show, such as CSV or JSON.
//
// A zip is looked at again once it is stored: store finds Word documents
// by their parts (TASK-021). An OLE compound file, which
// http.DetectContentType doesn't know, is an old .doc, .xls or .ppt or an
// Office file encrypted with a password; it gets its own type, so reading
// it can say which.
func DetectMIME(head []byte, key string) string {
	if isSVG(head) {
		return "image/svg+xml"
	}
	if bytes.HasPrefix(head, []byte("\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1")) {
		return "application/x-ole-storage"
	}
	t := http.DetectContentType(head)
	if base, params, err := mime.ParseMediaType(t); err == nil && base == "text/plain" {
		if refined, ok := textByExt[strings.ToLower(path.Ext(key))]; ok {
			if cs := params["charset"]; cs != "" {
				return refined + "; charset=" + cs
			}
			return refined
		}
	}
	return t
}

var textByExt = map[string]string{
	".csv":  "text/csv",
	".tsv":  "text/tab-separated-values",
	".json": "application/json",
	".md":   "text/markdown",
}

// isSVG reports whether head starts an SVG document: after a byte-order
// mark, an XML declaration, comments and a doctype, the first element is
// <svg. An SVG whose start doesn't fit in head is not found and is then
// served as text, which is safe.
func isSVG(head []byte) bool {
	b := bytes.TrimPrefix(head, []byte("\xef\xbb\xbf"))
	for {
		b = bytes.TrimLeft(b, " \t\r\n")
		var end string
		switch {
		case hasPrefixFold(b, "<?xml"):
			end = "?>"
		case bytes.HasPrefix(b, []byte("<!--")):
			end = "-->"
		case hasPrefixFold(b, "<!doctype"):
			end = ">"
		default:
			return hasPrefixFold(b, "<svg") && len(b) > 4 && (b[4] == '>' || b[4] == ' ' || b[4] == '\t' || b[4] == '\r' || b[4] == '\n' || b[4] == '/')
		}
		i := bytes.Index(b, []byte(end))
		if i < 0 {
			return false
		}
		b = b[i+len(end):]
	}
}

func hasPrefixFold(b []byte, prefix string) bool {
	return len(b) >= len(prefix) && strings.EqualFold(string(b[:len(prefix)]), prefix)
}
