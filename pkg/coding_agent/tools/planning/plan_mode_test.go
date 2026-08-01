package planning

import (
	"context"
	"strings"
	"testing"

	"github.com/openmodu/modu/pkg/types"
)

type fakePlanManager struct {
	entered      bool
	planMode     bool
	submitPlan   string
	submitSteps  []string
	submitResult string
}

func (m *fakePlanManager) EnterPlanMode() {
	m.entered = true
	m.planMode = true
}
func (m *fakePlanManager) IsPlanMode() bool { return m.planMode }
func (m *fakePlanManager) SubmitPlan(ctx context.Context, plan string, steps []string) string {
	m.submitPlan = plan
	m.submitSteps = steps
	return m.submitResult
}

func mustText(t *testing.T, res types.ToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("result has no content blocks")
	}
	text, ok := res.Content[0].(*types.TextContent)
	if !ok {
		t.Fatalf("content block is %T, want *types.TextContent", res.Content[0])
	}
	return text.Text
}

func TestEnterPlanModeToolBasics(t *testing.T) {
	tool := NewEnterPlanModeTool(&fakePlanManager{})
	if tool.Name() != "enter_plan_mode" {
		t.Errorf("Name() = %q, want enter_plan_mode", tool.Name())
	}
}

func TestEnterPlanModeToolCallsManager(t *testing.T) {
	mgr := &fakePlanManager{}
	tool := NewEnterPlanModeTool(mgr)
	res, err := tool.Execute(context.Background(), "id", nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !mgr.entered {
		t.Error("expected EnterPlanMode to be called on the manager")
	}
	if !strings.Contains(mustText(t, res), "enabled") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}

func TestEnterPlanModeToolNilManager(t *testing.T) {
	tool := NewEnterPlanModeTool(nil)
	res, err := tool.Execute(context.Background(), "id", nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "enabled") {
		t.Errorf("nil manager should not panic and still return a result: %q", mustText(t, res))
	}
}

func TestExitPlanModeToolBasics(t *testing.T) {
	tool := NewExitPlanModeTool(&fakePlanManager{})
	if tool.Name() != "exit_plan_mode" {
		t.Errorf("Name() = %q, want exit_plan_mode", tool.Name())
	}
}

func TestExitPlanModeToolPassesPlanAndStepsToManager(t *testing.T) {
	mgr := &fakePlanManager{submitResult: "Plan approved."}
	tool := NewExitPlanModeTool(mgr)
	res, err := tool.Execute(context.Background(), "id", map[string]any{
		"plan":  "do the thing",
		"steps": []any{"step1", "step2", 5, ""},
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mgr.submitPlan != "do the thing" {
		t.Errorf("submitPlan = %q, want %q", mgr.submitPlan, "do the thing")
	}
	if len(mgr.submitSteps) != 2 || mgr.submitSteps[0] != "step1" || mgr.submitSteps[1] != "step2" {
		t.Errorf("submitSteps = %#v, want [step1 step2] (non-string and empty entries dropped)", mgr.submitSteps)
	}
	if mustText(t, res) != "Plan approved." {
		t.Errorf("result = %q, want the manager's message relayed verbatim", mustText(t, res))
	}
}

func TestExitPlanModeToolNilManager(t *testing.T) {
	tool := NewExitPlanModeTool(nil)
	res, err := tool.Execute(context.Background(), "id", map[string]any{"plan": "x"}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(mustText(t, res), "recorded") {
		t.Errorf("unexpected result: %q", mustText(t, res))
	}
}
