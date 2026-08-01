package types

import "testing"

func TestToolContextFeatureEnabled(t *testing.T) {
	var zero ToolContext
	if zero.FeatureEnabled("x") {
		t.Error("nil Features map should report every feature disabled")
	}

	ctx := ToolContext{Features: map[string]bool{"worktree": true, "plan": false}}
	if !ctx.FeatureEnabled("worktree") {
		t.Error("worktree should be enabled")
	}
	if ctx.FeatureEnabled("plan") {
		t.Error("plan should be disabled")
	}
	if ctx.FeatureEnabled("unknown") {
		t.Error("an unset key should report disabled, not panic")
	}
}

func TestToolContextValue(t *testing.T) {
	var zero ToolContext
	if zero.Value("x") != nil {
		t.Error("nil Values map should return nil")
	}

	ctx := ToolContext{Values: map[string]any{"cwd": "/tmp"}}
	if got := ctx.Value("cwd"); got != "/tmp" {
		t.Errorf("Value(cwd) = %v, want /tmp", got)
	}
	if ctx.Value("missing") != nil {
		t.Error("a missing key should return nil, not panic")
	}
}

func TestToolApprovalDecisionIsAllow(t *testing.T) {
	tests := []struct {
		decision ToolApprovalDecision
		want     bool
	}{
		{ToolApprovalAllow, true},
		{ToolApprovalAllowAlways, true},
		{ToolApprovalDeny, false},
		{ToolApprovalDenyAlways, false},
		{ToolApprovalDecision("bogus"), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.decision), func(t *testing.T) {
			if got := tt.decision.IsAllow(); got != tt.want {
				t.Errorf("%q.IsAllow() = %v, want %v", tt.decision, got, tt.want)
			}
		})
	}
}
