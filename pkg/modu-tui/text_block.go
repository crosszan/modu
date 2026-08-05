package modutui

type TextBlock struct {
	Marker string
	Text   string
	// Dim renders the text faintly, for status/lifecycle lines that should
	// recede behind actual conversation content.
	Dim bool
}

func (b TextBlock) Render(ctx RenderContext) BlockRender {
	style := func(s string) string { return s }
	if b.Dim {
		style = func(s string) string { return statusStyle.Render(s) }
	}
	return bodyLines(b.Marker, b.Text, max(1, ctx.ContentWidth), style)
}
