package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
	"unsafe"
)

// The test editor stands in for Notepad, which on this machine would reopen the user's saved
// tabs. It is a plain Win32 window: a menu bar, a multi-line Edit box, two buttons and a
// character count. -freeze-after/-freeze-for block its UI thread once, for the hang test.

var (
	pRegisterClassExW = user32.NewProc("RegisterClassExW")
	pCreateWindowExW  = user32.NewProc("CreateWindowExW")
	pDefWindowProcW   = user32.NewProc("DefWindowProcW")
	pGetMessageW      = user32.NewProc("GetMessageW")
	pTranslateMessage = user32.NewProc("TranslateMessage")
	pDispatchMessageW = user32.NewProc("DispatchMessageW")
	pPostQuitMessage  = user32.NewProc("PostQuitMessage")
	pCreateMenu       = user32.NewProc("CreateMenu")
	pAppendMenuW      = user32.NewProc("AppendMenuW")
	pSetMenu          = user32.NewProc("SetMenu")
	pSendMessageW     = user32.NewProc("SendMessageW")
	pSetWindowTextW   = user32.NewProc("SetWindowTextW")
	pGetWindowTextW   = user32.NewProc("GetWindowTextW")
	pGetWindowTextLen = user32.NewProc("GetWindowTextLengthW")
	pMoveWindow       = user32.NewProc("MoveWindow")
	pSetTimer         = user32.NewProc("SetTimer")
	pKillTimer        = user32.NewProc("KillTimer")
	pLoadCursorW      = user32.NewProc("LoadCursorW")
	pGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

type wndClassEx struct {
	Size, Style                   uint32
	WndProc                       uintptr
	ClsExtra, WndExtra            int32
	Instance, Icon, Cursor, Brush uintptr
	MenuName, ClassName           *uint16
	IconSm                        uintptr
}

type winMsg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      [2]int32
	_       uint32
}

const (
	wsOverlappedWindow = 0x00CF0000
	wsVisible          = 0x10000000
	wsChild            = 0x40000000
	wsBorder           = 0x00800000
	wsVScroll          = 0x00200000
	wsTabStop          = 0x00010000
	esMultiline        = 0x0004
	esAutoVScroll      = 0x0040
	esWantReturn       = 0x1000
	wmCreate           = 0x0001
	wmDestroy          = 0x0002
	wmSize             = 0x0005
	wmCommand          = 0x0111
	wmTimer            = 0x0113
	enChange           = 0x0300
	emSetSel           = 0x00B1

	idNew, idExit, idSelectAll = 101, 102, 201
	idEdit, idClear, idUpper   = 1001, 1002, 1003
	idCount                    = 1004
)

type editorWin struct {
	edit, count, clear, upper uintptr
	freezeAfter, freezeFor    time.Duration
}

var ed editorWin

func u16(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }

func child(parent uintptr, class, text string, style uintptr, id int) uintptr {
	h, _, _ := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(u16(class))), uintptr(unsafe.Pointer(u16(text))),
		wsChild|wsVisible|style, 0, 0, 10, 10, parent, uintptr(id), 0, 0)
	return h
}

func windowText(h uintptr) string {
	n, _, _ := pGetWindowTextLen.Call(h)
	buf := make([]uint16, n+1)
	pGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}

func setText(h uintptr, s string) { pSetWindowTextW.Call(h, uintptr(unsafe.Pointer(u16(s)))) }

func editorProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case wmCreate:
		file, _, _ := pCreateMenu.Call()
		pAppendMenuW.Call(file, 0, idNew, uintptr(unsafe.Pointer(u16("&New"))))
		pAppendMenuW.Call(file, 0, idExit, uintptr(unsafe.Pointer(u16("E&xit"))))
		edit, _, _ := pCreateMenu.Call()
		pAppendMenuW.Call(edit, 0, idSelectAll, uintptr(unsafe.Pointer(u16("Select &all"))))
		bar, _, _ := pCreateMenu.Call()
		pAppendMenuW.Call(bar, 0x10, file, uintptr(unsafe.Pointer(u16("&File")))) // MF_POPUP
		pAppendMenuW.Call(bar, 0x10, edit, uintptr(unsafe.Pointer(u16("&Edit"))))
		pSetMenu.Call(hwnd, bar)
		ed.edit = child(hwnd, "Edit", "", wsBorder|wsVScroll|wsTabStop|esMultiline|esAutoVScroll|esWantReturn, idEdit)
		ed.clear = child(hwnd, "Button", "Clear", wsTabStop, idClear)
		ed.upper = child(hwnd, "Button", "Upper case", wsTabStop, idUpper)
		ed.count = child(hwnd, "Static", "0 characters", 0, idCount)
		if ed.freezeAfter > 0 {
			pSetTimer.Call(hwnd, 1, uintptr(ed.freezeAfter.Milliseconds()), 0)
		}
		return 0
	case wmSize:
		w, h := int(lp&0xFFFF), int((lp>>16)&0xFFFF)
		pMoveWindow.Call(ed.edit, 8, 8, uintptr(w-16), uintptr(max(h-56, 20)), 1)
		pMoveWindow.Call(ed.clear, 8, uintptr(h-40), 90, 30, 1)
		pMoveWindow.Call(ed.upper, 106, uintptr(h-40), 110, 30, 1)
		pMoveWindow.Call(ed.count, 232, uintptr(h-34), 200, 24, 1)
		return 0
	case wmCommand:
		id, code := int(wp&0xFFFF), int((wp>>16)&0xFFFF)
		switch {
		case id == idEdit && code == enChange:
			setText(ed.count, fmt.Sprintf("%d characters", utf8.RuneCountInString(windowText(ed.edit))))
		case id == idClear || id == idNew:
			setText(ed.edit, "")
		case id == idUpper:
			setText(ed.edit, strings.ToUpper(windowText(ed.edit)))
		case id == idSelectAll:
			pSendMessageW.Call(ed.edit, emSetSel, 0, ^uintptr(0))
		case id == idExit:
			pPostMessageW.Call(hwnd, wmClose, 0, 0)
		}
		return 0
	case wmTimer:
		pKillTimer.Call(hwnd, 1)
		time.Sleep(ed.freezeFor) // the UI thread stops answering, like a hung app
		return 0
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

func runEditor(args []string) {
	fs := flag.NewFlagSet("editor", flag.ExitOnError)
	title := fs.String("title", "SPIKE-040 Test Editor", "window title")
	fs.DurationVar(&ed.freezeAfter, "freeze-after", 0, "block the UI thread after this long (0: never)")
	fs.DurationVar(&ed.freezeFor, "freeze-for", 0, "how long the UI thread stays blocked")
	fs.Parse(args)
	runtime.LockOSThread()
	inst, _, _ := pGetModuleHandleW.Call(0)
	cursor, _, _ := pLoadCursorW.Call(0, 32512) // IDC_ARROW
	wc := wndClassEx{WndProc: syscall.NewCallback(editorProc), Instance: inst, Cursor: cursor,
		Brush: 16, ClassName: u16("Spike040Editor")} // COLOR_BTNFACE+1
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		fmt.Fprintln(os.Stderr, "RegisterClassExW:", err)
		os.Exit(1)
	}
	h, _, err := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(wc.ClassName)), uintptr(unsafe.Pointer(u16(*title))),
		wsOverlappedWindow|wsVisible, 200, 200, 640, 420, 0, 0, inst, 0)
	if h == 0 {
		fmt.Fprintln(os.Stderr, "CreateWindowExW:", err)
		os.Exit(1)
	}
	var m winMsg
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}
