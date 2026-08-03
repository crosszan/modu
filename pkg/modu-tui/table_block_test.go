package modutui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestTableBlockRendersBorders(t *testing.T) {
	block := TableBlock{
		Marker: botStyle.Render("● "),
		Rows: [][]string{
			{"Name", "Count"},
			{"apple", "12"},
		},
		Aligns: []lipgloss.Position{lipgloss.Left, lipgloss.Right},
	}

	got := strings.Join(renderedTexts(block.Render(RenderContext{ContentWidth: 60})), "\n")
	for _, want := range []string{"┌", "┬", "└", "│"} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered table missing border %q:\n%s", want, got)
		}
	}
}

func TestTableBlockCarriesMarkdownCopySource(t *testing.T) {
	block := TableBlock{
		Rows: [][]string{
			{"Name", "Note", "Count"},
			{"a|b", "line one\nline two", "12"},
		},
		Aligns: []lipgloss.Position{lipgloss.Left, lipgloss.Center, lipgloss.Right},
	}
	rendered := block.Render(RenderContext{ContentWidth: 60})
	want := "| Name | Note | Count |\n" +
		"| --- | :---: | ---: |\n" +
		"| a\\|b | line one<br>line two | 12 |"

	if len(rendered.Lines) == 0 {
		t.Fatal("expected rendered table lines")
	}
	for index, line := range rendered.Lines {
		if line.CopyBlock != want {
			t.Fatalf("line %d copy source = %q, want %q", index, line.CopyBlock, want)
		}
	}
}
