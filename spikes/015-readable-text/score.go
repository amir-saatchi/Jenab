package main

import (
	"regexp"
	"strings"
)

type Score struct {
	Err        bool
	MustHit    int
	MustTotal  int
	Leaks      int
	LeakTotal  int
	TitleOK    bool
	Links      int
	AbsLinks   int // hrefs already absolute in library output
	HrefTotal  int
	TextRows   int // table rows whose cells share a line in the text
	MDRows     int // same, after the Markdown step
	RowTotal   int
	Chars      int // extracted text length in runes
	MDChars    int
	RTLOk      bool // fa pages: phrases hit, ZWNJ phrase exact, no U+FFFD
	Mojibake   bool // U+FFFD or typical mis-decoding sequences in the text
	MissedMust []string
	LeakedNot  []string
}

var reMDLink = regexp.MustCompile(`\]\([^)]*\)`)
var mdStrip = strings.NewReplacer("[", "", "]", "", "*", "", "\\", "", "#", "", "|", " ", "`", "")

func mdPlain(s string) string {
	return mdStrip.Replace(reMDLink.ReplaceAllString(s, "]"))
}

var reMojibake = regexp.MustCompile(`Ã[\x{80}-\x{BF}¤¶¼Ÿ]|Ø[§¨ª«¬­®¯±²³´µ¶·¸¹º»¼½¾¿]|Ù[\x{80}-\x{8A}]|Û[Œ\x{8C}]|ï¿½`)

func scorePage(f *Fixture, r Result, md string, textIsMD bool) Score {
	s := Score{MustTotal: len(f.Must), LeakTotal: len(f.MustNot), RowTotal: len(f.Row)}
	if r.Err != nil {
		s.Err = true
		return s
	}
	text := r.Text
	if textIsMD {
		text = mdPlain(text)
	}
	nt := normalize(text)
	for _, p := range f.Must {
		if strings.Contains(nt, normalize(p)) {
			s.MustHit++
		} else {
			s.MissedMust = append(s.MissedMust, p)
		}
	}
	for _, p := range f.MustNot {
		if strings.Contains(nt, normalize(p)) {
			s.Leaks++
			s.LeakedNot = append(s.LeakedNot, p)
		}
	}
	s.TitleOK = f.Title != "" && strings.Contains(normalize(r.Title), normalize(f.Title))
	l, abs, tot := links(r.Node, f.URL)
	s.Links, s.AbsLinks, s.HrefTotal = len(l), abs, tot
	s.TextRows = rowsOnOneLine(text, f.Row)
	s.MDRows = rowsOnOneLine(mdPlain(md), f.Row)
	s.Chars = len([]rune(r.Text))
	s.MDChars = len([]rune(md))
	s.Mojibake = strings.ContainsRune(text, '�') || reMojibake.MatchString(text)
	if f.Lang == "fa" {
		zwnjOK := true
		for _, p := range f.Must {
			if strings.ContainsRune(p, '‌') {
				zwnjOK = zwnjOK && strings.Contains(nt, normalize(p))
			}
		}
		s.RTLOk = s.MustHit == s.MustTotal && zwnjOK && !s.Mojibake
	}
	return s
}

func rowsOnOneLine(text string, rows [][]string) int {
	if len(rows) == 0 {
		return 0
	}
	lines := strings.Split(text, "\n")
	n := 0
	for _, row := range rows {
		for _, line := range lines {
			nl := normalize(line)
			ok := true
			for _, cell := range row {
				if !strings.Contains(nl, normalize(cell)) {
					ok = false
					break
				}
			}
			if ok {
				n++
				break
			}
		}
	}
	return n
}
