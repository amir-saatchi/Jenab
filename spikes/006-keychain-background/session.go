package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	wtsapi32     = windows.NewLazySystemDLL("wtsapi32.dll")
	wtsQuery     = wtsapi32.NewProc("WTSQuerySessionInformationW")
	wtsFree      = wtsapi32.NewProc("WTSFreeMemory")
	kernel32     = windows.NewLazySystemDLL("kernel32.dll")
	getConsoleWd = kernel32.NewProc("GetConsoleWindow")
)

// session returns the current session ID and whether its screen is locked.
// The lock screen is shown by LogonUI.exe in the user's session, so its
// presence is the lock test. The WTS SessionFlags value is added for the
// record: on this Windows 11 build it reads 0 ("locked" per the docs) while
// the session is unlocked, so it is not used.
func session() (uint32, string) {
	var id uint32
	windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &id)
	state := "unlocked"
	if logonUIRunning(id) {
		state = "locked"
	}
	return id, state + " (WTS flag " + wtsFlag(id) + ")"
}

func logonUIRunning(session uint32) bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), "LogonUI.exe") {
			var s uint32
			if windows.ProcessIdToSessionId(e.ProcessID, &s) == nil && s == session {
				return true
			}
		}
	}
	return false
}

func wtsFlag(id uint32) string {
	const wtsSessionInfoEx = 25
	var buf uintptr
	var n uint32
	r, _, _ := wtsQuery.Call(0, uintptr(id), wtsSessionInfoEx, uintptr(unsafe.Pointer(&buf)), uintptr(unsafe.Pointer(&n)))
	if r == 0 || buf == 0 {
		return "?"
	}
	defer wtsFree.Call(buf)
	// WTSINFOEXW: Level uint32, then WTSINFOEX_LEVEL1_W: SessionId uint32, SessionState int32, SessionFlags int32
	return fmt.Sprint(*(*int32)(unsafe.Pointer(buf + 12)))
}

func hasConsole() bool {
	r, _, _ := getConsoleWd.Call()
	return r != 0
}

func init() {
	// Started by explorer.exe, the child gets no arguments; its output path is in kc-bg.args.
	if len(os.Args) == 1 {
		exe, _ := os.Executable()
		if b, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "kc-bg.args")); err == nil {
			os.Args = append(os.Args, "child", string(b))
		}
	}
}
