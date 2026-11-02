// Package docxtest builds small Word documents for tests outside the
// docx package.
package docxtest

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"sort"
	"strings"
)

const mainType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"

// Doc is a .docx whose body has a paragraph for each line; a line that
// starts with "# " is a Heading 1.
func Doc(lines ...string) []byte {
	var body strings.Builder
	for _, l := range lines {
		body.WriteString("<w:p>")
		if h, ok := strings.CutPrefix(l, "# "); ok {
			body.WriteString(`<w:pPr><w:pStyle w:val="Heading1"/></w:pPr>`)
			l = h
		}
		body.WriteString(`<w:r><w:t xml:space="preserve">`)
		xml.EscapeText(&body, []byte(l))
		body.WriteString(`</w:t></w:r></w:p>`)
	}
	ns := `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"`
	return Zip(map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/word/document.xml" ContentType="` + mainType + `"/></Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/_rels/document.xml.rels": `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`,
		"word/styles.xml": `<?xml version="1.0" encoding="UTF-8"?><w:styles ` + ns + `>` +
			`<w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/></w:style></w:styles>`,
		"word/document.xml": `<?xml version="1.0" encoding="UTF-8"?><w:document ` + ns + `><w:body>` + body.String() + `</w:body></w:document>`,
	})
}

// Zip writes parts into a zip, in name order.
func Zip(parts map[string]string) []byte {
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
			panic(err)
		}
		if _, err := w.Write([]byte(parts[n])); err != nil {
			panic(err)
		}
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// OLE is an OLE compound file of size bytes: an old .doc, or with
// encrypted an Office file saved with a password, which has an
// EncryptedPackage stream.
func OLE(encrypted bool, size int) []byte {
	b := make([]byte, max(size, 1024))
	copy(b, "\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1")
	name := "WordDocument"
	if encrypted {
		name = "EncryptedPackage"
	}
	for i, c := range name { // the first directory entry, at 512
		b[512+2*i] = byte(c)
	}
	b[512+64], b[512+66] = byte(2*len(name)+2), 2
	return b
}
