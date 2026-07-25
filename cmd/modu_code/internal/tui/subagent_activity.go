package tui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	coding_agent "github.com/openmodu/modu/pkg/coding_agent"
	modutui "github.com/openmodu/modu/pkg/modu-tui"
)

// maxSubagentToolLines is how many of a child's most recent tool calls stay
// on screen. Older ones collapse into a count — a child that runs thirty
// tools should not push the parent's conversation off the top of the
// terminal.
const maxSubagentToolLines = 5

// SubagentActivity turns a child agent's lifecycle and progress events into
// one transcript block per run, replaced in place as the run proceeds:
//
//	explorer · map auth flow
//	  ⎿ read(pkg/auth/handler.go)
//	    grep(login)
//	  Done (3 turns · 12.4K tokens · 8s)
//
// Without it a delegated run is two flat lines with a silent gap between
// them, and the user has no idea whether the child is working or wedged.
//
// Safe for concurrent use: background children report from their own
// goroutines.
type SubagentActivity struct {
	mu   sync.Mutex
	runs map[string]*subagentRun
	// now is swappable so tests can assert the running-duration line.
	now func() time.Time
}

type subagentRun struct {
	agent     string
	label     string
	tools     []string
	dropped   int
	turns     int
	tokens    int
	startedAt time.Time
}

func NewSubagentActivity() *SubagentActivity {
	return &SubagentActivity{runs: map[string]*subagentRun{}, now: time.Now}
}

// HandleSessionEvent folds one event into its run's block and returns the
// entry to upsert. The second result is false when the event is not part of a
// subagent run, or when it belongs to a run with no id to key on — the caller
// then falls back to its normal rendering.
func (a *SubagentActivity) HandleSessionEvent(ev coding_agent.SessionEvent) (modutui.Entry, bool) {
	switch ev.Type {
	case coding_agent.SessionEventSubagentStart,
		coding_agent.SessionEventSubagentProgress,
		coding_agent.SessionEventSubagentStop:
	default:
		return modutui.Entry{}, false
	}
	runID := strings.TrimSpace(ev.SubagentTaskID)
	if runID == "" {
		return modutui.Entry{}, false
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	run := a.runs[runID]
	if run == nil {
		// A progress or stop event without a start means the run began before
		// this UI attached (a resumed session, say). Start the block from
		// whatever the event carries rather than dropping the run.
		run = &subagentRun{agent: ev.SubagentName, startedAt: a.now()}
		a.runs[runID] = run
	}
	if ev.SubagentName != "" {
		run.agent = ev.SubagentName
	}
	if label := subagentLabel(ev); label != "" && ev.Type == coding_agent.SessionEventSubagentStart {
		run.label = label
	}

	switch ev.Type {
	case coding_agent.SessionEventSubagentProgress:
		switch ev.Reason {
		case "tool":
			run.addTool(ev.ToolName, ev.Message, ev.ErrorMessage != "")
		case "turn":
			run.turns++
			run.tokens += ev.SubagentTokens
		}
		return subagentEntry(runID, run.render(a.now(), "", "")), true
	case coding_agent.SessionEventSubagentStop:
		// The host's closing tally is authoritative — it counts the whole run,
		// including turns that happened before this UI was watching.
		if ev.SubagentTurns > 0 {
			run.turns = ev.SubagentTurns
		}
		if ev.SubagentTokens > 0 {
			run.tokens = ev.SubagentTokens
		}
		text := run.render(a.now(), closingStats(ev), ev.ErrorMessage)
		delete(a.runs, runID)
		return subagentEntry(runID, text), true
	default:
		return subagentEntry(runID, run.render(a.now(), "", "")), true
	}
}

func (r *subagentRun) addTool(name, detail string, failed bool) {
	line := strings.TrimSpace(name)
	if line == "" {
		line = "(tool)"
	}
	if detail != "" {
		line += "(" + detail + ")"
	}
	if failed {
		line += " ✗"
	}
	r.tools = append(r.tools, line)
	if len(r.tools) > maxSubagentToolLines {
		r.dropped += len(r.tools) - maxSubagentToolLines
		r.tools = r.tools[len(r.tools)-maxSubagentToolLines:]
	}
}

// render lays out the block. closing is the finished run's tally; when empty
// the run is still going and the footer shows live figures instead.
func (r *subagentRun) render(now time.Time, closing, errMessage string) string {
	var b strings.Builder
	b.WriteString(r.agent)
	if r.label != "" {
		b.WriteString(" · " + r.label)
	}
	for i, tool := range r.toolLines() {
		if i == 0 {
			b.WriteString("\n⎿ " + tool)
			continue
		}
		b.WriteString("\n  " + tool)
	}
	switch {
	case errMessage != "":
		b.WriteString("\nerror: " + firstLinePreview(errMessage, 140))
	case closing != "":
		b.WriteString("\nDone " + closing)
	default:
		if live := runningStats(r.turns, r.tokens, now.Sub(r.startedAt)); live != "" {
			b.WriteString("\n" + live)
		}
	}
	return b.String()
}

func (r *subagentRun) toolLines() []string {
	if r.dropped == 0 {
		return r.tools
	}
	return append([]string{fmt.Sprintf("… +%d earlier", r.dropped)}, r.tools...)
}

// runningStats renders the in-flight figures. Turns are omitted until the
// child finishes its first one, so a run that has only made tool calls does
// not claim "0 turns".
func runningStats(turns, tokens int, elapsed time.Duration) string {
	var parts []string
	if turns > 0 {
		parts = append(parts, pluralTurns(turns))
	}
	if tokens > 0 {
		parts = append(parts, compactTokens(tokens)+" tokens")
	}
	if ms := elapsed.Milliseconds(); ms > 0 {
		parts = append(parts, compactDuration(ms))
	}
	if len(parts) == 0 {
		return ""
	}
	return "running (" + strings.Join(parts, " · ") + ")"
}

func closingStats(ev coding_agent.SessionEvent) string {
	if stats := subagentRunStatsText(ev); stats != "" {
		return stats
	}
	return "(no activity recorded)"
}

// subagentEntry keys the block on the run id so every update replaces the
// same transcript row. It stays a normal assistant entry — the "● " marker
// aligns the header with the two-space indent the renderer gives the
// continuation lines, producing the nested shape.
func subagentEntry(runID, text string) modutui.Entry {
	return modutui.Entry{
		ID:    "subagent:" + runID,
		Role:  modutui.RoleAssistant,
		Nodes: []modutui.Node{modutui.TextNode{Text: text}},
	}
}
