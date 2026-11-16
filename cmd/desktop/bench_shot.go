//go:build bench

package main

import (
	"errors"
	"image"
	"image/png"
	"net/http"
	"os"
	"syscall"
	"unsafe"
)

var (
	gdi32                  = syscall.NewLazyDLL("gdi32.dll")
	procGetClientRect      = user32.NewProc("GetClientRect")
	procGetDC              = user32.NewProc("GetDC")
	procReleaseDC          = user32.NewProc("ReleaseDC")
	procPrintWindow        = user32.NewProc("PrintWindow")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
)

// shot saves a picture of the window's client area as PNG to the path
// given, as the window draws it: PrintWindow with PW_RENDERFULLCONTENT
// takes what DirectComposition shows, so the window may be covered.
func (b *bench) shot(w http.ResponseWriter, r *http.Request) {
	var img *image.RGBA
	var err error
	b.ui(func() { img, err = capture(b.win.NativeHandle()) })
	if err == nil {
		var f *os.File
		if f, err = os.Create(r.URL.Query().Get("path")); err == nil {
			err = png.Encode(f, img)
			if cerr := f.Close(); err == nil {
				err = cerr
			}
		}
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func capture(hwnd uintptr) (*image.RGBA, error) {
	var rc struct{ L, T, R, B int32 }
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	wd, ht := int(rc.R-rc.L), int(rc.B-rc.T)
	if wd <= 0 || ht <= 0 {
		return nil, errors.New("the window has no size")
	}
	screen, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, screen)
	mem, _, _ := procCreateCompatibleDC.Call(screen)
	defer procDeleteDC.Call(mem)
	bi := struct {
		Size                   uint32
		Width, Height          int32
		Planes, BitCount       uint16
		Compression, SizeImage uint32
		XPels, YPels           int32
		ClrUsed, ClrImportant  uint32
		Colors                 [1]uint32
	}{Size: 40, Width: int32(wd), Height: -int32(ht), Planes: 1, BitCount: 32}
	var bits unsafe.Pointer
	bmp, _, _ := procCreateDIBSection.Call(screen, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 {
		return nil, errors.New("CreateDIBSection failed")
	}
	defer procDeleteObject.Call(bmp)
	old, _, _ := procSelectObject.Call(mem, bmp)
	defer procSelectObject.Call(mem, old)
	const pwClientOnly, pwRenderFullContent = 1, 2
	if ok, _, err := procPrintWindow.Call(hwnd, mem, pwClientOnly|pwRenderFullContent); ok == 0 {
		return nil, err
	}
	src := unsafe.Slice((*byte)(bits), wd*ht*4)
	img := image.NewRGBA(image.Rect(0, 0, wd, ht))
	for i := 0; i < len(src); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = src[i+2], src[i+1], src[i], 255
	}
	return img, nil
}
