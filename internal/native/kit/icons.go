package kit

import (
	"embed"

	"github.com/egoist/mygo/ui"
)

// The icons are Lucide's (ISC, icons/LICENSE), from the lucide-react
// package the frontend uses, so both apps draw the same ones.
//
//go:embed icons/*.svg
var iconFiles embed.FS

func icon(name string) *SVG {
	b, err := iconFiles.ReadFile("icons/" + name + ".svg")
	if err != nil {
		panic(err)
	}
	return ui.MustParseSVG(b)
}

// The kit's icons.
var (
	IconX            = icon("x")
	IconCheck        = icon("check")
	IconInfo         = icon("info")
	IconWarning      = icon("triangle-alert")
	IconError        = icon("circle-alert")
	IconSuccess      = icon("circle-check")
	IconChevronDown  = icon("chevron-down")
	IconChevronRight = icon("chevron-right")
	IconMore         = icon("ellipsis")
	IconPlus         = icon("plus")
	IconInbox        = icon("inbox")
	IconChat         = icon("message-square")
	IconSidebar      = icon("panel-left")
	IconSearch       = icon("search")
	IconSettings     = icon("settings")
	IconCopy         = icon("copy")
)
