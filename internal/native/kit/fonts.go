package kit

import (
	_ "embed"
	"fmt"
	"sync"

	"github.com/egoist/mygo/ui"
)

// The fonts, variable in weight: see fonts/README.md.
var (
	//go:embed fonts/Geist-wght.ttf
	geist []byte
	//go:embed fonts/GeistMono-wght.ttf
	geistMono []byte
	//go:embed fonts/Vazirmatn-wght.ttf
	vazirmatn []byte
)

var (
	fontsOnce sync.Once
	fontsErr  error
)

// RegisterFonts adds Geist, Geist Mono and Vazirmatn, once. Call it before
// the first window builds.
func RegisterFonts() error {
	fontsOnce.Do(func() {
		for _, f := range []struct {
			data   []byte
			family string
		}{{geist, "Geist"}, {geistMono, "Geist Mono"}, {vazirmatn, "Vazirmatn"}} {
			if err := ui.RegisterFont(f.data, f.family); err != nil {
				fontsErr = fmt.Errorf("font %s: %w", f.family, err)
				return
			}
		}
	})
	return fontsErr
}
