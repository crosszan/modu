package trajectory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/openmodu/modu/pkg/coding_agent/services/session"
	"github.com/openmodu/modu/pkg/types"
)

// Project reads a persisted session file and projects its current branch into
// a trajectory. It never writes to the session.
func Project(sessionPath string, opts Options) (Trajectory, error) {
	opts = opts.normalized()
	head, entries, warnings, err := readSession(sessionPath)
	if err != nil {
		return Trajectory{}, err
	}
	p := newProjector(opts)
	for _, e := range entries {
		p.consume(e)
	}
	return p.finish(head, warnings), nil
}

type projector struct {
	opts Options

	records   []Record
	turns     []Turn
	pending   map[string]int
	toolStats map[string]*ToolStat
	toolOrder []string

	turnIndex              int
	recordsFromThisMessage int
	step                   int
	afterToolResult        bool
	activeTurn             bool

	stats      Stats
	name       string
	models     map[string]bool
	modelOrder []string
	provider   string
	model      string
	title      string
	firstMs    int64
	lastMs     int64
	turnLastMs []int64
	// lastEventMs is when the previous entry landed. A model call has no
	// recorded start — the assistant message is written once, already finished
	// — so the previous event is the only defensible start for it.
	lastEventMs int64
}

func newProjector(opts Options) *projector {
	return &projector{
		opts:      opts,
		pending:   make(map[string]int),
		toolStats: make(map[string]*ToolStat),
		models:    make(map[string]bool),
	}
}

func (p *projector) consume(e entry) {
	p.observeTime(e.TimeMs)
	previousEventMs := p.lastEventMs
	if e.TimeMs > 0 {
		p.lastEventMs = e.TimeMs
	}
	switch session.EntryType(e.Type) {
	case session.EntryTypeMessage:
		if e.Message == nil {
			return
		}
		switch e.Message.Role {
		case types.RoleUser:
			p.userMessage(e)
		case types.RoleAssistant:
			p.assistantMessage(e, previousEventMs)
		case types.RoleToolResult, "tool":
			p.toolResult(e)
		}
	case session.EntryTypeCompaction:
		p.compaction(e)
	case session.EntryTypeModelChange:
		p.noteModel(e.Provider, e.ModelID)
	case session.EntryTypeSessionInfo:
		// The last session_info wins, empty included: that is how the session
		// manager reads names back, and how "/session name" clears one.
		p.name = e.Name
	}
}

func (p *projector) observeTime(milliseconds int64) {
	if milliseconds <= 0 {
		return
	}
	if p.firstMs == 0 || milliseconds < p.firstMs {
		p.firstMs = milliseconds
	}
	if milliseconds > p.lastMs {
		p.lastMs = milliseconds
	}
}

// ── turns and steps ──────────────────────────────────────────────────────────

func (p *projector) startTurn(e entry, prompt string) {
	p.turnIndex++
	p.step = 0
	p.afterToolResult = false
	p.activeTurn = true
	p.turns = append(p.turns, Turn{
		Index:     p.turnIndex,
		ID:        e.ID,
		Prompt:    prompt,
		StartedAt: isoTime(e.TimeMs),
		Status:    StatusComplete,
	})
	p.turnLastMs = append(p.turnLastMs, e.TimeMs)
}

// ensureTurn opens a synthetic turn for sessions whose branch starts with model
// output rather than a prompt, which is what a branched or forked session looks
// like from the entry it was cut at.
func (p *projector) ensureTurn(e entry) {
	if !p.activeTurn {
		p.startTurn(e, "(continued)")
	}
}

func (p *projector) turn() *Turn {
	return &p.turns[len(p.turns)-1]
}

// modelStep returns the approximate model step for output produced now. The
// session log records no authoritative step boundary, so a new step begins when
// model output resumes after one or more tool results.
func (p *projector) modelStep() int {
	if p.step == 0 || p.afterToolResult {
		p.step++
		p.afterToolResult = false
	}
	turn := p.turn()
	turn.Steps = max(turn.Steps, p.step)
	return p.step
}

// ── records ──────────────────────────────────────────────────────────────────

// add appends a record spanning [startMs, endMs]. An instant event passes the
// same value for both; a running one passes 0 for endMs.
func (p *projector) add(record Record, startMs, endMs int64) int {
	p.stats.Records++
	record.Index = p.stats.Records
	record.Turn = p.turnIndex
	record.StartedAt = isoTime(startMs)
	record.startedMs = startMs
	if record.Status == "" {
		record.Status = StatusComplete
	}
	if record.Status != StatusRunning && endMs > 0 {
		record.CompletedAt = isoTime(endMs)
		record.completedMs = endMs
		if elapsed := endMs - startMs; elapsed > 0 {
			record.DurationMs = &elapsed
		}
	}
	p.records = append(p.records, record)
	p.turn().Records++
	p.touchTurnEnd(max(startMs, endMs))
	return len(p.records) - 1
}

func (p *projector) userMessage(e entry) {
	text := e.Message.text()
	prompt := shorten(text, summaryChars)
	p.startTurn(e, prompt)
	if p.title == "" {
		p.title = shorten(text, titleChars)
	}
	p.stats.Messages++
	p.add(Record{
		ID:      e.ID,
		Kind:    KindUser,
		Event:   "user_message",
		Summary: prompt,
	}, e.TimeMs, e.TimeMs)
}

func (p *projector) assistantMessage(e entry, previousEventMs int64) {
	p.ensureTurn(e)
	step := p.modelStep()
	p.stats.Messages++
	p.noteModel(e.Message.Provider, e.Message.Model)

	// The whole model call is charged to the first record it produced; the rest
	// are instants. One call is one span on the timeline, not one per block.
	callStart := e.TimeMs
	timing := ""
	if previousEventMs > 0 && e.TimeMs > previousEventMs {
		callStart = previousEventMs
		timing = TimingDerived
	}
	started := func() (int64, string) {
		if p.recordsFromThisMessage == 0 {
			return callStart, timing
		}
		return e.TimeMs, ""
	}
	p.recordsFromThisMessage = 0

	first := -1
	for _, block := range e.Message.blocks() {
		var index int
		start, provenance := started()
		switch block.Type {
		case "thinking":
			if block.Thinking == "" {
				continue
			}
			p.stats.Reasoning++
			index = p.add(Record{
				ID:      e.ID,
				Step:    step,
				Kind:    KindReasoning,
				Event:   "reasoning",
				Summary: shorten(block.Thinking, summaryChars),
				Output:  p.detail(block.Thinking),
				Timing:  provenance,
			}, start, e.TimeMs)
		case "text":
			if block.Text == "" {
				continue
			}
			index = p.add(Record{
				ID:      e.ID,
				Step:    step,
				Kind:    KindAssistant,
				Event:   "assistant_message",
				Summary: shorten(block.Text, summaryChars),
				Output:  p.detail(block.Text),
				Timing:  provenance,
			}, start, e.TimeMs)
		case "toolCall":
			index = p.toolCall(e, block, step)
		default:
			continue
		}
		p.recordsFromThisMessage++
		if first < 0 {
			first = index
		}
	}

	if usage := e.Message.Usage; !usage.empty() {
		converted := usage.toUsage()
		p.stats.Tokens.add(converted)
		p.turn().Tokens.add(converted)
		if usage.TotalTokens > 0 {
			p.stats.ContextTokens = usage.TotalTokens
		}
		if first >= 0 {
			p.records[first].Usage = &converted
		}
	}
}

func (p *projector) toolCall(e entry, block wireBlock, step int) int {
	kind := KindTool
	if subagentTools[block.Name] {
		kind = KindSubagent
	}
	index := p.add(Record{
		ID:       e.ID,
		Step:     step,
		Kind:     kind,
		Event:    "tool_call",
		ToolName: block.Name,
		CallID:   block.ID,
		Summary:  toolSummary(block),
		Status:   StatusRunning,
		Timing:   TimingMeasured,
		Input:    p.detail(compactJSON(block.Arguments)),
	}, e.TimeMs, 0)

	p.stats.ToolCalls++
	p.turn().ToolCalls++
	stat := p.toolStat(block.Name)
	stat.Calls++
	stat.Unfinished++
	if block.ID != "" {
		p.pending[block.ID] = index
	}
	return index
}

func (p *projector) toolResult(e entry) {
	p.afterToolResult = true
	p.stats.Messages++
	message := e.Message
	index, matched := p.pending[message.ToolCallID]
	if !matched {
		// A result with no visible call: the call is on an earlier branch, or
		// the session was resumed from a compaction that replaced it.
		p.ensureTurn(e)
		p.add(Record{
			ID:       e.ID,
			Kind:     KindTool,
			Event:    "tool_result",
			ToolName: message.ToolName,
			CallID:   message.ToolCallID,
			Summary:  shorten(message.text(), summaryChars),
			Status:   resultStatus(message.IsError),
			Output:   p.detail(message.text()),
		}, e.TimeMs, e.TimeMs)
		if message.IsError {
			p.stats.ToolFailures++
			p.turn().Failures++
		}
		return
	}
	delete(p.pending, message.ToolCallID)

	record := &p.records[index]
	record.Status = resultStatus(message.IsError)
	record.CompletedAt = isoTime(e.TimeMs)
	record.Output = p.detail(message.text())
	var elapsed int64
	if e.TimeMs > 0 && record.startedMs > 0 {
		elapsed = max(e.TimeMs-record.startedMs, 0)
		record.DurationMs = &elapsed
	}

	stat := p.toolStat(record.ToolName)
	stat.Unfinished--
	stat.TotalMs += elapsed
	stat.MaxMs = max(stat.MaxMs, elapsed)
	if message.IsError {
		stat.Failures++
		p.stats.ToolFailures++
		if turn := p.turnAt(record.Turn); turn != nil {
			turn.Failures++
		}
	}
	p.touchTurnEnd(e.TimeMs)
}

// touchTurnEnd advances the current turn's end to the latest event seen in it.
func (p *projector) touchTurnEnd(timeMs int64) {
	last := len(p.turnLastMs) - 1
	p.turnLastMs[last] = max(p.turnLastMs[last], timeMs)
}

func (p *projector) compaction(e entry) {
	p.ensureTurn(e)
	p.stats.Compactions++
	summary := "context compacted"
	if e.OriginalCount > 0 || e.NewCount > 0 {
		summary = fmt.Sprintf("context compacted: %d → %d messages", e.OriginalCount, e.NewCount)
	}
	p.add(Record{
		ID:      e.ID,
		Kind:    KindCompaction,
		Event:   "compaction",
		Summary: summary,
	}, e.TimeMs, e.TimeMs)
}

// ── helpers ──────────────────────────────────────────────────────────────────

func (p *projector) detail(text string) string {
	if p.opts.Detail != DetailFull || text == "" {
		return ""
	}
	return bound(text, maxDetailChars)
}

func (p *projector) toolStat(name string) *ToolStat {
	if stat, ok := p.toolStats[name]; ok {
		return stat
	}
	stat := &ToolStat{Name: name}
	p.toolStats[name] = stat
	p.toolOrder = append(p.toolOrder, name)
	return stat
}

func (p *projector) turnAt(index int) *Turn {
	if index < 1 || index > len(p.turns) {
		return nil
	}
	return &p.turns[index-1]
}

func (p *projector) noteModel(provider, model string) {
	if model == "" {
		return
	}
	label := model
	if provider != "" {
		label = provider + "/" + model
		p.provider = provider
	}
	p.model = model
	if !p.models[label] {
		p.models[label] = true
		p.modelOrder = append(p.modelOrder, label)
	}
}

func (p *projector) finish(head header, warnings []Warning) Trajectory {
	for index := range p.turns {
		turn := &p.turns[index]
		p.stats.Steps += turn.Steps
		last := p.turnLastMs[index]
		turn.CompletedAt = isoTime(last)
		if start := parseISO(turn.StartedAt); start > 0 && last > start {
			turn.DurationMs = last - start
		}
		if first := p.firstResponse(turn.Index); first != nil {
			turn.FirstResponseMs = first
		}
		p.stats.ActiveMs += turn.DurationMs
	}
	// Tool calls still without a result leave their turn (and the session) in
	// flight; that is the honest reading of a log written mid-run.
	for _, index := range p.pending {
		if turn := p.turnAt(p.records[index].Turn); turn != nil {
			turn.Status = StatusRunning
		}
	}

	p.stats.Turns = len(p.turns)
	if p.lastMs > p.firstMs {
		p.stats.DurationMs = p.lastMs - p.firstMs
	}
	p.stats.Tools = p.sortedToolStats()
	p.stats.Models = p.modelOrder

	records := p.records
	if p.opts.MaxRecords > 0 && len(records) > p.opts.MaxRecords {
		records = records[len(records)-p.opts.MaxRecords:]
	}

	title := p.title
	if title == "" {
		title = "(no prompt)"
	}
	return Trajectory{
		SchemaVersion: SchemaVersion,
		DetailLevel:   p.opts.Detail,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Session: Session{
			ID:        head.ID,
			Name:      p.name,
			Title:     title,
			Cwd:       head.Cwd,
			Model:     p.model,
			Provider:  p.provider,
			StartedAt: firstNonEmpty(isoTime(p.firstMs), head.Timestamp),
			UpdatedAt: isoTime(p.lastMs),
		},
		Stats:    p.stats,
		Turns:    p.turns,
		Records:  records,
		Warnings: warnings,
	}
}

func (p *projector) firstResponse(turnIndex int) *int64 {
	var start int64
	for _, record := range p.records {
		if record.Turn != turnIndex {
			continue
		}
		if record.Kind == KindUser {
			start = record.startedMs
			continue
		}
		if start == 0 {
			continue
		}
		// Measure to when the output completed: a model record's start is
		// derived from the turn's own start, so measuring to it would be zero.
		mark := record.completedMs
		if mark == 0 {
			mark = record.startedMs
		}
		if mark == 0 {
			continue
		}
		switch record.Kind {
		case KindAssistant, KindReasoning, KindTool, KindSubagent:
			elapsed := max(mark-start, 0)
			return &elapsed
		}
	}
	return nil
}

func (p *projector) sortedToolStats() []ToolStat {
	if len(p.toolOrder) == 0 {
		return nil
	}
	out := make([]ToolStat, 0, len(p.toolOrder))
	for _, name := range p.toolOrder {
		out = append(out, *p.toolStats[name])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].TotalMs != out[j].TotalMs {
			return out[i].TotalMs > out[j].TotalMs
		}
		return out[i].Calls > out[j].Calls
	})
	return out
}

func resultStatus(isError bool) string {
	if isError {
		return StatusError
	}
	return StatusComplete
}

func toolSummary(block wireBlock) string {
	arguments := compactJSON(block.Arguments)
	if arguments == "" || arguments == "{}" {
		return block.Name
	}
	return shorten(block.Name+" "+arguments, summaryChars)
}

func compactJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}

func parseISO(value string) int64 {
	if value == "" {
		return 0
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return 0
	}
	return parsed.UnixMilli()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
