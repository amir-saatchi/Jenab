// probe.go is shared by both apps (copied into v2app and v3app by build.sh).
// It serves the test page and the /objects route, then drives the window and
// checks with screen captures whether the page is still painted.
package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

//go:embed frontend
var assets embed.FS

const title = "Burrow SPIKE-003"

// win is what each app gives the probe.
type win interface {
	Minimise()
	Restore()
	Hide()
	Show()
	Notify() error
	Quit()
}

type result struct {
	App        string
	Page       map[string]string // what the page reported
	Painted    map[string]string // capture checks: name -> "n of m painted"
	Failures   []string          // failed captures, with time and painted share
	Ticks      map[string]int64  // background ticks per phase
	Second     string            // second-instance result
	Notify     string
	Events     []string // OnSuspend/OnResume etc. seen
	Extra      map[string]string
	TotalTime  string
	WindowHWND string
}

var (
	res      = result{Page: map[string]string{}, Painted: map[string]string{}, Ticks: map[string]int64{}, Extra: map[string]string{}}
	resMu    sync.Mutex
	pageDone = make(chan struct{})
	ticks    atomic.Int64
	target   atomic.Int64 // index into colours: what the page should show now
	clicks   atomic.Int64 // mouse clicks the page received
	keys     atomic.Int64 // key presses the page received
	second   = make(chan string, 1)
)

func note(f func(r *result)) {
	resMu.Lock()
	defer resMu.Unlock()
	f(&res)
}

// objects is the bucket stand-in for /objects/<project_id>/<key>.
var objects = map[string][]byte{
	"/objects/p1/hello.txt": []byte("hello from the bucket"),
	"/objects/p1/pic.png":   greenPNG(),
	"/objects/p1/big.bin":   bytes.Repeat([]byte("0123456789abcdef"), 5<<20/16), // 5 MB
}

// handler serves /objects/... and /report. Everything else falls through to the embedded assets.
func handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/objects/"):
			b, ok := objects[r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			http.ServeContent(w, r, r.URL.Path, time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC), bytes.NewReader(b))
		case r.URL.Path == "/input":
			switch r.URL.Query().Get("k") {
			case "click":
				clicks.Add(1)
			case "key":
				keys.Add(1)
			}
			w.WriteHeader(204)
		case r.URL.Path == "/state":
			c := colours[target.Load()%int64(len(colours))]
			w.Header().Set("Cache-Control", "no-store")
			fmt.Fprintf(w, "#%02x%02x%02x", c.R, c.G, c.B)
		case r.URL.Path == "/report":
			q := r.URL.Query()
			note(func(res *result) {
				for k := range q {
					res.Page[k] = q.Get(k)
				}
			})
			w.WriteHeader(204)
			if q.Get("done") == "1" {
				select {
				case <-pageDone:
				default:
					close(pageDone)
				}
			}
		default:
			http.NotFound(w, r)
		}
	})
}

func startTicker() {
	go func() {
		for range time.Tick(100 * time.Millisecond) {
			ticks.Add(1)
		}
	}()
}

// run is the test sequence; it runs in a goroutine once the app has started.
func run(w win, app string, out string) {
	start := time.Now()
	note(func(r *result) { r.App = app })
	select {
	case <-pageDone:
	case <-time.After(20 * time.Second):
		note(func(r *result) { r.Page["timeout"] = "page did not report within 20 s" })
	}
	for _, a := range os.Args[1:] {
		if a == "--manual" {
			return // leave the app running for a hands-on check
		}
	}
	time.Sleep(1 * time.Second)
	hwnd := findWindow()
	note(func(r *result) { r.WindowHWND = fmt.Sprintf("%#x", hwnd) })
	check := func(name string, rounds int, before, after func(), waits ...time.Duration) {
		counts := make([]int, len(waits))
		inputOK := 0
		for i := 0; i < rounds; i++ {
			before()
			target.Add(1) // the page must show a new colour after it comes back
			time.Sleep(600 * time.Millisecond)
			after()
			var slept time.Duration
			for j, wt := range waits {
				time.Sleep(wt - slept)
				slept = wt
				share := painted(hwnd)
				if share >= 0.9 {
					counts[j]++
				} else {
					fg, _, _ := getForeground.Call()
					icon, _, _ := isIconic.Call(hwnd)
					vis, _, _ := isVisible.Call(hwnd)
					file := fmt.Sprintf("fail-%s-%d-%d.png", strings.NewReplacer(" ", "_", "/", "_", ",", "", "(", "", ")", "").Replace(name), i+1, j)
					saveCapture(hwnd, file)
					note(func(r *result) {
						r.Failures = append(r.Failures, fmt.Sprintf("%s round %d, %v after: %.0f%% painted; our window in front: %v, minimised: %v, visible: %v; screen saved as %s",
							name, i+1, wt, share*100, fg == hwnd, icon != 0, vis != 0, file))
					})
				}
			}
			// does the page still get input? a real click in the window and a key press
			c0, k0 := clicks.Load(), keys.Load()
			clickAndType(hwnd)
			time.Sleep(700 * time.Millisecond)
			if clicks.Load() > c0 && keys.Load() > k0 {
				inputOK++
			} else {
				fg, _, _ := getForeground.Call()
				note(func(r *result) {
					r.Failures = append(r.Failures, fmt.Sprintf("%s round %d: input not received (clicks +%d, keys +%d); our window in front: %v",
						name, i+1, clicks.Load()-c0, keys.Load()-k0, fg == hwnd))
				})
			}
		}
		note(func(r *result) {
			r.Painted[name+", click and key after restore"] = fmt.Sprintf("%d of %d", inputOK, rounds)
		})
		for j, wt := range waits {
			note(func(r *result) {
				r.Painted[fmt.Sprintf("%s, %v after", name, wt)] = fmt.Sprintf("%d of %d", counts[j], rounds)
			})
		}
	}
	note(func(r *result) { r.Painted["at start"] = fmt.Sprintf("%.0f%%", painted(hwnd)*100) })
	sysMin := func() { postSysCommand(hwnd, scMinimize) }
	sysRestore := func() { postSysCommand(hwnd, scRestore); foreground(hwnd) }
	if longOnly() > 0 {
		// only the long minimised case, repeated
		check("minimised for 2m0s, restore by WM_SYSCOMMAND", longOnly(), func() { sysMin(); time.Sleep(2 * time.Minute) }, sysRestore, 300*time.Millisecond, 1500*time.Millisecond, 5*time.Second)
		finish(w, start, out)
		return
	}

	// what the user does: title-bar or taskbar minimise and restore
	check("minimise/restore by WM_SYSCOMMAND (like the taskbar)", 20, sysMin, sysRestore, 300*time.Millisecond, 1500*time.Millisecond)
	// the same through the Wails API
	check("minimise/restore by the Wails API", 10, w.Minimise, func() { w.Restore(); foreground(hwnd) }, 300*time.Millisecond, 1500*time.Millisecond)
	// what the tray or a second launch does: hide, then show
	check("hide/show by the Wails API", 20, w.Hide, func() { w.Show(); foreground(hwnd) }, 300*time.Millisecond, 1500*time.Millisecond)
	// long minimised periods: WebView2 may suspend a hidden page after a while
	for _, d := range []time.Duration{30 * time.Second, 2 * time.Minute} {
		check(fmt.Sprintf("minimised for %v, restore by WM_SYSCOMMAND", d), 1, func() { sysMin(); time.Sleep(d) }, sysRestore, 300*time.Millisecond, 1500*time.Millisecond, 5*time.Second)
	}

	// background work while hidden
	w.Hide()
	t0 := ticks.Load()
	time.Sleep(5 * time.Second)
	note(func(r *result) { r.Ticks["5 s hidden (expect ~50)"] = ticks.Load() - t0 })
	w.Show()
	foreground(hwnd)

	// second instance: it should hand over and exit
	exe, _ := os.Executable()
	cmd := exec.Command(exe, "--second")
	t1 := time.Now()
	err := cmd.Run()
	exitIn := time.Since(t1).Round(time.Millisecond)
	select {
	case s := <-second:
		note(func(r *result) {
			r.Second = fmt.Sprintf("first instance got %q; second exited after %v (err %v)", s, exitIn, err)
		})
	case <-time.After(5 * time.Second):
		note(func(r *result) {
			r.Second = fmt.Sprintf("no callback in the first instance; second exited after %v (err %v)", exitIn, err)
		})
	}

	if err := w.Notify(); err != nil {
		note(func(r *result) { r.Notify = "error: " + err.Error() })
	} else {
		note(func(r *result) { r.Notify = "sent without error" })
	}
	finish(w, start, out)
}

func finish(w win, start time.Time, out string) {
	time.Sleep(1 * time.Second)
	note(func(r *result) { r.TotalTime = time.Since(start).Round(time.Second).String() })
	resMu.Lock()
	b, _ := json.MarshalIndent(res, "", "  ")
	resMu.Unlock()
	os.WriteFile(out, b, 0o644)
	w.Quit()
}

func greenPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = []byte{0, 200, 83, 255}[i%4]
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

// ---- Win32 helpers ----

var (
	user32          = windows.NewLazySystemDLL("user32.dll")
	gdi32           = windows.NewLazySystemDLL("gdi32.dll")
	findWindowW     = user32.NewProc("FindWindowW")
	getClientRect   = user32.NewProc("GetClientRect")
	clientToScreen  = user32.NewProc("ClientToScreen")
	getDC           = user32.NewProc("GetDC")
	releaseDC       = user32.NewProc("ReleaseDC")
	postMessage     = user32.NewProc("PostMessageW")
	setForeground   = user32.NewProc("SetForegroundWindow")
	createCompatDC  = gdi32.NewProc("CreateCompatibleDC")
	createCompatBmp = gdi32.NewProc("CreateCompatibleBitmap")
	selectObject    = gdi32.NewProc("SelectObject")
	bitBlt          = gdi32.NewProc("BitBlt")
	getDIBits       = gdi32.NewProc("GetDIBits")
	deleteObject    = gdi32.NewProc("DeleteObject")
	deleteDC        = gdi32.NewProc("DeleteDC")
	scMinimize      = uintptr(0xF020)
	scRestore       = uintptr(0xF120)
	wmSysCommand    = uintptr(0x0112)
	colours         = []color.RGBA{{0, 200, 83, 255}, {0, 120, 255, 255}, {255, 140, 0, 255}, {128, 0, 200, 255}}
)

func findWindow() uintptr {
	t, _ := windows.UTF16PtrFromString(title)
	for i := 0; i < 50; i++ {
		h, _, _ := findWindowW.Call(0, uintptr(unsafe.Pointer(t)))
		if h != 0 {
			return h
		}
		time.Sleep(100 * time.Millisecond)
	}
	return 0
}

func postSysCommand(h, cmd uintptr) { postMessage.Call(h, wmSysCommand, cmd, 0) }
func foreground(h uintptr)          { setForeground.Call(h) }

// painted captures the window's client area from the screen and returns the
// share of pixels that have the colour the page should show now. The page is
// one flat colour, polled from /state, so a stale frame counts as not painted.
func painted(h uintptr) float64 {
	var rc struct{ L, T, R, B int32 }
	getClientRect.Call(h, uintptr(unsafe.Pointer(&rc)))
	pt := struct{ X, Y int32 }{0, 0}
	clientToScreen.Call(h, uintptr(unsafe.Pointer(&pt)))
	w, hgt := rc.R-rc.L, rc.B-rc.T
	if w <= 0 || hgt <= 0 {
		return 0
	}
	screen, _, _ := getDC.Call(0)
	defer releaseDC.Call(0, screen)
	mem, _, _ := createCompatDC.Call(screen)
	defer deleteDC.Call(mem)
	bmp, _, _ := createCompatBmp.Call(screen, uintptr(w), uintptr(hgt))
	defer deleteObject.Call(bmp)
	selectObject.Call(mem, bmp)
	const srcCopy = 0x00CC0020
	bitBlt.Call(mem, 0, 0, uintptr(w), uintptr(hgt), screen, uintptr(pt.X), uintptr(pt.Y), srcCopy)
	type bmiHeader struct {
		Size                   uint32
		Width, Height          int32
		Planes, BitCount       uint16
		Compression, SizeImage uint32
		XPels, YPels           int32
		ClrUsed, ClrImportant  uint32
	}
	hdr := bmiHeader{Width: w, Height: -hgt, Planes: 1, BitCount: 32}
	hdr.Size = uint32(unsafe.Sizeof(hdr))
	buf := make([]byte, int(w)*int(hgt)*4)
	getDIBits.Call(mem, bmp, 0, uintptr(hgt), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&hdr)), 0)
	want := colours[target.Load()%int64(len(colours))]
	match, total := 0, 0
	// sample every 4th pixel; BGRA order
	for i := 0; i+3 < len(buf); i += 16 {
		total++
		b, g, r := buf[i], buf[i+1], buf[i+2]
		if near(r, want.R) && near(g, want.G) && near(b, want.B) {
			match++
		}
	}
	return float64(match) / float64(total)
}

func near(a, b uint8) bool {
	d := int(a) - int(b)
	return d > -12 && d < 12
}

// longOnly returns N for "--long N": run only the long minimised case, N times.
func longOnly() int {
	for i, a := range os.Args {
		if a == "--long" && i+1 < len(os.Args) {
			var n int
			fmt.Sscan(os.Args[i+1], &n)
			return n
		}
	}
	return 0
}

var (
	getForeground = user32.NewProc("GetForegroundWindow")
	isIconic      = user32.NewProc("IsIconic")
	isVisible     = user32.NewProc("IsWindowVisible")
)

// saveCapture saves the screen area of the window's client rect as a PNG.
func saveCapture(h uintptr, file string) {
	var rc struct{ L, T, R, B int32 }
	getClientRect.Call(h, uintptr(unsafe.Pointer(&rc)))
	pt := struct{ X, Y int32 }{}
	clientToScreen.Call(h, uintptr(unsafe.Pointer(&pt)))
	w, hgt := rc.R-rc.L, rc.B-rc.T
	if w <= 0 || hgt <= 0 {
		return
	}
	screen, _, _ := getDC.Call(0)
	defer releaseDC.Call(0, screen)
	mem, _, _ := createCompatDC.Call(screen)
	defer deleteDC.Call(mem)
	bmp, _, _ := createCompatBmp.Call(screen, uintptr(w), uintptr(hgt))
	defer deleteObject.Call(bmp)
	selectObject.Call(mem, bmp)
	bitBlt.Call(mem, 0, 0, uintptr(w), uintptr(hgt), screen, uintptr(pt.X), uintptr(pt.Y), 0x00CC0020)
	type bmiHeader struct {
		Size                   uint32
		Width, Height          int32
		Planes, BitCount       uint16
		Compression, SizeImage uint32
		XPels, YPels           int32
		ClrUsed, ClrImportant  uint32
	}
	hdr := bmiHeader{Width: w, Height: -hgt, Planes: 1, BitCount: 32}
	hdr.Size = uint32(unsafe.Sizeof(hdr))
	img := image.NewRGBA(image.Rect(0, 0, int(w), int(hgt)))
	getDIBits.Call(mem, bmp, 0, uintptr(hgt), uintptr(unsafe.Pointer(&img.Pix[0])), uintptr(unsafe.Pointer(&hdr)), 0)
	for i := 0; i+3 < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+2], img.Pix[i+3] = img.Pix[i+2], img.Pix[i], 255
	}
	f, err := os.Create(file)
	if err != nil {
		return
	}
	defer f.Close()
	png.Encode(f, img)
}

var (
	setCursorPos = user32.NewProc("SetCursorPos")
	mouseEvent   = user32.NewProc("mouse_event")
	keybdEvent   = user32.NewProc("keybd_event")
)

// clickAndType clicks the middle of the window's client area and presses "A".
func clickAndType(h uintptr) {
	var rc struct{ L, T, R, B int32 }
	getClientRect.Call(h, uintptr(unsafe.Pointer(&rc)))
	pt := struct{ X, Y int32 }{(rc.R - rc.L) / 2, (rc.B - rc.T) / 2}
	clientToScreen.Call(h, uintptr(unsafe.Pointer(&pt)))
	setCursorPos.Call(uintptr(pt.X), uintptr(pt.Y))
	const leftDown, leftUp, keyUp = 0x0002, 0x0004, 0x0002
	mouseEvent.Call(leftDown, 0, 0, 0, 0)
	mouseEvent.Call(leftUp, 0, 0, 0, 0)
	time.Sleep(100 * time.Millisecond)
	keybdEvent.Call(0x41, 0, 0, 0)
	keybdEvent.Call(0x41, 0, keyUp, 0)
}
