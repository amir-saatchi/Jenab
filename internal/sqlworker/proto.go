package sqlworker

import (
	"bufio"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/amir-saatchi/jenab/internal/sqlguard"
)

// The pipe protocol. The pool writes one request frame to the worker's
// stdin and reads one response frame from its stdout, then the next. A
// frame is a 4-byte little-endian length and that many bytes; neither
// side reads a frame over maxFrame, so a worker that misbehaves can't make
// the app allocate more.
//
//	request:  max, query, args (name, value)…
//	response: 0, columns, rows, total, flags | 1, layer, reason, pos, line, col | 2, code, message
//
// Numbers are uvarints, strings and blobs a uvarint length and the bytes,
// and a value a tag and its data.

// maxFrame is the largest frame: the rows (ResultCap) with room for the
// column names and the tags.
const maxFrame = 2 * ResultCap

const (
	tNil byte = iota
	tInt
	tFloat
	tText
	tBlob
	tTime
	tBool // in arguments only
)

const (
	respRows byte = iota
	respReject
	respError
)

const (
	flagCut byte = 1 << iota
	flagCapped
	flagFull
)

func readFrame(r *bufio.Reader) ([]byte, error) {
	var h [4]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return nil, err
	}
	n := binary.LittleEndian.Uint32(h[:])
	if n > maxFrame {
		return nil, fmt.Errorf("sqlworker: a frame of %d bytes, at most %d are allowed", n, maxFrame)
	}
	b := make([]byte, n)
	_, err := io.ReadFull(r, b)
	return b, err
}

// enc builds a frame; its first 4 bytes are the length, set by frame.
type enc struct{ b []byte }

func newEnc() *enc { return &enc{b: make([]byte, 4, 256)} }

func (e *enc) byte(c byte)      { e.b = append(e.b, c) }
func (e *enc) uvarint(v uint64) { e.b = binary.AppendUvarint(e.b, v) }
func (e *enc) varint(v int64)   { e.b = binary.AppendVarint(e.b, v) }
func (e *enc) bytes(s []byte)   { e.uvarint(uint64(len(s))); e.b = append(e.b, s...) }
func (e *enc) str(s string)     { e.uvarint(uint64(len(s))); e.b = append(e.b, s...) }
func (e *enc) u64(v uint64)     { e.b = binary.LittleEndian.AppendUint64(e.b, v) }
func (e *enc) frame() ([]byte, error) {
	n := len(e.b) - 4
	if n > maxFrame {
		return nil, fmt.Errorf("%d MiB to send; at most %d MiB are allowed", n>>20, maxFrame>>20)
	}
	binary.LittleEndian.PutUint32(e.b, uint32(n))
	return e.b, nil
}

// value writes a driver value, as the driver gives it or as
// driver.DefaultParameterConverter makes it.
func (e *enc) value(v any) error {
	switch v := v.(type) {
	case nil:
		e.byte(tNil)
	case int64:
		e.byte(tInt)
		e.u64(uint64(v))
	case float64:
		e.byte(tFloat)
		e.u64(math.Float64bits(v))
	case string:
		e.byte(tText)
		e.str(v)
	case []byte:
		e.byte(tBlob)
		e.bytes(v)
	case time.Time:
		b, err := v.MarshalBinary()
		if err != nil {
			return err
		}
		e.byte(tTime)
		e.bytes(b)
	case bool:
		e.byte(tBool)
		if v {
			e.byte(1)
		} else {
			e.byte(0)
		}
	default:
		return fmt.Errorf("unsupported type %T", v)
	}
	return nil
}

// dec reads a frame. The first error sticks, and every read after it
// gives the zero value.
type dec struct {
	b   []byte
	err error
}

var errShort = errors.New("sqlworker: a frame ends early")

func (d *dec) fail() {
	if d.err == nil {
		d.err = errShort
	}
	d.b = nil
}

func (d *dec) byte() byte {
	if len(d.b) < 1 {
		d.fail()
		return 0
	}
	c := d.b[0]
	d.b = d.b[1:]
	return c
}

func (d *dec) uvarint() uint64 {
	v, n := binary.Uvarint(d.b)
	if n <= 0 {
		d.fail()
		return 0
	}
	d.b = d.b[n:]
	return v
}

func (d *dec) varint() int64 {
	v, n := binary.Varint(d.b)
	if n <= 0 {
		d.fail()
		return 0
	}
	d.b = d.b[n:]
	return v
}

// count reads a number of things that each take at least one byte, so a
// count the frame can't hold is an error rather than a huge allocation.
func (d *dec) count() int {
	n := d.uvarint()
	if n > uint64(len(d.b)) {
		d.fail()
		return 0
	}
	return int(n)
}

func (d *dec) raw() []byte {
	n := d.count()
	if d.err != nil {
		return nil
	}
	s := d.b[:n:n]
	d.b = d.b[n:]
	return s
}

func (d *dec) str() string { return string(d.raw()) }

func (d *dec) u64() uint64 {
	if len(d.b) < 8 {
		d.fail()
		return 0
	}
	v := binary.LittleEndian.Uint64(d.b)
	d.b = d.b[8:]
	return v
}

func (d *dec) value() any {
	switch d.byte() {
	case tNil:
		return nil
	case tInt:
		return int64(d.u64())
	case tFloat:
		return math.Float64frombits(d.u64())
	case tText:
		return d.str()
	case tBlob:
		return append([]byte{}, d.raw()...)
	case tTime:
		var t time.Time
		if err := t.UnmarshalBinary(d.raw()); err != nil && d.err == nil {
			d.err = err
		}
		return t
	case tBool:
		return d.byte() != 0
	}
	if d.err == nil {
		d.err = errors.New("sqlworker: unknown value tag")
	}
	return nil
}

// encodeRequest checks and converts the arguments the way database/sql
// would, so a bad one fails here rather than in the worker. They bind in
// the worker as they would here: a nil []byte as NULL, and a time.Time as
// the text the driver makes of it (time.Time.String), so its zone's name
// stays.
func encodeRequest(r Request) ([]byte, error) {
	e := newEnc()
	e.uvarint(uint64(max(r.Max, 1)))
	e.str(r.Query)
	e.uvarint(uint64(len(r.Args)))
	for i, a := range r.Args {
		name := ""
		if na, ok := a.(sql.NamedArg); ok {
			name, a = na.Name, na.Value
		}
		v, err := driver.DefaultParameterConverter.ConvertValue(a)
		if err != nil {
			return nil, fmt.Errorf("sql: converting argument $%d type: %w", i+1, err)
		}
		switch x := v.(type) {
		case []byte:
			if x == nil {
				v = nil
			}
		case time.Time:
			v = x.String()
		}
		e.str(name)
		if err := e.value(v); err != nil {
			return nil, fmt.Errorf("sql: converting argument $%d type: %w", i+1, err)
		}
	}
	b, err := e.frame()
	if err != nil {
		return nil, fmt.Errorf("the query and its arguments are too large: %w", err)
	}
	return b, nil
}

func decodeRequest(b []byte) (Request, error) {
	d := &dec{b: b}
	r := Request{Max: int(min(d.uvarint(), math.MaxInt32)), Query: d.str()}
	n := d.count()
	for range n {
		name, v := d.str(), d.value()
		if name != "" {
			r.Args = append(r.Args, sql.Named(name, v))
		} else {
			r.Args = append(r.Args, v)
		}
	}
	return r, d.err
}

// encodeResponse writes the worker's answer: the rows, a refusal, or
// SQLite's error with its code.
func encodeResponse(r Result, err error) []byte {
	e := newEnc()
	var re *sqlguard.RejectError
	switch {
	case errors.As(err, &re):
		e.byte(respReject)
		e.uvarint(uint64(re.Layer))
		e.str(re.Reason)
		e.varint(int64(re.Pos))
		e.uvarint(uint64(re.Line))
		e.uvarint(uint64(re.Col))
	case err != nil:
		e.byte(respError)
		e.uvarint(uint64(errorCode(err)))
		e.str(err.Error())
	default:
		e.byte(respRows)
		e.uvarint(uint64(len(r.Columns)))
		for _, c := range r.Columns {
			e.str(c)
		}
		e.uvarint(uint64(len(r.Values)))
		for _, row := range r.Values {
			for _, v := range row {
				if err := e.value(v); err != nil {
					return encodeResponse(Result{}, err)
				}
			}
		}
		e.uvarint(uint64(r.Total))
		var f byte
		for _, x := range []struct {
			on bool
			f  byte
		}{{r.Cut, flagCut}, {r.Capped, flagCapped}, {r.Full, flagFull}} {
			if x.on {
				f |= x.f
			}
		}
		e.byte(f)
	}
	b, ferr := e.frame()
	if ferr != nil {
		return encodeResponse(Result{}, fmt.Errorf("the result is too large: %w", ferr))
	}
	return b
}

func isEmptyBlob(v any) bool {
	b, ok := v.([]byte)
	return ok && len(b) == 0
}

// workerError is an error from SQLite in the worker, with its result
// code.
type workerError struct {
	code int
	msg  string
}

func (e *workerError) Error() string { return e.msg }

func decodeResponse(b []byte) (Result, error) {
	d := &dec{b: b}
	var r Result
	switch d.byte() {
	case respReject:
		re := &sqlguard.RejectError{Layer: int(d.uvarint()), Reason: d.str(), Pos: int(d.varint()), Line: int(d.uvarint()), Col: int(d.uvarint())}
		if d.err != nil {
			return r, d.err
		}
		return r, re
	case respError:
		we := &workerError{code: int(d.uvarint()), msg: d.str()}
		if d.err != nil {
			return r, d.err
		}
		return r, we
	case respRows:
	default:
		if d.err == nil {
			return r, errors.New("sqlworker: unknown response")
		}
		return r, d.err
	}
	// A NULL takes 1 byte in the frame and 16 in memory, so the counts are
	// held to what a worker sends (ResultCap) before anything is made.
	nc := d.count()
	if nc*16 > maxFrame {
		d.fail()
		nc = 0
	}
	r.Columns = make([]string, nc)
	for i := range r.Columns {
		r.Columns[i] = d.str()
	}
	nr := d.count()
	if nc == 0 && nr > 0 || uint64(nr)*uint64(24+nc*16) > maxFrame {
		d.fail()
		nr = 0
	}
	if nr > 0 {
		r.Values = make([][]any, 0, nr)
	}
	for range nr {
		if d.err != nil {
			break
		}
		row := make([]any, nc)
		for i := range row {
			// The driver gives a zero-length blob as a nil []byte.
			if row[i] = d.value(); isEmptyBlob(row[i]) {
				row[i] = []byte(nil)
			}
		}
		r.Values = append(r.Values, row)
	}
	r.Total = int(min(d.uvarint(), math.MaxInt32))
	f := d.byte()
	r.Cut, r.Capped, r.Full = f&flagCut != 0, f&flagCapped != 0, f&flagFull != 0
	if d.err != nil {
		return Result{}, d.err
	}
	return r, nil
}
