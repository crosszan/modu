package subagent

import (
	"fmt"
	"strings"

	"github.com/openmodu/modu/pkg/coding_agent/plugins/extension"
	"github.com/openmodu/modu/pkg/types"
)

// notifyResultLimit caps how much of a child's final text rides along in the
// completion notice. Anything longer is truncated with a pointer to the full
// record, which action=status can still print in full.
const notifyResultLimit = 4000

// onTaskDone turns a host "subagent_task_done" event into a follow-up message
// queued on the parent conversation. Without this the orchestrator only ever
// learns a background child finished by polling action=status, so a
// fire-and-forget dispatch is silently forgotten whenever the model doesn't
// think to check back.
//
// Children of a batch stay quiet: dispatchBatchAsync reports the batch's own
// completion once, and one notice per child would bury it.
func (e *Extension) onTaskDone(ev types.Event) {
	done, ok := ev.Args.(extension.SubagentTaskDone)
	if !ok || strings.TrimSpace(done.BatchID) != "" {
		return
	}
	e.pushNotice(formatTaskDoneNotice(done))
}

// pushNotice delivers text to the parent agent as a hidden follow-up turn.
// Falls back to a user-visible notification when the host has no follow-up
// channel wired, so the completion is never lost entirely.
func (e *Extension) pushNotice(text string) {
	if e == nil || e.api == nil || strings.TrimSpace(text) == "" {
		return
	}
	if err := e.api.SendFollowUpMessage(text); err != nil {
		e.api.Notify(e.Name(), text)
	}
}

// formatTaskDoneNotice renders the notice the parent model reads. The shape
// mirrors the tool-result vocabulary the model already sees ("Done (…)") so
// it can act on the result without a follow-up status call.
func formatTaskDoneNotice(done extension.SubagentTaskDone) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<subagent-notification task_id=%q agent=%q status=%q>\n",
		done.TaskID, done.Agent, done.Status)
	if label := strings.TrimSpace(done.Summary); label != "" {
		fmt.Fprintf(&b, "%s\n", firstLine(label, 120))
	}
	if stats := formatRunStats(done.Turns, done.Tokens, done.DurationMs); stats != "" {
		fmt.Fprintf(&b, "Done %s\n", stats)
	}
	if result := strings.TrimSpace(done.Result); result != "" {
		b.WriteString("\n")
		if len(result) > notifyResultLimit {
			b.WriteString(result[:notifyResultLimit])
			fmt.Fprintf(&b, "\n\n[truncated; run `subagent action=status id=%s` for the full result]", done.TaskID)
		} else {
			b.WriteString(result)
		}
		b.WriteString("\n")
	}
	b.WriteString("</subagent-notification>")
	return b.String()
}

// formatRunStats renders "(4 turns · 12.3K tokens · 8s)" from a run's tally,
// omitting whichever figures are zero. Returns "" when nothing was recorded.
func formatRunStats(turns, tokens int, durationMs int64) string {
	var parts []string
	if turns > 0 {
		unit := " turns"
		if turns == 1 {
			unit = " turn"
		}
		parts = append(parts, fmt.Sprintf("%d%s", turns, unit))
	}
	if tokens > 0 {
		parts = append(parts, formatTokensCompact(tokens)+" tokens")
	}
	if durationMs > 0 {
		parts = append(parts, formatDurationCompact(durationMs))
	}
	if len(parts) == 0 {
		return ""
	}
	return "(" + strings.Join(parts, " · ") + ")"
}

// formatTokensCompact renders 12345 as "12.3K" and 1200000 as "1.2M".
func formatTokensCompact(tokens int) string {
	switch {
	case tokens >= 1_000_000:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(tokens)/1_000_000), ".0") + "M"
	case tokens >= 1_000:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(tokens)/1_000), ".0") + "K"
	default:
		return fmt.Sprintf("%d", tokens)
	}
}

// formatDurationCompact renders a millisecond duration as "8s" / "2m10s".
func formatDurationCompact(ms int64) string {
	seconds := ms / 1000
	if ms > 0 && seconds == 0 {
		seconds = 1
	}
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	minutes := seconds / 60
	rest := seconds % 60
	if minutes < 60 {
		if rest == 0 {
			return fmt.Sprintf("%dm", minutes)
		}
		return fmt.Sprintf("%dm%ds", minutes, rest)
	}
	hours := minutes / 60
	minutes %= 60
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh%dm", hours, minutes)
}

// firstLine trims text to its first line and at most max runes, so a label
// built from a multi-line task never breaks the notice header.
func firstLine(text string, max int) string {
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		text = text[:idx]
	}
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return string(runes[:max]) + "..."
}
