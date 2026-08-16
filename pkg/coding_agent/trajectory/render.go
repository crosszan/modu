package trajectory

import (
	"fmt"
	"strconv"
	"strings"
)

// maxRenderedTurns bounds the turn list in terminal output. When it truncates
// it says so rather than quietly showing a prefix.
const maxRenderedTurns = 20

// renderPayloadChars bounds a tool input or output inside rendered text. It is
// far tighter than maxDetailChars because rendered text is read by a person or
// spent as model context, where a full 12k-character payload per record is not
// worth what it costs.
const renderPayloadChars = 800

// Overview renders the headline numbers for a trajectory.
func Overview(t Trajectory) []string {
	lines := []string{
		"session: " + shortID(t.Session.ID) + describeModel(t.Session),
		fmt.Sprintf("turns: %d · steps: %d · records: %d",
			t.Stats.Turns, t.Stats.Steps, t.Stats.Records),
		"tools: " + describeTools(t.Stats),
		fmt.Sprintf("active: %s (span %s)",
			formatDuration(t.Stats.ActiveMs), formatDuration(t.Stats.DurationMs)),
		fmt.Sprintf("tokens: %s in / %s out · context %s",
			formatCount(t.Stats.Tokens.Input),
			formatCount(t.Stats.Tokens.Output),
			formatCount(t.Stats.ContextTokens)),
	}
	if t.Stats.Reasoning > 0 {
		lines = append(lines, fmt.Sprintf("reasoning blocks: %d", t.Stats.Reasoning))
	}
	if t.Stats.Compactions > 0 {
		lines = append(lines, fmt.Sprintf("compactions: %d", t.Stats.Compactions))
	}
	if cost := t.Stats.Tokens.Cost; cost > 0 {
		lines = append(lines, fmt.Sprintf("cost: %.4f", cost))
	}
	return lines
}

// TurnLines renders one line per turn, most recent last.
func TurnLines(t Trajectory) []string {
	turns := t.Turns
	var lines []string
	if len(turns) > maxRenderedTurns {
		lines = append(lines, fmt.Sprintf("(showing the last %d of %d turns)", maxRenderedTurns, len(turns)))
		turns = turns[len(turns)-maxRenderedTurns:]
	}
	for _, turn := range turns {
		parts := []string{
			fmt.Sprintf("%3d.", turn.Index),
			pad(formatDuration(turn.DurationMs), 8),
			pad(fmt.Sprintf("%d steps", turn.Steps), 9),
			pad(fmt.Sprintf("%d tools", turn.ToolCalls), 9),
		}
		if turn.Failures > 0 {
			parts = append(parts, pad(fmt.Sprintf("%d failed", turn.Failures), 9))
		} else {
			parts = append(parts, pad("", 9))
		}
		if turn.FirstResponseMs != nil {
			parts = append(parts, pad("1st "+formatDuration(*turn.FirstResponseMs), 12))
		} else {
			parts = append(parts, pad("", 12))
		}
		if turn.Status == StatusRunning {
			parts = append(parts, "[running]")
		}
		parts = append(parts, shorten(turn.Prompt, 60))
		lines = append(lines, strings.Join(parts, " "))
	}
	return lines
}

// TurnDetail renders one turn's records step by step. Records carry inputs and
// outputs only when the trajectory was projected at DetailFull, so the caller
// chooses the depth through Options rather than through another argument.
// It returns nil when the turn does not exist.
func TurnDetail(t Trajectory, index int) []string {
	var turn *Turn
	for i := range t.Turns {
		if t.Turns[i].Index == index {
			turn = &t.Turns[i]
		}
	}
	if turn == nil {
		return nil
	}

	head := fmt.Sprintf("turn %d · %s · %d steps · %d tools",
		turn.Index, formatDuration(turn.DurationMs), turn.Steps, turn.ToolCalls)
	if turn.Failures > 0 {
		head += fmt.Sprintf(" · %d failed", turn.Failures)
	}
	if turn.FirstResponseMs != nil {
		head += " · 1st response " + formatDuration(*turn.FirstResponseMs)
	}
	if turn.Status == StatusRunning {
		head += " · running"
	}
	lines := []string{head, "prompt: " + turn.Prompt}

	records := t.TurnRecords(index)
	if len(records) == 0 {
		return append(lines, "(no records retained for this turn)")
	}
	for _, record := range records {
		label := record.Kind
		if record.ToolName != "" {
			label = record.ToolName
		}
		lines = append(lines, fmt.Sprintf("  %s %s %s %s %s",
			pad(stepLabel(record.Step), 7),
			pad(label, 12),
			pad(formatDuration(durationOf(record)), 8),
			pad(record.Status, 8),
			record.Summary))
		if record.Input != "" {
			lines = append(lines, indent("input: "+bound(record.Input, renderPayloadChars))...)
		}
		if record.Output != "" {
			lines = append(lines, indent("output: "+bound(record.Output, renderPayloadChars))...)
		}
	}
	return lines
}

func stepLabel(step int) string {
	if step <= 0 {
		return ""
	}
	return "step " + strconv.Itoa(step)
}

func durationOf(record Record) int64 {
	if record.DurationMs == nil {
		return 0
	}
	return *record.DurationMs
}

func indent(text string) []string {
	raw := strings.Split(text, "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		out = append(out, "      "+line)
	}
	return out
}

// ToolLines renders per-tool aggregates, slowest first.
func ToolLines(t Trajectory) []string {
	lines := make([]string, 0, len(t.Stats.Tools))
	width := 0
	for _, tool := range t.Stats.Tools {
		width = max(width, len(tool.Name))
	}
	for _, tool := range t.Stats.Tools {
		line := fmt.Sprintf("%s %s %s %s",
			pad(tool.Name, width),
			pad(fmt.Sprintf("%d calls", tool.Calls), 11),
			pad(formatDuration(tool.TotalMs)+" total", 14),
			pad(formatDuration(tool.MaxMs)+" max", 12))
		var notes []string
		if tool.Failures > 0 {
			notes = append(notes, fmt.Sprintf("%d failed", tool.Failures))
		}
		if tool.Unfinished > 0 {
			notes = append(notes, fmt.Sprintf("%d unfinished", tool.Unfinished))
		}
		if len(notes) > 0 {
			line += " " + strings.Join(notes, ", ")
		}
		lines = append(lines, strings.TrimRight(line, " "))
	}
	return lines
}

func describeModel(s Session) string {
	if s.Model == "" {
		return ""
	}
	if s.Provider != "" {
		return " · " + s.Provider + "/" + s.Model
	}
	return " · " + s.Model
}

func describeTools(stats Stats) string {
	text := fmt.Sprintf("%d calls", stats.ToolCalls)
	if stats.ToolFailures > 0 {
		text += fmt.Sprintf(", %d failed", stats.ToolFailures)
	}
	unfinished := 0
	for _, tool := range stats.Tools {
		unfinished += tool.Unfinished
	}
	if unfinished > 0 {
		text += fmt.Sprintf(", %d unfinished", unfinished)
	}
	return text
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func pad(text string, width int) string {
	if len([]rune(text)) >= width {
		return text
	}
	return text + strings.Repeat(" ", width-len([]rune(text)))
}

// formatDuration renders milliseconds at a precision that stays readable from
// sub-second tool calls up to multi-day session spans.
func formatDuration(milliseconds int64) string {
	switch {
	case milliseconds <= 0:
		return "—"
	case milliseconds < 1000:
		return strconv.FormatInt(milliseconds, 10) + "ms"
	case milliseconds < 60_000:
		return fmt.Sprintf("%.1fs", float64(milliseconds)/1000)
	case milliseconds < 3_600_000:
		return fmt.Sprintf("%dm%02ds", milliseconds/60_000, milliseconds%60_000/1000)
	case milliseconds < 86_400_000:
		return fmt.Sprintf("%dh%02dm", milliseconds/3_600_000, milliseconds%3_600_000/60_000)
	default:
		return fmt.Sprintf("%dd%02dh", milliseconds/86_400_000, milliseconds%86_400_000/3_600_000)
	}
}

// formatCount groups thousands so six- and seven-figure token counts stay
// readable in a terminal.
func formatCount(value int) string {
	text := strconv.Itoa(value)
	negative := strings.HasPrefix(text, "-")
	text = strings.TrimPrefix(text, "-")
	if len(text) <= 3 {
		if negative {
			return "-" + text
		}
		return text
	}
	var groups []string
	for len(text) > 3 {
		groups = append([]string{text[len(text)-3:]}, groups...)
		text = text[:len(text)-3]
	}
	groups = append([]string{text}, groups...)
	joined := strings.Join(groups, ",")
	if negative {
		return "-" + joined
	}
	return joined
}
