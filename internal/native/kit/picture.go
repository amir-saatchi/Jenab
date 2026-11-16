package kit

import "github.com/egoist/mygo/ui"

// Picture shows a picture from its bytes, as an image file opened (SPEC
// 5.12): PNG, JPEG, GIF, WebP, BMP or SVG, at its size or smaller to fit,
// at most maxHeight high, with a rounded border; label names it for
// assistive technology. key says which bytes they are, as a file's hash:
// they are decoded once per key. ok is false for bytes it can't decode,
// and the element is then empty.
func Picture(c *Context, key string, data []byte, label string, maxHeight float32) (e Element, ok bool) {
	k := TokensOf(c)
	box := ui.Row(c).Shrink(1).MinWidth(0)
	type decoded struct {
		src ui.ImageSource
		err error
	}
	d := ui.Local(box, key, func() decoded {
		src, err := ui.DecodeImage(data)
		return decoded{src, err}
	})
	if d.err != nil {
		return box, false
	}
	w, h := ui.ImageSize(d.src)
	return box.Children(func() {
		img := ui.Image(c, d.src).Label(label).Radius(Radius).Border(1, k.Border).MaxWidthPercent(100).Shrink(1)
		if h > maxHeight && h > 0 {
			img.Size(w*maxHeight/h, maxHeight)
		}
	}), true
}
