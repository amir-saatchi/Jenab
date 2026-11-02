// Package docx reads Word documents (.docx) as Markdown text (SPEC 3.7,
// 4.2, TASK-021), in pure Go with archive/zip and encoding/xml. A file is
// untrusted input: it is never unpacked to disk, only the parts that hold
// text are read, and they are read with limits on the number of entries,
// the bytes unpacked and the packing ratio. Macros are never run.
package docx

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"path"
	"strings"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// Version is the extractor's version. The text cache is keyed by it
// (cache/text/<sha256>-v<n>.md), so a better extractor makes the text
// again.
const Version = 1

// The MIME types a Word document is stored as. A macro-enabled one keeps
// its own type, so it is read like any other but never opened with the
// system app (SPEC 4.7).
const (
	MIME      = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	MIMEMacro = "application/vnd.ms-word.document.macroEnabled.12"
)

// MIMEOLE is an OLE compound file: an old .doc, .xls or .ppt, or an
// Office file encrypted with a password, which is no longer a zip.
const MIMEOLE = "application/x-ole-storage"

// The content types of a WordprocessingML main part in
// [Content_Types].xml.
const (
	mainType      = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
	macroMainType = "application/vnd.ms-word.document.macroEnabled.main+xml"
)

// Limits for a file, which is untrusted (TASK-021).
const (
	MaxEntries  = 2000     // zip entries
	MaxUnpacked = 50 << 20 // bytes unpacked, over all parts read
	MaxRatio    = 100      // a part may unpack to this many times its packed size…
	ratioFloor  = 1 << 20  // …or to this many bytes, whichever is more
	maxDir      = 4 << 20  // bytes the zip reader reads to list the entries
	maxSmall    = 4 << 20  // bytes of [Content_Types].xml or a .rels part
	maxDepth    = 256      // XML nesting
)

// IsWord reports whether objects of type m are Word documents.
func IsWord(m string) bool {
	b := baseType(m)
	return b == MIME || b == MIMEMacro
}

func baseType(m string) string {
	if b, _, err := mime.ParseMediaType(m); err == nil {
		return b
	}
	return m
}

// The kinds of Error.
var (
	ErrEncrypted = errors.New("it is password-protected")
	ErrOldFormat = errors.New("it is an old Word, Excel or PowerPoint file (.doc, .xls or .ppt); only .docx can be read, so save it as .docx")
	ErrNotWord   = errors.New("it is not a Word document")
	ErrBroken    = errors.New("it is damaged")
	ErrTooLarge  = errors.New("it is too large to read")
)

// Error is why a file can't be read, said to the user and the model as
// the end of "could not read report.docx: …". errors.Is matches its kind.
type Error struct {
	Kind error
	Msg  string
}

func (e *Error) Error() string        { return e.Msg }
func (e *Error) Is(target error) bool { return target == e.Kind }

func broken(format string, a ...any) error {
	return &Error{Kind: ErrBroken, Msg: "it is damaged: " + fmt.Sprintf(format, a...)}
}

func tooLarge(format string, a ...any) error {
	return &Error{Kind: ErrTooLarge, Msg: fmt.Sprintf(format, a...)}
}

// Detect returns MIME or MIMEMacro when the zip in r is a Word document:
// its [Content_Types].xml names a WordprocessingML main part. It returns
// "" for any other zip, and for one that can't be read.
func Detect(r io.ReaderAt, size int64) string {
	p, err := openPackage(context.Background(), r, size)
	if err != nil {
		return ""
	}
	_, ct, err := p.mainPart()
	if err != nil {
		return ""
	}
	if strings.EqualFold(ct, macroMainType) {
		return MIMEMacro
	}
	return MIME
}

// Extract returns the text of the Word document in r as Markdown. A file
// that can't be read returns an *Error.
func Extract(ctx context.Context, r io.ReaderAt, size int64) (string, error) {
	if isOLE(r) {
		if Encrypted(r, size) {
			return "", &Error{Kind: ErrEncrypted, Msg: ErrEncrypted.Error()}
		}
		return "", &Error{Kind: ErrOldFormat, Msg: ErrOldFormat.Error()}
	}
	p, err := openPackage(ctx, r, size)
	if err != nil {
		return "", err
	}
	main, _, err := p.mainPart()
	if err != nil {
		return "", err
	}
	return p.extract(ctx, main)
}

// pkg is an open zip, as an OPC package of parts.
type pkg struct {
	ctx      context.Context
	files    map[string]*zip.File // by part name in lower case, without the leading "/"
	unpacked int64
}

// openPackage reads the zip's directory. The reader may read only maxDir
// bytes for it, so a directory of millions of tiny entries stops early
// instead of filling memory.
func openPackage(ctx context.Context, r io.ReaderAt, size int64) (*pkg, error) {
	if size < 22 {
		return nil, broken("it is not a zip")
	}
	b := &budgetReader{r: r, left: maxDir}
	zr, err := zip.NewReader(b, size)
	if errors.Is(err, errBudget) {
		return nil, tooLarge("it has more than %s parts", count(MaxEntries))
	}
	// Names that would leave a folder don't matter: nothing is unpacked.
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		return nil, broken("the zip can't be read")
	}
	b.left = -1 // the parts are limited by their unpacked size
	if len(zr.File) > MaxEntries {
		return nil, tooLarge("it has more than %s parts", count(MaxEntries))
	}
	p := &pkg{ctx: ctx, files: make(map[string]*zip.File, len(zr.File))}
	for _, f := range zr.File {
		name := strings.ToLower(strings.TrimPrefix(f.Name, "/"))
		if _, ok := p.files[name]; !ok {
			p.files[name] = f
		}
	}
	return p, nil
}

var errBudget = errors.New("docx: the zip directory is too large")

// budgetReader fails once it has read left bytes; left < 0 means no
// limit.
type budgetReader struct {
	r    io.ReaderAt
	left int64
}

func (b *budgetReader) ReadAt(p []byte, off int64) (int, error) {
	if b.left >= 0 {
		if int64(len(p)) > b.left {
			return 0, errBudget
		}
		b.left -= int64(len(p))
	}
	return b.r.ReadAt(p, off)
}

// open returns a part's unpacked bytes as a stream, limited by the
// package's budget and the part's ratio. ok is false when there is no such
// part.
func (p *pkg) open(name string) (rc io.ReadCloser, ok bool, err error) {
	f := p.files[strings.ToLower(strings.TrimPrefix(name, "/"))]
	if f == nil {
		return nil, false, nil
	}
	if f.Flags&0x1 != 0 {
		return nil, true, &Error{Kind: ErrEncrypted, Msg: ErrEncrypted.Error()}
	}
	if err := p.reserve(name); err != nil {
		return nil, true, err
	}
	z, err := f.Open()
	switch {
	case errors.Is(err, zip.ErrAlgorithm):
		return nil, true, broken("%s is packed in a way that can't be read", name)
	case err != nil:
		return nil, true, broken("%s can't be unpacked", name)
	}
	limit := p.limit(f)
	return &partReader{p: p, name: name, rc: z, left: limit}, true, nil
}

// limit is what a part may unpack to before the ratio stops it. The
// content types and relationships are small in any real file.
func (p *pkg) limit(f *zip.File) int64 {
	limit := int64(MaxUnpacked)
	if f.CompressedSize64 < uint64(MaxUnpacked) {
		limit = min(limit, max(int64(f.CompressedSize64)*MaxRatio, ratioFloor))
	}
	if n := strings.ToLower(f.Name); n == "[content_types].xml" || strings.HasSuffix(n, ".rels") {
		limit = min(limit, maxSmall)
	}
	return limit
}

// reserve fails fast when the parts named would unpack to more than the
// limits allow. archive/zip stops a part at the size its entry declares,
// so the declared sizes bound what is read.
func (p *pkg) reserve(names ...string) error {
	total := p.unpacked
	for _, name := range names {
		f := p.files[strings.ToLower(strings.TrimPrefix(name, "/"))]
		if f == nil {
			continue
		}
		if f.UncompressedSize64 > uint64(p.limit(f)) {
			return p.ratioError(name)
		}
		total += int64(f.UncompressedSize64)
		if total > MaxUnpacked {
			return tooLarge("it unpacks to more than %d MB", MaxUnpacked>>20)
		}
	}
	return nil
}

func (p *pkg) ratioError(name string) error {
	return tooLarge("%s unpacks to more than %d times its packed size, as a zip bomb does", name, MaxRatio)
}

// partReader counts what a part unpacks to.
type partReader struct {
	p    *pkg
	name string
	rc   io.ReadCloser
	left int64 // before the ratio stops it
}

func (r *partReader) Read(b []byte) (int, error) {
	if err := r.p.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.rc.Read(b)
	r.left -= int64(n)
	r.p.unpacked += int64(n)
	switch {
	case r.left < 0:
		return n, r.p.ratioError(r.name)
	case r.p.unpacked > MaxUnpacked:
		return n, tooLarge("it unpacks to more than %d MB", MaxUnpacked>>20)
	case err != nil && err != io.EOF:
		var de *Error
		if errors.As(err, &de) {
			return n, err
		}
		return n, broken("%s can't be unpacked", r.name)
	}
	return n, err
}

func (r *partReader) Close() error { return r.rc.Close() }

// decoder reads a part's XML. UTF-16 parts, which OPC allows, are turned
// into UTF-8 first.
func decoder(r io.Reader) *xml.Decoder {
	br := bufio.NewReader(r)
	head, _ := br.Peek(3)
	var src io.Reader = br
	switch {
	case bytes.HasPrefix(head, []byte{0xef, 0xbb, 0xbf}):
		br.Discard(3)
	case bytes.HasPrefix(head, []byte{0xff, 0xfe}), bytes.HasPrefix(head, []byte{0xfe, 0xff}):
		src = transform.NewReader(br, unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder())
	}
	d := xml.NewDecoder(src)
	d.CharsetReader = func(label string, in io.Reader) (io.Reader, error) {
		switch strings.ToLower(label) {
		case "utf-8", "utf8", "utf-16", "utf16":
			return in, nil // UTF-16 is decoded above
		}
		return nil, fmt.Errorf("the encoding %q is not supported", label)
	}
	return d
}

// readXML parses a whole part with fn, which reads tokens from t. A
// missing part is no error: fn is not called.
func (p *pkg) readXML(name string, fn func(t *tokens) error) (bool, error) {
	rc, ok, err := p.open(name)
	if !ok || err != nil {
		return ok, err
	}
	defer rc.Close()
	t := &tokens{d: decoder(rc), part: name}
	if err := fn(t); err != nil {
		return true, err
	}
	return true, nil
}

// tokens is a part's XML as a stream, with its depth bounded.
type tokens struct {
	d     *xml.Decoder
	part  string
	depth int
}

// next returns the next start, end or text token; nil at the end.
func (t *tokens) next() (xml.Token, error) {
	for {
		tok, err := t.d.RawToken()
		if err == io.EOF {
			return nil, nil
		}
		if err != nil {
			var de *Error
			if errors.As(err, &de) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			return nil, broken("%s is not valid XML", t.part)
		}
		switch v := tok.(type) {
		case xml.StartElement:
			t.depth++
			if t.depth > maxDepth {
				return nil, broken("%s is nested too deeply", t.part)
			}
			return v, nil
		case xml.EndElement:
			t.depth--
			return v, nil
		case xml.CharData:
			return v, nil
		}
	}
}

// skip reads up to the end of the element just started.
func (t *tokens) skip() error {
	for depth := 1; depth > 0; {
		tok, err := t.next()
		if err != nil {
			return err
		}
		switch tok.(type) {
		case nil:
			return broken("%s ends too early", t.part)
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return nil
}

// attr is the value of the attribute with local name n, whatever its
// prefix.
func attr(e xml.StartElement, n string) string {
	for _, a := range e.Attr {
		if a.Name.Local == n {
			return a.Value
		}
	}
	return ""
}

// hasAttr reports whether e has the attribute with local name n.
func hasAttr(e xml.StartElement, n string) bool {
	for _, a := range e.Attr {
		if a.Name.Local == n {
			return true
		}
	}
	return false
}

// mainPart finds the main part in [Content_Types].xml and returns its
// name and content type.
func (p *pkg) mainPart() (name, ctype string, err error) {
	defaults := map[string]string{}
	found, err := p.readXML("[Content_Types].xml", func(t *tokens) error {
		for {
			tok, err := t.next()
			if tok == nil || err != nil {
				return err
			}
			e, ok := tok.(xml.StartElement)
			if !ok {
				continue
			}
			ct := strings.ToLower(strings.TrimSpace(attr(e, "ContentType")))
			switch e.Name.Local {
			case "Override":
				if name == "" && isMainType(ct) {
					name, ctype = attr(e, "PartName"), ct
				}
			case "Default":
				defaults[strings.ToLower(attr(e, "Extension"))] = ct
			}
		}
	})
	if err != nil {
		return "", "", err
	}
	if !found {
		return "", "", &Error{Kind: ErrNotWord, Msg: ErrNotWord.Error()}
	}
	if name != "" {
		return strings.TrimPrefix(name, "/"), ctype, nil
	}
	// A main part typed only by its extension: the package's relationship
	// to its office document names it.
	rels, err := p.rels("")
	if err != nil {
		return "", "", err
	}
	for _, r := range rels {
		if strings.HasSuffix(r.typ, "/officeDocument") && !r.external {
			ct := defaults[strings.ToLower(strings.TrimPrefix(path.Ext(r.target), "."))]
			if isMainType(ct) {
				return r.target, ct, nil
			}
		}
	}
	return "", "", &Error{Kind: ErrNotWord, Msg: ErrNotWord.Error()}
}

// rel is one relationship of a part.
type rel struct {
	id, typ, target string
	external        bool
}

// rels reads the relationships of the part named source; "" is the
// package. Internal targets are resolved to part names.
func (p *pkg) rels(source string) ([]rel, error) {
	dir, file := path.Split(source)
	var out []rel
	_, err := p.readXML(dir+"_rels/"+file+".rels", func(t *tokens) error {
		for {
			tok, err := t.next()
			if tok == nil || err != nil {
				return err
			}
			e, ok := tok.(xml.StartElement)
			if !ok || e.Name.Local != "Relationship" {
				continue
			}
			r := rel{id: attr(e, "Id"), typ: attr(e, "Type"), target: attr(e, "Target"),
				external: strings.EqualFold(attr(e, "TargetMode"), "External")}
			if !r.external {
				if strings.HasPrefix(r.target, "/") {
					r.target = strings.TrimPrefix(path.Clean(r.target), "/")
				} else {
					r.target = strings.TrimPrefix(path.Join("/"+dir, r.target), "/")
				}
			}
			out = append(out, r)
		}
	})
	return out, err
}

// count writes n with thousands separators.
func count(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// isMainType reports whether a content type names a WordprocessingML main
// part; content types are not case-sensitive.
func isMainType(ct string) bool {
	return strings.EqualFold(ct, mainType) || strings.EqualFold(ct, macroMainType)
}

// ReaderAt returns r as an io.ReaderAt: r itself when it is one, such as
// an *os.File, else a reader that seeks before each read.
func ReaderAt(r io.ReadSeeker) io.ReaderAt {
	if ra, ok := r.(io.ReaderAt); ok {
		return ra
	}
	return &seekReaderAt{r: r}
}

type seekReaderAt struct{ r io.ReadSeeker }

func (s *seekReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if _, err := s.r.Seek(off, io.SeekStart); err != nil {
		return 0, err
	}
	n, err := io.ReadFull(s.r, p)
	if err == io.ErrUnexpectedEOF {
		err = io.EOF
	}
	return n, err
}
