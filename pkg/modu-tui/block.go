package modutui

type RenderedLine struct {
	Text string
	// CopyBlock carries a semantic replacement for a contiguous rendered
	// block. When the user selects the whole block, copy uses this text
	// instead of terminal-only decoration such as table borders.
	CopyBlock string
	Gutter    int
}

type BlockRender struct {
	Lines []RenderedLine
}

type MarkdownRenderer interface {
	Render(string) (string, error)
}

type RenderContext struct {
	ContentWidth int
	Markdown     MarkdownRenderer
}

type Block interface {
	Render(RenderContext) BlockRender
}

func (r *BlockRender) Add(text string, gutter int) {
	r.Lines = append(r.Lines, RenderedLine{Text: text, Gutter: gutter})
}
