package modutui

import (
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
	glamouransi "github.com/charmbracelet/glamour/ansi"
	glamourstyles "github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/x/ansi"
)

const (
	orderedListMarker  = "\u2063"
	markdownBreakSpace = "\u2009"
)

// markdownRenderer builds a glamour renderer with the document Margin zeroed, so
// finalized markdown sits flush against the left edge.
func markdownRenderer(width int) *glamour.TermRenderer {
	style := markdownStyleConfig()
	r, _ := glamour.NewTermRenderer(glamour.WithStyles(style), glamour.WithWordWrap(width))
	return r
}

func markdownStyleConfig() glamouransi.StyleConfig {
	style := glamourstyles.DarkStyleConfig
	if glamourStyle() == "light" {
		style = glamourstyles.LightStyleConfig
	}
	noMargin := uint(0)
	style.Document = glamouransi.StyleBlock{
		StylePrimitive: style.Document.StylePrimitive,
		Margin:         &noMargin,
	}
	style.Code = glamouransi.StyleBlock{}
	style.Enumeration.BlockPrefix = ". " + orderedListMarker
	return style
}

// markdownWithPlaintextFences prevents Chroma from guessing a programming
// language for unlabelled fenced code blocks. Auto-detection can misclassify
// prose diagrams (for example, "toolimage" as GDScript) and paint Chinese or
// tree-drawing characters with the theme's red Error-token background.
func markdownWithPlaintextFences(markdown string) string {
	var out strings.Builder
	var fence byte
	var fenceLen int

	for _, raw := range strings.SplitAfter(markdown, "\n") {
		line, ending := raw, ""
		if strings.HasSuffix(line, "\n") {
			line = strings.TrimSuffix(line, "\n")
			ending = "\n"
			if strings.HasSuffix(line, "\r") {
				line = strings.TrimSuffix(line, "\r")
				ending = "\r\n"
			}
		}

		indent := 0
		for indent < len(line) && indent < 4 && line[indent] == ' ' {
			indent++
		}
		if indent <= 3 && indent < len(line) && (line[indent] == '`' || line[indent] == '~') {
			marker := line[indent]
			count := 0
			for indent+count < len(line) && line[indent+count] == marker {
				count++
			}
			rest := line[indent+count:]
			if fence == 0 && count >= 3 && !(marker == '`' && strings.Contains(rest, "`")) {
				fence, fenceLen = marker, count
				if strings.TrimSpace(rest) == "" {
					line += "text"
				}
			} else if fence == marker && count >= fenceLen && strings.TrimSpace(rest) == "" {
				fence, fenceLen = 0, 0
			}
		}

		out.WriteString(line)
		out.WriteString(ending)
	}
	return out.String()
}

// markdownWithHangingOrderedLists aligns wrapped list content after its numeric
// marker. Glamour wraps ordered-list continuations at the list's left edge.
func markdownWithHangingOrderedLists(rendered string, width int) string {
	lines := strings.Split(rendered, "\n")
	out := make([]string, 0, len(lines))
	indent := 0
	for _, line := range lines {
		if clean, prefixWidth, ok := orderedListPrefix(line); ok {
			indent = prefixWidth
			out = append(out, trimRightANSIWhitespace(clean))
			continue
		}
		if strings.TrimSpace(ansi.Strip(line)) == "" {
			indent = 0
			out = append(out, line)
			continue
		}
		if indent == 0 {
			out = append(out, line)
			continue
		}

		wrapped := ansi.Wrap(trimANSIWhitespace(line), max(1, width-indent), "")
		chunks := strings.Split(strings.TrimSuffix(wrapped, "\n"), "\n")
		for _, chunk := range chunks {
			out = append(out, strings.Repeat(" ", indent)+chunk)
		}
	}
	return strings.Join(out, "\n")
}

func orderedListPrefix(line string) (string, int, bool) {
	marker := strings.Index(line, orderedListMarker)
	if marker < 0 {
		return line, 0, false
	}
	prefixWidth := ansi.StringWidth(line[:marker])
	return strings.Replace(line, orderedListMarker, "", 1), prefixWidth, true
}

func trimRightANSIWhitespace(line string) string {
	plain := ansi.Strip(line)
	trimmed := strings.TrimRight(plain, " \t")
	if trimmed == plain {
		return line
	}
	return ansi.Cut(line, 0, ansi.StringWidth(trimmed))
}

func trimANSIWhitespace(line string) string {
	plain := ansi.Strip(line)
	trimmed := strings.TrimSpace(plain)
	if trimmed == plain {
		return line
	}
	left := ansi.StringWidth(plain) - ansi.StringWidth(strings.TrimLeft(plain, " \t"))
	return ansi.Cut(line, left, left+ansi.StringWidth(trimmed))
}

// glamourStyle picks dark/light WITHOUT querying the terminal (no OSC leak).
func glamourStyle() string {
	if s := os.Getenv("TUIPOC_STYLE"); s == "light" || s == "dark" {
		return s
	}
	if fgbg := os.Getenv("COLORFGBG"); fgbg != "" {
		parts := strings.Split(fgbg, ";")
		if last := parts[len(parts)-1]; last == "7" || last == "15" {
			return "light"
		}
	}
	return "dark"
}
