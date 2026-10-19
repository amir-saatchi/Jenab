package main

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// A chunk is one piece of a file: what is indexed and embedded, and the
// byte range of the file it covers. A chunk that overlaps a target's range
// is a hit (Method 2).
type chunk struct {
	ID         string // file#index
	File       string
	Start, End int
	Text       string
}

// A chunker cuts a document into chunks. Round 2's model-made chunks are
// one more chunker, which is why Chunk takes a context and can fail.
type chunker interface {
	Name() string
	Chunk(ctx context.Context, d doc) ([]chunk, error)
}

// Sizes are in tokens, estimated as the app does: 4 bytes a token.
const (
	bytesPerToken  = 4
	maxChunkTokens = 400
	overlapTokens  = 50
	maxChunkBytes  = maxChunkTokens * bytesPerToken
	overlapBytes   = overlapTokens * bytesPerToken
)

func tokens(s string) int { return len(s) / bytesPerToken }

var chunkers = []chunker{fixedChunker{}, headingChunker{}}

func chunkerByName(name string) (chunker, bool) {
	for _, c := range chunkers {
		if c.Name() == name {
			return c, true
		}
	}
	return nil, false
}

type span struct{ start, end int }

func makeChunks(d doc, spans []span, text func(span) string) []chunk {
	out := make([]chunk, 0, len(spans))
	for _, s := range spans {
		out = append(out, chunk{ID: d.File + "#" + strconv.Itoa(len(out)), File: d.File, Start: s.start, End: s.end, Text: text(s)})
	}
	return out
}

// fixedChunker: 400-token windows with 50 tokens of overlap, each cut at
// the nearest paragraph or sentence end before the limit.
type fixedChunker struct{}

func (fixedChunker) Name() string { return "fixed" }

func (fixedChunker) Chunk(_ context.Context, d doc) ([]chunk, error) {
	spans := windows(d.Text, 0, len(d.Text), maxChunkBytes, overlapBytes)
	return makeChunks(d, spans, func(s span) string { return d.Text[s.start:s.end] }), nil
}

// windows cuts text[from:to] into spans of at most limit bytes. Each span
// ends at the nearest paragraph or sentence end before the limit (else a
// word end, else a character end), and the next one starts at a sentence
// or word start within overlap bytes before that end.
func windows(text string, from, to, limit, overlap int) []span {
	var out []span
	start := skipSpace(text, from, to)
	for start < to {
		end := to
		if end-start > limit {
			end = cutBefore(text, start, start+limit)
		}
		if s := trimSpan(text, span{start, end}); s.start < s.end {
			out = append(out, s)
		}
		if end >= to {
			break
		}
		next := end
		if overlap > 0 {
			next = startAfter(text, max(end-overlap, start+1), end)
		}
		if next <= start {
			next = end
		}
		start = skipSpace(text, next, to)
	}
	return out
}

// sentenceEnd reports whether a cut at i ends a sentence or a line: the
// byte before is ., !, ?, ؟, … or ; and the byte at i is white space, or
// the byte at i is a line end.
func sentenceEnd(text string, i int) bool {
	if i >= len(text) {
		return true
	}
	if text[i] == '\n' {
		return true
	}
	if i == 0 || (text[i] != ' ' && text[i] != '\t') {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(text[:i])
	return strings.ContainsRune(".!?؟…;:", r)
}

// cutBefore finds where a span starting at lo ends, at most hi. It looks
// in the span's second half: the last paragraph or sentence end, else the
// last space, else the last character end.
func cutBefore(text string, lo, hi int) int {
	floor := lo + (hi-lo)/2
	for i := hi; i > floor; i-- {
		if sentenceEnd(text, i) {
			return i
		}
	}
	for i := hi; i > floor; i-- {
		if isSpace(text[i]) {
			return i
		}
	}
	for i := hi; i > lo; i-- {
		if utf8.RuneStart(text[i]) {
			return i
		}
	}
	return hi
}

// startAfter finds where the next span starts, in [lo, hi): the first
// sentence start, else the first word start, else hi.
func startAfter(text string, lo, hi int) int {
	for i := lo; i < hi; i++ {
		if sentenceEnd(text, i) {
			return i
		}
	}
	for i := lo; i < hi; i++ {
		if isSpace(text[i]) {
			return i
		}
	}
	return hi
}

// isSpace is ASCII white space. Bytes are tested, not runes: a byte of a
// longer character, such as 0x85 in م, isn't white space.
func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\v' || b == '\f'
}

func skipSpace(text string, i, to int) int {
	for i < to && isSpace(text[i]) {
		i++
	}
	return i
}

func trimSpan(text string, s span) span {
	s.start = skipSpace(text, s.start, s.end)
	for s.end > s.start && isSpace(text[s.end-1]) {
		s.end--
	}
	return s
}

// headingChunker: one chunk per section, led by the file name and heading
// path. A long section is split at paragraph ends. A table stays in one
// chunk if it fits; otherwise it is split by rows, the header repeated.
type headingChunker struct{}

func (headingChunker) Name() string { return "heading" }

const pathSep = " › "

var headingRE = regexp.MustCompile(`^(#{1,6})[ \t]+(.+?)[ \t#]*$`)

type lineKind int

const (
	blankLine lineKind = iota
	textLine
	tableLine
	headingLine
)

type line struct {
	start, end int // end excludes the line end
	kind       lineKind
	level      int
	title      string
}

// lines classifies each line. Lines in a fenced code block are text, so a
// # or a blank line in code neither starts a section nor ends a block.
func lines(text string) []line {
	var out []line
	fence := false
	for start := 0; start < len(text); {
		end := strings.IndexByte(text[start:], '\n')
		next := start + end + 1
		if end < 0 {
			end, next = len(text), len(text)
		} else {
			end += start
		}
		s := strings.TrimRight(text[start:end], "\r")
		t := strings.TrimSpace(s)
		l := line{start: start, end: start + len(s), kind: textLine}
		switch {
		case strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~"):
			fence = !fence
		case fence:
		case t == "":
			l.kind = blankLine
		case strings.HasPrefix(t, "|"):
			l.kind = tableLine
		default:
			if m := headingRE.FindStringSubmatch(s); m != nil {
				l.kind, l.level, l.title = headingLine, len(m[1]), m[2]
			}
		}
		out = append(out, l)
		start = next
	}
	return out
}

type section struct {
	path  []string
	start int // the heading line's start
	body  []line
}

func sections(ls []line) []section {
	var (
		out   []section
		stack []line
		cur   = section{}
	)
	flush := func() {
		for _, l := range cur.body {
			if l.kind != blankLine {
				out = append(out, cur)
				return
			}
		}
	}
	for _, l := range ls {
		if l.kind != headingLine {
			cur.body = append(cur.body, l)
			continue
		}
		flush()
		for len(stack) > 0 && stack[len(stack)-1].level >= l.level {
			stack = stack[:len(stack)-1]
		}
		stack = append(stack, l)
		path := make([]string, len(stack))
		for i, h := range stack {
			path[i] = h.title
		}
		cur = section{path: path, start: l.start}
	}
	flush()
	return out
}

// A block is a paragraph, a list, a table or a code block: lines between
// blank lines, with a table apart from the text around it.
type block struct {
	start, end int
	lines      []line // a table's rows
	table      bool
}

func blocks(body []line) []block {
	var out []block
	var cur *block
	for _, l := range body {
		if l.kind == blankLine {
			cur = nil
			continue
		}
		isTable := l.kind == tableLine
		if cur == nil || cur.table != isTable {
			out = append(out, block{start: l.start, table: isTable})
			cur = &out[len(out)-1]
		}
		cur.end = l.end
		cur.lines = append(cur.lines, l)
	}
	return out
}

func (headingChunker) Chunk(_ context.Context, d doc) ([]chunk, error) {
	var out []chunk
	for _, s := range sections(lines(d.Text)) {
		bs := blocks(s.body)
		if len(bs) == 0 {
			continue
		}
		lead := strings.Join(append([]string{d.SourceName}, s.path...), pathSep) + "\n\n"
		budget := maxChunkBytes - len(lead)
		// The first piece's range starts at the heading, so a quote of the
		// heading itself is in it. The text has the path in the lead instead.
		first := true
		add := func(start, end int, text string) {
			if first && len(s.path) > 0 {
				start = s.start
			}
			first = false
			out = append(out, chunk{ID: d.File + "#" + strconv.Itoa(len(out)), File: d.File, Start: start, End: end, Text: lead + text})
		}
		// Whole blocks are packed; a block too big for a chunk is split alone.
		from, to := -1, -1
		flush := func() {
			if from >= 0 {
				add(from, to, d.Text[from:to])
				from, to = -1, -1
			}
		}
		for _, b := range bs {
			if b.end-b.start > budget {
				flush()
				if b.table {
					for _, p := range tableParts(d.Text, b, budget) {
						add(p.start, p.end, p.text)
					}
				} else {
					for _, w := range windows(d.Text, b.start, b.end, budget, 0) {
						add(w.start, w.end, d.Text[w.start:w.end])
					}
				}
				continue
			}
			if from >= 0 && b.end-from > budget {
				flush()
			}
			if from < 0 {
				from = b.start
			}
			to = b.end
		}
		flush()
	}
	return out, nil
}

type tablePart struct {
	start, end int
	text       string
}

var separatorRE = regexp.MustCompile(`^\s*\|?\s*:?-{3,}`)

// tableParts splits a table by rows, each part led by the header row and
// its separator. A row too long for a part with the header is cut alone.
func tableParts(text string, b block, budget int) []tablePart {
	rows := b.lines
	nHead := 1
	if len(rows) > 1 && separatorRE.MatchString(text[rows[1].start:rows[1].end]) {
		nHead = 2
	}
	header := text[rows[0].start:rows[nHead-1].end] + "\n"
	var out []tablePart
	from, to := -1, -1
	flush := func() {
		if from >= 0 {
			p := tablePart{start: from, end: to, text: header + text[from:to]}
			if len(out) == 0 {
				p.start, p.text = rows[0].start, text[rows[0].start:to]
			}
			out = append(out, p)
			from, to = -1, -1
		}
	}
	for _, r := range rows[nHead:] {
		if len(header)+r.end-r.start > budget {
			flush()
			for _, w := range windows(text, r.start, r.end, budget, 0) {
				out = append(out, tablePart{start: w.start, end: w.end, text: text[w.start:w.end]})
			}
			continue
		}
		if from >= 0 && len(header)+r.end-from > budget {
			flush()
		}
		if from < 0 {
			from = r.start
		}
		to = r.end
	}
	flush()
	return out
}
