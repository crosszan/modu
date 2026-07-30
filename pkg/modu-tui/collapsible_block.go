package modutui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type CollapsibleBlock struct {
	Summary  string
	Detail   string
	Expanded bool
}

func (b CollapsibleBlock) Render(RenderContext) BlockRender {
	arrow := "▸"
	if b.Expanded {
		arrow = "▾"
	}
	out := BlockRender{}
	// The arrow and the detail indent are decoration, so they are declared as
	// gutter and stay out of copied text.
	marker := arrow + " "
	detailIndent := "    "
	out.Add(dimStyle.Render(marker+b.Summary), lipgloss.Width(marker))
	if b.Expanded {
		for dl := range strings.SplitSeq(strings.TrimRight(b.Detail, "\n"), "\n") {
			out.Add(dimStyle.Render(detailIndent+dl), lipgloss.Width(detailIndent))
		}
	}
	return out
}
