// Package trajectory projects a persisted modu_code session log into a
// turn-aware event ledger: turns, approximate model steps, and per-record
// timing, status, and token accounting.
//
// The session log is the only input. Nothing here writes to it, and the
// projection is derived entirely from what the session already persists, so
// enabling a trajectory view costs a session nothing at write time.
package trajectory

import "strings"

// SchemaVersion identifies the projected shape. Bump it when the JSON that
// hosts (the HTML viewer, the get_trajectory tool) consume changes shape.
const SchemaVersion = 1

// Detail selects how much record payload a projection carries.
type Detail string

const (
	// DetailSummary keeps event names, timing, status, and token counts but
	// drops tool inputs and outputs. It is the default because the projection
	// is often fed back into a model's context, where payloads are expensive.
	DetailSummary Detail = "summary"
	// DetailFull additionally carries bounded tool inputs and outputs.
	DetailFull Detail = "full"
)

// Record kinds.
const (
	KindUser       = "user"
	KindAssistant  = "assistant"
	KindReasoning  = "reasoning"
	KindTool       = "tool"
	KindSubagent   = "subagent"
	KindCompaction = "compaction"
	KindSystem     = "system"
)

// Prompt snapshot change kinds, mirroring what the session persists.
const (
	PromptChangeInitial        = "initial"
	PromptChangeSystem         = "system"
	PromptChangeTools          = "tools"
	PromptChangeSystemAndTools = "system-and-tools"
)

// Record and turn statuses.
const (
	StatusComplete = "complete"
	StatusError    = "error"
	StatusRunning  = "running"
)

// Timing provenance for a record's start and duration.
const (
	TimingMeasured = "measured"
	TimingDerived  = "derived"
)

const (
	defaultMaxRecords = 500
	minMaxRecords     = 50
	maxMaxRecords     = 1000

	// maxDetailChars bounds a single input or output field under DetailFull.
	maxDetailChars = 12000
	summaryChars   = 160
	titleChars     = 100
)

// subagentTools are the tools that run a nested agent rather than a plain
// action, so their records are projected as their own kind.
var subagentTools = map[string]bool{"subagent": true, "workflow": true}

// AllRecords disables the record cap. Use it for output a person reads
// directly; the cap exists to bound what is fed back into model context.
const AllRecords = -1

// Options controls a projection.
type Options struct {
	// MaxRecords bounds how many of the most recent records are retained.
	// Zero means defaultMaxRecords, AllRecords means no cap, and any other
	// value is clamped to [50, 1000]. Aggregate statistics always describe the
	// whole session regardless of the cap.
	MaxRecords int
	// Detail selects payload depth. Empty means DetailSummary.
	Detail Detail
}

func (o Options) normalized() Options {
	switch {
	case o.MaxRecords == 0:
		o.MaxRecords = defaultMaxRecords
	case o.MaxRecords < 0:
		o.MaxRecords = AllRecords
	case o.MaxRecords < minMaxRecords:
		o.MaxRecords = minMaxRecords
	case o.MaxRecords > maxMaxRecords:
		o.MaxRecords = maxMaxRecords
	}
	if o.Detail != DetailFull {
		o.Detail = DetailSummary
	}
	return o
}

// Trajectory is one projected session.
type Trajectory struct {
	SchemaVersion int       `json:"schemaVersion"`
	DetailLevel   Detail    `json:"detailLevel"`
	GeneratedAt   string    `json:"generatedAt"`
	Session       Session   `json:"session"`
	Stats         Stats     `json:"stats"`
	Turns         []Turn    `json:"turns"`
	Records       []Record  `json:"records"`
	Warnings      []Warning `json:"warnings,omitempty"`
}

// Session describes the projected session itself. The on-disk log path is
// deliberately absent: the projection travels into model context and rendered
// pages, and neither needs an absolute path into the user's home directory.
type Session struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	Title     string `json:"title"`
	Cwd       string `json:"cwd,omitempty"`
	Model     string `json:"model,omitempty"`
	Provider  string `json:"provider,omitempty"`
	StartedAt string `json:"startedAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
	// Prompt is the system prompt and tool catalog the session runs with. The
	// session log does not persist either, so it is only present when a live
	// session supplies it; a trajectory read from a file alone leaves it nil.
	Prompt *Prompt `json:"prompt,omitempty"`
}

// Prompt is the model-visible instruction state of a session.
//
// Change says what moved since the previous snapshot, and the Previous fields
// carry enough of that snapshot to diff against — a prompt edit is only
// meaningful next to what it replaced.
type Prompt struct {
	System         string `json:"system"`
	Bytes          int    `json:"bytes"`
	Tools          []Tool `json:"tools,omitempty"`
	Change         string `json:"change,omitempty"`
	PreviousSystem string `json:"previousSystem,omitempty"`
	PreviousTools  []Tool `json:"previousTools,omitempty"`
}

// Tool is one entry of the model-visible tool catalog.
type Tool struct {
	Name        string `json:"name"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	Schema      string `json:"schema,omitempty"`
}

// Turn is one user prompt and everything the agent did in response.
type Turn struct {
	Index       int    `json:"index"`
	ID          string `json:"id,omitempty"`
	Prompt      string `json:"prompt"`
	StartedAt   string `json:"startedAt,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
	DurationMs  int64  `json:"durationMs"`
	// FirstResponseMs is the gap from the prompt to the turn's first completed
	// model output. It is deliberately not called time-to-first-token: it
	// measures to when a message finished, which is all an older session
	// records. Nil when the turn produced no model output.
	FirstResponseMs *int64 `json:"firstResponseMs,omitempty"`
	// FirstTokenMs is the real time to first token, present only when the
	// session persisted the model call's clock.
	FirstTokenMs *int64 `json:"firstTokenMs,omitempty"`
	Steps        int    `json:"steps"`
	Records      int    `json:"records"`
	ToolCalls    int    `json:"toolCalls"`
	Failures     int    `json:"failures"`
	Tokens       Usage  `json:"tokens"`
	Status       string `json:"status"`
}

// Record is one projected event on the session's current branch.
type Record struct {
	// Index is the position in the complete session, kept stable even when
	// MaxRecords drops earlier records.
	Index int    `json:"index"`
	ID    string `json:"id"`
	Turn  int    `json:"turn"`
	// Step is the approximate model step within the turn, or 0 for records
	// that do not belong to one (the user prompt, compaction).
	Step        int    `json:"step,omitempty"`
	Kind        string `json:"kind"`
	Event       string `json:"event"`
	Summary     string `json:"summary"`
	StartedAt   string `json:"startedAt,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
	DurationMs  *int64 `json:"durationMs,omitempty"`
	Status      string `json:"status"`
	// Timing says where StartedAt and DurationMs came from. TimingMeasured
	// means both endpoints are recorded facts: a tool call paired with its
	// result, or a model call from a session that persisted its clock.
	// TimingDerived means the start was inferred from the previous event,
	// which is all an older session allows — an assistant message is written
	// once, when it is already complete.
	Timing string `json:"timing,omitempty"`
	// FirstTokenMs and DecodeMs split a measured model call into waiting for
	// the first token and decoding the rest. Both are absent for a
	// non-streaming reply, which has no first-token moment to record.
	FirstTokenMs *int64 `json:"firstTokenMs,omitempty"`
	DecodeMs     *int64 `json:"decodeMs,omitempty"`
	// Throughput is decoded output tokens per second.
	Throughput *float64 `json:"throughput,omitempty"`
	ToolName   string   `json:"toolName,omitempty"`
	CallID     string   `json:"callId,omitempty"`
	Input      string   `json:"input,omitempty"`
	Output     string   `json:"output,omitempty"`
	Usage      *Usage   `json:"usage,omitempty"`
	// Prompt is present on a system record: the instruction state introduced
	// at that point, with the previous one for comparison.
	Prompt *Prompt `json:"prompt,omitempty"`
	// Subagent is present on a record that ran a nested agent, summarising the
	// child's own session.
	Subagent *SubagentRun `json:"subagent,omitempty"`
	// Subagents holds the runs when one call forked several children, which
	// parallel and chain subagent calls do.
	Subagents []SubagentRun `json:"subagents,omitempty"`

	startedMs   int64
	completedMs int64
}

// Usage aggregates token counts and cost.
//
// Input is billed input, which restates the whole conversation on every
// request; summing it gives the billed total, not the context size. Use
// Stats.ContextTokens for how full the context window actually is.
type Usage struct {
	Input      int     `json:"input"`
	Output     int     `json:"output"`
	CacheRead  int     `json:"cacheRead,omitempty"`
	CacheWrite int     `json:"cacheWrite,omitempty"`
	Cost       float64 `json:"cost,omitempty"`
}

func (u *Usage) add(other Usage) {
	u.Input += other.Input
	u.Output += other.Output
	u.CacheRead += other.CacheRead
	u.CacheWrite += other.CacheWrite
	u.Cost += other.Cost
}

// Stats aggregates the complete session, including records dropped by
// MaxRecords.
type Stats struct {
	Turns         int `json:"turns"`
	Steps         int `json:"steps"`
	Records       int `json:"records"`
	Messages      int `json:"messages"`
	ToolCalls     int `json:"toolCalls"`
	ToolFailures  int `json:"toolFailures"`
	Reasoning     int `json:"reasoning"`
	Compactions   int `json:"compactions"`
	PromptChanges int `json:"promptChanges"`
	Subagents     int `json:"subagents"`
	// DurationMs spans the first and last projected event. A session resumed
	// across days spans far more wall clock than it spent working, so ActiveMs
	// sums the turns' own durations and is the one to read as "time spent".
	DurationMs int64 `json:"durationMs"`
	ActiveMs   int64 `json:"activeMs"`
	Tokens     Usage `json:"tokens"`
	// ContextTokens is the most recent reported context size.
	ContextTokens int        `json:"contextTokens"`
	Tools         []ToolStat `json:"tools,omitempty"`
	Models        []string   `json:"models,omitempty"`
}

// ToolStat aggregates one tool across the session.
type ToolStat struct {
	Name       string `json:"name"`
	Calls      int    `json:"calls"`
	Failures   int    `json:"failures"`
	TotalMs    int64  `json:"totalMs"`
	MaxMs      int64  `json:"maxMs"`
	Unfinished int    `json:"unfinished,omitempty"`
}

// SubagentRun summarises the session a nested agent ran in.
//
// A subagent gets its own session file, so the parent's log records only that
// the tool was called. Available reports whether that file could be read: a
// subagent run synchronously never writes one, and saying so is better than
// showing a run with all-zero statistics.
type SubagentRun struct {
	// RunID addresses this run for `/trajectory task <id>`. An asynchronous run
	// is addressed by its background task id; a synchronous one has no task, so
	// it is addressed by the transcript filed under its tool call.
	RunID     string     `json:"runId,omitempty"`
	Agent     string     `json:"agent,omitempty"`
	Available bool       `json:"available"`
	Reason    string     `json:"reason,omitempty"`
	Turns     int        `json:"turns,omitempty"`
	Steps     int        `json:"steps,omitempty"`
	Records   int        `json:"records,omitempty"`
	ToolCalls int        `json:"toolCalls,omitempty"`
	Failures  int        `json:"failures,omitempty"`
	ActiveMs  int64      `json:"activeMs,omitempty"`
	Tokens    Usage      `json:"tokens"`
	Tools     []ToolStat `json:"tools,omitempty"`
}

// Warning reports a line the projection could not use.
type Warning struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// SubagentRuns returns every nested run a record started, whether it forked
// one child or several.
func (r Record) SubagentRuns() []SubagentRun {
	if len(r.Subagents) > 0 {
		return r.Subagents
	}
	if r.Subagent != nil {
		return []SubagentRun{*r.Subagent}
	}
	return nil
}

// TurnRecords returns the records belonging to one turn index.
func (t Trajectory) TurnRecords(turn int) []Record {
	var out []Record
	for _, record := range t.Records {
		if record.Turn == turn {
			out = append(out, record)
		}
	}
	return out
}

// shorten truncates to at most limit runes, never splitting a rune. Session
// prompts and tool output are routinely CJK, where a byte-wise cut produces
// mojibake.
func shorten(text string, limit int) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return strings.TrimSpace(string(runes[:limit])) + "…"
}

// bound truncates a detail payload, keeping newlines intact.
func bound(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "\n… truncated"
}
