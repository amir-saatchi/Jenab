package main

import (
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Three independent lock signals, because the WTS flag proved wrong:
//   - desk: the input desktop name, "Default" when unlocked, "Winlogon" (or no access) when locked
//   - lockapp: LockApp.exe (the lock screen) running in the session; it also stays resident while unlocked, so it is weak
//   - events: WM_WTSSESSION_CHANGE lock/unlock notifications (listenSessionEvents)

var (
	user32            = windows.NewLazySystemDLL("user32.dll")
	openInputDesktop  = user32.NewProc("OpenInputDesktop")
	closeDesktop      = user32.NewProc("CloseDesktop")
	getUserObjectInfo = user32.NewProc("GetUserObjectInformationW")
	registerClassEx   = user32.NewProc("RegisterClassExW")
	createWindowEx    = user32.NewProc("CreateWindowExW")
	defWindowProc     = user32.NewProc("DefWindowProcW")
	getMessage        = user32.NewProc("GetMessageW")
	dispatchMessage   = user32.NewProc("DispatchMessageW")
	wtsRegister       = wtsapi32.NewProc("WTSRegisterSessionNotification")
)

func inputDesktop() string {
	const desktopReadObjects = 0x0001
	h, _, err := openInputDesktop.Call(0, 0, desktopReadObjects)
	if h == 0 {
		return "no access (" + err.Error() + ")"
	}
	defer closeDesktop.Call(h)
	var buf [128]uint16
	var n uint32
	const uoiName = 2
	r, _, _ := getUserObjectInfo.Call(h, uoiName, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)*2), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		return "?"
	}
	return windows.UTF16ToString(buf[:])
}

func processRunning(name string, session uint32) bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), name) {
			var s uint32
			if windows.ProcessIdToSessionId(e.ProcessID, &s) == nil && s == session {
				return true
			}
		}
	}
	return false
}

// lockSignals returns the desktop and LockApp signals as one log field.
func lockSignals() string {
	var id uint32
	windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &id)
	lockApp := "no"
	if processRunning("LockApp.exe", id) {
		lockApp = "yes"
	}
	return "desk=" + inputDesktop() + " lockapp=" + lockApp
}

// listenSessionEvents calls f with "lock" or "unlock" on each session change.
func listenSessionEvents(f func(string)) {
	go func() {
		runtime.LockOSThread()
		const wmWTSSessionChange = 0x02B1
		proc := syscall.NewCallback(func(hwnd, msg, wparam, lparam uintptr) uintptr {
			if msg == wmWTSSessionChange {
				switch wparam {
				case 7:
					f("lock")
				case 8:
					f("unlock")
				}
			}
			r, _, _ := defWindowProc.Call(hwnd, msg, wparam, lparam)
			return r
		})
		className, _ := windows.UTF16PtrFromString("BurrowSpike006")
		type wndClassEx struct {
			Size, Style                uint32
			WndProc                    uintptr
			ClsExtra, WndExtra         int32
			Instance, Icon, Cursor, Bg uintptr
			MenuName, ClassName        *uint16
			IconSm                     uintptr
		}
		wc := wndClassEx{WndProc: proc, ClassName: className}
		wc.Size = uint32(unsafe.Sizeof(wc))
		registerClassEx.Call(uintptr(unsafe.Pointer(&wc)))
		// a hidden top-level window (never shown) to receive the session notifications
		hwnd, _, _ := createWindowEx.Call(0, uintptr(unsafe.Pointer(className)), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
		const notifyForThisSession = 0
		wtsRegister.Call(hwnd, notifyForThisSession)
		var msg [48]byte
		for {
			r, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0)
			if r == 0 || int32(r) == -1 {
				return
			}
			dispatchMessage.Call(uintptr(unsafe.Pointer(&msg[0])))
		}
	}()
}
