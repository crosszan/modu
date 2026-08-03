package modutui

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/yuin/goldmark"
	goldast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	tableast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

var markdownTableParser = goldmark.New(goldmark.WithExtensions(extension.Table))

type markdownTableRenderSegment struct {
	start  int
	stop   int
	rows   [][]string
	aligns []lipgloss.Position
}

func renderMarkdownWithBorderedTables(renderer MarkdownRenderer, content string, width int) (string, error) {
	source := []byte(content)
	tables := extractMarkdownTables(source)
	if len(tables) == 0 {
		return renderMarkdownText(renderer, content, width)
	}

	parts := make([]string, 0, len(tables)*2+1)
	offset := 0
	for _, table := range tables {
		if table.start > offset {
			rendered, err := renderMarkdownText(renderer, string(source[offset:table.start]), width)
			if err != nil {
				return "", err
			}
			if strings.TrimSpace(rendered) != "" {
				parts = append(parts, rendered)
			}
		}

		rendered := TableBlock{Rows: table.rows, Aligns: table.aligns}.renderBody(width)
		if rendered != "" {
			parts = append(parts, rendered)
		}
		offset = table.stop
	}
	if offset < len(source) {
		rendered, err := renderMarkdownText(renderer, string(source[offset:]), width)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(rendered) != "" {
			parts = append(parts, rendered)
		}
	}
	return strings.Join(parts, "\n\n"), nil
}

func renderMarkdownText(renderer MarkdownRenderer, text string, width int) (string, error) {
	text = strings.Trim(text, "\n")
	if text == "" {
		return "", nil
	}
	if renderer == nil {
		return text, nil
	}
	normalized := markdownSoftBreaksAsSpaces(text)
	normalized = markdownWithCJKBreakOpportunities(normalized)
	out, err := renderer.Render(markdownWithPlaintextFences(normalized))
	if err != nil {
		return "", err
	}
	out = markdownWithHangingOrderedLists(strings.Trim(out, "\n"), width)
	return strings.ReplaceAll(out, markdownBreakSpace, ""), nil
}

// markdownWithCJKBreakOpportunities adds removable thin spaces after Chinese
// separator, sentence-ending, and closing punctuation. Consecutive punctuation
// stays together so a continuation line does not begin with a closing mark.
func markdownWithCJKBreakOpportunities(markdown string) string {
	const breakPunctuation = "、，；：。！？）】》」』”’"
	if !strings.ContainsAny(markdown, breakPunctuation) {
		return markdown
	}

	source := []byte(markdown)
	doc := markdownTableParser.Parser().Parse(text.NewReader(source))
	var positions []int
	_ = goldast.Walk(doc, func(node goldast.Node, entering bool) (goldast.WalkStatus, error) {
		textNode, ok := node.(*goldast.Text)
		if !entering || !ok || textNode.Parent().Kind() == goldast.KindCodeSpan {
			return goldast.WalkContinue, nil
		}

		value := textNode.Segment.Value(source)
		for offset := 0; offset < len(value); {
			r, size := utf8.DecodeRune(value[offset:])
			offset += size
			if !strings.ContainsRune(breakPunctuation, r) {
				continue
			}
			next, _ := utf8.DecodeRune(value[offset:])
			if !strings.ContainsRune(breakPunctuation, next) {
				positions = append(positions, textNode.Segment.Start+offset)
			}
		}
		return goldast.WalkContinue, nil
	})
	if len(positions) == 0 {
		return markdown
	}

	var out strings.Builder
	offset := 0
	for _, position := range positions {
		out.Write(source[offset:position])
		out.WriteString(markdownBreakSpace)
		offset = position
	}
	out.Write(source[offset:])
	return out.String()
}

func markdownSoftBreaksAsSpaces(markdown string) string {
	source := []byte(markdown)
	doc := markdownTableParser.Parser().Parse(text.NewReader(source))
	type replacement struct {
		start int
		stop  int
	}
	var replacements []replacement
	_ = goldast.Walk(doc, func(node goldast.Node, entering bool) (goldast.WalkStatus, error) {
		textNode, ok := node.(*goldast.Text)
		if !entering || !ok || !textNode.SoftLineBreak() || textNode.HardLineBreak() {
			return goldast.WalkContinue, nil
		}

		start := textNode.Segment.Stop
		stop := start
		for stop < len(source) && source[stop] != '\n' && source[stop] != '\r' {
			stop++
		}
		if stop >= len(source) {
			return goldast.WalkContinue, nil
		}
		if source[stop] == '\r' {
			stop++
			if stop < len(source) && source[stop] == '\n' {
				stop++
			}
		} else {
			stop++
		}
		for stop < len(source) && (source[stop] == ' ' || source[stop] == '\t') {
			stop++
		}
		replacements = append(replacements, replacement{start: start, stop: stop})
		return goldast.WalkContinue, nil
	})
	if len(replacements) == 0 {
		return markdown
	}

	var out strings.Builder
	offset := 0
	for _, replacement := range replacements {
		out.Write(source[offset:replacement.start])
		out.WriteByte(' ')
		offset = replacement.stop
	}
	out.Write(source[offset:])
	return out.String()
}

func extractMarkdownTables(source []byte) []markdownTableRenderSegment {
	doc := markdownTableParser.Parser().Parse(text.NewReader(source))
	var tables []markdownTableRenderSegment
	_ = goldast.Walk(doc, func(node goldast.Node, entering bool) (goldast.WalkStatus, error) {
		if !entering {
			return goldast.WalkContinue, nil
		}
		tableNode, ok := node.(*tableast.Table)
		if !ok {
			return goldast.WalkContinue, nil
		}
		segment, ok := tableRenderSegmentFromAST(tableNode, source)
		if ok {
			tables = append(tables, segment)
		}
		return goldast.WalkSkipChildren, nil
	})
	return tables
}

func tableRenderSegmentFromAST(tableNode *tableast.Table, source []byte) (markdownTableRenderSegment, bool) {
	rows := make([][]string, 0, tableNode.ChildCount())
	aligns := make([]lipgloss.Position, 0, len(tableNode.Alignments))
	start := len(source)
	stop := 0

	for _, alignment := range tableNode.Alignments {
		aligns = append(aligns, tableAlignmentToLipgloss(alignment))
	}

	for rowNode := tableNode.FirstChild(); rowNode != nil; rowNode = rowNode.NextSibling() {
		row := make([]string, 0, rowNode.ChildCount())
		for cellNode := rowNode.FirstChild(); cellNode != nil; cellNode = cellNode.NextSibling() {
			cell, ok := cellNode.(*tableast.TableCell)
			if !ok {
				continue
			}
			row = append(row, strings.TrimSpace(string(cell.Text(source))))
			cellStart, cellStop, ok := nodeSourceRange(cell)
			if ok {
				if cellStart < start {
					start = cellStart
				}
				if cellStop > stop {
					stop = cellStop
				}
			}
		}
		rows = append(rows, row)
	}

	if len(rows) == 0 || start > stop {
		return markdownTableRenderSegment{}, false
	}
	start = lineStart(source, start)
	stop = lineStop(source, stop)
	if len(rows) == 1 {
		stop = lineStop(source, stop+1)
	}
	return markdownTableRenderSegment{start: start, stop: stop, rows: rows, aligns: aligns}, true
}

func nodeSourceRange(node goldast.Node) (int, int, bool) {
	lines := node.Lines()
	if lines.Len() == 0 {
		return 0, 0, false
	}
	start := lines.At(0).Start
	stop := lines.At(lines.Len() - 1).Stop
	return start, stop, true
}

func lineStart(source []byte, pos int) int {
	if pos > len(source) {
		pos = len(source)
	}
	for pos > 0 && source[pos-1] != '\n' {
		pos--
	}
	return pos
}

func lineStop(source []byte, pos int) int {
	if pos > len(source) {
		pos = len(source)
	}
	for pos < len(source) && source[pos] != '\n' {
		pos++
	}
	if pos < len(source) {
		pos++
	}
	return pos
}

func tableAlignmentToLipgloss(alignment tableast.Alignment) lipgloss.Position {
	switch alignment {
	case tableast.AlignRight:
		return lipgloss.Right
	case tableast.AlignCenter:
		return lipgloss.Center
	default:
		return lipgloss.Left
	}
}
