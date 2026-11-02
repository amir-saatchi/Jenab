package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

// DLL loads agent_desktop_ffi.dll without cgo (syscall.LazyDLL) and runs every call on one
// locked OS thread: the library's element handles are thread-affine. Only the calls the spike
// needs are bound. Struct layouts follow agent_desktop.h (ABI major 6, x64) and are checked
// against the library's own ad_*_size functions at load.

const adABIMajor = 6

type DLL struct {
	dll     *syscall.LazyDLL
	adapter uintptr
	calls   chan func()
}

// adAction mirrors AdAction (96 bytes).
type adAction struct {
	Kind      int32
	_         int32
	Text      *byte
	ScrollDir int32
	ScrollAmt uint32
	Key       *byte
	Mods      *int32
	ModCount  uint32
	_         uint32
	Drag      [6]uint64 // AdDragParams, unused
}

// adWindowInfo mirrors AdWindowInfo (72 bytes); adExactWindowInfo mirrors AdExactWindowInfo (96).
type adWindowInfo struct {
	ID, Title, AppName *byte
	PID                uint32
	_                  uint32
	Bounds             [4]float64
	HasBounds          bool
	IsFocused          bool
	_                  [6]byte
}

type adExactWindowInfo struct {
	Version, Size   uint32
	Window          adWindowInfo
	ProcessInstance *byte
	Accessible      bool
	_               [7]byte
}

// adTreeOptions mirrors AdTreeOptions (8); adNodeTree AdNodeTree (16); adNode AdNode (112).
type adTreeOptions struct {
	MaxDepth        uint8
	IncludeBounds   bool
	InteractiveOnly bool
	Compact         bool
	Surface         int32
}

type adNodeTree struct {
	Nodes *adNode
	Count uint32
	_     uint32
}

type adNode struct {
	RefID, Role, Name, Value, Description, Hint *byte
	States                                      **byte
	Bounds                                      [4]float64
	StateCount                                  uint32
	HasBounds                                   bool
	_                                           [3]byte
	Parent                                      int32
	ChildStart, ChildCount                      uint32
	_                                           uint32
}

// adFindQuery mirrors AdFindQuery (112): AdFindControl (24) + AdFindFilter (88).
type adFindQuery struct {
	Version     uint32
	SelKind     int32
	SelNth      uint32
	_           uint32
	TimeoutMS   uint64
	Role, Name  *byte
	Description *byte
	NativeID    *byte
	Value       *byte
	HasText     *byte
	States      uintptr
	StateCount  uintptr
	Has, HasNot uintptr
	Exact       bool
	_           [7]byte
}

// adActionResult mirrors AdActionResult (56).
type adActionResult struct {
	Action, RefID *byte
	PostState     uintptr
	Steps         uintptr
	StepCount     uint32
	_             uint32
	DetailsJSON   *byte
	Delivery      int32
	Retry         int32
}

func LoadDLL(path, home string) (*DLL, error) {
	// The library reads AGENT_DESKTOP_HOME from its own process's environment.
	os.Setenv("AGENT_DESKTOP_HOME", home)
	d := &DLL{dll: syscall.NewLazyDLL(path), calls: make(chan func())}
	if err := d.dll.Load(); err != nil {
		return nil, err
	}
	go func() {
		runtime.LockOSThread()
		for f := range d.calls {
			f()
		}
	}()
	var err error
	d.do(func() {
		if r := d.call("ad_init", adABIMajor); r != 0 {
			err = fmt.Errorf("ad_init: %d %s", int32(r), d.lastError())
			return
		}
		for name, want := range map[string]uintptr{
			"ad_action_size": unsafe.Sizeof(adAction{}), "ad_exact_window_info_size": unsafe.Sizeof(adExactWindowInfo{}),
			"ad_action_result_size": unsafe.Sizeof(adActionResult{}),
		} {
			if got := d.call(name); got != want {
				err = fmt.Errorf("%s = %d, Go struct is %d", name, got, want)
				return
			}
		}
		if unsafe.Sizeof(adNode{}) != 112 || unsafe.Sizeof(adFindQuery{}) != 112 || unsafe.Sizeof(adTreeOptions{}) != 8 {
			err = errors.New("Go struct sizes differ from agent_desktop.h")
			return
		}
		d.adapter = d.call("ad_adapter_create")
		if d.adapter == 0 {
			err = errors.New("ad_adapter_create returned null")
		}
	})
	return d, err
}

// do runs f on the DLL's thread and waits.
func (d *DLL) do(f func()) {
	done := make(chan struct{})
	d.calls <- func() { f(); close(done) }
	<-done
}

// doTimeout runs f on the DLL's thread and gives up waiting after limit. The call itself cannot
// be stopped: it keeps the DLL's thread (and every later call) until it returns by itself.
func (d *DLL) doTimeout(limit time.Duration, f func()) error {
	done := make(chan struct{})
	select {
	case d.calls <- func() { f(); close(done) }:
	case <-time.After(limit):
		return ErrTimeout
	}
	select {
	case <-done:
		return nil
	case <-time.After(limit):
		return ErrTimeout
	}
}

// call moves pointer arguments to the heap and keeps them alive for the call (like Proc.Call).
//
//go:uintptrescapes
func (d *DLL) call(name string, args ...uintptr) uintptr {
	r, _, _ := d.dll.NewProc(name).Call(args...)
	return r
}

func cstr(s string) *byte {
	b, _ := syscall.BytePtrFromString(s)
	return b
}

func gostr(p *byte) string {
	if p == nil {
		return ""
	}
	var n int
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}

func (d *DLL) lastError() string {
	return gostr((*byte)(unsafe.Pointer(d.call("ad_last_error_message"))))
}

func (d *DLL) takeString(p uintptr) string {
	if p == 0 {
		return ""
	}
	s := gostr((*byte)(unsafe.Pointer(p)))
	d.call("ad_free_string", p)
	return s
}

func b2u(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}

// Snapshot is ad_snapshot: the named app's focused window only (no window id, no --root).
func (d *DLL) Snapshot(app string, interactive bool, maxDepth uint8) (string, int32) {
	var out uintptr
	var r uintptr
	d.do(func() {
		r = d.call("ad_snapshot", d.adapter, uintptr(unsafe.Pointer(cstr(app))), 0, uintptr(maxDepth),
			b2u(interactive), 0, uintptr(unsafe.Pointer(&out)))
	})
	var s string
	d.do(func() { s = d.takeString(out) })
	return s, int32(r)
}

// ClickRef is ad_execute_by_ref with a click, headless, after a snapshot that made the ref.
func (d *DLL) ClickRef(ref string) (string, int32) {
	act := adAction{Kind: 0}
	var out uintptr
	var r uintptr
	d.do(func() {
		r = d.call("ad_execute_by_ref_timeout", d.adapter, uintptr(unsafe.Pointer(cstr(ref))),
			uintptr(unsafe.Pointer(&act)), 0, 0, uintptr(unsafe.Pointer(&out)))
		runtime.KeepAlive(&act)
	})
	var s string
	d.do(func() { s = d.takeString(out) })
	return s, int32(r)
}

// ExactWindow is a copy of a listed window; the list stays alive until Close.
type ExactWindow struct {
	ID, Title, App string
	PID            uint32
	ptr            *adExactWindowInfo
}

type ExactList struct {
	d    *DLL
	list uintptr
	Wins []ExactWindow
}

func (d *DLL) ListWindowsExact(app string) (*ExactList, error) {
	l := &ExactList{d: d}
	var r uintptr
	d.do(func() {
		var fp uintptr
		if app != "" {
			fp = uintptr(unsafe.Pointer(cstr(app)))
		}
		r = d.call("ad_list_windows_exact", d.adapter, fp, 0, uintptr(unsafe.Pointer(&l.list)))
		if r != 0 {
			return
		}
		n := uint32(d.call("ad_exact_window_list_count", l.list))
		for i := uint32(0); i < n; i++ {
			p := (*adExactWindowInfo)(unsafe.Pointer(d.call("ad_exact_window_list_get", l.list, uintptr(i))))
			l.Wins = append(l.Wins, ExactWindow{ID: gostr(p.Window.ID), Title: gostr(p.Window.Title),
				App: gostr(p.Window.AppName), PID: p.Window.PID, ptr: p})
		}
	})
	if r != 0 {
		return nil, fmt.Errorf("ad_list_windows_exact: %d %s", int32(r), d.lastError())
	}
	return l, nil
}

func (l *ExactList) Close() { l.d.do(func() { l.d.call("ad_exact_window_list_free", l.list) }) }

// TreeNode is one node of ad_get_tree_exact's flat tree.
type TreeNode struct {
	Role, Name, Value string
	States            []string
	Parent            int32
}

// TreeExact snapshots one pinned window. The nodes have no refs.
func (d *DLL) TreeExact(w ExactWindow, interactive bool, maxDepth uint8) ([]TreeNode, error) {
	var nodes []TreeNode
	var r uintptr
	d.do(func() {
		opts := adTreeOptions{MaxDepth: maxDepth, InteractiveOnly: interactive}
		var t adNodeTree
		r = d.call("ad_get_tree_exact", d.adapter, uintptr(unsafe.Pointer(w.ptr)), uintptr(unsafe.Pointer(&opts)),
			uintptr(unsafe.Pointer(&t)))
		if r != 0 {
			return
		}
		for _, n := range unsafe.Slice(t.Nodes, t.Count) {
			tn := TreeNode{Role: gostr(n.Role), Name: gostr(n.Name), Value: gostr(n.Value), Parent: n.Parent}
			if n.StateCount > 0 {
				for _, s := range unsafe.Slice(n.States, n.StateCount) {
					tn.States = append(tn.States, gostr(s))
				}
			}
			nodes = append(nodes, tn)
		}
		d.call("ad_free_tree", uintptr(unsafe.Pointer(&t)))
	})
	if r != 0 {
		return nil, fmt.Errorf("ad_get_tree_exact: %d %s", int32(r), d.lastError())
	}
	return nodes, nil
}

// ClickExact finds one element by role and exact name in a pinned window and clicks it,
// headless: ad_find_exact, then ad_execute_action_with_policy, then ad_free_handle.
func (d *DLL) ClickExact(w ExactWindow, role, name string) (delivery int32, err error) {
	d.do(func() {
		q := adFindQuery{Version: 1, TimeoutMS: 2000, Role: cstr(role), Name: cstr(name), Exact: true}
		var h uintptr
		if r := d.call("ad_find_exact", d.adapter, uintptr(unsafe.Pointer(w.ptr)), uintptr(unsafe.Pointer(&q)),
			uintptr(unsafe.Pointer(&h))); r != 0 {
			err = fmt.Errorf("ad_find_exact: %d %s", int32(r), d.lastError())
			return
		}
		act := adAction{Kind: 0}
		var res adActionResult
		r := d.call("ad_execute_action_with_policy", d.adapter, uintptr(unsafe.Pointer(&h)),
			uintptr(unsafe.Pointer(&act)), 0, uintptr(unsafe.Pointer(&res)))
		delivery = res.Delivery
		if r != 0 {
			err = fmt.Errorf("ad_execute_action_with_policy: %d %s", int32(r), d.lastError())
		} else {
			d.call("ad_free_action_result", uintptr(unsafe.Pointer(&res)))
		}
		d.call("ad_free_handle", d.adapter, uintptr(unsafe.Pointer(&h)))
		runtime.KeepAlive(&q)
	})
	return delivery, err
}
