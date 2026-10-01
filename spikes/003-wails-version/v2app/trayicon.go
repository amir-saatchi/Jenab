package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
)

// trayIcon returns a 16x16 green .ico (a PNG inside an ICO container, which Windows accepts).
func trayIcon() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for i := range img.Pix {
		img.Pix[i] = []byte{0, 200, 83, 255}[i%4]
	}
	var p bytes.Buffer
	png.Encode(&p, img)
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, []uint16{0, 1, 1})                 // reserved, type icon, 1 image
	b.Write([]byte{16, 16, 0, 0})                                            // width, height, colours, reserved
	binary.Write(&b, binary.LittleEndian, []uint16{1, 32})                   // planes, bits per pixel
	binary.Write(&b, binary.LittleEndian, []uint32{uint32(p.Len()), 6 + 16}) // size, offset
	b.Write(p.Bytes())
	return b.Bytes()
}
