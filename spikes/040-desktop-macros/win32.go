package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	pPostMessageW        = user32.NewProc("PostMessageW")
	pSendMessageTimeoutW = user32.NewProc("SendMessageTimeoutW")
	pIsWindow            = user32.NewProc("IsWindow")
	pFindWindowExW       = user32.NewProc("FindWindowExW")
	pGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	pGetWindowThreadPID  = user32.NewProc("GetWindowThreadProcessId")
	pOpenProcess         = kernel32.NewProc("OpenProcess")
	pGetExitCodeProcess  = kernel32.NewProc("GetExitCodeProcess")
	pCloseHandle         = kernel32.NewProc("CloseHandle")
)

const (
	wmClose       = 0x0010
	wmGetText     = 0x000D
	wmGetTextLen  = 0x000E
	smtoAbortHung = 0x0002
)

// hwndOf turns agent-desktop's window id ("w-11406890") into the HWND it encodes.
func hwndOf(id string) uintptr {
	n, err := strconv.ParseUint(strings.TrimPrefix(id, "w-"), 10, 64)
	if err != nil {
		return 0
	}
	return uintptr(n)
}

func isWindow(h uintptr) bool { r, _, _ := pIsWindow.Call(h); return r != 0 }

func windowPID(h uintptr) uint32 {
	var pid uint32
	pGetWindowThreadPID.Call(h, uintptr(unsafe.Pointer(&pid)))
	return pid
}

// closeWindow asks one window to close and waits for it to go. It never kills a process.
func closeWindow(h uintptr, wait time.Duration) bool {
	if !isWindow(h) {
		return true
	}
	pPostMessageW.Call(h, wmClose, 0, 0)
	for end := time.Now().Add(wait); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		if !isWindow(h) {
			return true
		}
	}
	return false
}

func processAlive(pid int) bool {
	const queryLimited = 0x1000
	h, _, _ := pOpenProcess.Call(queryLimited, 0, uintptr(pid))
	if h == 0 {
		return false
	}
	defer pCloseHandle.Call(h)
	var code uint32
	pGetExitCodeProcess.Call(h, uintptr(unsafe.Pointer(&code)))
	return code == 259 // STILL_ACTIVE
}

// killTree ends a process the spike started, and its children, by PID.
func killTree(pid int) {
	if pid <= 0 || pid == os.Getpid() || !processAlive(pid) {
		return
	}
	exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}

// editText reads the text of the first Edit child of a window (works across processes).
func editText(top uintptr) (string, error) {
	cls, _ := syscall.UTF16PtrFromString("Edit")
	h, _, _ := pFindWindowExW.Call(top, 0, uintptr(unsafe.Pointer(cls)), 0)
	if h == 0 {
		return "", fmt.Errorf("no Edit child")
	}
	var n uintptr
	pSendMessageTimeoutW.Call(h, wmGetTextLen, 0, 0, smtoAbortHung, 2000, uintptr(unsafe.Pointer(&n)))
	buf := make([]uint16, n+1)
	var got uintptr
	pSendMessageTimeoutW.Call(h, wmGetText, n+1, uintptr(unsafe.Pointer(&buf[0])), smtoAbortHung, 2000,
		uintptr(unsafe.Pointer(&got)))
	return syscall.UTF16ToString(buf), nil
}

func foregroundWindow() uintptr { r, _, _ := pGetForegroundWindow.Call(); return r }
