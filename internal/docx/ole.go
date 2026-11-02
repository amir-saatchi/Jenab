package docx

import (
	"bytes"
	"io"
	"unicode/utf16"
)

// oleMagic starts an OLE compound file.
var oleMagic = []byte("\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1")

func isOLE(r io.ReaderAt) bool {
	head := make([]byte, len(oleMagic))
	_, err := r.ReadAt(head, 0)
	return err == nil && bytes.Equal(head, oleMagic)
}

// encryptedEntry is the directory entry of the stream that holds an
// encrypted Office file: its UTF-16 name, padded to 64 bytes, the name's
// length in bytes with the terminator, and the type 2, a stream.
var encryptedEntry = func() []byte {
	b := make([]byte, 67)
	for i, c := range utf16.Encode([]rune("EncryptedPackage")) {
		b[2*i], b[2*i+1] = byte(c), byte(c>>8)
	}
	b[64], b[66] = 34, 2
	return b
}()

// Encrypted reports whether r is an OLE compound file that holds an
// encrypted Office file. Directory entries are 128 bytes and sectors a
// multiple of that, so the entry is looked for at each 128-byte step.
func Encrypted(r io.ReaderAt, size int64) bool {
	if !isOLE(r) {
		return false
	}
	buf := make([]byte, 1<<20)
	for off := int64(512); off < size; off += int64(len(buf)) {
		n, err := r.ReadAt(buf, off)
		for i := 0; i+len(encryptedEntry) <= n; i += 128 {
			if bytes.Equal(buf[i:i+len(encryptedEntry)], encryptedEntry) {
				return true
			}
		}
		if err != nil {
			return false
		}
	}
	return false
}
