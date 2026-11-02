package docx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"
)

// Documents for the tests are built here: the XML parts written into a
// zip, as Word lays them out.

const (
	nsW = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" ` +
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" ` +
		`xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" ` +
		`xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" ` +
		`xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture" ` +
		`xmlns:wps="http://schemas.microsoft.com/office/word/2010/wordprocessingShape" ` +
		`xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006" ` +
		`xmlns:v="urn:schemas-microsoft-com:vml" xmlns:o="urn:schemas-microsoft-com:office:office" ` +
		`xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math"`
	relBase = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"
)

// doc is a Word document to build.
type doc struct {
	body      string            // the children of w:body
	styles    string            // the children of w:styles
	numbering string            // the children of w:numbering
	footnotes string            // the children of w:footnotes
	endnotes  string            // the children of w:endnotes
	links     map[string]string // relationship ID → external URL
	mainType  string            // the main part's content type; "" is a .docx
	extra     map[string]string // more parts, by name
}

func (d doc) parts() map[string]string {
	mt := d.mainType
	if mt == "" {
		mt = mainType
	}
	ct := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Override PartName="/word/document.xml" ContentType="` + mt + `"/></Types>`
	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="` + relBase + `officeDocument" Target="word/document.xml"/></Relationships>`
	var docRels strings.Builder
	docRels.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	p := map[string]string{
		"[Content_Types].xml": ct,
		"_rels/.rels":         rels,
		"word/document.xml":   `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document ` + nsW + `><w:body>` + d.body + `</w:body></w:document>`,
	}
	add := func(id, typ, target, body, root string) {
		if body == "" {
			return
		}
		fmt.Fprintf(&docRels, `<Relationship Id="%s" Type="%s%s" Target="%s"/>`, id, relBase, typ, target)
		p["word/"+target] = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:` + root + ` ` + nsW + `>` + body + `</w:` + root + `>`
	}
	add("rIdS", "styles", "styles.xml", d.styles, "styles")
	add("rIdN", "numbering", "numbering.xml", d.numbering, "numbering")
	add("rIdF", "footnotes", "footnotes.xml", d.footnotes, "footnotes")
	add("rIdE", "endnotes", "endnotes.xml", d.endnotes, "endnotes")
	ids := make([]string, 0, len(d.links))
	for id := range d.links {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		fmt.Fprintf(&docRels, `<Relationship Id="%s" Type="%shyperlink" Target="%s" TargetMode="External"/>`, id, relBase, d.links[id])
	}
	docRels.WriteString(`</Relationships>`)
	p["word/_rels/document.xml.rels"] = docRels.String()
	for k, v := range d.extra {
		p[k] = v
	}
	return p
}

func (d doc) zip(t testing.TB) []byte { return zipOf(t, d.parts()) }

// zipOf writes parts into a zip, in name order.
func zipOf(t testing.TB, parts map[string]string) []byte {
	t.Helper()
	names := make([]string, 0, len(parts))
	for n := range parts {
		names = append(names, n)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(parts[n])); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// p is a paragraph with props and runs of text.
func p(props string, runs ...string) string {
	var b strings.Builder
	b.WriteString("<w:p>")
	if props != "" {
		b.WriteString("<w:pPr>" + props + "</w:pPr>")
	}
	for _, r := range runs {
		b.WriteString(r)
	}
	b.WriteString("</w:p>")
	return b.String()
}

// r is a run of text.
func r(text string) string {
	return `<w:r><w:t xml:space="preserve">` + text + `</w:t></w:r>`
}

func pStyle(id string) string { return `<w:pStyle w:val="` + id + `"/>` }

func numPr(id string, ilvl int) string {
	return fmt.Sprintf(`<w:numPr><w:ilvl w:val="%d"/><w:numId w:val="%s"/></w:numPr>`, ilvl, id)
}

// styleDef is a paragraph style.
func styleDef(id, name, basedOn, ppr string) string {
	s := `<w:style w:type="paragraph" w:styleId="` + id + `"><w:name w:val="` + name + `"/>`
	if basedOn != "" {
		s += `<w:basedOn w:val="` + basedOn + `"/>`
	}
	if ppr != "" {
		s += `<w:pPr>` + ppr + `</w:pPr>`
	}
	return s + `</w:style>`
}

// tc is a table cell.
func tc(props string, paras ...string) string {
	s := "<w:tc>"
	if props != "" {
		s += "<w:tcPr>" + props + "</w:tcPr>"
	}
	if len(paras) == 0 {
		paras = []string{p("")}
	}
	return s + strings.Join(paras, "") + "</w:tc>"
}

func tr(props string, cells ...string) string {
	s := "<w:tr>"
	if props != "" {
		s += "<w:trPr>" + props + "</w:trPr>"
	}
	return s + strings.Join(cells, "") + "</w:tr>"
}

func tbl(rows ...string) string { return "<w:tbl><w:tblPr/>" + strings.Join(rows, "") + "</w:tbl>" }
