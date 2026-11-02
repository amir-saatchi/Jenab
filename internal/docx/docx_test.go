package docx

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "write the Markdown of the test documents to testdata")

func extract(t testing.TB, b []byte) (string, error) {
	t.Helper()
	return Extract(context.Background(), bytes.NewReader(b), int64(len(b)))
}

// golden compares md with testdata/name.md.
func golden(t *testing.T, name, md string) {
	t.Helper()
	file := filepath.Join("testdata", name+".md")
	if *update {
		if err := os.WriteFile(file, []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("%v (run with -update to write it)", err)
	}
	if md != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("%s: got\n%s\nwant\n%s", name, md, want)
	}
}

const headingStyles = `<w:docDefaults/>` +
	`<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>` +
	`<w:style w:type="character" w:styleId="berschrift1Zchn"><w:name w:val="heading 1"/></w:style>`

// The built documents, each with its Markdown in testdata.
var built = map[string]doc{
	"headings": {
		styles: headingStyles +
			// A German Word names the styles' IDs in German; their names stay English.
			styleDef("berschrift1", "heading 1", "Normal", `<w:outlineLvl w:val="0"/>`) +
			styleDef("berschrift2", "Heading 2", "Normal", "") +
			styleDef("Titel", "Title", "Normal", "") +
			styleDef("Kapitel", "Kapitel", "berschrift2", "") + // based on Heading 2
			styleDef("Heading3", "Custom three", "Normal", `<w:outlineLvl w:val="2"/>`) +
			styleDef("Heading1", "Not a heading", "Normal", "") + // the ID lies
			styleDef("Loop1", "Loop one", "Loop2", "") + styleDef("Loop2", "Loop two", "Loop1", ""),
		body: p(pStyle("Titel"), r("Vertrag über Leistungen")) +
			p(pStyle("berschrift1"), r("1 Gegenstand")) +
			p("", r("Der Auftragnehmer erbringt "), r("die Leistungen.")) +
			p(pStyle("berschrift2"), r("1.1 Umfang")) +
			p(pStyle("Kapitel"), r("1.2 Kapitel nach Heading 2")) +
			p(pStyle("Heading3"), r("Level three by outline")) +
			p(pStyle("Heading1"), r("Only the ID says heading")) +
			p(`<w:outlineLvl w:val="3"/>`, r("Own outline level four")) +
			p(pStyle("berschrift1")+`<w:outlineLvl w:val="9"/>`, r("Body text by its own outline level")) +
			p(pStyle("Loop1"), r("A loop of styles")) +
			p(pStyle("berschrift1")) + // an empty heading is left out
			p("", r("Line one"), `<w:r><w:br/></w:r>`, r("  line two")) +
			`<w:sectPr/>`,
	},
	"lists": {
		styles: headingStyles + styleDef("ListBullet", "List Bullet", "Normal", numPr("2", 0)),
		numbering: `<w:abstractNum w:abstractNumId="0">` +
			`<w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="bullet"/></w:lvl>` +
			`<w:lvl w:ilvl="1"><w:start w:val="1"/><w:numFmt w:val="bullet"/></w:lvl></w:abstractNum>` +
			`<w:abstractNum w:abstractNumId="1">` +
			`<w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="decimal"/></w:lvl>` +
			`<w:lvl w:ilvl="1"><w:start w:val="1"/><w:numFmt w:val="lowerLetter"/></w:lvl>` +
			`<w:lvl w:ilvl="2"><w:start w:val="1"/><w:numFmt w:val="none"/></w:lvl></w:abstractNum>` +
			`<w:num w:numId="1"><w:abstractNumId w:val="1"/></w:num>` +
			`<w:num w:numId="2"><w:abstractNumId w:val="0"/></w:num>` +
			`<w:num w:numId="3"><w:abstractNumId w:val="1"/><w:lvlOverride w:ilvl="0"><w:startOverride w:val="5"/></w:lvlOverride></w:num>`,
		body: p(numPr("2", 0), r("Apples")) +
			p(numPr("2", 1), r("Green")) +
			p(numPr("2", 1), r("Red")) +
			p(numPr("2", 0), r("Pears")) +
			p("", r("Between the lists.")) +
			p(numPr("1", 0), r("First")) +
			p(numPr("1", 1), r("Sub a")) +
			p(numPr("1", 1), r("Sub b")) +
			p(numPr("1", 2), r("No marker")) +
			p(numPr("1", 0), r("Second"), `<w:r><w:br/></w:r>`, r("with a second line")) +
			p(numPr("1", 1), r("Sub again")) +
			p(numPr("1", 0)) + // an empty item still counts
			p(numPr("1", 0), r("Fourth")) +
			p(pStyle("ListBullet"), r("Bullet from the style")) +
			p(numPr("3", 0), r("Starts at five")) +
			p(numPr("0", 0), r("numId 0 is no list")) +
			p(numPr("9", 0), r("An unknown list")),
	},
	"tables": {
		body: tbl(
			tr("", tc("", p("", r("Name"))), tc("", p("", r("Role"))), tc("", p("", r("Note")))),
			tr("", tc(`<w:gridSpan w:val="2"/>`, p("", r("Spans two"))), tc(`<w:vMerge w:val="restart"/>`, p("", r("Down")))),
			tr("", tc("", p("", r("a|b"))), tc("", p("", r("Two")), p("", r("paragraphs"))), tc(`<w:vMerge/>`)),
			tr(`<w:del w:id="1" w:author="x" w:date="2026-01-01T00:00:00Z"/>`, tc("", p("", r("Deleted row")))),
			tr(`<w:gridBefore w:val="1"/>`, tc("", tbl(tr("", tc("", p("", r("Nested"))), tc("", p("", r("table"))))))),
		) + p("", r("After the table.")) +
			tbl(tr("", tc(`<w:gridSpan w:val="2000"/>`, p("", r("Too wide"))))),
	},
	"links-notes": {
		links: map[string]string{"rId7": "https://example.com/a b?(x)", "rId8": "https://example.org/note"},
		body: p("", r("See "), `<w:hyperlink r:id="rId7"><w:r><w:t>the [site]</w:t></w:r></w:hyperlink>`, r(" or "),
			`<w:hyperlink w:anchor="_Toc1"><w:r><w:t>a heading</w:t></w:r></w:hyperlink>`, r(".")) +
			p("", r("A field: "),
				`<w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText xml:space="preserve"> HYPERLINK "https://example.net/" \o "tip" </w:instrText></w:r>`,
				`<w:r><w:fldChar w:fldCharType="separate"/></w:r>`, r("field link"), `<w:r><w:fldChar w:fldCharType="end"/></w:r>`,
				r(", page "), `<w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText>PAGE</w:instrText></w:r>`,
				`<w:r><w:fldChar w:fldCharType="separate"/></w:r>`, r("3"), `<w:r><w:fldChar w:fldCharType="end"/></w:r>`,
				`<w:fldSimple w:instr="HYPERLINK &quot;https://example.com/simple&quot;"><w:r><w:t> simple</w:t></w:r></w:fldSimple>`) +
			p("", r("Claim"), `<w:r><w:footnoteReference w:id="2"/></w:r>`, r(" and more"), `<w:r><w:endnoteReference w:id="1"/></w:r>`,
				r(" again"), `<w:r><w:footnoteReference w:id="2"/></w:r>`, `<w:r><w:footnoteReference w:id="5"/></w:r>`),
		footnotes: `<w:footnote w:type="separator" w:id="-1"><w:p><w:r><w:separator/></w:r></w:p></w:footnote>` +
			`<w:footnote w:id="2"><w:p><w:r><w:footnoteRef/></w:r><w:r><w:t xml:space="preserve"> The source, </w:t></w:r>` +
			`<w:hyperlink r:id="rId8"><w:r><w:t>linked</w:t></w:r></w:hyperlink></w:p><w:p>` + r("second paragraph") + `</w:p></w:footnote>` +
			`<w:footnote w:id="3"><w:p>` + r("Not referenced") + `</w:p></w:footnote>`,
		endnotes: `<w:endnote w:id="1"><w:p><w:r><w:endnoteRef/></w:r>` + r(" An endnote.") + `</w:p></w:endnote>`,
		extra: map[string]string{"word/_rels/footnotes.xml.rels": `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId8" Type="` + relBase + `hyperlink" Target="https://example.org/note" TargetMode="External"/></Relationships>`},
	},
	"changes": {
		body: p("", r("Kept "), `<w:ins w:id="1" w:author="a" w:date="2026-01-01T00:00:00Z">`+r("inserted ")+`</w:ins>`,
			`<w:del w:id="2" w:author="a" w:date="2026-01-01T00:00:00Z"><w:r><w:delText>deleted </w:delText></w:r></w:del>`, r("text.")) +
			p(`<w:rPr><w:del w:id="3" w:author="a" w:date="2026-01-01T00:00:00Z"/></w:rPr>`, r("This paragraph's mark was deleted, ")) +
			p("", r("so it joins this one.")) +
			p("", `<w:moveFrom w:id="4" w:author="a" w:date="2026-01-01T00:00:00Z">`+r("Moved away.")+`</w:moveFrom>`,
				`<w:moveTo w:id="5" w:author="a" w:date="2026-01-01T00:00:00Z">`+r("Moved here.")+`</w:moveTo>`) +
			p(`<w:pPrChange w:id="6" w:author="a" w:date="2026-01-01T00:00:00Z"><w:pPr><w:pStyle w:val="Heading1"/></w:pPr></w:pPrChange>`,
				`<w:r><w:rPr><w:vanish/></w:rPr><w:t>hidden</w:t></w:r>`,
				`<w:r><w:rPr><w:vanish w:val="0"/><w:rPrChange w:id="7" w:author="a" w:date="2026-01-01T00:00:00Z"><w:rPr><w:vanish/></w:rPr></w:rPrChange></w:rPr><w:t>Shown, its style change ignored.</w:t></w:r>`,
				`<w:commentRangeStart w:id="0"/><w:r><w:commentReference w:id="0"/></w:r>`),
		styles: headingStyles + styleDef("Heading1", "heading 1", "", ""),
	},
	"objects": {
		body: p("", r("Before the box. "),
			`<w:r><mc:AlternateContent><mc:Choice Requires="wps"><w:drawing><wp:anchor><wp:docPr id="1" name="Text Box 1"/>`+
				`<a:graphic><a:graphicData><wps:wsp><wps:txbx><w:txbxContent>`+p("", r("Text in a box."))+`</w:txbxContent></wps:txbx></wps:wsp></a:graphicData></a:graphic></wp:anchor></w:drawing></mc:Choice>`+
				`<mc:Fallback><w:pict><v:shape><v:textbox><w:txbxContent>`+p("", r("Text in a box."))+`</w:txbxContent></v:textbox></v:shape></w:pict></mc:Fallback></mc:AlternateContent></w:r>`,
			r("After the box.")) +
			p("", `<w:r><w:drawing><wp:inline><wp:docPr id="2" name="Picture 2" descr="A cat on a [mat]&#10;at night"/><a:graphic><a:graphicData><pic:pic><pic:blipFill><a:blip r:embed="rId9"/></pic:blipFill></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`) +
			p("", r("No alt: "), `<w:r><w:drawing><wp:inline><wp:docPr id="3" name="Picture 3"/><a:graphic><a:graphicData><pic:pic/></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`) +
			p("", r("Old picture: "), `<w:r><w:pict><v:shape alt="Logo"><v:imagedata r:id="rId10" o:title=""/></v:shape></w:pict></w:r>`) +
			p("", r("E = "), `<m:oMath><m:r><m:t>mc</m:t></m:r><m:sSup><m:e><m:r><m:t>2</m:t></m:r></m:e></m:sSup></m:oMath>`) +
			`<m:oMathPara><m:oMath><m:r><m:t>x</m:t></m:r></m:oMath></m:oMathPara>` +
			p("", `<w:r><w:object><v:shape><v:imagedata r:id="rId11"/></v:shape><o:OLEObject ProgID="Equation.3"/></w:object></w:r>`) +
			`<w:sdt><w:sdtPr><w:alias w:val="Field"/></w:sdtPr><w:sdtContent>` + p("", r("In a content control.")) + `</w:sdtContent></w:sdt>` +
			p("", r("Tab\there"), `<w:r><w:tab/><w:t>and</w:t><w:noBreakHyphen/><w:t>hyphen</w:t><w:softHyphen/><w:t>s</w:t></w:r>`),
	},
	"persian": {
		styles: headingStyles + styleDef("Heading1", "heading 1", "Normal", ""),
		// The text is in logical order, as Word stores it. "می‌خواهم" has a
		// ZWNJ, and "ي" + hamza above (U+0654) is decomposed, so NFC joins it.
		body: p(pStyle("Heading1")+`<w:bidi/>`, r("قرارداد خدمات")) +
			p(`<w:bidi/>`, `<w:r><w:rPr><w:rtl/></w:rPr><w:t>من می‌خواهم این متن را بخوانم.</w:t></w:r>`) +
			p(`<w:bidi/>`, r("ئ تجزیه شده")) +
			p("", r("Mixed: نسخه ۲ of the file")),
	},
}

func TestBuilt(t *testing.T) {
	for name, d := range built {
		t.Run(name, func(t *testing.T) {
			b := d.zip(t)
			if got := Detect(bytes.NewReader(b), int64(len(b))); got != MIME {
				t.Errorf("Detect = %q", got)
			}
			md, err := extract(t, b)
			if err != nil {
				t.Fatal(err)
			}
			golden(t, name, md)
		})
	}
}

// A paragraph with no style has the default paragraph style, which only
// w:default="1" makes one: a missing w:default is off, unlike a toggle
// such as w:b.
func TestDefaultStyle(t *testing.T) {
	d := doc{
		styles: styleDef("Heading1", "heading 1", "", "") +
			`<w:style w:type="paragraph" w:default="0" w:styleId="Big"><w:name w:val="heading 2"/></w:style>` +
			`<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>`,
		body: p(pStyle("Heading1"), r("A")) + p("", r("b")),
	}
	md, err := extract(t, d.zip(t))
	if err != nil || md != "# A\n\nb\n" {
		t.Errorf("got %q, %v", md, err)
	}
	d.styles = styleDef("Heading1", "heading 1", "", "") +
		`<w:style w:type="paragraph" w:default="1" w:styleId="Body"><w:name w:val="heading 3"/></w:style>`
	if md, err := extract(t, d.zip(t)); err != nil || md != "# A\n\n### b\n" {
		t.Errorf("heading as default: %q, %v", md, err)
	}
}

// Real documents saved by Word, LibreOffice and Google Docs, each with
// its Markdown next to it.
func TestRealFiles(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "*.docx"))
	if len(files) == 0 {
		t.Skip("no .docx files in testdata")
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".docx")
		t.Run(name, func(t *testing.T) {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(name, "password") { // saved with a password
				if _, err := extract(t, b); !errors.Is(err, ErrEncrypted) {
					t.Errorf("got %v, want ErrEncrypted", err)
				}
				return
			}
			if got := Detect(bytes.NewReader(b), int64(len(b))); !IsWord(got) {
				t.Errorf("Detect = %q", got)
			}
			md, err := extract(t, b)
			if err != nil {
				t.Fatal(err)
			}
			golden(t, name, md)
		})
	}
}

func TestDetect(t *testing.T) {
	macro := doc{body: p("", r("Macros are never run.")), mainType: macroMainType}.zip(t)
	if got := Detect(bytes.NewReader(macro), int64(len(macro))); got != MIMEMacro {
		t.Errorf("a .docm: %q", got)
	}
	if md, err := extract(t, macro); err != nil || md != "Macros are never run.\n" {
		t.Errorf("a .docm reads %q, %v", md, err)
	}
	xlsx := zipOf(t, map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/></Types>`,
		"xl/workbook.xml":     `<workbook/>`,
	})
	plain := zipOf(t, map[string]string{"word/document.xml": doc{}.parts()["word/document.xml"]})
	for name, b := range map[string][]byte{"a workbook": xlsx, "a zip with Word's parts but no content types": plain, "not a zip": []byte("PK\x03\x04 hello"), "empty": nil} {
		if got := Detect(bytes.NewReader(b), int64(len(b))); got != "" {
			t.Errorf("%s: %q", name, got)
		}
		if _, err := extract(t, b); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if _, err := extract(t, xlsx); !errors.Is(err, ErrNotWord) {
		t.Errorf("a workbook: %v", err)
	}
	// A main part typed only by the extension, named by the package's relationship.
	byExt := doc{body: p("", r("Typed by extension."))}.parts()
	byExt["[Content_Types].xml"] = `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="` + mainType + `"/></Types>`
	if md, err := extract(t, zipOf(t, byExt)); err != nil || md != "Typed by extension.\n" {
		t.Errorf("typed by extension: %q, %v", md, err)
	}
	// UTF-16 parts, which OPC allows.
	u16 := doc{body: p("", r("Sechzehn Bits: ü"))}.parts()
	u16["word/document.xml"] = utf16LE(strings.Replace(u16["word/document.xml"], `encoding="UTF-8"`, `encoding="UTF-16"`, 1))
	if md, err := extract(t, zipOf(t, u16)); err != nil || md != "Sechzehn Bits: ü\n" {
		t.Errorf("UTF-16: %q, %v", md, err)
	}
}

func utf16LE(s string) string {
	b := []byte{0xff, 0xfe}
	for _, r := range s {
		if r > 0xffff {
			panic("outside the BMP")
		}
		b = append(b, byte(r), byte(r>>8))
	}
	return string(b)
}

// bad is a file that fails, with the error it should give.
type bad struct {
	file []byte
	kind error
	msg  string
}

func badFiles(t testing.TB) map[string]bad {
	good := doc{body: p("", r("Hello."))}.zip(t)

	// A bomb: a document part that unpacks to 1,000 times its size.
	bomb := doc{}.parts()
	bomb["word/document.xml"] = `<w:document ` + nsW + `><w:body>` + strings.Repeat(" ", 30<<20) + `</w:body></w:document>`

	// Many parts that each pass the ratio but add up past 50 MB.
	big := doc{body: p("", r("Big."))}.parts()
	chunk := lowRatioXML(6 << 20)
	for i := range 10 {
		big[filepathJoin("word/media", i)] = chunk
	}
	bigRels := `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`
	for i := range 10 {
		bigRels += `<Relationship Id="rS` + string(rune('a'+i)) + `" Type="` + relBase + `styles" Target="media/` + itoa(i) + `.xml"/>`
	}
	big["word/_rels/document.xml.rels"] = bigRels + `</Relationships>`

	many := doc{body: p("", r("Many."))}.parts()
	for i := range MaxEntries {
		many[filepathJoin("word/x", i)] = "x"
	}
	tiny := map[string]string{}
	for i := range 120_000 {
		tiny[filepathJoin("t", i)] = ""
	}

	deep := doc{body: strings.Repeat("<w:sdt><w:sdtContent>", 1000) + p("", r("Deep.")) + strings.Repeat("</w:sdtContent></w:sdt>", 1000)}.zip(t)

	broken := doc{body: p("", r("Broken"))}.parts()
	broken["word/document.xml"] = `<w:document ` + nsW + `><w:body><w:p a=b></w:p></w:body></w:document>`
	noMain := doc{}.parts()
	delete(noMain, "word/document.xml")

	// A zip whose entries are encrypted with the zip's own password
	// scheme: the flag is set on an otherwise good file.
	zipCrypto := bytes.Clone(good)
	setEncryptedFlags(zipCrypto)

	return map[string]bad{
		"zip bomb":                 {zipOf(t, bomb), ErrTooLarge, "word/document.xml unpacks to more than 100 times its packed size, as a zip bomb does"},
		"over 50 MB unpacked":      {zipOf(t, big), ErrTooLarge, "it unpacks to more than 50 MB"},
		"over 2,000 parts":         {zipOf(t, many), ErrTooLarge, "it has more than 2,000 parts"},
		"a huge directory":         {zipOf(t, tiny), ErrTooLarge, "it has more than 2,000 parts"},
		"nested too deeply":        {deep, ErrBroken, "it is damaged: word/document.xml is nested too deeply"},
		"cut short":                {good[:len(good)/2], ErrBroken, "it is damaged: the zip can't be read"},
		"not XML":                  {zipOf(t, broken), ErrBroken, "it is damaged: word/document.xml is not valid XML"},
		"no document part":         {zipOf(t, noMain), ErrBroken, "it is damaged: its document part word/document.xml is missing"},
		"a zip that only claims":   {zipOf(t, map[string]string{"[Content_Types].xml": "<Types/>", "word/document.xml": "<x/>"}), ErrNotWord, "it is not a Word document"},
		"encrypted (OLE)":          {oleWith(true), ErrEncrypted, "it is password-protected"},
		"an old .doc":              {oleWith(false), ErrOldFormat, ErrOldFormat.Error()},
		"zip entries encrypted":    {zipCrypto, ErrEncrypted, "it is password-protected"},
		"random bytes after PK":    {append([]byte("PK\x03\x04"), bytes.Repeat([]byte{0x5a}, 4000)...), ErrBroken, "it is damaged: the zip can't be read"},
		"too short to be anything": {[]byte("PK"), ErrBroken, "it is damaged: it is not a zip"},
	}
}

func filepathJoin(dir string, i int) string { return dir + "/" + itoa(i) + ".xml" }

func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return itoa(i/10) + string(rune('0'+i%10))
}

// lowRatioXML is about n bytes of XML that packs well under the ratio.
func lowRatioXML(n int) string {
	var b strings.Builder
	b.WriteString(`<w:styles ` + nsW + `>`)
	x := uint32(1)
	for b.Len() < n {
		x = x*1664525 + 1013904223
		b.WriteString(`<w:style w:styleId="s`)
		b.WriteString(itoa(int(x % 100000)))
		b.WriteString(`"/>`)
	}
	b.WriteString(`</w:styles>`)
	return b.String()
}

// setEncryptedFlags sets bit 0 of every entry's flags in the central
// directory.
func setEncryptedFlags(b []byte) {
	sig := []byte("PK\x01\x02")
	for i := 0; ; {
		j := bytes.Index(b[i:], sig)
		if j < 0 {
			return
		}
		i += j
		b[i+8] |= 1
		i += 4
	}
}

// oleWith is the start of an OLE compound file, with the directory entry
// of an encrypted package or of a Word 97 document.
func oleWith(encrypted bool) []byte {
	b := make([]byte, 512*4)
	copy(b, oleMagic)
	name := "WordDocument"
	if encrypted {
		name = "EncryptedPackage"
	}
	entry := b[1024:]
	for i, c := range name {
		entry[2*i] = byte(c)
	}
	entry[64], entry[66] = byte(2*len(name)+2), 2
	return b
}

func TestBadFiles(t *testing.T) {
	for name, c := range badFiles(t) {
		t.Run(name, func(t *testing.T) {
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			start := time.Now()
			_, err := extract(t, c.file)
			took := time.Since(start)
			runtime.ReadMemStats(&after)
			if !errors.Is(err, c.kind) || err.Error() != c.msg {
				t.Errorf("error %v, want %q", err, c.msg)
			}
			var de *Error
			if !errors.As(err, &de) {
				t.Errorf("%T is not an *Error", err)
			}
			if took > time.Second {
				t.Errorf("took %v", took)
			}
			if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 100<<20 {
				t.Errorf("allocated %d MB", alloc>>20)
			}
		})
	}
}

func TestCanceled(t *testing.T) {
	b := built["tables"].zip(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Extract(ctx, bytes.NewReader(b), int64(len(b))); !errors.Is(err, context.Canceled) {
		t.Errorf("%v", err)
	}
}

func TestEncryptedNotOLE(t *testing.T) {
	b := built["lists"].zip(t)
	if Encrypted(bytes.NewReader(b), int64(len(b))) {
		t.Error("a zip is not encrypted")
	}
	if e := oleWith(true); !Encrypted(bytes.NewReader(e), int64(len(e))) {
		t.Error("not found")
	}
}

// A bounded table: the cells a hostile span asks for are counted before
// any are made.
func TestTableBounds(t *testing.T) {
	rows := make([]string, 0, 300)
	rows = append(rows, tr("", tc(`<w:gridSpan w:val="900"/>`, p("", r("wide")))))
	for range 299 {
		rows = append(rows, tr("", tc(`<w:gridSpan w:val="900"/><w:vMerge/>`)))
	}
	b := doc{body: tbl(rows...)}.zip(t)
	md, err := extract(t, b)
	if err != nil {
		t.Fatal(err)
	}
	if md != "wide\n" {
		t.Errorf("%q", md)
	}
}

// Mutations is a small fuzz run that works where the fuzzing engine
// doesn't: on Windows, go test -fuzz stops making progress after a few
// seconds, even for a trivial target. The documents are changed at random
// bytes, with a fixed seed; each must end in text or an *Error, quickly.
func TestMutations(t *testing.T) {
	var seeds [][]byte
	for _, name := range []string{"headings", "lists", "tables", "links-notes", "changes", "objects", "persian"} {
		seeds = append(seeds, built[name].zip(t))
	}
	rng := rand.New(rand.NewPCG(1, 2))
	n := 20_000
	if testing.Short() {
		n = 3000
	}
	for range n {
		b := bytes.Clone(seeds[rng.IntN(len(seeds))])
		for range 1 + rng.IntN(4) {
			switch i := rng.IntN(len(b)); rng.IntN(3) {
			case 0:
				b[i] ^= byte(1 << rng.IntN(8))
			case 1:
				b[i] = byte(rng.IntN(256))
			case 2:
				b = append(b[:i], b[min(len(b), i+1+rng.IntN(8)):]...)
			}
		}
		start := time.Now()
		_, err := extract(t, b)
		var de *Error
		if err != nil && !errors.As(err, &de) {
			t.Fatalf("%T %v is not an *Error", err, err)
		}
		if d := time.Since(start); d > time.Second {
			t.Fatalf("took %v", d)
		}
	}
}

func FuzzExtract(f *testing.F) {
	for _, d := range built {
		f.Add(d.zip(f))
	}
	good := built["lists"].zip(f)
	f.Add(good[:len(good)/2])
	f.Add(oleWith(true))
	f.Add(zipOf(f, map[string]string{"[Content_Types].xml": "<Types/>", "word/document.xml": "<x/>"}))
	f.Fuzz(func(t *testing.T, b []byte) {
		md, err := Extract(context.Background(), bytes.NewReader(b), int64(len(b)))
		if err != nil {
			var de *Error
			if !errors.As(err, &de) {
				t.Fatalf("%T %v is not an *Error", err, err)
			}
			return
		}
		if len(md) > 64<<20 {
			t.Fatalf("%d bytes of text", len(md))
		}
	})
}
