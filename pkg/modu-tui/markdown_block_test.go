package modutui

import (
	"strings"
	"testing"
)

func TestMarkdownBlockRendersMarkdown(t *testing.T) {
	block := MarkdownBlock{Marker: botStyle.Render("● "), Text: "**bold**"}
	rendered := block.Render(RenderContext{ContentWidth: 40, Markdown: markdownRenderer(40)})
	got := strings.Join(renderedTexts(rendered), "\n")
	if !strings.Contains(got, "bold") {
		t.Fatalf("markdown block missing rendered content:\n%s", got)
	}
}

func TestMarkdownBlockReflowsSoftLineBreaks(t *testing.T) {
	block := MarkdownBlock{
		Marker: botStyle.Render("● "),
		Text: "1. Blitz 使用的\n" +
			"   API (GetTool, CreateSession, GetSession, ListSessions) 的输入输出 struct 在两种接口中完全一致\n" +
			"2. 第二项",
	}
	lines := renderedTexts(block.Render(RenderContext{
		ContentWidth: 72,
		Markdown:     markdownRenderer(72),
	}))

	firstItem := ""
	for _, line := range lines {
		if strings.Contains(line, "1. Blitz") {
			firstItem = line
			break
		}
	}
	if !strings.Contains(firstItem, "API") {
		t.Fatalf("soft line break should reflow within the first list item:\n%s", strings.Join(lines, "\n"))
	}
	if strings.Contains(strings.Join(lines, "\n"), orderedListMarker) {
		t.Fatalf("internal ordered-list marker leaked into rendered output:\n%s", strings.Join(lines, "\n"))
	}
}

func TestMarkdownBlockCanWrapAPINamesAtChinesePunctuation(t *testing.T) {
	block := MarkdownBlock{
		Marker: botStyle.Render("● "),
		Text: "1. Blitz 使用的\n" +
			"   API（GetTool、CreateSession、GetSession、ListSessions、InnerGetPrivateLinkEndpoint）的输入输出一致",
	}
	lines := renderedTexts(block.Render(RenderContext{
		ContentWidth: 72,
		Markdown:     markdownRenderer(72),
	}))

	var compact strings.Builder
	firstLineUsesRemainingWidth := false
	for _, line := range lines {
		compact.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "1. ")))
		if strings.Contains(line, "1. Blitz") {
			if !strings.Contains(line, "API（") {
				t.Fatalf("API name group should use the remaining width before wrapping:\n%s", strings.Join(lines, "\n"))
			}
			firstLineUsesRemainingWidth = true
		}
	}
	if !firstLineUsesRemainingWidth {
		t.Fatalf("first ordered-list line missing:\n%s", strings.Join(lines, "\n"))
	}
	if !strings.Contains(compact.String(), "API（GetTool、CreateSession、GetSession、ListSessions、InnerGetPrivateLinkEndpoint）") {
		t.Fatalf("wrapping should not add spaces inside the API name group:\n%s", strings.Join(lines, "\n"))
	}
	if strings.Contains(compact.String(), markdownBreakSpace) {
		t.Fatalf("internal CJK break marker leaked into rendered output:\n%s", strings.Join(lines, "\n"))
	}
}

func TestMarkdownBlockUsesHangingIndentForWrappedOrderedListItem(t *testing.T) {
	block := MarkdownBlock{
		Marker: botStyle.Render("● "),
		Text:   "1. alpha beta gamma delta epsilon zeta eta theta",
	}
	lines := renderedTexts(block.Render(RenderContext{
		ContentWidth: 24,
		Markdown:     markdownRenderer(24),
	}))

	if len(lines) < 2 {
		t.Fatalf("expected ordered list item to wrap:\n%s", strings.Join(lines, "\n"))
	}
	itemLine := -1
	for i, line := range lines {
		if strings.Contains(line, "1. alpha") {
			itemLine = i
			break
		}
	}
	if itemLine < 0 || itemLine+1 >= len(lines) || !strings.HasPrefix(lines[itemLine+1], "     ") {
		t.Fatalf("ordered list continuation should align after the list marker:\n%s", strings.Join(lines, "\n"))
	}
}

func TestMarkdownOrderedListIndentDoesNotChangeCodeLines(t *testing.T) {
	source := "1. code-shaped text\nplain continuation"
	if got := markdownWithHangingOrderedLists(source, 24); got != source {
		t.Fatalf("ordered-list indentation should only process renderer-marked list items:\n%s", got)
	}
}

func TestMarkdownCJKBreakOpportunitiesSkipCode(t *testing.T) {
	source := "甲、乙，丙；丁：戊。己！庚？！”辛）壬】癸《末》 and `code、value`\n\n```\nfenced，code\n```"
	want := "甲、" + markdownBreakSpace +
		"乙，" + markdownBreakSpace +
		"丙；" + markdownBreakSpace +
		"丁：" + markdownBreakSpace +
		"戊。" + markdownBreakSpace +
		"己！" + markdownBreakSpace +
		"庚？！”" + markdownBreakSpace +
		"辛）" + markdownBreakSpace +
		"壬】" + markdownBreakSpace +
		"癸《末》" + markdownBreakSpace +
		" and `code、value`\n\n```\nfenced，code\n```"
	if got := markdownWithCJKBreakOpportunities(source); got != want {
		t.Fatalf("CJK break opportunities should only be added to prose:\nwant:\n%q\n\ngot:\n%q", want, got)
	}
}

func TestMarkdownSoftLineBreaksPreserveStructuralAndHardBreaks(t *testing.T) {
	source := "paragraph\ncontinued\n\n" +
		"1. first\n" +
		"2. second\n\n" +
		"hard break  \nkept\n\n" +
		"```\ncode\nlines\n```"
	want := "paragraph continued\n\n" +
		"1. first\n" +
		"2. second\n\n" +
		"hard break  \nkept\n\n" +
		"```\ncode\nlines\n```"

	if got := markdownSoftBreaksAsSpaces(source); got != want {
		t.Fatalf("soft line break normalization changed structural or hard breaks:\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestMarkdownInlineCodeDoesNotRenderAsRedBackgroundBlock(t *testing.T) {
	style := markdownStyleConfig()
	if style.Code.BackgroundColor != nil {
		t.Fatalf("inline code background should be disabled, got %q", *style.Code.BackgroundColor)
	}
	if style.Code.Color != nil {
		t.Fatalf("inline code foreground should use surrounding text color, got %q", *style.Code.Color)
	}
	if style.Code.Prefix != "" || style.Code.Suffix != "" {
		t.Fatalf("inline code should not add padding, prefix=%q suffix=%q", style.Code.Prefix, style.Code.Suffix)
	}

	block := MarkdownBlock{
		Marker: botStyle.Render("● "),
		Text:   "Commit: `233945e` on branch `codex/modu-code-modu-tui` Subject: `feat(modu-code): migrate`",
	}
	rendered := block.Render(RenderContext{ContentWidth: 120, Markdown: markdownRenderer(120)})
	got := strings.Join(renderedTexts(rendered), "\n")
	if !strings.Contains(got, "233945e") || !strings.Contains(got, "feat(modu-code): migrate") {
		t.Fatalf("commit summary lost inline code text:\n%s", got)
	}
	if strings.Contains(got, "\x1b[48;5;236m") || strings.Contains(got, "\x1b[38;5;203m") {
		t.Fatalf("commit summary should not use glamour inline-code red/background style:\n%q", got)
	}
}

func TestMarkdownUnlabelledFlowCodeBlockRendersAsPlaintext(t *testing.T) {
	block := MarkdownBlock{
		Marker: botStyle.Render("● "),
		Text: "```\n" +
			"创建工具请求\n" +
			"  → toolimage.ValidateToolType() // 校验 ToolType\n" +
			"       └── 未预热 → 调用 PrecacheSandboxImages 发起预热\n" +
			"```",
	}
	normalized := markdownWithPlaintextFences(block.Text)
	if !strings.HasPrefix(normalized, "```text\n") {
		t.Fatalf("unlabelled code fence should default to text:\n%s", normalized)
	}
	rendered := block.Render(RenderContext{ContentWidth: 100, Markdown: markdownRenderer(100)})
	got := strings.Join(renderedTexts(rendered), "\n")
	if strings.Contains(got, "\x1b[48;5;203m") || strings.Contains(got, "\x1b[48;2;240;91;91m") {
		t.Fatalf("unlabelled flow code block should not emit Chroma error backgrounds:\n%q", got)
	}
}

func TestMarkdownPlaintextFenceNormalizationPreservesExplicitLanguage(t *testing.T) {
	source := "```go\nfmt.Println(\"ok\")\n```\n\n~~~json\n{}\n~~~"
	if got := markdownWithPlaintextFences(source); got != source {
		t.Fatalf("explicit code fence languages changed:\n%s", got)
	}
}

func TestMarkdownBlockRendersTableWithBorders(t *testing.T) {
	block := MarkdownBlock{
		Marker: botStyle.Render("● "),
		Text:   "| Name | Count |\n| --- | ---: |\n| apple | 12 |\n| banana | 3 |",
	}
	rendered := block.Render(RenderContext{ContentWidth: 60, Markdown: markdownRenderer(60)})
	got := strings.Join(renderedTexts(rendered), "\n")

	for _, want := range []string{"┌", "┬", "└", "│"} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered table missing border %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "| --- |") {
		t.Fatalf("rendered table leaked markdown delimiter:\n%s", got)
	}
}

func TestMarkdownBlockDoesNotRenderTablesInsideCodeFence(t *testing.T) {
	block := MarkdownBlock{
		Marker: botStyle.Render("● "),
		Text:   "```\n| not | table |\n| --- | --- |\n```",
	}
	rendered := block.Render(RenderContext{ContentWidth: 60, Markdown: markdownRenderer(60)})
	got := strings.Join(renderedTexts(rendered), "\n")

	if strings.Contains(got, "┌") || strings.Contains(got, "└") {
		t.Fatalf("code fence table text should not render as a table:\n%s", got)
	}
	if !strings.Contains(got, "| not | table |") {
		t.Fatalf("code fence content missing:\n%s", got)
	}
}
