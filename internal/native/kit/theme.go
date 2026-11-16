package kit

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Tone is one of the tones configs may name (SPEC 5.11).
type Tone string

const (
	Neutral Tone = "neutral"
	Blue    Tone = "blue"
	Green   Tone = "green"
	Amber   Tone = "amber"
	Red     Tone = "red"
	Purple  Tone = "purple"
)

// Tones are the tones in the order chart series take them.
var Tones = []Tone{Neutral, Blue, Green, Amber, Red, Purple}

// ToneColors is a tone's text color and its soft fill. Text in the tone
// meets WCAG AA on the background and on the soft fill, in both themes.
type ToneColors struct{ Text, Soft Color }

// Tokens is a theme's colors, as the React app's index.css names them for
// shadcn/ui, and the tones.
type Tokens struct {
	Background, Foreground           Color
	Card, CardForeground             Color
	Popover, PopoverForeground       Color
	Primary, PrimaryForeground       Color
	Secondary, SecondaryForeground   Color
	Muted, MutedForeground           Color
	Accent, AccentForeground         Color
	Destructive                      Color
	Border, Input, Ring              Color
	Sidebar, SidebarForeground       Color
	SidebarPrimary, SidebarPrimaryFg Color
	SidebarAccent, SidebarAccentFg   Color
	SidebarBorder, SidebarRing       Color
	Tones                            map[Tone]ToneColors
}

// Tone is the tone's colors; anything unknown is Neutral, as in configs.
func (t *Tokens) Tone(tone Tone) ToneColors {
	if c, ok := t.Tones[tone]; ok {
		return c
	}
	return t.Tones[Neutral]
}

var oklch = ui.Oklch

// Light is :root in index.css.
var Light = Tokens{
	Background: oklch(1, 0, 0), Foreground: oklch(0.145, 0, 0),
	Card: oklch(1, 0, 0), CardForeground: oklch(0.145, 0, 0),
	Popover: oklch(1, 0, 0), PopoverForeground: oklch(0.145, 0, 0),
	Primary: oklch(0.205, 0, 0), PrimaryForeground: oklch(0.985, 0, 0),
	Secondary: oklch(0.97, 0, 0), SecondaryForeground: oklch(0.205, 0, 0),
	Muted: oklch(0.97, 0, 0), MutedForeground: oklch(0.53, 0, 0),
	Accent: oklch(0.97, 0, 0), AccentForeground: oklch(0.205, 0, 0),
	Destructive: oklch(0.577, 0.245, 27.325),
	Border:      oklch(0.922, 0, 0), Input: oklch(0.922, 0, 0), Ring: oklch(0.708, 0, 0),
	Sidebar: oklch(0.985, 0, 0), SidebarForeground: oklch(0.145, 0, 0),
	SidebarPrimary: oklch(0.205, 0, 0), SidebarPrimaryFg: oklch(0.985, 0, 0),
	SidebarAccent: oklch(0.97, 0, 0), SidebarAccentFg: oklch(0.205, 0, 0),
	SidebarBorder: oklch(0.922, 0, 0), SidebarRing: oklch(0.708, 0, 0),
	Tones: map[Tone]ToneColors{
		Neutral: {oklch(0.45, 0, 0), oklch(0.96, 0, 0)},
		Blue:    {oklch(0.49, 0.14, 258), oklch(0.95, 0.02, 258)},
		Green:   {oklch(0.48, 0.12, 152), oklch(0.96, 0.03, 152)},
		Amber:   {oklch(0.5, 0.11, 65), oklch(0.96, 0.04, 85)},
		Red:     {oklch(0.51, 0.17, 27), oklch(0.95, 0.02, 27)},
		Purple:  {oklch(0.49, 0.16, 300), oklch(0.95, 0.02, 300)},
	},
}

// Dark is .dark in index.css.
var Dark = Tokens{
	Background: oklch(0.145, 0, 0), Foreground: oklch(0.985, 0, 0),
	Card: oklch(0.205, 0, 0), CardForeground: oklch(0.985, 0, 0),
	Popover: oklch(0.205, 0, 0), PopoverForeground: oklch(0.985, 0, 0),
	Primary: oklch(0.922, 0, 0), PrimaryForeground: oklch(0.205, 0, 0),
	Secondary: oklch(0.269, 0, 0), SecondaryForeground: oklch(0.985, 0, 0),
	Muted: oklch(0.269, 0, 0), MutedForeground: oklch(0.708, 0, 0),
	Accent: oklch(0.269, 0, 0), AccentForeground: oklch(0.985, 0, 0),
	Destructive: oklch(0.704, 0.191, 22.216),
	Border:      oklch(1, 0, 0).Alpha(0.10), Input: oklch(1, 0, 0).Alpha(0.15), Ring: oklch(0.556, 0, 0),
	Sidebar: oklch(0.205, 0, 0), SidebarForeground: oklch(0.985, 0, 0),
	SidebarPrimary: oklch(0.488, 0.243, 264.376), SidebarPrimaryFg: oklch(0.985, 0, 0),
	SidebarAccent: oklch(0.269, 0, 0), SidebarAccentFg: oklch(0.985, 0, 0),
	SidebarBorder: oklch(1, 0, 0).Alpha(0.10), SidebarRing: oklch(0.556, 0, 0),
	Tones: map[Tone]ToneColors{
		Neutral: {oklch(0.8, 0, 0), oklch(0.27, 0, 0)},
		Blue:    {oklch(0.8, 0.08, 258), oklch(0.28, 0.05, 258)},
		Green:   {oklch(0.8, 0.13, 152), oklch(0.28, 0.05, 152)},
		Amber:   {oklch(0.83, 0.12, 80), oklch(0.29, 0.05, 75)},
		Red:     {oklch(0.8, 0.11, 25), oklch(0.29, 0.07, 25)},
		Purple:  {oklch(0.8, 0.1, 300), oklch(0.29, 0.06, 300)},
	},
}

// Fonts are the families of text and code: Geist, with Vazirmatn for
// Persian and Arabic (SPEC 5.8), and Geist Mono. RegisterFonts adds them.
const (
	FontSans = "Geist, Vazirmatn"
	FontMono = "Geist Mono, Vazirmatn"
)

// Radius is --radius in index.css, 0.625rem; controls take rounded-md,
// 0.8 of it.
const Radius = 10

// Theme is MyGo's theme from the tokens of the light or the dark theme.
func Theme(dark bool) *ui.Theme {
	k := &Light
	if dark {
		k = &Dark
	}
	t := &ui.Theme{
		Dark:       dark,
		Background: k.Background,
		// Controls: shadcn's outline look, the background with a border, the
		// accent while hovered.
		Surface:        k.Background,
		SurfaceHover:   k.Accent,
		SurfacePressed: k.Accent.Mix(k.Foreground, 0.06),
		Border:         k.Input,
		Text:           k.Foreground,
		TextMuted:      k.MutedForeground,
		Accent:         k.Primary,
		AccentHover:    k.Primary.Alpha(0.9),
		AccentPressed:  k.Primary.Alpha(0.8),
		AccentText:     k.PrimaryForeground,
		Danger:         k.Destructive,
		Warning:        k.Tones[Amber].Text,
		Success:        k.Tones[Green].Text,
		Selection:      k.Tones[Blue].Text.Alpha(0.3),
		Focus:          k.Ring.Alpha(0.5),
		// Tooltips and toasts: the foreground with the background on it, as
		// shadcn's tooltip.
		Inverse:     k.Foreground,
		InverseText: k.Background,
		Scrollbar:   k.Foreground.Alpha(0.25),
		Radius:      Radius * 0.8,
		Spacing:     4,
		FontSize:    14,
		Font:        FontSans,
	}
	if dark {
		// dark:bg-input/30 on inputs and outline buttons.
		t.Surface = k.Background.Mix(ui.Oklch(1, 0, 0), 0.045)
		t.SurfaceHover = k.Input.Mix(k.Background, 0.5)
	}
	return t
}

// SetThemeSource makes the app's windows light, dark or follow the
// system (SPEC 5.11). Anything but light and dark is System.
func SetThemeSource(s string) {
	switch s {
	case "light":
		mygo.Theme.SetSource(mygo.ThemeLight)
	case "dark":
		mygo.Theme.SetSource(mygo.ThemeDark)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
}
