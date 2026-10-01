// other.exe: a separate program of the same user that reads a Generic
// credential with CredReadW directly, to show there is no per-app protection.
package main

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

func main() {
	advapi := windows.NewLazySystemDLL("advapi32.dll")
	credRead := advapi.NewProc("CredReadW")
	credFree := advapi.NewProc("CredFree")
	name, _ := windows.UTF16PtrFromString(os.Args[1])
	var c *credential
	const credTypeGeneric = 1
	r, _, err := credRead.Call(uintptr(unsafe.Pointer(name)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&c)))
	if r == 0 {
		fmt.Println("read failed:", err)
		os.Exit(1)
	}
	defer credFree.Call(uintptr(unsafe.Pointer(c)))
	blob := unsafe.Slice(c.CredentialBlob, c.CredentialBlobSize)
	shown := string(blob)
	if len(shown) > 12 {
		shown = shown[:12] + "…"
	}
	fmt.Printf("read the value (%d bytes, starts with %q), persist mode %d, no prompt.\n", len(blob), shown, c.Persist)
}
