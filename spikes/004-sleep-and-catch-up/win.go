package main

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	k32       = windows.NewLazySystemDLL("kernel32.dll")
	kbase     = windows.NewLazySystemDLL("kernelbase.dll")
	user32    = windows.NewLazySystemDLL("user32.dll")
	wtsapi32  = windows.NewLazySystemDLL("wtsapi32.dll")
	powrprof  = windows.NewLazySystemDLL("powrprof.dll")
	pIntTime  = pick("QueryInterruptTime")
	pUIntTime = pick("QueryUnbiasedInterruptTime")
	pTick64   = k32.NewProc("GetTickCount64")

	openInputDesktop  = user32.NewProc("OpenInputDesktop")
	closeDesktop      = user32.NewProc("CloseDesktop")
	getUserObjectInfo = user32.NewProc("GetUserObjectInformationW")
	registerClassEx   = user32.NewProc("RegisterClassExW")
	createWindowEx    = user32.NewProc("CreateWindowExW")
	defWindowProc     = user32.NewProc("DefWindowProcW")
	getMessage        = user32.NewProc("GetMessageW")
	dispatchMessage   = user32.NewProc("DispatchMessageW")
	regPowerSetting   = user32.NewProc("RegisterPowerSettingNotification")
	wtsRegister       = wtsapi32.NewProc("WTSRegisterSessionNotification")
	powerRegSR        = powrprof.NewProc("PowerRegisterSuspendResumeNotification")
)

// pick finds a kernel32 function, falling back to kernelbase.dll.
func pick(name string) *windows.LazyProc {
	p := k32.NewProc(name)
	if p.Find() != nil {
		p = kbase.NewProc(name)
	}
	return p
}

// clocks holds one reading of every clock, taken together.
type clocks struct {
	qpc       int64
	it, uit   uint64 // interrupt time, biased (counts sleep) and unbiased (doesn't), in 100 ns units
	tickMilli uint64
}

func readClocks() clocks {
	var c clocks
	qpc.Call(uintptr(unsafe.Pointer(&c.qpc)))
	pIntTime.Call(uintptr(unsafe.Pointer(&c.it)))
	pUIntTime.Call(uintptr(unsafe.Pointer(&c.uit)))
	r, _, _ := pTick64.Call()
	c.tickMilli = uint64(r)
	return c
}

// inputDesktop returns the input desktop name: "Default" when unlocked,
// "Winlogon" (or "no access ...") while the lock screen is up. From SPIKE-006.
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

const (
	wmQueryEndSession  = 0x0011
	wmEndSession       = 0x0016
	wmPowerBroadcast   = 0x0218
	wmWTSSessionChange = 0x02B1

	pbtAPMPowerStatusChange = 10
	pbtAPMResumeAutomatic   = 18
	pbtAPMResumeSuspend     = 7
	pbtAPMSuspend           = 4
	pbtPowerSettingChange   = 0x8013
)

func pbtName(code uintptr) string {
	switch code {
	case pbtAPMSuspend:
		return "PBT_APMSUSPEND"
	case pbtAPMResumeSuspend:
		return "PBT_APMRESUMESUSPEND"
	case pbtAPMResumeAutomatic:
		return "PBT_APMRESUMEAUTOMATIC"
	case pbtAPMPowerStatusChange:
		return "PBT_APMPOWERSTATUSCHANGE"
	case pbtPowerSettingChange:
		return "PBT_POWERSETTINGCHANGE"
	}
	return fmt.Sprintf("PBT_0x%X", code)
}

func wtsName(code uintptr) string {
	names := map[uintptr]string{1: "console_connect", 2: "console_disconnect", 3: "remote_connect", 4: "remote_disconnect",
		5: "logon", 6: "logoff", 7: "lock", 8: "unlock", 9: "remote_control", 10: "create", 11: "terminate"}
	if n, ok := names[code]; ok {
		return n
	}
	return fmt.Sprintf("wts_%d", code)
}

// powerSettings are the GUIDs registered with RegisterPowerSettingNotification.
// The last two are extras, not in the ticket list.
var powerSettings = []struct{ guid, name string }{
	{"{6FE69556-704A-47A0-8F24-C28D936FDA47}", "GUID_CONSOLE_DISPLAY_STATE"},
	{"{02731015-4510-4526-99E6-E5A17EBD1AEA}", "GUID_MONITOR_POWER_ON"},
	{"{BA3E0F4D-B817-4094-A2D1-D56379E6A0F3}", "GUID_LIDSWITCH_STATE_CHANGE"},
	{"{98A7F580-01F7-48AA-9C0F-44352C29E5C0}", "GUID_SYSTEM_AWAYMODE"},
	{"{5D3E9A59-E9D5-4B00-A6BD-FF34FF516548}", "GUID_ACDC_POWER_SOURCE"},
	{"{2B84C20E-AD23-4DDF-93DB-05FFBD7EFCA5}", "GUID_SESSION_DISPLAY_STATUS"},
	{"{3C0F4548-C03F-4C4D-B9F2-237EDE686376}", "GUID_SESSION_USER_PRESENCE"},
}

// settingValue turns a power setting value into words.
func settingValue(name string, v uint32) string {
	var m map[uint32]string
	switch name {
	case "GUID_CONSOLE_DISPLAY_STATE", "GUID_SESSION_DISPLAY_STATUS":
		m = map[uint32]string{0: "off", 1: "on", 2: "dimmed"}
	case "GUID_MONITOR_POWER_ON":
		m = map[uint32]string{0: "off", 1: "on"}
	case "GUID_LIDSWITCH_STATE_CHANGE":
		m = map[uint32]string{0: "closed", 1: "open"}
	case "GUID_SYSTEM_AWAYMODE":
		m = map[uint32]string{0: "exit", 1: "enter"}
	case "GUID_ACDC_POWER_SOURCE":
		m = map[uint32]string{0: "AC", 1: "battery", 2: "short-term (UPS)"}
	case "GUID_SESSION_USER_PRESENCE":
		m = map[uint32]string{0: "present", 2: "not present"}
	}
	if s, ok := m[v]; ok {
		return fmt.Sprintf("%s (%d)", s, v)
	}
	return fmt.Sprint(v)
}

type powerBroadcastSetting struct {
	PowerSetting windows.GUID
	DataLength   uint32
	Data         [4]byte
}

// winHandlers receive OS notifications. power and session run on the window
// thread; powerCallback runs on a system thread.
type winHandlers struct {
	power         func(code uintptr)
	setting       func(name string, value uint32)
	session       func(code, sessionID uintptr)
	endSession    func(msg string)
	powerCallback func(code uintptr)
}

// startWindow creates a hidden top-level window (never shown, like Wails'
// "__wails_hidden_mainthread" window, which is also top-level and so gets
// broadcasts; a message-only window would not), registers it for power
// settings and session changes, and pumps its messages. It returns a
// description of each registration.
func startWindow(h winHandlers) []string {
	res := make(chan []string, 1)
	go func() {
		runtime.LockOSThread()
		var notes []string
		guidName := map[windows.GUID]string{}
		proc := syscall.NewCallback(func(hwnd, msg, wparam, lparam uintptr) uintptr {
			switch msg {
			case wmPowerBroadcast:
				if wparam == pbtPowerSettingChange && lparam != 0 {
					s := (*powerBroadcastSetting)(unsafe.Pointer(lparam))
					var v uint32
					switch {
					case s.DataLength >= 4:
						v = *(*uint32)(unsafe.Pointer(&s.Data[0]))
					case s.DataLength >= 1:
						v = uint32(s.Data[0])
					}
					name, ok := guidName[s.PowerSetting]
					if !ok {
						name = s.PowerSetting.String()
					}
					h.setting(name, v)
				} else {
					h.power(wparam)
				}
				return 1
			case wmWTSSessionChange:
				h.session(wparam, lparam)
			case wmQueryEndSession:
				h.endSession("WM_QUERYENDSESSION")
			case wmEndSession:
				h.endSession(fmt.Sprintf("WM_ENDSESSION wparam=%d", wparam))
			}
			r, _, _ := defWindowProc.Call(hwnd, msg, wparam, lparam)
			return r
		})
		className, _ := windows.UTF16PtrFromString("BurrowSpike004")
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
		hwnd, _, err := createWindowEx.Call(0, uintptr(unsafe.Pointer(className)), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
		if hwnd == 0 {
			res <- []string{"CreateWindowEx failed: " + err.Error()}
			return
		}
		for _, ps := range powerSettings {
			g, _ := windows.GUIDFromString(ps.guid)
			guidName[g] = ps.name
			const deviceNotifyWindowHandle = 0
			r, _, err := regPowerSetting.Call(hwnd, uintptr(unsafe.Pointer(&g)), deviceNotifyWindowHandle)
			if r == 0 {
				notes = append(notes, "RegisterPowerSettingNotification "+ps.name+": "+err.Error())
			} else {
				notes = append(notes, "RegisterPowerSettingNotification "+ps.name+": ok")
			}
		}
		const notifyForThisSession = 0
		if r, _, err := wtsRegister.Call(hwnd, notifyForThisSession); r == 0 {
			notes = append(notes, "WTSRegisterSessionNotification: "+err.Error())
		} else {
			notes = append(notes, "WTSRegisterSessionNotification: ok")
		}
		res <- notes
		var msg [48]byte
		for {
			r, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0)
			if r == 0 || int32(r) == -1 {
				return
			}
			dispatchMessage.Call(uintptr(unsafe.Pointer(&msg[0])))
		}
	}()
	return <-res
}

type deviceNotifySubscribeParameters struct {
	Callback uintptr
	Context  uintptr
}

var (
	srParams deviceNotifySubscribeParameters // kept global: Windows holds the pointer
	srHandle uintptr
)

// registerSuspendResume registers a DEVICE_NOTIFY_CALLBACK with
// PowerRegisterSuspendResumeNotification (the API Microsoft recommends for
// Modern Standby; the Go runtime uses it too). No window is needed.
func registerSuspendResume(f func(code uintptr)) string {
	if err := powerRegSR.Find(); err != nil {
		return "PowerRegisterSuspendResumeNotification: " + err.Error()
	}
	srParams.Callback = syscall.NewCallback(func(context, typ, setting uintptr) uintptr {
		f(typ)
		return 0
	})
	const deviceNotifyCallback = 2
	r, _, _ := powerRegSR.Call(deviceNotifyCallback, uintptr(unsafe.Pointer(&srParams)), uintptr(unsafe.Pointer(&srHandle)))
	if r != 0 {
		return fmt.Sprintf("PowerRegisterSuspendResumeNotification: error %d", r)
	}
	return "PowerRegisterSuspendResumeNotification: ok"
}
