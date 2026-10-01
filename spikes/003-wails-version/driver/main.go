// driver: minimises and restores any window by title, and compares a screen
// capture after each restore with the capture taken before. For apps whose
// page is not ours (e.g. Nord-Agent), so the only check is "same as before".
//
// Usage: driver.exe "<window title>" <rounds> <out-dir>
package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

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
	setProcessDPI   = user32.NewProc("SetProcessDpiAwarenessContext")
	createCompatDC  = gdi32.NewProc("CreateCompatibleDC")
	createCompatBmp = gdi32.NewProc("CreateCompatibleBitmap")
	selectObject    = gdi32.NewProc("SelectObject")
	bitBlt          = gdi32.NewProc("BitBlt")
	getDIBits       = gdi32.NewProc("GetDIBits")
	deleteObject    = gdi32.NewProc("DeleteObject")
	deleteDC        = gdi32.NewProc("DeleteDC")
)

func main() {
	title, out := os.Args[1], os.Args[3]
	rounds, _ := strconv.Atoi(os.Args[2])
	os.MkdirAll(out, 0o755)
	setProcessDPI.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4), so coordinates are physical pixels
	t, _ := windows.UTF16PtrFromString(title)
	h, _, _ := findWindowW.Call(0, uintptr(unsafe.Pointer(t)))
	if h == 0 {
		fmt.Println("window not found:", title)
		os.Exit(1)
	}
	setForeground.Call(h)
	time.Sleep(1500 * time.Millisecond)
	base := capture(h)
	save(filepath.Join(out, "before.png"), base)
	fmt.Printf("window %#x, %dx%d\n", h, base.Bounds().Dx(), base.Bounds().Dy())
	fmt.Println("| Round | Minimised for | 300 ms after restore | 1.5 s after | 5 s after |")
	fmt.Println("|---|---|---|---|---|")
	bad := 0
	for i := 1; i <= rounds; i++ {
		d := 600 * time.Millisecond
		if i == rounds-1 {
			d = 30 * time.Second
		} else if i == rounds {
			d = 2 * time.Minute
		}
		postMessage.Call(h, 0x0112, 0xF020, 0) // WM_SYSCOMMAND SC_MINIMIZE
		time.Sleep(d)
		postMessage.Call(h, 0x0112, 0xF120, 0) // SC_RESTORE
		setForeground.Call(h)
		row := fmt.Sprintf("| %d | %v |", i, d)
		var slept time.Duration
		for _, w := range []time.Duration{300 * time.Millisecond, 1500 * time.Millisecond, 5 * time.Second} {
			time.Sleep(w - slept)
			slept = w
			img := capture(h)
			same := similarity(base, img)
			row += fmt.Sprintf(" %.1f%% same |", same*100)
			if same < 0.98 {
				bad++
				save(filepath.Join(out, fmt.Sprintf("round%02d-%v.png", i, w)), img)
			}
		}
		fmt.Println(row)
	}
	fmt.Printf("\nCaptures below 98%% the same as before: %d (saved in %s).\n", bad, out)
}

func capture(h uintptr) *image.RGBA {
	var rc struct{ L, T, R, B int32 }
	getClientRect.Call(h, uintptr(unsafe.Pointer(&rc)))
	pt := struct{ X, Y int32 }{}
	clientToScreen.Call(h, uintptr(unsafe.Pointer(&pt)))
	w, hgt := rc.R-rc.L, rc.B-rc.T
	img := image.NewRGBA(image.Rect(0, 0, int(w), int(hgt)))
	if w <= 0 || hgt <= 0 {
		return img
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
	getDIBits.Call(mem, bmp, 0, uintptr(hgt), uintptr(unsafe.Pointer(&img.Pix[0])), uintptr(unsafe.Pointer(&hdr)), 0)
	for i := 0; i+3 < len(img.Pix); i += 4 { // BGRA -> RGBA
		img.Pix[i], img.Pix[i+2], img.Pix[i+3] = img.Pix[i+2], img.Pix[i], 255
	}
	return img
}

// similarity is the share of pixels within a small tolerance of the baseline.
func similarity(a, b *image.RGBA) float64 {
	if a.Bounds() != b.Bounds() {
		return 0
	}
	same, total := 0, 0
	for i := 0; i+3 < len(a.Pix); i += 4 * 3 {
		total++
		d := 0
		for k := 0; k < 3; k++ {
			x := int(a.Pix[i+k]) - int(b.Pix[i+k])
			if x < 0 {
				x = -x
			}
			d += x
		}
		if d < 24 {
			same++
		}
	}
	return float64(same) / float64(total)
}

func save(path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	png.Encode(f, img)
}
