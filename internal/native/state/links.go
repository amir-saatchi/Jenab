package state

import (
	"regexp"
	"strings"
)

// The chat's jenab:// links (SPEC 4.5): in an answer, a link written as a
// link, bare, or alone in inline code opens the file, as the frontend's
// rehypeJenabLinks makes them (chat/markdown-impl.tsx). MyGo's Markdown
// links only http, https, www and e-mail addresses written bare.

var (
	fenceRe   = regexp.MustCompile("^ {0,3}(```|~~~)")
	refDefRe  = regexp.MustCompile(`^ {0,3}\[[^\]]+\]:`)
	codeRunRe = regexp.MustCompile("`+")
)

// MarkLinks writes the bare jenab:// links of Markdown src as autolinks,
// <jenab://…>, which MyGo's Markdown links. Links written as links, and
// code, stay as they are.
func MarkLinks(src string) string {
	if !strings.Contains(src, "jenab://") {
		return src
	}
	lines := strings.SplitAfter(src, "\n")
	fence := ""
	for i, line := range lines {
		if m := fenceRe.FindStringSubmatch(line); m != nil {
			switch {
			case fence == "":
				fence = m[1]
			case m[1] == fence:
				fence = ""
			}
			continue
		}
		if fence != "" || refDefRe.MatchString(line) || !strings.Contains(line, "jenab://") {
			continue
		}
		lines[i] = markLine(line)
	}
	return strings.Join(lines, "")
}

// markLine marks the bare links of a line outside its code spans.
func markLine(line string) string {
	var b strings.Builder
	at := 0
	for at < len(line) {
		run := codeRunRe.FindStringIndex(line[at:])
		if run == nil {
			b.WriteString(markText(line[at:]))
			break
		}
		open := line[at+run[0] : at+run[1]]
		b.WriteString(markText(line[at : at+run[0]]))
		rest := line[at+run[1]:]
		// The span ends at the same run of backticks; without one the
		// backticks are text.
		end := closingRun(rest, len(open))
		if end < 0 {
			b.WriteString(open)
			at += run[1]
			continue
		}
		b.WriteString(open + rest[:end+len(open)])
		at += run[1] + end + len(open)
	}
	return b.String()
}

// closingRun is where a run of exactly n backticks starts in s, or -1.
func closingRun(s string, n int) int {
	for _, m := range codeRunRe.FindAllStringIndex(s, -1) {
		if m[1]-m[0] == n {
			return m[0]
		}
	}
	return -1
}

func markText(s string) string {
	ms := linksRe.FindAllStringIndex(s, -1)
	if ms == nil {
		return s
	}
	var b strings.Builder
	at := 0
	for _, m := range ms {
		// Inside a link already: [text](jenab://…), <jenab://…> or
		// [jenab://…](…).
		if m[0] > 0 && strings.ContainsRune("(<[", rune(s[m[0]-1])) {
			continue
		}
		if _, _, ok := ParseLink(s[m[0]:m[1]]); !ok {
			continue
		}
		b.WriteString(s[at:m[0]])
		b.WriteString("<" + s[m[0]:m[1]] + ">")
		at = m[1]
	}
	b.WriteString(s[at:])
	return b.String()
}

// CodeLink is the link inline code holds alone, for MyGo's Markdown to
// make the code a link.
func CodeLink(code string) (string, bool) {
	t := strings.TrimSpace(code)
	if _, _, ok := ParseLink(t); !ok {
		return "", false
	}
	return t, true
}
