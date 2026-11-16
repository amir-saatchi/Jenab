package kit

import (
	"fmt"
	"math"
	"testing"
)

// The kit's text colors meet WCAG AA, 4.5:1, on the fills they are drawn
// on, in the light and the dark theme. These are the pairs and the
// threshold of the React app's contrast.test.ts, which this test keeps when
// the frontend goes (SPEC 5.11). The colors are those the kit draws, in
// 8-bit sRGB, where the web app's test computes them from oklch.

const minContrast = 4.5

// luminance is the WCAG relative luminance of c's sRGB color.
func luminance(c Color) float64 {
	lin := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

// contrast is the WCAG contrast ratio of two colors, 1 to 21.
func contrast(a, b Color) float64 {
	la, lb := luminance(a), luminance(b)
	return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
}

type contrastPair struct {
	name   string
	fg, bg Color
}

func contrastPairs(k *Tokens) []contrastPair {
	p := []contrastPair{
		{"foreground on background", k.Foreground, k.Background},
		{"card-foreground on card", k.CardForeground, k.Card},
		{"popover-foreground on popover", k.PopoverForeground, k.Popover},
		{"primary-foreground on primary", k.PrimaryForeground, k.Primary},
		{"secondary-foreground on secondary", k.SecondaryForeground, k.Secondary},
		{"muted-foreground on muted", k.MutedForeground, k.Muted},
		{"muted-foreground on background", k.MutedForeground, k.Background},
		{"muted-foreground on sidebar", k.MutedForeground, k.Sidebar},
		{"accent-foreground on accent", k.AccentForeground, k.Accent},
		{"sidebar-foreground on sidebar", k.SidebarForeground, k.Sidebar},
		{"sidebar-accent-foreground on sidebar-accent", k.SidebarAccentFg, k.SidebarAccent},
		{"sidebar-primary-foreground on sidebar-primary", k.SidebarPrimaryFg, k.SidebarPrimary},
		{"destructive on background", k.Destructive, k.Background},
		{"background on tone-amber", k.Background, k.Tone(Amber).Text},
	}
	for _, tone := range Tones {
		c := k.Tone(tone)
		p = append(p,
			contrastPair{fmt.Sprintf("tone-%s on background", tone), c.Text, k.Background},
			contrastPair{fmt.Sprintf("tone-%s on tone-%s-soft", tone, tone), c.Text, c.Soft},
			contrastPair{fmt.Sprintf("tone-%s on sidebar", tone), c.Text, k.Sidebar},
		)
	}
	return p
}

func TestTheColorsMeetAA(t *testing.T) {
	for _, theme := range []struct {
		name string
		k    *Tokens
	}{{"light", &Light}, {"dark", &Dark}} {
		t.Run(theme.name, func(t *testing.T) {
			if len(theme.k.Tones) != len(Tones) {
				t.Errorf("%d tones, want %d", len(theme.k.Tones), len(Tones))
			}
			for _, p := range contrastPairs(theme.k) {
				if r := contrast(p.fg, p.bg); r < minContrast {
					t.Errorf("%s: %.2f, under %.1f", p.name, r, minContrast)
				}
			}
		})
	}
}

func TestContrastRatio(t *testing.T) {
	black, white := Color{A: 255}, Color{R: 255, G: 255, B: 255, A: 255}
	if r := contrast(black, white); math.Abs(r-21) > 0.01 {
		t.Errorf("black on white: %.2f, want 21", r)
	}
	if r := contrast(white, white); r != 1 {
		t.Errorf("white on white: %.2f, want 1", r)
	}
	if r := contrast(oklch(0.708, 0, 0), white); r >= minContrast {
		t.Errorf("oklch(0.708 0 0) on white: %.2f, want under %.1f", r, minContrast)
	}
}
