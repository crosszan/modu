// Package trajectory exposes the running session's own event ledger to the
// agent, so it can review how a run actually went — where time went, which
// tools failed, how many steps a turn took — instead of guessing from what is
// still in its context.
package trajectory

import (
	"context"
	"fmt"
	"strings"

	traj "github.com/openmodu/modu/pkg/coding_agent/trajectory"
	"github.com/openmodu/modu/pkg/types"
)

// Provider projects the running session. CodingSession implements it.
type Provider interface {
	Trajectory(opts traj.Options) (traj.Trajectory, error)
}

// Tool reports the session's own trajectory.
type Tool struct {
	provider Provider
}

func New(provider Provider) types.Tool {
	return &Tool{provider: provider}
}

func (t *Tool) Name() string {
	return "get_trajectory"
}

func (t *Tool) Label() string {
	return "Trajectory"
}

func (t *Tool) Description() string {
	return strings.Join([]string{
		"Inspect this session's own execution trajectory, projected from its persisted log.",
		"Without arguments it returns session-wide totals: turns, approximate model steps, per-tool call counts and timing, failures, and token usage.",
		"Pass `turn` to get one turn's records step by step.",
		"Pass `detail: \"full\"` with `turn` to also include each record's tool input and output; that is much more expensive, so ask for it only when the summary is not enough.",
		"Use this to answer questions about how a run went — what took the time, which tool calls failed, how many steps a turn needed.",
	}, " ")
}

func (t *Tool) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"turn": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"description": "Show this turn's records step by step. Omit for session-wide totals.",
			},
			"detail": map[string]any{
				"type":        "string",
				"enum":        []string{"summary", "full"},
				"description": "\"full\" adds tool inputs and outputs to the per-turn records. Defaults to \"summary\".",
			},
		},
		"additionalProperties": false,
	}
}

func (t *Tool) Execute(_ context.Context, _ string, args map[string]any, _ types.ToolUpdateCallback) (types.ToolResult, error) {
	if t.provider == nil {
		return errorResult("trajectory is not available in this session"), nil
	}
	turn := intArg(args, "turn")
	detail := traj.DetailSummary
	if value, _ := args["detail"].(string); value == string(traj.DetailFull) {
		detail = traj.DetailFull
	}

	// Payloads are only ever projected for a single requested turn: a full
	// session at full detail would be an unbounded amount of context.
	options := traj.Options{}
	if turn > 0 {
		options.Detail = detail
		options.MaxRecords = traj.AllRecords
	}
	result, err := t.provider.Trajectory(options)
	if err != nil {
		return errorResult("could not read the session trajectory: " + err.Error()), nil
	}
	if result.Stats.Records == 0 {
		return textResult("This session has not recorded any events yet.", result), nil
	}

	if turn > 0 {
		lines := traj.TurnDetail(result, turn)
		if lines == nil {
			return errorResult(fmt.Sprintf("turn %d does not exist; this session has %d turns", turn, result.Stats.Turns)), nil
		}
		return textResult(strings.Join(lines, "\n"), result), nil
	}
	return textResult(strings.Join(overview(result), "\n"), result), nil
}

func overview(result traj.Trajectory) []string {
	lines := traj.Overview(result)
	if turns := traj.TurnLines(result); len(turns) > 0 {
		lines = append(lines, "", "Turns")
		lines = append(lines, turns...)
	}
	if tools := traj.ToolLines(result); len(tools) > 0 {
		lines = append(lines, "", "Tools")
		lines = append(lines, tools...)
	}
	return lines
}

func textResult(text string, result traj.Trajectory) types.ToolResult {
	return types.ToolResult{
		Content: []types.ContentBlock{&types.TextContent{Type: "text", Text: text}},
		Details: map[string]any{
			"schemaVersion": result.SchemaVersion,
			"session":       result.Session,
			"stats":         result.Stats,
			"turns":         result.Turns,
		},
	}
}

func errorResult(message string) types.ToolResult {
	return types.ToolResult{
		Content: []types.ContentBlock{&types.TextContent{Type: "text", Text: message}},
		IsError: true,
	}
}

// intArg reads an integer argument. JSON numbers decode as float64, and some
// providers send them as strings.
func intArg(args map[string]any, name string) int {
	switch value := args[name].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case int64:
		return int(value)
	case string:
		var parsed int
		if _, err := fmt.Sscanf(value, "%d", &parsed); err == nil {
			return parsed
		}
	}
	return 0
}
