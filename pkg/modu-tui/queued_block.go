package modutui

import (
	"fmt"
	"strings"
)

// queuedBlockMaxRows caps how many queued messages are listed before the rest
// collapse into a "+N" line.
const queuedBlockMaxRows = 5

// QueuedBlock lists messages typed while the agent was busy. They sit between
// the transcript and the input rather than inside the transcript, because they
// have not been sent yet — putting them in the transcript would show a user
// message above output that was produced before it was typed.
type QueuedBlock struct {
	Messages []string
	MaxRows  int
}

func (b QueuedBlock) RenderWidth(width int) []string {
	if len(b.Messages) == 0 {
		return nil
	}
	maxRows := b.MaxRows
	if maxRows <= 0 {
		maxRows = queuedBlockMaxRows
	}
	header := fmt.Sprintf("⏳ 已排队 %d 条 · 本轮结束后依次发送 · Backspace 取回", len(b.Messages))
	lines := []string{dimStyle.Render(fitLine(header, width))}
	for i, message := range b.Messages {
		if i >= maxRows {
			lines = append(lines, dimStyle.Render(fitLine(fmt.Sprintf("  … 还有 %d 条", len(b.Messages)-i), width)))
			break
		}
		lines = append(lines, dimStyle.Render(fitLine("  › "+queuedPreview(message), width)))
	}
	return lines
}

// queuedPreview flattens a message to one line. A queued message is a
// reminder of what is pending, not a place to read it back in full.
func queuedPreview(message string) string {
	fields := strings.Fields(strings.ReplaceAll(message, "\n", " "))
	return strings.Join(fields, " ")
}
