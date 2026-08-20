package modutui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func testTextEntry(role Role, text string) Entry {
	return Entry{Role: role, Nodes: []Node{TextNode{Text: text}}}
}

func testMarkdownEntry(role Role, text string) Entry {
	return Entry{Role: role, Nodes: []Node{MarkdownNode{Text: text}}}
}

func testToolEntry(call ToolCall, permission ToolPermissionState, expanded bool) Entry {
	return Entry{
		ID:   call.ID,
		Role: RoleAssistant,
		Nodes: []Node{ToolNode{
			Call:       call,
			Permission: permission,
			Expanded:   expanded,
		}},
	}
}

func testEntryText(entry Entry) string {
	if len(entry.Nodes) == 0 {
		return ""
	}
	switch node := entry.Nodes[0].(type) {
	case TextNode:
		return node.Text
	case MarkdownNode:
		return node.Text
	default:
		return ""
	}
}

func TestPOC2MarkdownOrderedListReflowsAndUsesHangingIndent(t *testing.T) {
	m := NewModel(Options{
		Width:  44,
		Height: 14,
		InitialEntries: []Entry{testMarkdownEntry(RoleAssistant,
			"1. Blitz 使用的\n"+
				"   API（GetTool、CreateSession、GetSession、ListSessions、InnerGetPrivateLinkEndpoint）的输入输出一致\n"+
				"2. second item",
		)},
	})
	lines := strings.Split(ansi.Strip(strings.Join(m.Lines(), "\n")), "\n")

	itemLine := -1
	for i, line := range lines {
		if strings.Contains(line, "1. Blitz") {
			itemLine = i
			if !strings.Contains(line, "API") {
				t.Fatalf("soft line break should reflow in the transcript:\n%s", strings.Join(lines, "\n"))
			}
			break
		}
	}
	if itemLine < 0 || itemLine+1 >= len(lines) || !strings.HasPrefix(lines[itemLine+1], "     ") {
		t.Fatalf("ordered-list continuation should use a hanging indent in the transcript:\n%s", strings.Join(lines, "\n"))
	}
	if strings.Contains(strings.Join(lines, "\n"), orderedListMarker) {
		t.Fatalf("internal ordered-list marker leaked into the transcript:\n%s", strings.Join(lines, "\n"))
	}
}

type testIntentCallbacks struct {
	submit       func(SubmitEvent)
	interrupt    func()
	approval     func(ToolApprovalResult)
	panelAction  func(PanelAction)
	panelClosed  func(string)
	history      func([]string)
	slashCommand func(string)
}

func testIntentHandler(callbacks testIntentCallbacks) func(Intent) {
	return func(intent Intent) {
		switch intent := intent.(type) {
		case SubmitIntent:
			if callbacks.submit != nil {
				callbacks.submit(intent.Event)
			}
		case InterruptIntent:
			if callbacks.interrupt != nil {
				callbacks.interrupt()
			}
		case ToolApprovalDecisionIntent:
			if callbacks.approval != nil {
				callbacks.approval(intent.Result)
			}
		case PanelActionIntent:
			if callbacks.panelAction != nil {
				callbacks.panelAction(intent.Action)
			}
		case PanelClosedIntent:
			if callbacks.panelClosed != nil {
				callbacks.panelClosed(intent.PanelID)
			}
		case InputHistoryChangedIntent:
			if callbacks.history != nil {
				callbacks.history(intent.History)
			}
		case SlashCommandIntent:
			if callbacks.slashCommand != nil {
				callbacks.slashCommand(intent.Line)
			}
		}
	}
}

func TestPOC2MultilineInputModifiedEnterAndAutoHeight(t *testing.T) {
	for _, tt := range []struct {
		name     string
		modifier tea.KeyMod
	}{
		{name: "shift", modifier: tea.ModShift},
		{name: "alt", modifier: tea.ModAlt},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var tm tea.Model = NewModel(Options{Width: 40, Height: 20})
			tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Code: 'a', Text: "a"}))
			// Shift+Enter follows Codex's enhanced-key behavior; Alt+Enter
			// remains a newline fallback when the terminal distinguishes it.
			tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Mod: tt.modifier}))
			tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Code: 'b', Text: "b"}))
			m := tm.(Model)

			if got := m.input.ExpandedValue(); got != "a\nb" {
				t.Fatalf("input value = %q, want %q", got, "a\nb")
			}
			if got := m.inputRows(); got != 2 {
				t.Fatalf("inputRows = %d, want 2", got)
			}
			if got, want := m.bottomFixedRows(), bottomFixedRowsBase+2; got != want {
				t.Fatalf("bottomFixedRows = %d, want %d", got, want)
			}
			lines, cursorRow, _ := m.input.Render(m.inputRenderWidth(), maxInputRows)
			if len(lines) != 2 {
				t.Fatalf("rendered input lines = %d, want 2", len(lines))
			}
			if cursorRow != 1 {
				t.Fatalf("cursorRow = %d, want 1 (caret on second line)", cursorRow)
			}
			if !strings.Contains(ansi.Strip(lines[0]), "❯") {
				t.Fatalf("first line should carry the ❯ prefix: %q", ansi.Strip(lines[0]))
			}
		})
	}

	// Input height is capped at maxInputRows even with more logical lines.
	m := NewModel(Options{Width: 40, Height: 20})
	for range maxInputRows + 3 {
		m.input.InsertNewline()
	}
	if got := m.inputRows(); got != maxInputRows {
		t.Fatalf("inputRows = %d, want capped at %d", got, maxInputRows)
	}
	capped, _, _ := m.input.Render(m.inputRenderWidth(), maxInputRows)
	if len(capped) != maxInputRows {
		t.Fatalf("rendered input lines = %d, want capped at %d", len(capped), maxInputRows)
	}
}

func TestSpellingSuggestionsReplaceIssueAtCursor(t *testing.T) {
	var checked []string
	var suggested string
	var tm tea.Model = NewModel(Options{
		Width: 60, Height: 20,
		Services: Services{
			CheckSpelling: func(text string) []SpellingIssue {
				checked = append(checked, text)
				if text == "wrld" {
					return []SpellingIssue{{Start: 0, End: 4, Word: "wrld"}}
				}
				return nil
			},
			SuggestSpelling: func(word string, limit int) ([]string, error) {
				suggested = word
				return []string{"world", "weld"}, nil
			},
		},
	})

	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Text: "wrld", Code: 'w'}))
	m := tm.(Model)
	if len(checked) == 0 || len(m.spellingIssues) != 1 {
		t.Fatalf("spell check did not populate issue: checked=%#v issues=%#v", checked, m.spellingIssues)
	}

	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	m = tm.(Model)
	if suggested != "wrld" || len(m.spellingSuggestions) != 2 {
		t.Fatalf("suggestion popup not opened: word=%q suggestions=%#v", suggested, m.spellingSuggestions)
	}
	if popup := ansi.Strip(strings.Join(m.completionPanelLines(), "\n")); !strings.Contains(popup, "world") || !strings.Contains(popup, "拼写建议") {
		t.Fatalf("suggestion popup was not rendered: %q", popup)
	}

	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	if len(tm.(Model).spellingSuggestions) != 0 {
		t.Fatal("Esc should dismiss spelling suggestions")
	}
	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))

	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = tm.(Model)
	if m.input.Value != "weld" || m.input.Cursor != 4 {
		t.Fatalf("replacement result = %q cursor=%d, want weld cursor=4", m.input.Value, m.input.Cursor)
	}
	if len(m.spellingSuggestions) != 0 || len(m.spellingIssues) != 0 {
		t.Fatalf("replacement should refresh spelling state: suggestions=%#v issues=%#v", m.spellingSuggestions, m.spellingIssues)
	}
}

func TestPOC2LongInputSoftWrapsAndIncreasesHeight(t *testing.T) {
	m := NewModel(Options{Width: 18, Height: 12})
	m.input.Insert(strings.Repeat("a", 50))
	if got := m.inputRows(); got <= 1 {
		t.Fatalf("inputRows = %d, want soft-wrapped long input to use more than one row", got)
	}
	lines, cursorRow, _ := m.input.Render(m.inputRenderWidth(), maxInputRows)
	if len(lines) != m.inputRows() {
		t.Fatalf("rendered input lines = %d, want %d", len(lines), m.inputRows())
	}
	if cursorRow != len(lines)-1 {
		t.Fatalf("cursorRow = %d, want last rendered line %d", cursorRow, len(lines)-1)
	}
	for _, line := range lines {
		if ansi.StringWidth(line) > m.inputRenderWidth() {
			t.Fatalf("wrapped input line exceeds width %d: %q", m.inputRenderWidth(), line)
		}
	}
}

func TestPOC2PageKeysScrollViewport(t *testing.T) {
	var tm tea.Model = NewModel()
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 80, Height: 12})

	m := tm.(Model)
	for range 60 {
		m.entries = append(m.entries, testTextEntry(RoleAssistant, "history line"))
	}
	m.follow = true
	m.rebuild()
	if m.yOffset == 0 {
		t.Fatal("setup should be scrollable")
	}

	before := m.yOffset
	tm, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp}))
	afterUp := tm.(Model)
	if afterUp.yOffset >= before {
		t.Fatalf("PageUp did not scroll up: %d -> %d", before, afterUp.yOffset)
	}
	if afterUp.follow {
		t.Fatal("PageUp away from bottom should disable follow")
	}

	tm, _ = afterUp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
	afterDown := tm.(Model)
	if afterDown.yOffset <= afterUp.yOffset {
		t.Fatalf("PageDown did not scroll down: %d -> %d", afterUp.yOffset, afterDown.yOffset)
	}
}

func TestPOC2ResizeClampsSelection(t *testing.T) {
	m := NewModel()
	m.width, m.height = 80, 12
	for range 20 {
		m.entries = append(m.entries, testTextEntry(RoleAssistant, "history line"))
	}
	m.rebuild()
	if len(m.lines) == 0 {
		t.Fatal("setup should produce transcript lines")
	}

	m.selStart = cell{line: 0, col: 0}
	m.selEnd = cell{line: len(m.lines) + 50, col: 999}
	m.entries = []Entry{testTextEntry(RoleAssistant, "short")}
	m.width, m.height = 20, 8
	m.rebuild()

	if !m.hasSelection() {
		t.Fatal("selection should be retained and clamped")
	}
	if m.selStart.line < 0 || m.selStart.line >= len(m.lines) {
		t.Fatalf("selStart line out of range after resize: %+v, lines=%d", m.selStart, len(m.lines))
	}
	if m.selEnd.line < 0 || m.selEnd.line >= len(m.lines) {
		t.Fatalf("selEnd line out of range after resize: %+v, lines=%d", m.selEnd, len(m.lines))
	}
	_ = m.selectedText()
	for i := range m.lines {
		_ = m.highlightLine(i)
	}
}

// Left-hand decoration (the ⏺ marker, the │ continuation, tree branches, the
// summary indent) is gutter, not content: selecting a tool block must copy
// the invocation and output text without dragging the drawing along.
func TestPOC2CopyingToolBlockExcludesLeftDecoration(t *testing.T) {
	m := NewModel(Options{
		Width:  80,
		Height: 16,
		InitialEntries: []Entry{testToolEntry(ToolCall{
			ID: "call-1", Name: "bash", Summary: "Ran 1 shell command",
			Input: "go test ./pkg/modu-tui", Output: "ok ./pkg/modu-tui", Done: true,
		}, ToolPermissionUnknown, true)},
	})

	m.selStart = cell{line: 0, col: 0}
	last := len(m.lines) - 1
	m.selEnd = cell{line: last, col: m.lineWidth(last)}

	got := m.selectedText()
	if got == "" {
		t.Fatal("expected the expanded tool block to yield selectable text")
	}
	for _, decoration := range []string{"⏺", "│"} {
		if strings.Contains(got, decoration) {
			t.Fatalf("copied text should not contain the %q decoration, got:\n%s", decoration, got)
		}
	}
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, " ") {
			t.Fatalf("copied line should start at content, not inside the gutter: %q\nfull:\n%s", line, got)
		}
	}
	if !strings.Contains(got, "go test ./pkg/modu-tui") {
		t.Fatalf("copied text should keep the invocation, got:\n%s", got)
	}
}

func TestPOC2CopyingCompleteMarkdownTableUsesMarkdownSource(t *testing.T) {
	const source = "| Name | Count |\n| --- | ---: |\n| apple | 12 |\n| banana | 3 |"
	m := NewModel(Options{
		Width:          60,
		Height:         16,
		InitialEntries: []Entry{testMarkdownEntry(RoleAssistant, source)},
	})
	if len(m.lines) == 0 {
		t.Fatal("expected rendered table lines")
	}

	last := len(m.lines) - 1
	m.selStart = cell{line: 0, col: m.gutterAt(0)}
	m.selEnd = cell{line: last, col: m.lineWidth(last) - 1}

	if got := m.selectedText(); got != source {
		t.Fatalf("complete table copy = %q, want markdown source %q", got, source)
	}
	oldWrite := writeLocalClipboard
	var copied string
	writeLocalClipboard = func(text string) error {
		copied = text
		return nil
	}
	t.Cleanup(func() { writeLocalClipboard = oldWrite })
	cmd := m.copySelection()
	if cmd == nil {
		t.Fatal("complete table selection should produce a clipboard command")
	}
	result, ok := cmd().(clipboardCopyResultMsg)
	if !ok || !result.copied || copied != source {
		t.Fatalf("clipboard result = %#v, copied %q, want %q", result, copied, source)
	}
}

func TestPOC2CopyingPartialMarkdownTableKeepsVisibleSelection(t *testing.T) {
	const source = "| Name | Count |\n| --- | ---: |\n| apple | 12 |\n| banana | 3 |"
	m := NewModel(Options{
		Width:          60,
		Height:         16,
		InitialEntries: []Entry{testMarkdownEntry(RoleAssistant, source)},
	})
	contentLine := -1
	for index, line := range m.lines {
		if strings.Contains(ansi.Strip(line), "apple") {
			contentLine = index
			break
		}
	}
	if contentLine < 0 {
		t.Fatal("expected rendered apple row")
	}

	m.selStart = cell{line: contentLine, col: m.gutterAt(contentLine)}
	m.selEnd = cell{line: contentLine, col: m.lineWidth(contentLine)}
	got := m.selectedText()
	if !strings.Contains(got, "apple") || !strings.Contains(got, "│") {
		t.Fatalf("partial table selection should keep visible row text, got %q", got)
	}
	if strings.Contains(got, "| --- |") {
		t.Fatalf("partial table selection unexpectedly expanded to markdown table: %q", got)
	}
}

func TestPOC2CopyingMarkdownOmitsRightPadding(t *testing.T) {
	m := NewModel(Options{
		Width:  60,
		Height: 16,
		InitialEntries: []Entry{testMarkdownEntry(RoleAssistant,
			"### Summary\n\n1. first item\n2. second item\n\n```text\n  indented  content\n```",
		)},
	})
	if len(m.lines) == 0 {
		t.Fatal("expected rendered markdown lines")
	}

	m.selStart = cell{line: 0, col: 0}
	last := len(m.lines) - 1
	m.selEnd = cell{line: last, col: m.lineWidth(last)}
	got := m.selectedText()
	for _, line := range strings.Split(got, "\n") {
		if strings.HasSuffix(line, " ") {
			t.Fatalf("copied markdown line contains right-side render padding: %q\nfull copy:\n%q", line, got)
		}
	}
	if !strings.Contains(got, "  indented  content") {
		t.Fatalf("copy should preserve meaningful left indentation and internal spaces:\n%q", got)
	}
}

func TestPOC2CopySelectionUsesOSC52OverSSH(t *testing.T) {
	t.Setenv("SSH_TTY", "/dev/pts/1")
	// Isolate multiplexer env so the sequence is plain OSC52, not screen/tmux
	// DCS-wrapped, regardless of the ambient TERM/TMUX (e.g. running over SSH
	// inside screen).
	t.Setenv("TMUX", "")
	t.Setenv("TERM", "xterm-256color")
	oldWrite := writeLocalClipboard
	writeLocalClipboard = func(string) error { return nil }
	t.Cleanup(func() { writeLocalClipboard = oldWrite })

	m := NewModel(Options{
		Width:          40,
		Height:         8,
		InitialEntries: []Entry{testTextEntry(RoleAssistant, "copy me")},
	})
	m.selStart = cell{line: 0, col: 2}
	m.selEnd = cell{line: 0, col: 9}

	cmd := m.copySelection()
	if cmd == nil {
		t.Fatal("copySelection should return an OSC52 command over SSH")
	}
	tm, finalCmd := m.Update(cmd())
	m = tm.(Model)
	raw, hasSetClipboard := copyCommandMessages(finalCmd)
	if !strings.Contains(raw, "\x1b]52;c;") || !strings.HasSuffix(raw, "\x07") {
		t.Fatalf("raw clipboard sequence should be OSC52, got %q", raw)
	}
	if !hasSetClipboard {
		t.Fatal("copySelection should also send Bubble Tea SetClipboard for SSH compatibility")
	}
	if !strings.Contains(m.status, "local+OSC52") {
		t.Fatalf("copy status should report OSC52 path, got %q", m.status)
	}
	if !strings.Contains(m.status, "Shift+drag") {
		t.Fatalf("OSC52 copy status should hint at terminal-native Shift+drag fallback, got %q", m.status)
	}
}

func TestPOC2CopySelectionSkipsOSC52WhenSelectionTooLarge(t *testing.T) {
	t.Setenv("SSH_TTY", "/dev/pts/1")
	t.Setenv("TMUX", "")
	t.Setenv("TERM", "xterm-256color")
	oldWrite := writeLocalClipboard
	writeLocalClipboard = func(string) error { return nil }
	t.Cleanup(func() { writeLocalClipboard = oldWrite })

	// A touch-drag selection extended by edge auto-scroll can span most of a
	// long scrollback; simulate that with enough lines to clear the cap
	// regardless of per-line wrapping.
	rawLines := make([]string, 5000)
	for i := range rawLines {
		rawLines[i] = "xxxxxxxxxxxxxxxxxxxx"
	}
	m := NewModel(Options{
		Width:          40,
		Height:         8,
		InitialEntries: []Entry{testTextEntry(RoleAssistant, strings.Join(rawLines, "\n"))},
	})
	m.selStart = cell{line: 0, col: 0}
	lastLine := len(m.lines) - 1
	m.selEnd = cell{line: lastLine, col: m.lineWidth(lastLine)}

	cmd := m.copySelection()
	if cmd == nil {
		t.Fatal("copySelection should still return a command so the local clipboard write happens")
	}
	tm, finalCmd := m.Update(cmd())
	m = tm.(Model)
	if finalCmd != nil {
		t.Fatal("an oversized selection has nothing left to send once OSC52 is skipped, so Update should return a nil command")
	}
	if strings.Contains(m.status, "OSC52") {
		t.Fatalf("oversized selection should not report an OSC52 path, got %q", m.status)
	}
	if !strings.Contains(m.status, "too large") {
		t.Fatalf("status should explain the selection was too large for terminal clipboard, got %q", m.status)
	}
}

func TestPOC2CopySelectionUsesTmuxPassthrough(t *testing.T) {
	t.Setenv("SSH_TTY", "/dev/pts/1")
	t.Setenv("TMUX", "/tmp/tmux")

	seq := clipboardSequence("hi")
	if !strings.Contains(seq, "\x1bPtmux;") || !strings.Contains(seq, "52;c;") {
		t.Fatalf("tmux clipboard sequence missing passthrough wrapper: %q", seq)
	}
}

func TestPOC2ClipboardSequenceScreenTermEmitsBothWrappings(t *testing.T) {
	// TERM=screen-256color is also tmux's default TERM, and over SSH only
	// TERM (not TMUX) is forwarded from the local side — so the actual
	// multiplexer is unknowable. Both wrappings must be emitted so whichever
	// one is really there unwraps its own format.
	t.Setenv("TMUX", "")
	t.Setenv("TERM", "screen-256color")

	seq := clipboardSequence("hi")
	if !strings.Contains(seq, "\x1bPtmux;") {
		t.Fatalf("screen TERM sequence should include tmux passthrough wrapping: %q", seq)
	}
	if !strings.HasPrefix(seq, "\x1bP") || strings.HasPrefix(seq, "\x1bPtmux;") {
		t.Fatalf("screen TERM sequence should start with a screen DCS chunk: %q", seq)
	}
}

func TestPOC2CopySelectionUsesOSC52InsideTmuxWithoutSSHEnv(t *testing.T) {
	// Reattaching to a tmux session over SSH after it was created locally
	// leaves SSH_TTY/SSH_CONNECTION/SSH_CLIENT unset inside the pane, even
	// though the attached client may now be remote. isRemoteSession must
	// treat "inside tmux" itself as reason enough to try OSC52, or a
	// successful local clipboard write on the tmux host would be mistaken
	// for a successful copy to the actual (possibly remote) client.
	t.Setenv("SSH_TTY", "")
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_CLIENT", "")
	t.Setenv("TMUX", "/tmp/tmux")
	oldWrite := writeLocalClipboard
	writeLocalClipboard = func(string) error { return nil }
	t.Cleanup(func() { writeLocalClipboard = oldWrite })

	m := NewModel(Options{
		Width:          40,
		Height:         8,
		InitialEntries: []Entry{testTextEntry(RoleAssistant, "copy me")},
	})
	m.selStart = cell{line: 0, col: 2}
	m.selEnd = cell{line: 0, col: 9}

	cmd := m.copySelection()
	if cmd == nil {
		t.Fatal("copySelection should still try OSC52 inside tmux even without SSH env vars")
	}
	tm, finalCmd := m.Update(cmd())
	m = tm.(Model)
	raw, _ := copyCommandMessages(finalCmd)
	if !strings.Contains(raw, "\x1bPtmux;") || !strings.Contains(raw, "52;c;") {
		t.Fatalf("expected tmux-wrapped OSC52 sequence, got %q", raw)
	}
}

func TestPOC2CopySelectionUsesLocalClipboardWithoutOSC52WhenLocalSucceeds(t *testing.T) {
	// This case asserts the non-remote path, so clear any inherited SSH/tmux/
	// screen env (e.g. when the test itself runs over SSH or inside tmux).
	t.Setenv("SSH_TTY", "")
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_CLIENT", "")
	t.Setenv("TMUX", "")
	t.Setenv("STY", "")
	t.Setenv("TERM", "xterm-256color")
	oldWrite := writeLocalClipboard
	writeLocalClipboard = func(string) error { return nil }
	t.Cleanup(func() { writeLocalClipboard = oldWrite })

	m := NewModel(Options{
		Width:          40,
		Height:         8,
		InitialEntries: []Entry{testTextEntry(RoleAssistant, "copy me")},
	})
	m.selStart = cell{line: 0, col: 2}
	m.selEnd = cell{line: 0, col: 9}

	cmd := m.copySelection()
	if cmd == nil {
		t.Fatal("local clipboard copy should execute outside Update")
	}
	tm, finalCmd := m.Update(cmd())
	m = tm.(Model)
	if finalCmd != nil {
		t.Fatalf("local successful clipboard copy should not emit OSC52 command, got %#v", finalCmd())
	}
	if !strings.Contains(m.status, "(clipboard)") {
		t.Fatalf("copy status should report local clipboard path, got %q", m.status)
	}
}

func TestPOC2RenderConstrainsLineWidths(t *testing.T) {
	m := NewModel()
	m.width, m.height = 24, 8
	m.entries = []Entry{
		testTextEntry(RoleUser, strings.Repeat("a", 120)),
		testToolEntry(ToolCall{Summary: strings.Repeat("tool", 30), Detail: strings.Repeat("detail", 30)}, ToolPermissionUnknown, true),
	}
	m.input.Value = strings.Repeat("input", 30)
	m.input.Cursor = m.input.Len()
	m.rebuild()

	for i, line := range strings.Split(m.render(), "\n") {
		if got := ansi.StringWidth(line); got > m.width {
			t.Fatalf("render line %d width = %d, want <= %d: %q", i, got, m.width, line)
		}
	}
}

func TestPOC2RenderPadsEveryLineToTerminalWidth(t *testing.T) {
	m := NewModel(Options{
		Width:  32,
		Height: 10,
		InitialEntries: []Entry{
			testMarkdownEntry(RoleAssistant, "short"),
		},
	})

	lines := strings.Split(m.render(), "\n")
	if got, want := len(lines), m.height; got != want {
		t.Fatalf("rendered line count = %d, want %d", got, want)
	}
	inputRow := m.vpHeight() + 3
	for i, line := range lines {
		stripped := ansi.Strip(strings.TrimSuffix(line, "\x1b[K"))
		want := m.width
		if i == inputRow {
			want = m.inputRenderWidth()
		}
		if got := ansi.StringWidth(stripped); got != want {
			t.Fatalf("render line %d width = %d, want %d: %q", i, got, want, line)
		}
	}
}

func TestSpinnerAnimatesWhileBusyAndStopsWhenIdle(t *testing.T) {
	m := NewModel(Options{Width: 56, Height: 8})
	m.busy = true

	tm, cmd := m.Update(spinnerTickMsg{})
	m = tm.(Model)
	if m.spinnerFrame != 1 {
		t.Fatalf("spinnerFrame after one tick = %d, want 1", m.spinnerFrame)
	}
	if cmd == nil {
		t.Fatal("still busy: spinnerTickMsg should re-arm the next tick")
	}
	first := ansi.Strip(m.render())
	if !strings.Contains(first, spinnerFrames[1]) {
		t.Fatalf("rendered status should show spinner frame %q, got:\n%s", spinnerFrames[1], first)
	}

	m.busy = false
	tm, cmd = m.Update(spinnerTickMsg{})
	m = tm.(Model)
	if cmd != nil {
		t.Fatal("idle: spinnerTickMsg should stop re-arming once busy and streaming both end")
	}
	if m.spinnerRunning {
		t.Fatal("spinnerRunning should clear once the loop stops")
	}
}

func TestPOC2RenderPlacesAgentStatusAboveInputAndFooterAtBottom(t *testing.T) {
	m := NewModel(Options{
		Width:  56,
		Height: 8,
		Footer: "ctx 1K/10K · test · …/repo",
	})
	m.busy = true
	rendered := ansi.Strip(m.render())
	lines := strings.Split(rendered, "\n")
	if len(lines) != m.height {
		t.Fatalf("rendered lines = %d, want %d:\n%s", len(lines), m.height, rendered)
	}
	gapRow := lines[len(lines)-6]
	statusRow := lines[len(lines)-5]
	inputRow := lines[len(lines)-3]
	footerRow := lines[len(lines)-1]
	if strings.TrimSpace(gapRow) != "" {
		t.Fatalf("agent status should have a blank row above it, got %q in:\n%s", gapRow, rendered)
	}
	if !strings.Contains(statusRow, "running") {
		t.Fatalf("agent status should render above input, got %q in:\n%s", statusRow, rendered)
	}
	if !strings.Contains(inputRow, "❯") {
		t.Fatalf("input row should remain between rules, got %q in:\n%s", inputRow, rendered)
	}
	if !strings.Contains(footerRow, "ctx 1K/10K") || !strings.Contains(footerRow, "test") {
		t.Fatalf("footer should render at bottom, got %q in:\n%s", footerRow, rendered)
	}
	if strings.HasPrefix(statusRow, " ") {
		t.Fatalf("status row should start flush at column 0, aligned with the input row's \"❯\", got %q", statusRow)
	}
	if !strings.HasPrefix(inputRow, "❯") {
		t.Fatalf("input row should start flush at column 0, got %q", inputRow)
	}
	if strings.HasPrefix(footerRow, " ") {
		t.Fatalf("footer row should start flush at column 0, aligned with the input row's \"❯\", got %q", footerRow)
	}
}

func TestPOC2EscInterruptsRunningAgent(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tea.Key
	}{
		{name: "key code", key: tea.Key{Code: tea.KeyEsc}},
		{name: "legacy ctrl bracket", key: tea.Key{Code: '[', Mod: tea.ModCtrl}},
		{name: "raw text", key: tea.Key{Text: "\x1b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			interrupted := false
			var tm tea.Model = NewModel(Options{
				Width:  40,
				Height: 8,
				IntentHandler: testIntentHandler(testIntentCallbacks{interrupt: func() {
					interrupted = true
				}}),
			})
			m := tm.(Model)
			m.busy = true

			tm = updateAndRunImmediate(t, m, tea.KeyPressMsg(tc.key))
			m = tm.(Model)
			if !interrupted {
				t.Fatal("esc should emit interrupt intent while busy")
			}
			if got, want := m.status, "interrupting"; got != want {
				t.Fatalf("status = %q, want %q", got, want)
			}
		})
	}
}

func TestPOC2CtrlCQuitsWithSSHKeyShapes(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tea.Key
	}{
		{name: "ctrl modifier", key: tea.Key{Code: 'c', Mod: tea.ModCtrl}},
		{name: "raw text", key: tea.Key{Text: "\x03"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var tm tea.Model = NewModel(Options{Width: 40, Height: 8})
			_, cmd := tm.Update(tea.KeyPressMsg(tc.key))
			requireQuitCmd(t, cmd)
		})
	}
}

func TestPOC2CtrlCClearsNonEmptyInputBeforeQuit(t *testing.T) {
	var tm tea.Model = NewModel(Options{
		Width:         40,
		Height:        8,
		SlashCommands: []SlashCommand{{Name: "/goal", Description: "Set a goal"}},
	})
	tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Text: "/go"}))
	m := tm.(Model)
	if m.input.Value == "" || len(m.slashMatches) == 0 {
		t.Fatalf("setup should have input and slash matches: input=%q matches=%#v", m.input.Value, m.slashMatches)
	}

	tm, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
	if cmd != nil {
		t.Fatalf("ctrl+c with input should clear input, not quit: %#v", cmd)
	}
	m = tm.(Model)
	if m.input.Value != "" || len(m.slashMatches) != 0 {
		t.Fatalf("ctrl+c should clear input and slash matches, input=%q matches=%#v", m.input.Value, m.slashMatches)
	}

	_, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
	requireQuitCmd(t, cmd)
}

func TestPOC2ApprovalEscDeniesWithSSHKeyShape(t *testing.T) {
	decisions := make(chan ToolApprovalDecision, 1)
	var tm tea.Model = NewModel(Options{Width: 40, Height: 8})
	tm, _ = tm.Update(RequestToolApprovalMsg{
		Request: ToolApprovalRequest{ID: "call-1", ToolName: "bash"},
		Respond: decisions,
	})

	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: '[', Mod: tea.ModCtrl}))
	if tm.(Model).approval != nil {
		t.Fatal("approval should clear after esc")
	}
	select {
	case got := <-decisions:
		if got != ToolApprovalDeny {
			t.Fatalf("decision = %q, want %q", got, ToolApprovalDeny)
		}
	case <-time.After(time.Second):
		t.Fatal("expected approval decision")
	}
}

func TestPOC2CompletionStatusDoesNotShowIdlePrefix(t *testing.T) {
	m := NewModel(Options{Width: 40, Height: 8})
	m.status = "✓ Completed 2s"
	rendered := ansi.Strip(m.render())
	lines := strings.Split(rendered, "\n")
	statusRow := lines[len(lines)-5]
	if !strings.Contains(statusRow, "✓ Completed 2s") || strings.Contains(statusRow, "idle") {
		t.Fatalf("completion status should be compact, got %q in:\n%s", statusRow, rendered)
	}
}

func requireQuitCmd(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected quit command, got nil")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected quit command, got %T", msg)
	}
}

func updateAndRunImmediate(t *testing.T, model tea.Model, msg tea.Msg) tea.Model {
	t.Helper()
	next, cmd := model.Update(msg)
	return runImmediateCmd(t, next, cmd)
}

func runImmediateCmd(t *testing.T, model tea.Model, cmd tea.Cmd) tea.Model {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == nil {
			continue
		}
		msg := current()
		if msg == nil {
			continue
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		var nextCmd tea.Cmd
		model, nextCmd = model.Update(msg)
		queue = append(queue, nextCmd)
	}
	return model
}

func copyCommandMessages(cmd tea.Cmd) (raw string, hasSetClipboard bool) {
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		if rawMsg, ok := msg.(tea.RawMsg); ok {
			return fmt.Sprint(rawMsg.Msg), false
		}
		return "", fmt.Sprintf("%T", msg) == "tea.setClipboardMsg"
	}
	for _, child := range batch {
		childMsg := child()
		switch msg := childMsg.(type) {
		case tea.RawMsg:
			raw += fmt.Sprint(msg.Msg)
		default:
			if fmt.Sprintf("%T", childMsg) == "tea.setClipboardMsg" {
				hasSetClipboard = true
			}
		}
	}
	return raw, hasSetClipboard
}

func TestPOC2InfoCardStaysAtTopAfterFirstMessage(t *testing.T) {
	var submitted string
	var tm tea.Model = NewModel(Options{
		Width:         48,
		Height:        12,
		InfoCardLines: []string{"modu_code", "model: Test", "commands: type /"},
		IntentHandler: testIntentHandler(testIntentCallbacks{submit: func(event SubmitEvent) {
			submitted = event.Text
		}}),
	})

	m := tm.(Model)
	rendered := ansi.Strip(m.render())
	for _, want := range []string{"┏", "modu_code", "model: Test", "commands: type /"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("initial info card missing %q:\n%s", want, rendered)
		}
	}

	tm, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "h", Code: 'h'}))
	tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Text: "i", Code: 'i'}))
	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = tm.(Model)

	if got, want := submitted, "hi"; got != want {
		t.Fatalf("submitted = %q, want %q", got, want)
	}
	afterSubmit := ansi.Strip(m.render())
	if !strings.Contains(afterSubmit, "commands: type /") {
		t.Fatalf("info card should stay at the top after the first submitted message:\n%s", afterSubmit)
	}
	if !strings.Contains(afterSubmit, "❯ hi") {
		t.Fatalf("submitted message should render below the info card:\n%s", afterSubmit)
	}
}

func TestPOC2InputCoalescesMobileSSHChineseIMEPreedit(t *testing.T) {
	var tm tea.Model = NewModel(Options{Width: 48, Height: 10})
	for _, text := range []string{"z", "zh", "zhe", "zheg", "zhege", "这个", "这"} {
		tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Text: text}))
	}

	m := tm.(Model)
	if got, want := m.input.Value, "这个"; got != want {
		t.Fatalf("input value = %q, want %q", got, want)
	}
}

func TestPOC2InputKeepsNormalASCIIKeyPresses(t *testing.T) {
	var tm tea.Model = NewModel(Options{Width: 48, Height: 10})
	for _, text := range []string{"z", "h", "e", "g", "e"} {
		tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Text: text, Code: []rune(text)[0]}))
	}

	m := tm.(Model)
	if got, want := m.input.Value, "zhege"; got != want {
		t.Fatalf("input value = %q, want %q", got, want)
	}
}

func TestPOC2CtrlWDeletesWordBeforeCursor(t *testing.T) {
	var tm tea.Model = NewModel(Options{Width: 48, Height: 10})
	for _, text := range []string{"hello", " ", "world"} {
		tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Text: text}))
	}

	tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Code: 'w', Mod: tea.ModCtrl}))
	m := tm.(Model)
	if got, want := m.input.Value, "hello "; got != want {
		t.Fatalf("input value after ctrl+w = %q, want %q", got, want)
	}
}

func TestPOC2InputDoesNotReplaceSingleASCIIBeforeChinese(t *testing.T) {
	var tm tea.Model = NewModel(Options{Width: 48, Height: 10})
	for _, text := range []string{"a", "你"} {
		tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Text: text}))
	}

	m := tm.(Model)
	if got, want := m.input.Value, "a你"; got != want {
		t.Fatalf("input value = %q, want %q", got, want)
	}
}

func TestPOC2InputCoalescesConsecutiveChineseIMEWords(t *testing.T) {
	var tm tea.Model = NewModel(Options{Width: 48, Height: 10})
	for _, text := range []string{"z", "zh", "zhe", "zhege", "这个", "n", "ni", "你"} {
		tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Text: text}))
	}

	m := tm.(Model)
	if got, want := m.input.Value, "这个你"; got != want {
		t.Fatalf("input value = %q, want %q", got, want)
	}
}

func TestPOC2PreformattedAssistantMessagePreservesSlashHelpLines(t *testing.T) {
	m := NewModel(Options{
		Width:  72,
		Height: 12,
		InitialEntries: []Entry{testTextEntry(
			RoleAssistant,
			"Help\n/help, /h           — show this help\n/quit, /exit        — exit\n\nkeys\nctrl+j         — insert newline",
		)},
	})

	rendered := ansi.Strip(m.render())
	for _, want := range []string{
		"● Help",
		"  /help, /h           — show this help",
		"  /quit, /exit        — exit",
		"  keys",
		"  ctrl+j         — insert newline",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("preformatted help missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "Help /help") {
		t.Fatalf("preformatted help should not collapse newlines into a paragraph:\n%s", rendered)
	}
}

func TestPOC2ThinkingBlockIsCollapsedAndClickable(t *testing.T) {
	m := NewModel(Options{
		Width:  72,
		Height: 10,
		InitialEntries: []Entry{{
			Role:  RoleAssistant,
			Nodes: []Node{ThinkingNode{Text: "reasoning detail"}},
		}},
	})
	rendered := ansi.Strip(m.render())
	if !strings.Contains(rendered, "● Thinking") {
		t.Fatalf("thinking block summary missing:\n%s", rendered)
	}
	if strings.Contains(rendered, "reasoning detail") {
		t.Fatalf("thinking block should default collapsed:\n%s", rendered)
	}
	if _, ok := m.headers[0]; !ok {
		t.Fatalf("thinking block header should be clickable, headers=%#v", m.headers)
	}

	m = mouseClick(m, 1, 0)
	rendered = ansi.Strip(m.render())
	if !strings.Contains(rendered, "reasoning detail") {
		t.Fatalf("clicking thinking block should expand detail:\n%s", rendered)
	}
}

// mouseClick drives a press-and-release with no pointer movement through
// Update — the gesture that toggles a collapsible block.
func mouseClick(m Model, x, y int) Model {
	tm, _ := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	m = tm.(Model)
	tm, _ = m.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	return tm.(Model)
}

// mouseDrag drives a press, a move to a second cell, and a release — the
// gesture that selects text rather than toggling.
func mouseDrag(m Model, x1, y1, x2, y2 int) (Model, tea.Cmd) {
	tm, _ := m.Update(tea.MouseClickMsg{X: x1, Y: y1, Button: tea.MouseLeft})
	m = tm.(Model)
	tm, _ = m.Update(tea.MouseMotionMsg{X: x2, Y: y2, Button: tea.MouseLeft})
	m = tm.(Model)
	tm, cmd := m.Update(tea.MouseReleaseMsg{X: x2, Y: y2, Button: tea.MouseLeft})
	return tm.(Model), cmd
}

func TestPOC2StreamingAssistantMarkerBlinks(t *testing.T) {
	m := NewModel(Options{Width: 72, Height: 10, StreamReply: "streaming reply"})
	m.startStream()
	m.streamIdx = len(m.streamRunes)
	m.rebuild()
	if got, want := streamingAssistantMarkerStyle.GetForeground(), lipgloss.Color("231"); got != want {
		t.Fatalf("streaming assistant marker foreground = %#v, want %#v", got, want)
	}
	if !streamingAssistantMarkerStyle.GetBlink() {
		t.Fatal("streaming assistant marker should blink")
	}
}

func TestPOC2JumpHintSharesAgentStatusRow(t *testing.T) {
	m := NewModel(Options{Width: 72, Height: 8})
	m.busy = true
	for i := 0; i < 20; i++ {
		m.entries = append(m.entries, testTextEntry(RoleAssistant, "history"))
	}
	m.rebuild()
	m.scroll(-2)

	rendered := ansi.Strip(m.render())
	if got := strings.Count(rendered, jumpHintText()); got != 1 {
		t.Fatalf("jump hint count = %d, want 1:\n%s", got, rendered)
	}
	lines := strings.Split(rendered, "\n")
	statusRow := m.vpHeight() + m.approvalPanelHeight() + m.humanPromptPanelHeight() + m.completionPanelHeight() + m.todoPanelHeight() + 1
	if !strings.Contains(lines[statusRow], "running") || !strings.Contains(lines[statusRow], jumpHintText()) {
		t.Fatalf("jump hint should share the agent status row, got %q in:\n%s", lines[statusRow], rendered)
	}
	idx := strings.Index(lines[statusRow], jumpHintText())
	if idx < 0 {
		t.Fatalf("status row missing jump text: %q", lines[statusRow])
	}
	gotCol := ansi.StringWidth(lines[statusRow][:idx])
	wantTextCol := (m.width-ansi.StringWidth(" "+jumpHintText()+" "))/2 + 1
	if gotCol != wantTextCol {
		t.Fatalf("jump hint text column = %d, want centered block text at %d in row %q", gotCol, wantTextCol, lines[statusRow])
	}
	if raw := m.render(); !strings.Contains(raw, "48;5;63") {
		t.Fatalf("jump hint should keep its background style, raw render missing background escape:\n%q", raw)
	}
}

func TestPOC2JumpHintShowsNewMessageCountWithCtrlEnd(t *testing.T) {
	var tm tea.Model = NewModel(Options{Width: 72, Height: 8})
	m := tm.(Model)
	for i := 0; i < 20; i++ {
		m.entries = append(m.entries, testTextEntry(RoleAssistant, "history"))
	}
	m.rebuild()
	m.scroll(-2)

	tm, _ = m.Update(UpdateMsg{Update: AppendEntryUpdate{Entry: testTextEntry(RoleAssistant, "one")}})
	m = tm.(Model)
	rendered := ansi.Strip(m.render())
	if !strings.Contains(rendered, "Have 1 new message (ctrl+End) ↓") {
		t.Fatalf("new message hint should include count and ctrl+End:\n%s", rendered)
	}

	tm, _ = m.Update(UpdateMsg{Update: AppendEntryUpdate{Entry: testTextEntry(RoleAssistant, "two")}})
	m = tm.(Model)
	rendered = ansi.Strip(m.render())
	if !strings.Contains(rendered, "Have 2 new messages (ctrl+End) ↓") {
		t.Fatalf("new message hint should increment for newly appended messages:\n%s", rendered)
	}
}

func TestPOC2MergedToolUpdateDoesNotIncrementNewMessageCount(t *testing.T) {
	var tm tea.Model = NewModel(Options{Width: 72, Height: 8})
	m := tm.(Model)
	for i := 0; i < 20; i++ {
		m.entries = append(m.entries, testTextEntry(RoleAssistant, "history"))
	}
	m.entries = append(m.entries, testToolEntry(ToolCall{
		ID:      "call-1",
		Name:    "bash",
		Summary: "Running shell command",
		Input:   "go test ./pkg/modu-tui",
	}, ToolPermissionUnknown, false))
	m.rebuild()
	m.scroll(-2)

	tm, _ = m.Update(UpdateMsg{Update: AppendEntryUpdate{Entry: testToolEntry(ToolCall{
		ID:      "call-1",
		Name:    "bash",
		Summary: "Ran 1 shell command",
		Output:  "ok",
		Done:    true,
	}, ToolPermissionUnknown, false)}})
	m = tm.(Model)
	rendered := ansi.Strip(m.render())
	if strings.Contains(rendered, "Have 1 new message") {
		t.Fatalf("merged tool update should not count as a newly appended message:\n%s", rendered)
	}
	if !strings.Contains(rendered, jumpHintText()) {
		t.Fatalf("away-from-bottom hint should fall back to jump text after a merge-only update:\n%s", rendered)
	}
}

func TestPOC2JumpRowClickScrollsToBottom(t *testing.T) {
	m := NewModel(Options{Width: 72, Height: 8})
	for i := 0; i < 20; i++ {
		m.entries = append(m.entries, testTextEntry(RoleAssistant, "history"))
	}
	m.rebuild()
	m.scroll(-2)
	if m.atBottom() {
		t.Fatal("setup should be scrolled away from bottom")
	}

	statusRow := m.vpHeight() + m.approvalPanelHeight() + m.humanPromptPanelHeight() + m.completionPanelHeight() + m.todoPanelHeight() + 1
	_ = m.onPress(1, statusRow)
	if !m.atBottom() {
		t.Fatalf("jump row click should scroll to bottom, offset=%d max=%d", m.yOffset, m.maxOffset())
	}
}

func TestPOC2InputHasTopAndBottomRules(t *testing.T) {
	m := NewModel(Options{Width: 16, Height: 8})
	lines := strings.Split(m.render(), "\n")
	if got, want := len(lines), m.height; got != want {
		t.Fatalf("rendered line count = %d, want %d", got, want)
	}
	topRule := ansi.Strip(lines[m.vpHeight()+2])
	bottomRule := ansi.Strip(lines[m.vpHeight()+4])
	wantRule := strings.Repeat("─", m.width)
	if topRule != wantRule {
		t.Fatalf("top input rule = %q, want %q", topRule, wantRule)
	}
	if bottomRule != wantRule {
		t.Fatalf("bottom input rule = %q, want %q", bottomRule, wantRule)
	}
}

func TestPOC2HistoryHintRendersOnTopInputRule(t *testing.T) {
	var tm tea.Model = NewModel(Options{
		Width:        32,
		Height:       8,
		InputHistory: []string{"first", "second"},
	})
	tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	m := tm.(Model)
	lines := strings.Split(ansi.Strip(m.render()), "\n")
	topRule := lines[m.vpHeight()+2]
	inputLine := lines[m.vpHeight()+3]
	if !strings.Contains(topRule, "History 2/2") {
		t.Fatalf("history hint should render on top rule, got %q in:\n%s", topRule, strings.Join(lines, "\n"))
	}
	if strings.Contains(inputLine, "History") {
		t.Fatalf("history hint should not render inside input line, got %q", inputLine)
	}
	if !strings.Contains(inputLine, "❯ second") {
		t.Fatalf("history input line should keep selected text only, got %q", inputLine)
	}
}

func TestPOC2InputLineLeavesLastColumnForMobileTerminals(t *testing.T) {
	m := NewModel(Options{Width: 24, Height: 8})
	m.input.Insert(strings.Repeat("j", 120))
	rendered := m.render()
	lines := strings.Split(rendered, "\n")
	inputLine := lines[len(lines)-3]
	if strings.Contains(inputLine, "\x1b[?7l") || strings.Contains(inputLine, "\x1b[?7h") {
		t.Fatalf("input line should not toggle terminal autowrap, got %q", inputLine)
	}
	if !strings.HasSuffix(inputLine, "\x1b[K") {
		t.Fatalf("input line should clear to end of line, got %q", inputLine)
	}
	stripped := ansi.Strip(strings.TrimSuffix(inputLine, "\x1b[K"))
	if strings.Contains(stripped, "\r") {
		t.Fatalf("input line should not return carriage, got %q", inputLine)
	}
	if got, want := ansi.StringWidth(stripped), m.inputRenderWidth(); got != want {
		t.Fatalf("stripped input line width = %d, want %d: %q", got, want, stripped)
	}
}

func TestPOC2AddsGapBetweenBlocks(t *testing.T) {
	m := NewModel(Options{
		Width:  40,
		Height: 12,
		InitialEntries: []Entry{
			testTextEntry(RoleUser, "alpha"),
			testTextEntry(RoleUser, "beta"),
		},
	})
	lines := m.Lines()
	blankBetween := false
	for i := 1; i < len(lines)-1; i++ {
		if strings.TrimSpace(ansi.Strip(lines[i])) == "" &&
			strings.Contains(ansi.Strip(lines[i-1]), "alpha") &&
			strings.Contains(ansi.Strip(lines[i+1]), "beta") {
			blankBetween = true
			break
		}
	}
	if !blankBetween {
		t.Fatalf("expected a blank line between blocks:\n%s", strings.Join(lines, "\n"))
	}
}

func TestPOC2PasteStaysSingleLine(t *testing.T) {
	var tm tea.Model = NewModel()
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 30, Height: 8})
	tm, _ = tm.Update(tea.PasteMsg{Content: "alpha\nbeta\rgamma\r\ndelta"})

	m := tm.(Model)
	if strings.ContainsAny(m.input.Value, "\r\n") {
		t.Fatalf("paste left newline characters in input: %q", m.input.Value)
	}
	if got, want := m.input.Value, "alpha beta gamma delta"; got != want {
		t.Fatalf("input = %q, want %q", got, want)
	}
}

func TestPOC2LargePasteCollapsesInInputAndSubmitsExpandedText(t *testing.T) {
	pasted := strings.Join([]string{
		"line 1",
		"line 2",
		"line 3",
		"line 4",
		"line 5",
		"line 6",
	}, "\n")
	var submitted string
	var tm tea.Model = NewModel(Options{
		Width:  72,
		Height: 10,
		IntentHandler: testIntentHandler(testIntentCallbacks{submit: func(event SubmitEvent) {
			submitted = event.Text
		}}),
	})
	tm, _ = tm.Update(tea.PasteMsg{Content: pasted})

	m := tm.(Model)
	rendered := ansi.Strip(m.render())
	if !strings.Contains(rendered, "[Pasted text 6 lines]") {
		t.Fatalf("large paste should render as a collapsed label:\n%s", rendered)
	}
	if strings.Contains(rendered, "line 6") {
		t.Fatalf("large paste content should not be expanded in the input:\n%s", rendered)
	}

	tm = updateAndRunImmediate(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = tm.(Model)
	if got := submitted; got != pasted {
		t.Fatalf("submitted paste = %q, want %q", got, pasted)
	}
	if len(m.entries) != 1 || testEntryText(m.entries[0]) != pasted {
		t.Fatalf("transcript message should keep the expanded paste: %#v", m.entries)
	}
}

func TestPOC2SubmitHookReceivesEnteredText(t *testing.T) {
	var submitted string
	var tm tea.Model = NewModel(Options{
		IntentHandler: testIntentHandler(testIntentCallbacks{submit: func(event SubmitEvent) {
			submitted = event.Text
		}}),
	})
	tm, _ = tm.Update(tea.PasteMsg{Content: "hello"})
	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	m := tm.(Model)
	if got, want := submitted, "hello"; got != want {
		t.Fatalf("submitted = %q, want %q", got, want)
	}
	if got := m.input.Value; got != "" {
		t.Fatalf("input should reset after submit, got %q", got)
	}
	if len(m.entries) != 1 || m.entries[0].Role != RoleUser || testEntryText(m.entries[0]) != "hello" {
		t.Fatalf("submitted message not appended: %#v", m.entries)
	}
}

func TestPOC2SubmitThenWideTranscriptKeepsComposerCleared(t *testing.T) {
	var tm tea.Model = NewModel(Options{
		Width:  52,
		Height: 18,
		IntentHandler: testIntentHandler(testIntentCallbacks{submit: func(SubmitEvent) {
		}}),
	})
	tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Text: "why"}))
	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	tm, _ = tm.Update(UpdateMsg{Update: AppendEntryUpdate{Entry: Entry{
		Role: RoleAssistant,
		Nodes: []Node{MarkdownNode{Text: strings.Repeat(
			"● 中文回答会触发整屏滚动 👋\n\n", 8,
		)}},
	}}})
	tm, _ = tm.Update(UpdateMsg{Update: AppendEntryUpdate{Entry: Entry{
		Role:  RoleAssistant,
		Nodes: []Node{TextNode{Text: "✓ Completed (3s)"}},
		Plain: true,
	}}})

	m := tm.(Model)
	if got := m.input.Value; got != "" {
		t.Fatalf("input state should stay cleared after transcript scroll, got %q", got)
	}
	view := m.View()
	lines := strings.Split(ansi.Strip(view.Content), "\n")
	if view.Cursor == nil || view.Cursor.Y < 0 || view.Cursor.Y >= len(lines) {
		t.Fatalf("cursor should remain on the composer, cursor=%#v lines=%d", view.Cursor, len(lines))
	}
	inputLine := lines[view.Cursor.Y]
	if strings.Contains(inputLine, "why") {
		t.Fatalf("submitted input should not remain in the composer row: %q", inputLine)
	}
	if got, want := strings.TrimSpace(inputLine), "❯"; got != want {
		t.Fatalf("cleared composer row = %q, want %q", got, want)
	}
	if got, want := view.Cursor.X, lipgloss.Width(youStyle.Render("❯ ")); got != want {
		t.Fatalf("cleared composer cursor x = %d, want %d", got, want)
	}
}

func TestPOC2CtrlVPastesClipboardImageAndSubmitsAttachment(t *testing.T) {
	var submitted SubmitEvent
	var tm tea.Model = NewModel(Options{
		Width:  50,
		Height: 10,
		Services: Services{
			ReadClipboardImages: func() ([]ImageAttachment, error) {
				return []ImageAttachment{{
					Name:     "clipboard.png",
					MimeType: "image/png",
					Data:     []byte("png"),
				}}, nil
			},
		},
		IntentHandler: testIntentHandler(testIntentCallbacks{submit: func(event SubmitEvent) {
			submitted = event
		}}),
	})

	var cmd tea.Cmd
	tm, cmd = tm.Update(tea.KeyPressMsg(tea.Key{Code: 'v', Mod: tea.ModCtrl}))
	if cmd == nil {
		t.Fatal("ctrl+v should return an asynchronous clipboard command")
	}
	tm, _ = tm.Update(cmd())
	m := tm.(Model)
	if got := ansi.Strip(m.render()); !strings.Contains(got, "[Image #1]") {
		t.Fatalf("input should render the pasted image attachment:\n%s", got)
	}

	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = tm.(Model)
	if submitted.Text != "" || len(submitted.Images) != 1 || submitted.Images[0].MimeType != "image/png" {
		t.Fatalf("submitted event = %#v", submitted)
	}
	if len(m.entries) != 1 || testEntryText(m.entries[0]) != "[Image #1]" {
		t.Fatalf("transcript should show the image label, got %#v", m.entries)
	}
	if len(m.input.ImageAttachments()) != 0 {
		t.Fatalf("input attachments should reset after submit: %#v", m.input.ImageAttachments())
	}
}

func TestPOC2PastedImagePathBecomesAttachment(t *testing.T) {
	var resolved string
	var tm tea.Model = NewModel(Options{
		Services: Services{
			ResolvePastedImages: func(value string) ([]ImageAttachment, bool, error) {
				resolved = value
				return []ImageAttachment{{
					Name:     "screen shot.png",
					MimeType: "image/png",
					Data:     []byte("png"),
				}}, true, nil
			},
		},
	})

	tm = updateAndRunImmediate(t, tm, tea.PasteMsg{Content: `/tmp/screen\ shot.png `})
	m := tm.(Model)
	if resolved != `/tmp/screen\ shot.png ` {
		t.Fatalf("resolver input = %q", resolved)
	}
	if got := m.input.ImageAttachments(); len(got) != 1 || got[0].Name != "screen shot.png" {
		t.Fatalf("resolved attachments = %#v", got)
	}
	if strings.Contains(m.input.ExpandedValue(), "/tmp/") {
		t.Fatalf("resolved image path should not remain as prompt text: %q", m.input.ExpandedValue())
	}
}

func TestEnterSendsWhenIdleAndQueuesWhileBusy(t *testing.T) {
	newModel := func(busy bool, submit func(SubmitEvent)) tea.Model {
		var tm tea.Model = NewModel(Options{
			IntentHandler: testIntentHandler(testIntentCallbacks{submit: submit}),
		})
		if busy {
			tm, _ = tm.Update(UpdateMsg{Update: SetBusyUpdate{Busy: true}})
		}
		tm, _ = tm.Update(tea.PasteMsg{Content: "next instruction"})
		return tm
	}

	t.Run("idle enter sends a prompt", func(t *testing.T) {
		var got SubmitEvent
		tm := newModel(false, func(ev SubmitEvent) { got = ev })
		tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		if got.Text != "next instruction" || got.Kind != SubmitKindPrompt {
			t.Fatalf("submit event = %#v, want a prompt carrying the typed text", got)
		}
		if len(tm.(Model).queued) != 0 {
			t.Fatal("an idle message should go straight out, not into the queue")
		}
	})

	t.Run("busy enter queues instead of sending", func(t *testing.T) {
		var submitted []SubmitEvent
		tm := newModel(true, func(ev SubmitEvent) { submitted = append(submitted, ev) })
		tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		m := tm.(Model)
		if len(submitted) != 0 {
			t.Fatalf("a mid-run message should not reach the host yet: %#v", submitted)
		}
		if len(m.queued) != 1 || m.queued[0].ExpandedValue() != "next instruction" {
			t.Fatalf("queue = %#v, want the typed message parked", m.queued)
		}
		// The transcript is the record of what was actually sent. Showing the
		// message there now would place it above output that predates it.
		if len(m.entries) != 0 {
			t.Fatalf("a queued message should stay out of the transcript, got %d entries", len(m.entries))
		}
		if m.input.Value != "" {
			t.Fatalf("queueing should clear the composer, got %q", m.input.Value)
		}
	})

	t.Run("busy tab only completes", func(t *testing.T) {
		var submitted []SubmitEvent
		tm := newModel(true, func(ev SubmitEvent) { submitted = append(submitted, ev) })
		tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
		m := tm.(Model)
		if len(submitted) != 0 || len(m.queued) != 0 {
			t.Fatalf("Tab is completion-only; submitted=%#v queued=%#v", submitted, m.queued)
		}
		if m.input.Value != "next instruction" {
			t.Fatalf("Tab with nothing to complete should leave the input alone, got %q", m.input.Value)
		}
	})
}

func TestBusySlashCommandRunsImmediatelyInsteadOfQueueing(t *testing.T) {
	// Slash commands act on the UI or the session right now (/stop, /clear),
	// so waiting for the turn to end would defeat them.
	var lines []string
	var submitted []SubmitEvent
	var tm tea.Model = NewModel(Options{
		SlashCommands: []SlashCommand{{Name: "/compact", Description: "Compact the context"}},
		IntentHandler: testIntentHandler(testIntentCallbacks{
			submit:       func(event SubmitEvent) { submitted = append(submitted, event) },
			slashCommand: func(line string) { lines = append(lines, line) },
		}),
	})
	tm, _ = tm.Update(UpdateMsg{Update: SetBusyUpdate{Busy: true}})
	tm = typeInto(t, tm, "/compact")
	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	m := tm.(Model)
	if len(lines) != 1 || lines[0] != "/compact" {
		t.Fatalf("slash command lines = %#v, want it dispatched immediately", lines)
	}
	if len(m.queued) != 0 || len(submitted) != 0 {
		t.Fatalf("a slash command should not be queued: queued=%#v submitted=%#v", m.queued, submitted)
	}
}

func TestBusyTabCompletesWithoutSubmitting(t *testing.T) {
	var submitted []SubmitEvent
	var tm tea.Model = NewModel(Options{
		SlashCommands: []SlashCommand{{Name: "/steer", Description: "Steer the turn"}},
		IntentHandler: testIntentHandler(testIntentCallbacks{submit: func(event SubmitEvent) {
			submitted = append(submitted, event)
		}}),
	})
	tm, _ = tm.Update(UpdateMsg{Update: SetBusyUpdate{Busy: true}})
	tm = typeInto(t, tm, "/st")
	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))

	m := tm.(Model)
	if m.input.Value != "/steer " {
		t.Fatalf("Tab completion = %q, want %q", m.input.Value, "/steer ")
	}
	if len(submitted) != 0 || len(m.queued) != 0 {
		t.Fatalf("completion also submitted input: submitted=%#v queued=%#v", submitted, m.queued)
	}
}

func TestQueuedMessagesFlushOneTurnAtATimeWhenIdle(t *testing.T) {
	var submitted []SubmitEvent
	var tm tea.Model = NewModel(Options{
		IntentHandler: testIntentHandler(testIntentCallbacks{submit: func(event SubmitEvent) {
			submitted = append(submitted, event)
		}}),
	})
	tm, _ = tm.Update(UpdateMsg{Update: SetBusyUpdate{Busy: true}})
	for _, text := range []string{"first queued", "second queued"} {
		tm, _ = tm.Update(tea.PasteMsg{Content: text})
		tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	}

	tm = updateAndRunImmediate(t, tm, UpdateMsg{Update: SetBusyUpdate{Busy: false}})
	m := tm.(Model)
	if len(submitted) != 1 || submitted[0].Text != "first queued" {
		t.Fatalf("going idle should release exactly the oldest message, got %#v", submitted)
	}
	// Follow-up rather than prompt: the host's follow-up path starts a fresh
	// turn when idle but will not race a second foreground run if one already
	// started between the idle update and this send.
	if submitted[0].Kind != SubmitKindFollowUp {
		t.Fatalf("released message kind = %q, want %q", submitted[0].Kind, SubmitKindFollowUp)
	}
	if len(m.queued) != 1 {
		t.Fatalf("the second message should still be waiting, got %#v", m.queued)
	}
	// Only now does it join the transcript, in prompt/reply order.
	if len(m.entries) != 1 {
		t.Fatalf("the sent message should appear in the transcript, got %d entries", len(m.entries))
	}

	tm, _ = tm.Update(UpdateMsg{Update: SetBusyUpdate{Busy: true}})
	tm = updateAndRunImmediate(t, tm, UpdateMsg{Update: SetBusyUpdate{Busy: false}})
	if len(submitted) != 2 || submitted[1].Text != "second queued" {
		t.Fatalf("the next idle turn should release the second message, got %#v", submitted)
	}
	if len(tm.(Model).queued) != 0 {
		t.Fatal("the queue should be drained")
	}
}

func TestQueuedMessagesStillFlushWithAWizardOpen(t *testing.T) {
	// A slash command dispatches immediately even mid-run, so a host wizard
	// can be open at the moment the turn ends. Holding the queue for it would
	// strand the messages: the pending region is hidden behind the wizard and
	// nothing re-triggers the flush when the wizard closes.
	var submitted []SubmitEvent
	var tm tea.Model = NewModel(Options{
		IntentHandler: testIntentHandler(testIntentCallbacks{submit: func(event SubmitEvent) {
			submitted = append(submitted, event)
		}}),
	})
	tm, _ = tm.Update(UpdateMsg{Update: SetBusyUpdate{Busy: true}})
	tm, _ = tm.Update(tea.PasteMsg{Content: "queued before the wizard"})
	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	tm, _ = tm.Update(RequestHumanTextMsg{
		Request: HumanTextRequest{ID: "cfg", Title: "API key"},
		Respond: make(chan string, 1),
	})
	withWizard := tm.(Model)
	if !withWizard.hasBlockingPrompt() {
		t.Fatal("the wizard should be open for this test to mean anything")
	}

	tm = updateAndRunImmediate(t, tm, UpdateMsg{Update: SetBusyUpdate{Busy: false}})
	if len(submitted) != 1 || submitted[0].Text != "queued before the wizard" {
		t.Fatalf("the queued message should still go out, got %#v", submitted)
	}
	if len(tm.(Model).queued) != 0 {
		t.Fatal("the queue should be drained rather than stranded behind the overlay")
	}
}

func TestBackspaceTakesTheLastQueuedMessageBack(t *testing.T) {
	var tm tea.Model = NewModel(Options{})
	tm, _ = tm.Update(UpdateMsg{Update: SetBusyUpdate{Busy: true}})
	for _, text := range []string{"first queued", "second queued"} {
		tm, _ = tm.Update(tea.PasteMsg{Content: text})
		tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	}

	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	m := tm.(Model)
	if m.input.ExpandedValue() != "second queued" {
		t.Fatalf("Backspace on an empty input should restore the newest queued message, got %q", m.input.ExpandedValue())
	}
	if m.input.Cursor != m.input.Len() {
		t.Fatalf("the restored message should be ready to edit at its end, cursor = %d", m.input.Cursor)
	}
	if len(m.queued) != 1 {
		t.Fatalf("only one message should have come back, queue = %#v", m.queued)
	}

	// With text in the input again, Backspace goes back to deleting.
	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	if got := tm.(Model).input.ExpandedValue(); got != "second queue" {
		t.Fatalf("Backspace should resume deleting characters, got %q", got)
	}
	if len(tm.(Model).queued) != 1 {
		t.Fatal("deleting a character should not touch the queue")
	}
}

func TestInterruptDropsQueuedMessages(t *testing.T) {
	var interrupted int
	var submitted []SubmitEvent
	var tm tea.Model = NewModel(Options{
		IntentHandler: testIntentHandler(testIntentCallbacks{
			submit:    func(event SubmitEvent) { submitted = append(submitted, event) },
			interrupt: func() { interrupted++ },
		}),
	})
	tm, _ = tm.Update(UpdateMsg{Update: SetBusyUpdate{Busy: true}})
	tm, _ = tm.Update(tea.PasteMsg{Content: "queued behind the run"})
	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	if interrupted != 1 {
		t.Fatalf("Esc should still interrupt, got %d interrupts", interrupted)
	}
	if len(tm.(Model).queued) != 0 {
		t.Fatal("stopping the run should drop what was queued behind it")
	}

	// And going idle afterwards must not resurrect it.
	tm = updateAndRunImmediate(t, tm, UpdateMsg{Update: SetBusyUpdate{Busy: false}})
	if len(submitted) != 0 {
		t.Fatalf("a dropped message must not be sent after the interrupt: %#v", submitted)
	}
	// It is still recallable from input history, so nothing is really lost.
	if got := tm.(Model).inputHistory; len(got) != 1 || got[0] != "queued behind the run" {
		t.Fatalf("input history = %#v, want the dropped message recallable", got)
	}
}

func TestQueuedMessagesRenderAbovTheInput(t *testing.T) {
	var tm tea.Model = NewModel(Options{Width: 100, Height: 24})
	tm, _ = tm.Update(UpdateMsg{Update: SetBusyUpdate{Busy: true}})
	tm, _ = tm.Update(tea.PasteMsg{Content: "跑一下测试"})
	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	pending := tm.(Model)
	rendered := ansi.Strip(pending.render())
	if !strings.Contains(rendered, "已排队 1 条") {
		t.Fatalf("the pending region should count what is waiting:\n%s", rendered)
	}
	if !strings.Contains(rendered, "跑一下测试") {
		t.Fatalf("the pending region should show the message:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Backspace") {
		t.Fatalf("the pending region should say how to take a message back:\n%s", rendered)
	}
}

func TestPOC2InputHistoryNavigatesWithUpAndDown(t *testing.T) {
	var tm tea.Model = NewModel(Options{
		Width:        72,
		Height:       10,
		InputHistory: []string{"first", "second", "third"},
	})
	tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Text: "d", Code: 'd'}))
	tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Text: "r", Code: 'r'}))
	tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Text: "a", Code: 'a'}))
	tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Text: "f", Code: 'f'}))
	tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Text: "t", Code: 't'}))

	tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	m := tm.(Model)
	if got, want := m.input.Value, "third"; got != want {
		t.Fatalf("first history up = %q, want %q", got, want)
	}
	if got := ansi.Strip(m.render()); !strings.Contains(got, "History 3/3") {
		t.Fatalf("history hint missing after up:\n%s", got)
	}

	tm, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	m = tm.(Model)
	if got, want := m.input.Value, "second"; got != want {
		t.Fatalf("second history up = %q, want %q", got, want)
	}
	if got := ansi.Strip(m.render()); !strings.Contains(got, "History 2/3") {
		t.Fatalf("history hint should update index:\n%s", got)
	}

	tm, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m = tm.(Model)
	if got, want := m.input.Value, "draft"; got != want {
		t.Fatalf("down should restore held draft = %q, want %q", got, want)
	}
	if got := ansi.Strip(m.render()); strings.Contains(got, "History ") {
		t.Fatalf("history hint should hide after returning to draft:\n%s", got)
	}
}

func TestPOC2ArrowKeysScrollWhenConfiguredAndInputEmpty(t *testing.T) {
	var tm tea.Model = NewModel(Options{
		Width:           40,
		Height:          8,
		ArrowKeysScroll: true,
	})
	m := tm.(Model)
	for i := 0; i < 30; i++ {
		m.entries = append(m.entries, testTextEntry(RoleAssistant, "history line"))
	}
	m.rebuild()
	before := m.yOffset
	if before == 0 {
		t.Fatal("setup should start at scrollable bottom")
	}

	tm, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	m = tm.(Model)
	if got := m.yOffset; got >= before {
		t.Fatalf("up arrow should scroll transcript when input is empty: %d -> %d", before, got)
	}
	if got := m.input.Value; got != "" {
		t.Fatalf("up arrow should not enter input history when input is empty in arrow-scroll mode, got %q", got)
	}
}

func TestPOC2ArrowKeysPreferHistoryWhenConfiguredAndHistoryExists(t *testing.T) {
	var tm tea.Model = NewModel(Options{
		Width:           40,
		Height:          8,
		InputHistory:    []string{"previous prompt"},
		ArrowKeysScroll: true,
	})
	m := tm.(Model)
	for i := 0; i < 30; i++ {
		m.entries = append(m.entries, testTextEntry(RoleAssistant, "history line"))
	}
	m.rebuild()
	before := m.yOffset

	tm, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	m = tm.(Model)
	if got, want := m.input.Value, "previous prompt"; got != want {
		t.Fatalf("up arrow should navigate input history before scrolling, got %q want %q", got, want)
	}
	if got := m.yOffset; got != before {
		t.Fatalf("up arrow should not scroll when history exists: %d -> %d", before, got)
	}
}

func TestPOC2ArrowKeysStillNavigateHistoryWhenInputHasText(t *testing.T) {
	var tm tea.Model = NewModel(Options{
		Width:           40,
		Height:          8,
		InputHistory:    []string{"previous prompt"},
		ArrowKeysScroll: true,
	})
	tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Text: "d", Code: 'd'}))
	tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))

	m := tm.(Model)
	if got, want := m.input.Value, "previous prompt"; got != want {
		t.Fatalf("up arrow should still navigate history when input has text, got %q want %q", got, want)
	}
}

func TestPOC2InputHistoryKeepsMostRecent100AndSavesOnSubmit(t *testing.T) {
	history := make([]string, 105)
	for i := range history {
		history[i] = fmt.Sprintf("old-%03d", i)
	}
	var saved []string
	var submitted string
	var tm tea.Model = NewModel(Options{
		InputHistory: history,
		IntentHandler: testIntentHandler(testIntentCallbacks{
			history: func(history []string) {
				saved = append([]string(nil), history...)
			},
			submit: func(event SubmitEvent) {
				submitted = event.Text
			},
		}),
	})
	m := tm.(Model)
	if got, want := len(m.inputHistory), 100; got != want {
		t.Fatalf("initial input history len = %d, want %d", got, want)
	}
	if got, want := m.inputHistory[0], "old-005"; got != want {
		t.Fatalf("oldest retained history = %q, want %q", got, want)
	}
	tm, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	m = tm.(Model)
	if got := ansi.Strip(m.render()); !strings.Contains(got, "History 100/100") {
		t.Fatalf("full history hint should render History 100/100:\n%s", got)
	}
	tm, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m = tm.(Model)

	tm, _ = m.Update(tea.PasteMsg{Content: "new prompt"})
	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = tm.(Model)
	if got, want := submitted, "new prompt"; got != want {
		t.Fatalf("submitted = %q, want %q", got, want)
	}
	if got, want := len(m.inputHistory), 100; got != want {
		t.Fatalf("history len after submit = %d, want %d", got, want)
	}
	if got, want := m.inputHistory[len(m.inputHistory)-1], "new prompt"; got != want {
		t.Fatalf("newest history = %q, want %q", got, want)
	}
	if len(saved) != 100 || saved[len(saved)-1] != "new prompt" {
		t.Fatalf("saved history should receive trimmed latest 100 entries: len=%d last=%q", len(saved), saved[len(saved)-1])
	}
}

func TestPOC2SlashPickerCompletesCommandWithTab(t *testing.T) {
	suggestionCalls := 0
	var tm tea.Model = NewModel(Options{
		Width:         50,
		Height:        10,
		SlashCommands: []SlashCommand{{Name: "/help", Description: "Show help"}},
		Services: Services{SuggestSpelling: func(string, int) ([]string, error) {
			suggestionCalls++
			return []string{"help"}, nil
		}},
	})
	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Text: "/", Code: '/'}))
	m := tm.(Model)
	// Even a malformed host result must not steal Tab from an active slash
	// completion popup.
	m.spellingIssues = []SpellingIssue{{Start: 0, End: 1, Word: "/"}}
	if got := ansi.Strip(m.render()); !strings.Contains(got, "/help") || !strings.Contains(got, "┏") {
		t.Fatalf("slash picker not rendered:\n%s", got)
	}

	tm, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	m = tm.(Model)
	if got, want := m.input.Value, "/help "; got != want {
		t.Fatalf("completed slash input = %q, want %q", got, want)
	}
	if len(m.slashMatches) != 0 {
		t.Fatalf("slash matches should clear after completion: %#v", m.slashMatches)
	}
	if suggestionCalls != 0 {
		t.Fatalf("slash completion should take precedence over spelling, calls=%d", suggestionCalls)
	}
}

func TestPOC2SlashPickerRefreshesCommandsFromProvider(t *testing.T) {
	commands := []SlashCommand{{Name: "/old", Description: "Old command"}}
	var tm tea.Model = NewModel(Options{
		Width:  50,
		Height: 10,
		Services: Services{
			SlashCommands: func() []SlashCommand {
				return commands
			},
		},
	})
	commands = []SlashCommand{{Name: "/fresh", Description: "Fresh command"}}

	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Text: "/", Code: '/'}))
	m := tm.(Model)
	rendered := ansi.Strip(m.render())
	if !strings.Contains(rendered, "/fresh") {
		t.Fatalf("slash picker should include refreshed command:\n%s", rendered)
	}
	if strings.Contains(rendered, "/old") {
		t.Fatalf("slash picker should not keep stale command:\n%s", rendered)
	}
}

func TestPOC2SlashPickerDoesNotShowJumpHintAtBottom(t *testing.T) {
	var tm tea.Model = NewModel(Options{
		Width:         72,
		Height:        14,
		SlashCommands: []SlashCommand{{Name: "/goal", Description: "Set a goal"}},
	})
	m := tm.(Model)
	for i := 0; i < 20; i++ {
		m.entries = append(m.entries, testTextEntry(RoleAssistant, "history"))
	}
	m.rebuild()
	if !m.atBottom() {
		t.Fatal("setup should be at bottom")
	}

	tm, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "/", Code: '/'}))
	m = tm.(Model)
	rendered := ansi.Strip(m.render())
	if !strings.Contains(rendered, "/goal") {
		t.Fatalf("slash picker should be visible:\n%s", rendered)
	}
	if strings.Contains(rendered, jumpHintText()) {
		t.Fatalf("slash picker should not trigger jump hint at bottom:\n%s", rendered)
	}
}

func TestPOC2SlashPickerKeepsJumpHintWhenAwayFromBottom(t *testing.T) {
	var tm tea.Model = NewModel(Options{
		Width:         72,
		Height:        14,
		SlashCommands: []SlashCommand{{Name: "/goal", Description: "Set a goal"}},
	})
	m := tm.(Model)
	for i := 0; i < 20; i++ {
		m.entries = append(m.entries, testTextEntry(RoleAssistant, "history"))
	}
	m.rebuild()
	m.scroll(-2)
	if m.atBottom() {
		t.Fatal("setup should be away from bottom")
	}

	tm, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "/", Code: '/'}))
	m = tm.(Model)
	rendered := ansi.Strip(m.render())
	if !strings.Contains(rendered, "/goal") {
		t.Fatalf("slash picker should be visible:\n%s", rendered)
	}
	if !strings.Contains(rendered, jumpHintText()) {
		t.Fatalf("slash picker should keep jump hint when away from bottom:\n%s", rendered)
	}
}

func TestPOC2ResizeKeepsInputAndCursorAlignedWithSlashPanel(t *testing.T) {
	var tm tea.Model = NewModel(Options{
		Width:  50,
		Height: 14,
		SlashCommands: []SlashCommand{
			{Name: "/help", Description: "Show help"},
			{Name: "/model", Description: "Switch model"},
			{Name: "/tokens", Description: "Show tokens"},
			{Name: "/tools", Description: "Show tools"},
		},
	})
	tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Text: "/", Code: '/'}))
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 28, Height: 8})

	m := tm.(Model)
	view := m.View()
	lines := strings.Split(ansi.Strip(view.Content), "\n")
	if got, want := len(lines), m.height; got != want {
		t.Fatalf("rendered lines after resize = %d, want %d:\n%s", got, want, strings.Join(lines, "\n"))
	}
	if view.Cursor == nil || view.Cursor.Y < 0 || view.Cursor.Y >= len(lines) {
		t.Fatalf("cursor should stay inside resized view, cursor=%#v lines=%d", view.Cursor, len(lines))
	}
	if got := lines[view.Cursor.Y]; !strings.Contains(got, "❯ /") {
		t.Fatalf("cursor row should be the input line after resize, got row %d: %q\n%s", view.Cursor.Y, got, strings.Join(lines, "\n"))
	}
}

func TestPOC2ViewCanDisableMouseReporting(t *testing.T) {
	enabled := NewModel(Options{Width: 24, Height: 8}).View()
	if got, want := enabled.MouseMode, tea.MouseModeCellMotion; got != want {
		t.Fatalf("default mouse mode = %v, want %v", got, want)
	}

	disabled := NewModel(Options{Width: 24, Height: 8, DisableMouse: true}).View()
	if got, want := disabled.MouseMode, tea.MouseModeNone; got != want {
		t.Fatalf("disabled mouse mode = %v, want %v", got, want)
	}
}

func TestPOC2ViewCanDisableAlternateScreen(t *testing.T) {
	fullScreen := NewModel(Options{Width: 24, Height: 8}).View()
	if !fullScreen.AltScreen {
		t.Fatal("alternate screen should stay enabled by default")
	}

	inline := NewModel(Options{Width: 24, Height: 8, DisableAltScreen: true}).View()
	if inline.AltScreen {
		t.Fatal("DisableAltScreen should select inline rendering")
	}
}

func TestPOC2AutoScrollStopsWhenMouseReleaseIsMissing(t *testing.T) {
	m := NewModel(Options{Width: 40, Height: 8})
	for i := 0; i < 30; i++ {
		m.entries = append(m.entries, testTextEntry(RoleAssistant, "history line"))
	}
	m.rebuild()
	if cmd := m.onPress(1, 0); cmd != nil {
		t.Fatalf("press should not start a command, got %#v", cmd)
	}
	if cmd := m.onDrag(1, m.vpHeight()); cmd == nil {
		t.Fatal("dragging past viewport edge should start auto-scroll")
	}
	if !m.selecting || !m.autoScrolling || m.autoScroll == 0 {
		t.Fatalf("setup should be auto-scrolling, selecting=%v autoScrolling=%v autoScroll=%d", m.selecting, m.autoScrolling, m.autoScroll)
	}

	var tm tea.Model = m
	for i := 0; i <= maxAutoScrollTicksWithoutDrag; i++ {
		tm, _ = tm.Update(autoScrollTickMsg{})
	}
	m = tm.(Model)
	if m.selecting || m.autoScrolling || m.autoScroll != 0 {
		t.Fatalf("missing mouse release should stop auto-scroll, selecting=%v autoScrolling=%v autoScroll=%d ticks=%d",
			m.selecting, m.autoScrolling, m.autoScroll, m.autoScrollTicks)
	}
	if m.hasSelection() {
		t.Fatal("missing mouse release should clear the partial selection")
	}
}

func TestPOC2SlashCommandHookReceivesSelectedCommand(t *testing.T) {
	var submitted string
	var slashLine string
	var tm tea.Model = NewModel(Options{
		Width:         50,
		Height:        10,
		SlashCommands: []SlashCommand{{Name: "/help", Description: "Show help"}},
		IntentHandler: testIntentHandler(testIntentCallbacks{
			submit: func(event SubmitEvent) {
				submitted = event.Text
			},
			slashCommand: func(line string) {
				slashLine = line
			},
		}),
	})
	tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Text: "/", Code: '/'}))
	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	if got, want := slashLine, "/help"; got != want {
		t.Fatalf("slash command line = %q, want %q", got, want)
	}
	if submitted != "" {
		t.Fatalf("normal submit should not run for slash command, got %q", submitted)
	}
}

func TestPOC2ResizeKeepsApprovalInputAndCursorVisible(t *testing.T) {
	var tm tea.Model = NewModel(Options{Width: 42, Height: 12})
	tm, _ = tm.Update(RequestToolApprovalMsg{
		Request: ToolApprovalRequest{
			ID:       "call-1",
			ToolName: "bash",
			Summary:  "approval required: bash",
			Detail:   "go test ./pkg/modu-tui && git diff --check",
		},
		Respond: make(chan ToolApprovalDecision, 1),
	})
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 30, Height: 8})

	m := tm.(Model)
	view := m.View()
	lines := strings.Split(ansi.Strip(view.Content), "\n")
	if got, want := len(lines), m.height; got != want {
		t.Fatalf("rendered lines after approval resize = %d, want %d:\n%s", got, want, strings.Join(lines, "\n"))
	}
	if view.Cursor == nil || view.Cursor.Y < 0 || view.Cursor.Y >= len(lines) {
		t.Fatalf("approval cursor should stay inside resized view, cursor=%#v lines=%d", view.Cursor, len(lines))
	}
	if got := lines[view.Cursor.Y]; !strings.Contains(got, "approval pending") {
		t.Fatalf("approval cursor row should be the fixed input line, got row %d: %q\n%s", view.Cursor.Y, got, strings.Join(lines, "\n"))
	}
	if m.approvalPanelHeight() > m.approvalPanelBudget() {
		t.Fatalf("approval panel height = %d exceeds budget %d", m.approvalPanelHeight(), m.approvalPanelBudget())
	}
}

func TestPOC2AcceptsExternalMessagesAndBusyState(t *testing.T) {
	var tm tea.Model = NewModel(Options{Width: 40, Height: 8})
	tm, _ = tm.Update(UpdateMsg{Update: AppendEntryUpdate{Entry: testMarkdownEntry(RoleAssistant, "external reply")}})
	tm, _ = tm.Update(UpdateMsg{Update: SetBusyUpdate{Busy: true}})

	m := tm.(Model)
	if got := strings.Join(m.Lines(), "\n"); !strings.Contains(ansi.Strip(got), "external reply") {
		t.Fatalf("external message missing:\n%s", got)
	}
	if got := ansi.Strip(m.render()); !strings.Contains(got, "running") {
		t.Fatalf("running state missing:\n%s", got)
	}
}

func TestPOC2MergesToolMessagesByToolID(t *testing.T) {
	var tm tea.Model = NewModel(Options{Width: 80, Height: 18})
	tm, _ = tm.Update(UpdateMsg{Update: AppendEntryUpdate{Entry: testToolEntry(ToolCall{
		ID: "call-1", Name: "bash", Summary: "Running shell command", Input: "go test ./...",
	}, ToolPermissionUnknown, false)}})
	tm, _ = tm.Update(UpdateMsg{Update: AppendEntryUpdate{Entry: testToolEntry(ToolCall{
		ID: "call-1", Name: "bash", Summary: "Ran 1 shell command", Output: "ok ./pkg/modu-tui", Done: true,
	}, ToolPermissionUnknown, false)}})

	m := tm.(Model)
	if len(m.entries) != 1 {
		t.Fatalf("tool messages should merge into one block, got %d: %#v", len(m.entries), m.entries)
	}
	node, _, ok := toolNodeFromEntry(m.entries[0])
	if !ok {
		t.Fatal("merged entry missing ToolNode")
	}
	if got := node.Call.Summary; got != "Ran 1 shell command" {
		t.Fatalf("merged summary = %q, want Ran 1 shell command", got)
	}
}

func TestPOC2InitialToolMessagesAreMerged(t *testing.T) {
	m := NewModel(Options{
		Width:  80,
		Height: 12,
		InitialEntries: []Entry{
			testToolEntry(ToolCall{ID: "call-1", Name: "bash", Summary: "Running shell command", Input: "git diff --stat"}, ToolPermissionUnknown, false),
			testToolEntry(ToolCall{ID: "call-1", Name: "bash", Summary: "Ran 1 shell command", Output: "1 file changed", Done: true}, ToolPermissionUnknown, false),
		},
	})
	if len(m.entries) != 1 {
		t.Fatalf("initial tool messages should merge into one block, got %d: %#v", len(m.entries), m.entries)
	}
}

func TestPOC2ExpandedToolBlockCanCollapseFromAnyRenderedLine(t *testing.T) {
	m := NewModel(Options{
		Width:  80,
		Height: 12,
		InitialEntries: []Entry{testToolEntry(ToolCall{
			ID: "call-1", Name: "bash", Summary: "Ran 1 shell command",
			Input: "go test ./pkg/modu-tui", Output: "ok ./pkg/modu-tui", Done: true,
		}, ToolPermissionUnknown, true)},
	})
	if !entryExpanded(m.entries[0]) {
		t.Fatal("setup should start expanded")
	}
	if _, ok := m.headers[1]; !ok {
		t.Fatalf("expanded tool output line should be clickable, headers=%#v", m.headers)
	}

	m = mouseClick(m, 1, 1)
	if entryExpanded(m.entries[0]) {
		t.Fatal("clicking an expanded tool output line should collapse the block")
	}
}

// An expanded tool block registers every rendered line as a clickable header
// so it can be collapsed from anywhere. That must not cost the ability to
// select its content: a press that turns into a drag is a selection, and only
// a press that never moved collapses.
func TestPOC2DraggingAcrossExpandedToolBlockSelectsInsteadOfCollapsing(t *testing.T) {
	m := NewModel(Options{
		Width:  80,
		Height: 12,
		InitialEntries: []Entry{testToolEntry(ToolCall{
			ID: "call-1", Name: "bash", Summary: "Ran 1 shell command",
			Input: "go test ./pkg/modu-tui", Output: "ok ./pkg/modu-tui", Done: true,
		}, ToolPermissionUnknown, true)},
	})
	if _, ok := m.headers[1]; !ok {
		t.Fatalf("setup expects line 1 to be a clickable header, headers=%#v", m.headers)
	}

	m, _ = mouseDrag(m, 2, 1, 20, 1)
	if !entryExpanded(m.entries[0]) {
		t.Fatal("dragging across an expanded tool block should select text, not collapse it")
	}
}

func TestPOC2CtrlOTogglesLatestToolAndReadsArtifact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "call-1.output")
	if err := os.WriteFile(path, []byte("full artifact output\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var tm tea.Model = NewModel(Options{
		Width:  100,
		Height: 20,
		InitialEntries: []Entry{testToolEntry(ToolCall{
			ID: "call-1", Name: "bash", Summary: "Ran 1 shell command",
			Output: "preview only", ArtifactID: "call-1", ArtifactPath: path,
			Truncated: true, Done: true,
		}, ToolPermissionUnknown, false)},
		Services: Services{LoadToolArtifact: func(path string) (string, error) {
			data, err := os.ReadFile(path)
			return string(data), err
		}},
	})
	m := tm.(Model)
	if entryExpanded(m.entries[0]) {
		t.Fatal("setup should start collapsed")
	}
	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))
	m = tm.(Model)
	if !entryExpanded(m.entries[0]) {
		t.Fatal("ctrl+o should expand latest tool")
	}
	rendered := ansi.Strip(m.render())
	if !strings.Contains(rendered, "full artifact output") || strings.Contains(rendered, "preview only") {
		t.Fatalf("expanded latest tool should render artifact, got:\n%s", rendered)
	}
}

func TestPOC2ExpandedArtifactIsCachedAcrossRebuilds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "call-1.output")
	if err := os.WriteFile(path, []byte("first artifact output\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var tm tea.Model = NewModel(Options{
		Width:  100,
		Height: 20,
		InitialEntries: []Entry{testToolEntry(ToolCall{
			ID: "call-1", Name: "bash", Summary: "Ran 1 shell command",
			Output: "preview only", ArtifactID: "call-1", ArtifactPath: path,
			Truncated: true, Done: true,
		}, ToolPermissionUnknown, false)},
		Services: Services{LoadToolArtifact: func(path string) (string, error) {
			data, err := os.ReadFile(path)
			return string(data), err
		}},
	})
	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: 'o', Mod: tea.ModCtrl}))
	m := tm.(Model)
	if !strings.Contains(ansi.Strip(m.render()), "first artifact output") {
		t.Fatalf("expanded tool should render first artifact read, got:\n%s", ansi.Strip(m.render()))
	}
	if err := os.WriteFile(path, []byte("second artifact output\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.rebuild()
	rendered := ansi.Strip(m.render())
	if !strings.Contains(rendered, "first artifact output") || strings.Contains(rendered, "second artifact output") {
		t.Fatalf("expanded tool should reuse cached artifact across rebuilds, got:\n%s", rendered)
	}
}

func TestPOC2ToolApprovalResolvesFromKeyboard(t *testing.T) {
	results := make(chan ToolApprovalResult, 1)
	decisions := make(chan ToolApprovalDecision, 1)
	var tm tea.Model = NewModel(Options{
		Width:  80,
		Height: 12,
		IntentHandler: testIntentHandler(testIntentCallbacks{approval: func(result ToolApprovalResult) {
			results <- result
		}}),
	})
	tm, _ = tm.Update(RequestToolApprovalMsg{
		Request: ToolApprovalRequest{
			ID:       "call-1",
			ToolName: "bash",
			Detail:   `{"command":"go test ./..."}`,
		},
		Respond: decisions,
	})

	pending := tm.(Model)
	if pending.approval == nil {
		t.Fatal("expected pending approval")
	}
	rendered := ansi.Strip(pending.render())
	if !strings.Contains(rendered, "Approval required for Bash") || !strings.Contains(rendered, "[y] allow") {
		t.Fatalf("pending approval not rendered:\n%s", rendered)
	}
	if got := strings.Join(pending.Lines(), "\n"); strings.Contains(ansi.Strip(got), "approval required") {
		t.Fatalf("approval should not be part of transcript lines:\n%s", got)
	}

	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Text: "a", Code: 'a'}))
	resolved := tm.(Model)
	if resolved.approval != nil {
		t.Fatal("approval should clear after decision")
	}
	select {
	case got := <-decisions:
		if got != ToolApprovalAllowAlways {
			t.Fatalf("decision = %q, want %q", got, ToolApprovalAllowAlways)
		}
	case <-time.After(time.Second):
		t.Fatal("expected approval decision")
	}
	select {
	case got := <-results:
		if got.Request.ID != "call-1" || got.Decision != ToolApprovalAllowAlways {
			t.Fatalf("intent result = %#v", got)
		}
	default:
		t.Fatal("expected approval intent result")
	}
}

func TestPOC2HumanPromptResolvesFromKeyboard(t *testing.T) {
	responses := make(chan string, 1)
	var tm tea.Model = NewModel(Options{Width: 80, Height: 12})
	tm, _ = tm.Update(RequestHumanPromptMsg{
		Request: HumanPromptRequest{
			Title: "Choose commit shape",
			Body:  "Split into 2 commits, or merge into 1?",
			Options: []HumanPromptOption{
				{Label: "2 commits", Value: "two"},
				{Label: "1 commit", Value: "one"},
			},
			DefaultIndex: 0,
		},
		Respond: responses,
	})

	pending := tm.(Model)
	if pending.humanPrompt == nil {
		t.Fatal("expected pending human prompt")
	}
	rendered := ansi.Strip(pending.render())
	for _, want := range []string{"Human input required", "Choose commit shape", "1. 2 commits", "[up/down] select", "human input pending"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("human prompt missing %q:\n%s", want, rendered)
		}
	}

	tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	moved := tm.(Model)
	if moved.humanPrompt == nil || moved.humanPrompt.selected != 1 {
		t.Fatalf("expected down key to select second option, got %#v", moved.humanPrompt)
	}

	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	resolved := tm.(Model)
	if resolved.humanPrompt != nil {
		t.Fatal("human prompt should clear after response")
	}
	select {
	case got := <-responses:
		if got != "one" {
			t.Fatalf("response = %q, want one", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expected human prompt response")
	}
}

func TestHumanPromptNavigatesPastNineOptions(t *testing.T) {
	responses := make(chan string, 1)
	options := make([]HumanPromptOption, 12)
	for i := range options {
		options[i] = HumanPromptOption{
			Label: fmt.Sprintf("Option %d", i+1),
			Value: fmt.Sprintf("value-%d", i+1),
		}
	}

	var tm tea.Model = NewModel(Options{Width: 80, Height: 30})
	tm, _ = tm.Update(RequestHumanPromptMsg{
		Request: HumanPromptRequest{
			Title:   "Choose an option",
			Options: options,
		},
		Respond: responses,
	})

	initial := tm.(Model)
	rendered := ansi.Strip(initial.render())
	for _, want := range []string{"1. Option 1", "9. Option 9", "1-9 of 12"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("initial prompt missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "10. Option 10") {
		t.Fatalf("initial prompt should keep later options below the visible window:\n%s", rendered)
	}

	for range 9 {
		tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	}
	moved := tm.(Model)
	if moved.humanPrompt == nil || moved.humanPrompt.selected != 9 {
		t.Fatalf("expected down key to select tenth option, got %#v", moved.humanPrompt)
	}
	rendered = ansi.Strip(moved.render())
	for _, want := range []string{"10. Option 10", "12. Option 12", "10-12 of 12"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("scrolled prompt missing %q:\n%s", want, rendered)
		}
	}

	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if resolved := tm.(Model); resolved.humanPrompt != nil {
		t.Fatal("human prompt should clear after selecting a later option")
	}
	select {
	case got := <-responses:
		if got != "value-10" {
			t.Fatalf("response = %q, want value-10", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expected human prompt response")
	}
}

func TestPOC2PanelRendersScrollableMainViewAndCloses(t *testing.T) {
	var tm tea.Model = NewModel(Options{Width: 60, Height: 12, InitialEntries: []Entry{
		testMarkdownEntry(RoleAssistant, "transcript stays behind panel"),
	}})
	tm, _ = tm.Update(UpdateMsg{Update: ShowPanelUpdate{Panel: Panel{
		ID:       "workflow",
		Title:    "Workflow Cockpit",
		Subtitle: "completed 1  running 0",
		Lines: []string{
			"overview",
			"run one",
			"run two",
			"run three",
			"run four",
			"run five",
			"run six",
			"run seven",
			"run eight",
			"run nine",
		},
		Footer: "[esc/q] close",
	}}})

	open := tm.(Model)
	rendered := ansi.Strip(open.render())
	for _, want := range []string{"Workflow Cockpit", "completed 1", "overview", "panel open", "● panel"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("panel render missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "transcript stays behind panel") {
		t.Fatalf("panel should replace viewport, not append transcript:\n%s", rendered)
	}

	tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
	scrolled := tm.(Model)
	if scrolled.panelOffset == 0 {
		t.Fatal("expected PgDown to scroll panel")
	}

	tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Text: "q"}))
	closed := tm.(Model)
	if closed.panel != nil {
		t.Fatal("expected q to close panel")
	}
	if got := ansi.Strip(closed.render()); !strings.Contains(got, "transcript stays behind panel") {
		t.Fatalf("transcript should return after panel closes:\n%s", got)
	}
}

func TestPOC2PanelRowsSelectAndEmitAction(t *testing.T) {
	actions := make(chan PanelAction, 1)
	var tm tea.Model = NewModel(Options{
		Width:  72,
		Height: 12,
		IntentHandler: testIntentHandler(testIntentCallbacks{
			panelAction: func(action PanelAction) {
				actions <- action
			},
		}),
	})
	tm, _ = tm.Update(UpdateMsg{Update: ShowPanelUpdate{Panel: Panel{
		ID:    "workflow",
		Title: "Workflow Cockpit",
		Rows: []PanelRow{
			{Label: "run one [completed]", Detail: "5/5 · 1min", Value: "run-one", Command: "/workflows show run-one"},
			{Label: "run two [running]", Detail: "2/5 · Research", Value: "run-two", Command: "/workflows show run-two"},
		},
	}}})
	open := tm.(Model)
	if open.panelSelected != 0 {
		t.Fatalf("panelSelected = %d, want 0", open.panelSelected)
	}
	rendered := ansi.Strip(open.render())
	for _, want := range []string{"run one [completed]", "5/5", "[up/down] select"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("selectable panel missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "↑") || strings.Contains(rendered, "↓") {
		t.Fatalf("panel footer should avoid arrow glyphs:\n%s", rendered)
	}

	tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	selected := tm.(Model)
	if selected.panelSelected != 1 {
		t.Fatalf("panelSelected = %d, want 1", selected.panelSelected)
	}
	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	afterEnter := tm.(Model)
	if afterEnter.panel == nil {
		t.Fatal("panel should stay open until the host replaces or clears it, to avoid a no-panel flicker frame")
	}
	select {
	case action := <-actions:
		if action.PanelID != "workflow" || action.Index != 1 || action.Row.Value != "run-two" || action.Command != "/workflows show run-two" {
			t.Fatalf("unexpected panel action: %#v", action)
		}
	case <-time.After(time.Second):
		t.Fatal("expected panel action")
	}
}

func TestPOC2PanelStylesTitleAndRendersMarkdownBlocks(t *testing.T) {
	var tm tea.Model = NewModel(Options{Width: 72, Height: 16})
	tm, _ = tm.Update(UpdateMsg{Update: ShowPanelUpdate{Panel: Panel{
		ID:       "workflow-result",
		Title:    "Workflow Result",
		Markdown: true,
		Lines: []string{
			"context",
			"workflow: market_watch",
			"",
			"# Report",
			"- market breadth improved",
			"- watch policy headlines",
		},
		Rows: []PanelRow{{Label: "Back", Command: "back"}},
	}}})

	model := tm.(Model)
	rendered := model.render()
	stripped := ansi.Strip(rendered)
	if !strings.Contains(rendered, panelTitleStyle.Render("Workflow Result")) {
		t.Fatalf("panel title should use panel title style:\n%q", rendered)
	}
	if !strings.Contains(rendered, panelSectionStyle.Render("context")) {
		t.Fatalf("plain section heading should use panel section style:\n%q", rendered)
	}
	if strings.Contains(stripped, "# Report") {
		t.Fatalf("markdown heading should be rendered instead of shown raw:\n%s", stripped)
	}
	for _, want := range []string{"Report", "market breadth improved", "watch policy headlines"} {
		if !strings.Contains(stripped, want) {
			t.Fatalf("rendered panel markdown missing %q:\n%s", want, stripped)
		}
	}
}

func TestPOC2PanelRendersMarkdownParagraphsAndFencedCode(t *testing.T) {
	var tm tea.Model = NewModel(Options{Width: 80, Height: 18})
	tm, _ = tm.Update(UpdateMsg{Update: ShowPanelUpdate{Panel: Panel{
		ID:       "workflow-result",
		Title:    "Workflow Result",
		Markdown: true,
		Lines: []string{
			"## Markdown report",
			"This is **important** and `inline`.",
			"",
			"```go",
			"package main",
			"",
			"func main() {}",
			"```",
		},
	}}})

	model := tm.(Model)
	stripped := ansi.Strip(model.render())
	for _, raw := range []string{"## Markdown report", "**important**", "`inline`", "```go"} {
		if strings.Contains(stripped, raw) {
			t.Fatalf("panel markdown should render %q instead of showing it raw:\n%s", raw, stripped)
		}
	}
	for _, want := range []string{"Markdown report", "important", "inline", "package main", "func main() {}"} {
		if !strings.Contains(stripped, want) {
			t.Fatalf("rendered panel markdown missing %q:\n%s", want, stripped)
		}
	}
}

func TestPOC2PanelDoesNotRenderMarkdownByDefault(t *testing.T) {
	var tm tea.Model = NewModel(Options{Width: 80, Height: 18})
	tm, _ = tm.Update(UpdateMsg{Update: ShowPanelUpdate{Panel: Panel{
		ID:    "workflow-script",
		Title: "Workflow Script",
		Lines: []string{
			"script",
			"return \"# Smoke Report\"",
			"```txt",
			"result code block",
			"```",
		},
	}}})

	model := tm.(Model)
	stripped := ansi.Strip(model.render())
	for _, want := range []string{"return \"# Smoke Report\"", "```txt", "result code block", "```"} {
		if !strings.Contains(stripped, want) {
			t.Fatalf("plain panel should keep script markdown-looking text %q:\n%s", want, stripped)
		}
	}
}

func TestPOC2PanelShortcutEmitsAction(t *testing.T) {
	actions := make(chan PanelAction, 1)
	var tm tea.Model = NewModel(Options{
		Width:  72,
		Height: 12,
		IntentHandler: testIntentHandler(testIntentCallbacks{
			panelAction: func(action PanelAction) {
				actions <- action
			},
		}),
	})
	tm, _ = tm.Update(UpdateMsg{Update: ShowPanelUpdate{Panel: Panel{
		ID:    "workflow-run",
		Title: "Workflow Run",
		Rows: []PanelRow{
			{Label: "Open agents", Command: "workflow-panel:agents:run-1"},
		},
		Shortcuts: []PanelShortcut{{
			Key:     "x",
			Label:   "Stop",
			Command: "workflow-panel:control:stop:run-1",
		}},
	}}})
	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Text: "x", Code: 'x'}))

	select {
	case action := <-actions:
		if action.PanelID != "workflow-run" || action.Index != -1 || action.Command != "workflow-panel:control:stop:run-1" || action.Row.Label != "Stop" {
			t.Fatalf("unexpected shortcut action: %#v", action)
		}
	case <-time.After(time.Second):
		t.Fatal("expected shortcut action")
	}
	if tm.(Model).panel == nil {
		t.Fatal("panel should stay open until the host replaces or clears it, to avoid a no-panel flicker frame")
	}
}

func TestPOC2PanelRefreshPreservesSelectionAndCloseHook(t *testing.T) {
	closed := make(chan string, 1)
	var tm tea.Model = NewModel(Options{
		Width:  72,
		Height: 12,
		IntentHandler: testIntentHandler(testIntentCallbacks{
			panelClosed: func(panelID string) {
				closed <- panelID
			},
		}),
	})
	tm, _ = tm.Update(UpdateMsg{Update: ShowPanelUpdate{Panel: Panel{
		ID:    "workflow",
		Title: "Workflow Cockpit",
		Rows: []PanelRow{
			{Label: "run one", Value: "run-one"},
			{Label: "run two", Value: "run-two"},
		},
	}}})
	tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	selected := tm.(Model)
	if selected.panelSelected != 1 {
		t.Fatalf("panelSelected before refresh = %d, want 1", selected.panelSelected)
	}

	tm, _ = tm.Update(UpdateMsg{Update: RefreshPanelUpdate{Panel: Panel{
		ID:    "workflow",
		Title: "Workflow Cockpit",
		Rows: []PanelRow{
			{Label: "run one [done]", Value: "run-one"},
			{Label: "run two [running]", Value: "run-two"},
			{Label: "run three [queued]", Value: "run-three"},
		},
	}}})
	refreshed := tm.(Model)
	if refreshed.panelSelected != 1 {
		t.Fatalf("panelSelected after refresh = %d, want 1", refreshed.panelSelected)
	}
	rendered := ansi.Strip(refreshed.render())
	if !strings.Contains(rendered, "run two [running]") {
		t.Fatalf("refreshed panel content missing updated row:\n%s", rendered)
	}

	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Text: "q"}))
	if tm.(Model).panel != nil {
		t.Fatal("panel should close after q")
	}
	select {
	case panelID := <-closed:
		if panelID != "workflow" {
			t.Fatalf("closed panel id = %q, want workflow", panelID)
		}
	case <-time.After(time.Second):
		t.Fatal("expected panel close intent")
	}
}

func TestPOC2PanelSelectionStaysVisible(t *testing.T) {
	rows := make([]PanelRow, 0, 16)
	for i := 0; i < 16; i++ {
		rows = append(rows, PanelRow{Label: fmt.Sprintf("run-%02d", i+1), Command: fmt.Sprintf("/workflows show run-%02d", i+1)})
	}
	var tm tea.Model = NewModel(Options{Width: 60, Height: 10})
	tm, _ = tm.Update(UpdateMsg{Update: ShowPanelUpdate{Panel: Panel{
		ID:    "workflow",
		Title: "Workflow Cockpit",
		Rows:  rows,
	}}})
	for i := 0; i < 12; i++ {
		tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	}
	m := tm.(Model)
	if m.panelSelected != 12 {
		t.Fatalf("panelSelected = %d, want 12", m.panelSelected)
	}
	if m.panelOffset == 0 {
		t.Fatal("expected panel offset to follow selected row")
	}
	rendered := ansi.Strip(m.render())
	if !strings.Contains(rendered, "run-13") {
		t.Fatalf("selected row should be visible:\n%s", rendered)
	}
}

func TestPOC2HumanTextSecretInputMasksAndResolves(t *testing.T) {
	responses := make(chan string, 1)
	var tm tea.Model = NewModel(Options{Width: 80, Height: 18})
	tm, _ = tm.Update(RequestHumanTextMsg{
		Request: HumanTextRequest{
			Title:       "API key",
			Body:        "Paste API key",
			Placeholder: "sk-...",
			Secret:      true,
			Required:    true,
		},
		Respond: responses,
	})
	for _, r := range "sk-secret" {
		tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Text: string(r), Code: r}))
	}
	pending := tm.(Model)
	rendered := ansi.Strip(pending.render())
	if strings.Contains(rendered, "sk-secret") {
		t.Fatalf("secret input should be masked:\n%s", rendered)
	}
	if !strings.Contains(rendered, "*********") || !strings.Contains(rendered, "[enter] save") {
		t.Fatalf("secret prompt missing masked value/actions:\n%s", rendered)
	}

	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	resolved := tm.(Model)
	if resolved.humanText != nil {
		t.Fatal("human text prompt should clear after response")
	}
	select {
	case got := <-responses:
		if got != "sk-secret" {
			t.Fatalf("response = %q, want sk-secret", got)
		}
	default:
		t.Fatal("expected human text response")
	}
}

func TestPOC2ToolApprovalPanelIsFixedAboveInput(t *testing.T) {
	var tm tea.Model = NewModel(Options{
		Width:  50,
		Height: 12,
		InitialEntries: []Entry{
			testTextEntry(RoleAssistant, strings.Repeat("history\n", 12)),
		},
	})
	tm, _ = tm.Update(RequestToolApprovalMsg{
		Request: ToolApprovalRequest{
			ID:       "call-1",
			ToolName: "bash",
			Summary:  "approval required: bash",
			Detail:   "go test ./...",
		},
		Respond: make(chan ToolApprovalDecision, 1),
	})

	m := tm.(Model)
	rendered := strings.Split(ansi.Strip(m.render()), "\n")
	if got, want := len(rendered), m.height; got != want {
		t.Fatalf("rendered lines = %d, want %d:\n%s", got, want, strings.Join(rendered, "\n"))
	}
	panelTop := m.vpHeight()
	if !strings.HasPrefix(rendered[panelTop], "┏") {
		t.Fatalf("approval panel should start immediately below viewport at line %d:\n%s", panelTop, strings.Join(rendered, "\n"))
	}
	if got, want := approvalBorderStyle.GetForeground(), lipgloss.Color("248"); got != want {
		t.Fatalf("approval border color = %#v, want %#v", got, want)
	}
	inputRule := m.vpHeight() + m.approvalPanelHeight() + 2
	if got, want := rendered[inputRule], strings.Repeat("─", m.width); got != want {
		t.Fatalf("input top rule line = %q, want %q", got, want)
	}
	panelEnd := panelTop + m.approvalPanelHeight()
	if !strings.Contains(strings.Join(rendered[panelTop:panelEnd], "\n"), "[y] allow") {
		t.Fatalf("approval panel should include actions:\n%s", strings.Join(rendered[panelTop:panelEnd], "\n"))
	}
	if !strings.Contains(strings.Join(rendered[panelTop:panelEnd], "\n"), "go test ./...") {
		t.Fatalf("approval panel should include command preview:\n%s", strings.Join(rendered[panelTop:panelEnd], "\n"))
	}
}

func TestPOC2TodoPanelRendersAboveInput(t *testing.T) {
	var tm tea.Model = NewModel(Options{
		Width:  50,
		Height: 12,
		InitialEntries: []Entry{
			testTextEntry(RoleAssistant, strings.Repeat("history\n", 12)),
		},
		Todos: []TodoItem{
			{Content: "first step", Status: "in_progress"},
			{Content: "second step", Status: "pending"},
		},
	})
	tm, _ = tm.Update(UpdateMsg{Update: SetBusyUpdate{Busy: true}})
	tm, _ = tm.Update(UpdateMsg{Update: SetTodoListUpdate{Items: []TodoItem{
		{Content: "first step", Status: "in_progress"},
		{Content: "second step", Status: "pending"},
	}}})
	m := tm.(Model)

	rendered := strings.Split(ansi.Strip(m.render()), "\n")
	if got, want := len(rendered), m.height; got != want {
		t.Fatalf("rendered lines = %d, want %d:\n%s", got, want, strings.Join(rendered, "\n"))
	}
	panelTop := m.vpHeight()
	inputRule := m.vpHeight() + m.todoPanelHeight() + 2
	if !strings.HasPrefix(rendered[panelTop], "┏") {
		t.Fatalf("todo panel should start immediately below viewport at line %d:\n%s", panelTop, strings.Join(rendered, "\n"))
	}
	panel := strings.Join(rendered[panelTop:inputRule], "\n")
	for _, want := range []string{"Todos", "first step", "second step"} {
		if !strings.Contains(panel, want) {
			t.Fatalf("todo panel missing %q:\n%s", want, panel)
		}
	}
	if got, want := rendered[inputRule], strings.Repeat("─", m.width); got != want {
		t.Fatalf("input top rule line = %q, want %q", got, want)
	}
}

func TestPOC2TodoPanelHiddenWhileIdle(t *testing.T) {
	m := NewModel(Options{
		Width:  50,
		Height: 10,
		Todos:  []TodoItem{{Content: "stale task", Status: "pending"}},
	})

	if got := ansi.Strip(m.render()); strings.Contains(got, "Todos") || strings.Contains(got, "stale task") {
		t.Fatalf("idle model should hide todo panel:\n%s", got)
	}
}

func TestPOC2TodoPanelIgnoresStaleTodosOnNewRun(t *testing.T) {
	var tm tea.Model = NewModel(Options{
		Width:  50,
		Height: 10,
		Todos:  []TodoItem{{Content: "old task", Status: "pending"}},
	})
	tm, _ = tm.Update(UpdateMsg{Update: SetBusyUpdate{Busy: true}})

	m := tm.(Model)
	if got := ansi.Strip(m.render()); strings.Contains(got, "Todos") || strings.Contains(got, "old task") {
		t.Fatalf("new run should not show stale todos before current SetTodoListUpdate:\n%s", got)
	}

	tm, _ = tm.Update(UpdateMsg{Update: SetTodoListUpdate{Items: []TodoItem{{Content: "current task", Status: "pending"}}}})
	m = tm.(Model)
	if got := ansi.Strip(m.render()); !strings.Contains(got, "current task") {
		t.Fatalf("current-run todo update should show panel:\n%s", got)
	}
}

func TestPOC2SetTodoListUpdateUpdatesTodoPanel(t *testing.T) {
	var tm tea.Model = NewModel(Options{Width: 50, Height: 10})
	tm, _ = tm.Update(UpdateMsg{Update: SetBusyUpdate{Busy: true}})
	tm, _ = tm.Update(UpdateMsg{Update: SetTodoListUpdate{Items: []TodoItem{{Content: "new task", Status: "pending"}}}})
	m := tm.(Model)
	if got := ansi.Strip(m.render()); !strings.Contains(got, "new task") {
		t.Fatalf("expected todo panel after SetTodoListUpdate:\n%s", got)
	}

	tm, _ = tm.Update(UpdateMsg{Update: SetTodoListUpdate{Items: []TodoItem{{Content: "new task", Status: "completed"}}}})
	m = tm.(Model)
	if got := ansi.Strip(m.render()); strings.Contains(got, "new task") || strings.Contains(got, "Todos") {
		t.Fatalf("completed-only todos should hide panel:\n%s", got)
	}
}

func TestPOC2TransientStatusExpiresWithoutClearingNewStatus(t *testing.T) {
	var tm tea.Model = NewModel()
	var cmd tea.Cmd
	tm, cmd = tm.Update(UpdateMsg{Update: SetStatusUpdate{Status: "✓ Completed 1s", TTL: time.Second}})
	if cmd == nil {
		t.Fatal("transient status should schedule an expiry command")
	}
	m := tm.(Model)
	if got := m.status; got != "✓ Completed 1s" {
		t.Fatalf("status = %q", got)
	}

	tm, _ = tm.Update(UpdateMsg{Update: SetStatusUpdate{Status: "running"}})
	tm, _ = tm.Update(statusExpireMsg{status: "✓ Completed 1s"})
	m = tm.(Model)
	if got := m.status; got != "running" {
		t.Fatalf("old transient expiry should not clear new status, got %q", got)
	}

	tm, _ = tm.Update(UpdateMsg{Update: SetStatusUpdate{Status: "done", TTL: time.Second}})
	m = tm.(Model)
	m.statusExpiresAt = time.Now().Add(-time.Millisecond)
	tm = m
	tm, _ = tm.Update(statusExpireMsg{status: "done"})
	m = tm.(Model)
	if got := m.status; got != "" {
		t.Fatalf("expired transient status should clear, got %q", got)
	}
}

func TestPOC2ToolApprovalBlocksInputEditing(t *testing.T) {
	var tm tea.Model = NewModel()
	tm, _ = tm.Update(RequestToolApprovalMsg{
		Request: ToolApprovalRequest{ID: "call-1", ToolName: "read"},
		Respond: make(chan ToolApprovalDecision, 1),
	})
	tm, _ = tm.Update(tea.KeyPressMsg(tea.Key{Text: "x", Code: 'x'}))

	m := tm.(Model)
	if got := m.input.Value; got != "" {
		t.Fatalf("approval mode should not edit input, got %q", got)
	}
	if m.approval == nil {
		t.Fatal("approval should remain pending after unrelated key")
	}
}

func TestReplaceEntriesUpdateReplacesTranscript(t *testing.T) {
	var tm tea.Model = NewModel(Options{Width: 72, Height: 12, InitialEntries: []Entry{
		testTextEntry(RoleUser, "old conversation"),
		testMarkdownEntry(RoleAssistant, "old reply"),
	}})
	m := tm.(Model)

	tm, _ = m.Update(UpdateMsg{Update: ReplaceEntriesUpdate{Entries: []Entry{
		testTextEntry(RoleUser, "resumed question"),
		testMarkdownEntry(RoleAssistant, "resumed answer"),
	}}})
	m = tm.(Model)

	rendered := ansi.Strip(m.render())
	if strings.Contains(rendered, "old conversation") || strings.Contains(rendered, "old reply") {
		t.Fatalf("old transcript should be gone after ReplaceEntriesUpdate:\n%s", rendered)
	}
	for _, want := range []string{"resumed question", "resumed answer"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("replayed transcript missing %q:\n%s", want, rendered)
		}
	}
}

func TestStreamingTextNodeEntryRendersLiteralSource(t *testing.T) {
	// Model doesn't force a rendering mode onto Streaming entries — whichever
	// Node kind the host builds it from is respected. Exercised at the
	// render level (entries + rebuild directly) rather than through
	// Update(UpsertEntryUpdate{...}), since that path now throttles
	// streaming rebuilds (see TestUpsertEntryUpdateThrottlesStreamingRebuild)
	// and this test is about rendering, not scheduling.
	var tm tea.Model = NewModel(Options{Width: 72, Height: 12})
	m := tm.(Model)
	m.entries = append(m.entries, Entry{
		ID:        "live-1",
		Role:      RoleAssistant,
		Nodes:     []Node{TextNode{Text: "**bold** stays literal as TextNode"}},
		Streaming: true,
	})
	m.rebuild()

	rendered := ansi.Strip(m.render())
	if !strings.Contains(rendered, "**bold** stays literal as TextNode") {
		t.Fatalf("a Streaming TextNode entry should render its source verbatim (no glamour pass), got:\n%s", rendered)
	}
}

func TestStreamingMarkdownNodeEntryRendersStyled(t *testing.T) {
	// The live assistant reply is built as a MarkdownNode (see
	// moduTUILiveAssistantTextEntry in cmd/modu_code) specifically so it
	// looks the same mid-stream as it will once finished, instead of
	// showing raw markdown syntax that suddenly formats at message_end.
	// This confirms Model actually renders a Streaming MarkdownNode entry
	// through glamour, not just literally.
	var tm tea.Model = NewModel(Options{Width: 72, Height: 12})
	m := tm.(Model)
	m.entries = append(m.entries, Entry{
		ID:        "live-1",
		Role:      RoleAssistant,
		Nodes:     []Node{MarkdownNode{Text: "**bold**"}},
		Streaming: true,
	})
	m.rebuild()

	rendered := m.render()
	if strings.Contains(ansi.Strip(rendered), "**bold**") {
		t.Fatalf("a Streaming MarkdownNode entry should be glamour-rendered, not shown as literal source:\n%s", rendered)
	}
	if !strings.Contains(rendered, "\x1b[") {
		t.Fatalf("expected glamour's styling (ANSI escapes) in the streamed markdown render:\n%q", rendered)
	}
}

func TestUpsertEntryUpdateThrottlesStreamingRebuild(t *testing.T) {
	var tm tea.Model = NewModel(Options{Width: 72, Height: 12})
	m := tm.(Model)

	tm, cmd := m.Update(UpdateMsg{Update: UpsertEntryUpdate{Entry: Entry{
		ID:        "live-1",
		Role:      RoleAssistant,
		Nodes:     []Node{MarkdownNode{Text: "partial reply"}},
		Streaming: true,
	}}})
	m = tm.(Model)
	if cmd == nil {
		t.Fatal("a streaming upsert should arm the throttled render tick")
	}
	if !m.streamRenderPending {
		t.Fatal("streaming upsert should mark a render pending rather than rebuilding immediately")
	}
	if strings.Contains(ansi.Strip(m.render()), "partial reply") {
		t.Fatal("the transcript should not show the new content before the throttle tick flushes it")
	}

	tm, _ = m.Update(streamRenderTickMsg{})
	m = tm.(Model)
	if m.streamRenderPending {
		t.Fatal("the tick should have cleared the pending flag")
	}
	if !strings.Contains(ansi.Strip(m.render()), "partial reply") {
		t.Fatal("the transcript should show the streamed content once the throttle tick flushes it")
	}
}

func TestRenderEntryLinesReusesCacheForUnchangedEntry(t *testing.T) {
	var tm tea.Model = NewModel(Options{Width: 72, Height: 12})
	m := tm.(Model)
	m.entries = append(m.entries, Entry{ID: "msg-1", Role: RoleAssistant, Nodes: []Node{MarkdownNode{Text: "hello **world**"}}})
	m.rebuild()

	first, ok := m.blockRenderCache["msg-1"]
	if !ok {
		t.Fatal("finalized entry should populate the render cache after rebuild")
	}

	// Poison the cached lines directly: if rebuild() actually re-rendered the
	// entry (instead of reusing the cache), it would overwrite this with the
	// real render and the assertion below would fail.
	m.blockRenderCache["msg-1"] = blockRenderCacheEntry{entries: first.entries, width: first.width, lines: []RenderedLine{{Text: "POISONED"}}}
	m.rebuild()

	rendered := ansi.Strip(m.render())
	if !strings.Contains(rendered, "POISONED") {
		t.Fatalf("rebuild should have reused the cached lines for an unchanged entry, got:\n%s", rendered)
	}
}

func TestRenderEntryLinesInvalidatesCacheOnContentChange(t *testing.T) {
	var tm tea.Model = NewModel(Options{Width: 72, Height: 12})
	m := tm.(Model)
	m.entries = append(m.entries, Entry{ID: "msg-1", Role: RoleAssistant, Nodes: []Node{MarkdownNode{Text: "first version"}}})
	m.rebuild()
	if rendered := ansi.Strip(m.render()); !strings.Contains(rendered, "first version") {
		t.Fatalf("expected first version rendered, got:\n%s", rendered)
	}

	m.entries[0] = Entry{ID: "msg-1", Role: RoleAssistant, Nodes: []Node{MarkdownNode{Text: "second version"}}}
	m.rebuild()

	rendered := ansi.Strip(m.render())
	if strings.Contains(rendered, "first version") {
		t.Fatalf("stale cached content should not survive a content change:\n%s", rendered)
	}
	if !strings.Contains(rendered, "second version") {
		t.Fatalf("updated content should be rendered after a content change:\n%s", rendered)
	}
}

func TestUpsertEntryFinalizesStreamingEntryInPlace(t *testing.T) {
	var tm tea.Model = NewModel(Options{Width: 72, Height: 12, InitialEntries: []Entry{
		testTextEntry(RoleUser, "question"),
	}})
	m := tm.(Model)

	tm, _ = m.Update(UpdateMsg{Update: UpsertEntryUpdate{Entry: Entry{
		ID:        "live-1",
		Role:      RoleAssistant,
		Nodes:     []Node{TextNode{Text: "partial reply"}},
		Streaming: true,
	}}})
	m = tm.(Model)
	if len(m.entries) != 2 {
		t.Fatalf("live update should upsert into a single entry, got %d entries", len(m.entries))
	}

	tm, _ = m.Update(UpdateMsg{Update: UpsertEntryUpdate{Entry: Entry{
		ID:    "live-1",
		Role:  RoleAssistant,
		Nodes: []Node{MarkdownNode{Text: "final **reply**"}},
	}}})
	m = tm.(Model)

	if len(m.entries) != 2 {
		t.Fatalf("finalizing the streaming entry should replace it in place, not append; got %d entries", len(m.entries))
	}
	rendered := ansi.Strip(m.render())
	if strings.Contains(rendered, "partial reply") {
		t.Fatalf("finalized transcript should not still show the partial streaming text:\n%s", rendered)
	}
	if !strings.Contains(rendered, "reply") {
		t.Fatalf("finalized transcript missing the final reply:\n%s", rendered)
	}
}

func TestRenderGroupLinesReusesCacheForUnchangedBatch(t *testing.T) {
	mv := NewModel(Options{Width: 80, Height: 24})
	m := &mv
	m.blockGap = 0
	batch := "batch-x"
	for _, call := range []ToolCall{
		{ID: "1", Name: "read", Summary: "Read a.txt", Done: true, BatchSize: 2, BatchID: batch},
		{ID: "2", Name: "read", Summary: "Read b.txt", Done: true, BatchSize: 2, BatchID: batch},
	} {
		m.appendEntry(Entry{Role: RoleAssistant, Nodes: []Node{ToolNode{Call: call}}})
	}
	m.rebuild()

	cached, ok := m.blockRenderCache["group:1"]
	if !ok {
		t.Fatal("batched group should populate the render cache under its first entry's id")
	}

	// Poison the cached lines: if rebuild() actually re-rendered the group
	// (instead of reusing the cache), it would overwrite this with the real
	// render and the assertion below would fail.
	m.blockRenderCache["group:1"] = blockRenderCacheEntry{entries: cached.entries, width: cached.width, lines: []RenderedLine{{Text: "POISONED"}}}
	m.rebuild()

	rendered := ansi.Strip(m.render())
	if !strings.Contains(rendered, "POISONED") {
		t.Fatalf("rebuild should have reused the cached lines for an unchanged batch, got:\n%s", rendered)
	}
}

func TestRenderGroupLinesInvalidatesOnExpandToggle(t *testing.T) {
	// Regression test: cachedRenderLines' snapshot must be a deep clone
	// (cloneEntry), not a shallow slice copy. setEntryExpanded mutates
	// Entry.Nodes in place (entry.Nodes[index] = tool); a shallow copy would
	// keep sharing that backing array, so the cached "old" snapshot would
	// silently pick up the mutation too and never appear to differ.
	mv := NewModel(Options{Width: 80, Height: 24})
	m := &mv
	m.blockGap = 0
	batch := "batch-x"
	for _, call := range []ToolCall{
		{ID: "1", Name: "read", Summary: "Read a.txt", Done: true, BatchSize: 2, BatchID: batch},
		{ID: "2", Name: "read", Summary: "Read b.txt", Done: true, BatchSize: 2, BatchID: batch},
	} {
		m.appendEntry(Entry{Role: RoleAssistant, Nodes: []Node{ToolNode{Call: call}}})
	}
	m.rebuild()
	if rendered := ansi.Strip(m.render()); strings.Contains(rendered, "Read b.txt") {
		t.Fatalf("collapsed batch should not show child rows:\n%s", rendered)
	}

	if !m.toggleLatestToolExpansion() {
		t.Fatal("toggleLatestToolExpansion returned false")
	}
	m.rebuild()

	rendered := ansi.Strip(m.render())
	for _, want := range []string{"Read a.txt", "Read b.txt"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expanded batch missing %q (stale cache?):\n%s", want, rendered)
		}
	}
}

func TestMarkdownRendererForWidthMemoizes(t *testing.T) {
	var tm tea.Model = NewModel(Options{Width: 72, Height: 12})
	m := tm.(Model)

	r1 := m.markdownRendererForWidth(72)
	r2 := m.markdownRendererForWidth(72)
	if r1 != r2 {
		t.Fatal("same width should return the memoized renderer, not a new one")
	}

	r3 := m.markdownRendererForWidth(40)
	if r3 == r1 {
		t.Fatal("a different width should not reuse another width's renderer")
	}
}

func newAtMentionTestModel(t *testing.T, files ...string) tea.Model {
	t.Helper()
	var tm tea.Model = NewModel(Options{
		Width: 80, Height: 20,
		Services: Services{
			ListFiles: func(query string) []string {
				return rankTestFiles(files, query)
			},
		},
	})
	return tm
}

func rankTestFiles(files []string, query string) []string {
	if query == "" {
		return files
	}
	var out []string
	for _, f := range files {
		if strings.Contains(strings.ToLower(f), strings.ToLower(query)) {
			out = append(out, f)
		}
	}
	return out
}

func typeInto(t *testing.T, model tea.Model, text string) tea.Model {
	t.Helper()
	for _, r := range text {
		model = updateAndRunImmediate(t, model, tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
	return model
}

func TestAtMentionPopupOpensAndCompletes(t *testing.T) {
	model := newAtMentionTestModel(t, "pkg/modu-tui/model.go", "cmd/modu_code/main.go")

	model = typeInto(t, model, "@mod")
	m := model.(Model)
	if len(m.atMatches) == 0 {
		t.Fatal("typing an @query should populate file matches")
	}
	if !strings.Contains(ansi.Strip(m.render()), "pkg/modu-tui/model.go") {
		t.Fatalf("the completion popup should list matching files:\n%s", ansi.Strip(m.render()))
	}

	model = updateAndRunImmediate(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	m = model.(Model)
	if got, want := m.input.Value, "@pkg/modu-tui/model.go "; got != want {
		t.Fatalf("Tab should replace the @query with the selected path: got %q, want %q", got, want)
	}
	if len(m.atMatches) != 0 {
		t.Fatal("completing should dismiss the popup")
	}
}

func TestAtMentionCompletesMidSentenceWithoutTouchingOtherText(t *testing.T) {
	model := newAtMentionTestModel(t, "pkg/modu-tui/model.go")

	model = typeInto(t, model, "explain @mod")
	model = updateAndRunImmediate(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))

	m := model.(Model)
	if got, want := m.input.Value, "explain @pkg/modu-tui/model.go "; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAtMentionEnterAcceptsInsteadOfSubmitting(t *testing.T) {
	model := newAtMentionTestModel(t, "pkg/modu-tui/model.go")
	model = typeInto(t, model, "@mod")

	model = updateAndRunImmediate(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m := model.(Model)
	if got, want := m.input.Value, "@pkg/modu-tui/model.go "; got != want {
		t.Fatalf("Enter with the popup open should accept the path, not submit: input = %q, want %q", got, want)
	}
	if len(m.entries) != 0 {
		t.Fatalf("Enter should not have submitted a message, got %d entries", len(m.entries))
	}
}

func TestAtMentionEscDismissesPopupWithoutClearingInput(t *testing.T) {
	model := newAtMentionTestModel(t, "pkg/modu-tui/model.go")
	model = typeInto(t, model, "@mod")

	model = updateAndRunImmediate(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	m := model.(Model)
	if len(m.atMatches) != 0 {
		t.Fatal("Esc should dismiss the file popup")
	}
	if m.input.Value != "@mod" {
		t.Fatalf("Esc should leave typed text alone, got %q", m.input.Value)
	}
}

func TestAtMentionArrowKeysMoveSelection(t *testing.T) {
	model := newAtMentionTestModel(t, "a/model.go", "b/model.go", "c/model.go")
	model = typeInto(t, model, "@model")

	model = updateAndRunImmediate(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	if got := model.(Model).atIndex; got != 1 {
		t.Fatalf("Down should advance the selection, got %d", got)
	}
	model = updateAndRunImmediate(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	if got := model.(Model).atIndex; got != 0 {
		t.Fatalf("Up should move the selection back, got %d", got)
	}

	model = updateAndRunImmediate(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	if got, want := model.(Model).input.Value, "@a/model.go "; got != want {
		t.Fatalf("Tab should complete the highlighted entry, got %q want %q", got, want)
	}
}

func TestAtMentionStaleResultsAreDiscarded(t *testing.T) {
	var tm tea.Model = NewModel(Options{Width: 80, Height: 20})
	m := tm.(Model)
	m.input.Insert("@newer")
	m.input.Cursor = m.input.Len()

	// A lookup for an older, shorter query resolving late must not replace
	// what the user has since typed.
	m.applyAtMatches(atFilesLoadedMsg{query: "old", paths: []string{"stale/path.go"}})
	if len(m.atMatches) != 0 {
		t.Fatalf("results for a stale query should be discarded, got %#v", m.atMatches)
	}

	m.applyAtMatches(atFilesLoadedMsg{query: "newer", paths: []string{"fresh/path.go"}})
	if len(m.atMatches) != 1 || m.atMatches[0] != "fresh/path.go" {
		t.Fatalf("results matching the current query should apply, got %#v", m.atMatches)
	}
}

func TestAtMentionDoesNotOpenWithoutListFilesService(t *testing.T) {
	var model tea.Model = NewModel(Options{Width: 80, Height: 20})
	model = typeInto(t, model, "@mod")
	if len(model.(Model).atMatches) != 0 {
		t.Fatal("no popup should appear when the host provides no ListFiles service")
	}
}

func TestRunningHintMatchesWhatTheKeysActuallyDo(t *testing.T) {
	// This is the test that was missing when the keys were swapped: the hint
	// kept advertising the old mapping. Deriving it from Enter's real behavior
	// means the hint cannot silently drift from it again.
	var submitted []SubmitEvent
	var tm tea.Model = NewModel(Options{
		IntentHandler: testIntentHandler(testIntentCallbacks{submit: func(ev SubmitEvent) {
			submitted = append(submitted, ev)
		}}),
	})
	tm, _ = tm.Update(UpdateMsg{Update: SetBusyUpdate{Busy: true}})
	tm, _ = tm.Update(tea.PasteMsg{Content: "mid-run message"})
	tm = updateAndRunImmediate(t, tm, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))

	if len(submitted) != 0 || len(tm.(Model).queued) != 1 {
		t.Fatalf("Enter mid-run must queue; submitted=%#v queued=%#v", submitted, tm.(Model).queued)
	}
	if !strings.Contains(runningInputHint, "Enter 排队") {
		t.Fatalf("hint %q should say Enter queues", runningInputHint)
	}
	if !strings.Contains(runningInputHint, "Esc") {
		t.Fatalf("hint %q should mention Esc, the other thing you can do to a running task", runningInputHint)
	}
}

func TestRunningHintYieldsToTransientStatuses(t *testing.T) {
	// The hint shares one line with the status, so a just-sent message's
	// status must replace it rather than being appended after it.
	for _, status := range []string{StatusInterjected, StatusQueued, "interrupting"} {
		if runStatusAllowsHint(status) {
			t.Fatalf("status %q should hide the key hint", status)
		}
	}
	for _, status := range []string{"", "running", "idle"} {
		if !runStatusAllowsHint(status) {
			t.Fatalf("status %q should still show the key hint", status)
		}
	}
}

func TestTransientStatusReplacesHintThenRestoresIt(t *testing.T) {
	var tm tea.Model = NewModel(Options{Width: 100, Height: 12})
	tm, _ = tm.Update(UpdateMsg{Update: SetBusyUpdate{Busy: true}})
	busy := tm.(Model)
	if !strings.Contains(ansi.Strip(busy.render()), "Enter 排队") {
		t.Fatal("a running task should advertise the keys")
	}

	// A short TTL here so the test exercises the restore path without
	// sleeping for TransientStatusTTL; the constant itself is checked below.
	tm, _ = tm.Update(UpdateMsg{Update: SetStatusUpdate{Status: StatusInterjected, TTL: time.Millisecond}})
	shown := tm.(Model)
	rendered := ansi.Strip(shown.render())
	if !strings.Contains(rendered, "已插话") {
		t.Fatalf("the interjected status should be visible:\n%s", rendered)
	}
	if strings.Contains(rendered, "Enter 排队") {
		t.Fatalf("the hint should step aside while the status shows:\n%s", rendered)
	}

	// Expiry restores the hint, so the keys are not hidden for the rest of
	// the turn.
	time.Sleep(5 * time.Millisecond)
	tm, _ = tm.Update(statusExpireMsg{status: StatusInterjected})
	restored := tm.(Model)
	if !strings.Contains(ansi.Strip(restored.render()), "Enter 排队") {
		t.Fatal("the hint should come back once the transient status expires")
	}
}

func TestTransientStatusTTLIsReadableButNotSticky(t *testing.T) {
	// Long enough to read the status, short enough that the key hint is not
	// hidden for the rest of a long turn.
	if TransientStatusTTL < time.Second || TransientStatusTTL > 10*time.Second {
		t.Fatalf("TransientStatusTTL = %v, want a few seconds", TransientStatusTTL)
	}
}
