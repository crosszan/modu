package modutui

type MarkdownBlock struct {
	Marker string
	Text   string
}

func (b MarkdownBlock) Render(ctx RenderContext) BlockRender {
	width := max(1, ctx.ContentWidth)
	segments, err := renderMarkdownSegments(ctx.Markdown, b.Text, width)
	if err != nil {
		return bodyLines(b.Marker, b.Text, width, func(s string) string { return s })
	}
	if len(segments) == 0 {
		return bodyLines(b.Marker, "", width, func(s string) string { return s })
	}

	var out BlockRender
	for index, segment := range segments {
		marker := b.Marker
		if index > 0 {
			out.Add("  ", 2)
			marker = "  "
		}
		rendered := bodyLines(marker, segment.body, width, func(s string) string { return s })
		for i := range rendered.Lines {
			rendered.Lines[i].CopyBlock = segment.copyBlock
		}
		out.Lines = append(out.Lines, rendered.Lines...)
	}
	return out
}
