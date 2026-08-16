package trajectory

import (
	"context"
	"errors"
	"strings"
	"testing"

	traj "github.com/openmodu/modu/pkg/coding_agent/trajectory"
	"github.com/openmodu/modu/pkg/types"
)

type fakeProvider struct {
	result traj.Trajectory
	err    error
	opts   traj.Options
}

func (p *fakeProvider) Trajectory(opts traj.Options) (traj.Trajectory, error) {
	p.opts = opts
	return p.result, p.err
}

func sample() traj.Trajectory {
	duration := int64(3000)
	ttft := int64(2000)
	return traj.Trajectory{
		SchemaVersion: traj.SchemaVersion,
		Session:       traj.Session{ID: "sess-1", Model: "deepseek-v4", Provider: "deepseek"},
		Stats: traj.Stats{
			Turns: 1, Steps: 2, Records: 3, ToolCalls: 1, ToolFailures: 1,
			ActiveMs: 6000, DurationMs: 6000,
			Tokens: traj.Usage{Input: 1000, Output: 50},
			Tools:  []traj.ToolStat{{Name: "bash", Calls: 1, Failures: 1, TotalMs: 3000, MaxMs: 3000}},
		},
		Turns: []traj.Turn{{
			Index: 1, Prompt: "fix the build", DurationMs: 6000, FirstResponseMs: &ttft,
			Steps: 2, Records: 3, ToolCalls: 1, Failures: 1, Status: traj.StatusComplete,
		}},
		Records: []traj.Record{
			{Index: 1, Turn: 1, Kind: traj.KindUser, Event: "user_message", Summary: "fix the build", Status: traj.StatusComplete},
			{Index: 2, Turn: 1, Step: 1, Kind: traj.KindTool, Event: "tool_call", ToolName: "bash",
				Summary: `bash {"command":"go build ./..."}`, Status: traj.StatusError, DurationMs: &duration,
				Input: `{"command":"go build ./..."}`, Output: "build failed"},
			{Index: 3, Turn: 1, Step: 2, Kind: traj.KindAssistant, Event: "assistant_message", Summary: "Fixing.", Status: traj.StatusComplete},
		},
	}
}

func mustText(t *testing.T, result types.ToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	text, ok := result.Content[0].(*types.TextContent)
	if !ok {
		t.Fatalf("content is %T, want *types.TextContent", result.Content[0])
	}
	return text.Text
}

func TestToolBasics(t *testing.T) {
	tool := New(&fakeProvider{})
	if tool.Name() != "get_trajectory" {
		t.Errorf("Name() = %q, want get_trajectory", tool.Name())
	}
	params, ok := tool.Parameters().(map[string]any)
	if !ok {
		t.Fatalf("Parameters() = %T, want map", tool.Parameters())
	}
	properties, ok := params["properties"].(map[string]any)
	if !ok || properties["turn"] == nil || properties["detail"] == nil {
		t.Errorf("Parameters() missing turn/detail: %v", params)
	}
}

func TestExecuteReturnsSessionOverview(t *testing.T) {
	provider := &fakeProvider{result: sample()}
	result, err := New(provider).Execute(context.Background(), "id", nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	text := mustText(t, result)
	for _, want := range []string{"turns: 1", "steps: 2", "1 calls, 1 failed", "Turns", "Tools", "bash"} {
		if !strings.Contains(text, want) {
			t.Errorf("overview missing %q:\n%s", want, text)
		}
	}
	// Without a turn the projection stays at summary depth so payloads never
	// enter model context implicitly.
	if provider.opts.Detail == traj.DetailFull {
		t.Errorf("session overview requested full detail: %+v", provider.opts)
	}
	if result.IsError {
		t.Error("overview should not be an error result")
	}
}

func TestExecuteReturnsTurnDetail(t *testing.T) {
	provider := &fakeProvider{result: sample()}
	result, err := New(provider).Execute(context.Background(), "id", map[string]any{"turn": float64(1)}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	text := mustText(t, result)
	for _, want := range []string{"turn 1", "prompt: fix the build", "step 1", "bash", "error", "1st response"} {
		if !strings.Contains(text, want) {
			t.Errorf("turn detail missing %q:\n%s", want, text)
		}
	}
}

func TestExecuteFullDetailRequestsPayloads(t *testing.T) {
	provider := &fakeProvider{result: sample()}
	result, err := New(provider).Execute(context.Background(), "id",
		map[string]any{"turn": float64(1), "detail": "full"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if provider.opts.Detail != traj.DetailFull {
		t.Errorf("Detail = %q, want %q", provider.opts.Detail, traj.DetailFull)
	}
	if provider.opts.MaxRecords != traj.AllRecords {
		t.Errorf("MaxRecords = %d, want %d", provider.opts.MaxRecords, traj.AllRecords)
	}
	text := mustText(t, result)
	if !strings.Contains(text, "output: build failed") {
		t.Errorf("full detail missing payloads:\n%s", text)
	}
}

func TestExecuteRejectsUnknownTurn(t *testing.T) {
	result, err := New(&fakeProvider{result: sample()}).Execute(context.Background(), "id",
		map[string]any{"turn": float64(9)}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Error("unknown turn should return an error result")
	}
	if !strings.Contains(mustText(t, result), "turn 9 does not exist") {
		t.Errorf("unexpected message: %q", mustText(t, result))
	}
}

func TestExecuteReportsEmptySession(t *testing.T) {
	result, err := New(&fakeProvider{}).Execute(context.Background(), "id", nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Error("an empty session is not an error")
	}
	if !strings.Contains(mustText(t, result), "not recorded any events") {
		t.Errorf("unexpected message: %q", mustText(t, result))
	}
}

func TestExecuteSurfacesProviderError(t *testing.T) {
	result, err := New(&fakeProvider{err: errors.New("boom")}).Execute(context.Background(), "id", nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError || !strings.Contains(mustText(t, result), "boom") {
		t.Errorf("provider error not surfaced: %+v", result)
	}
}

func TestExecuteWithoutProvider(t *testing.T) {
	result, err := New(nil).Execute(context.Background(), "id", nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Error("a session without a trajectory source should return an error result")
	}
}

func TestIntArgAcceptsProviderNumberShapes(t *testing.T) {
	for name, args := range map[string]map[string]any{
		"float":  {"turn": float64(2)},
		"int":    {"turn": 2},
		"string": {"turn": "2"},
	} {
		if got := intArg(args, "turn"); got != 2 {
			t.Errorf("%s: intArg = %d, want 2", name, got)
		}
	}
	if got := intArg(map[string]any{}, "turn"); got != 0 {
		t.Errorf("missing arg: intArg = %d, want 0", got)
	}
}
