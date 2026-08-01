package plan

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openmodu/modu/pkg/coding_agent/services/todo"
)

// fakeHost is a minimal Host implementation for testing the Controller
// without a real kernel.
type fakeHost struct {
	mu sync.Mutex

	enabled   bool
	snapshots []Snapshot
	todos     []todo.Item

	allowedTools       []string
	refreshPromptCalls int
	writeStateCalls    int
}

func (h *fakeHost) AppendPlanSnapshot(s Snapshot) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.snapshots = append(h.snapshots, s)
	return nil
}
func (h *fakeHost) PlanSnapshots() []Snapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]Snapshot(nil), h.snapshots...)
}
func (h *fakeHost) GetTodos() []todo.Item {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.todos
}
func (h *fakeHost) SetTodos(items []todo.Item) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.todos = items
}
func (h *fakeHost) AllowToolAlways(tool string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.allowedTools = append(h.allowedTools, tool)
}
func (h *fakeHost) RefreshSystemPrompt() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.refreshPromptCalls++
}
func (h *fakeHost) WriteRuntimeState() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.writeStateCalls++
}
func (h *fakeHost) PlanModeEnabled() bool { return h.enabled }

func TestIsPlanModeNilSafety(t *testing.T) {
	var c *Controller
	if c.IsPlanMode() {
		t.Error("nil Controller.IsPlanMode() should be false")
	}
	c = New(nil)
	if c.IsPlanMode() {
		t.Error("Controller with nil host should report IsPlanMode() = false")
	}
}

func TestEnterPlanModeNoopWhenDisabled(t *testing.T) {
	host := &fakeHost{enabled: false}
	c := New(host)
	c.EnterPlanMode()
	if c.IsPlanMode() {
		t.Error("EnterPlanMode should be a no-op when the host has plan mode disabled")
	}
	if host.refreshPromptCalls != 0 || host.writeStateCalls != 0 {
		t.Error("disabled EnterPlanMode should not touch the host")
	}
}

func TestEnterPlanModeSetsModeWhenEnabled(t *testing.T) {
	host := &fakeHost{enabled: true}
	c := New(host)
	c.EnterPlanMode()
	if !c.IsPlanMode() {
		t.Error("EnterPlanMode should set IsPlanMode() = true when enabled")
	}
	if host.refreshPromptCalls != 1 {
		t.Errorf("refreshPromptCalls = %d, want 1", host.refreshPromptCalls)
	}
	if host.writeStateCalls != 1 {
		t.Errorf("writeStateCalls = %d, want 1", host.writeStateCalls)
	}
}

func TestExitPlanModePersistsPlanAndSeedsTodos(t *testing.T) {
	host := &fakeHost{enabled: true}
	c := New(host)
	c.EnterPlanMode()
	c.ExitPlanMode("do the thing", []string{"step one", "step two"})

	if c.IsPlanMode() {
		t.Error("ExitPlanMode should clear IsPlanMode()")
	}
	if len(host.snapshots) != 1 || host.snapshots[0].Content != "do the thing" {
		t.Errorf("expected the plan to be persisted, got %+v", host.snapshots)
	}
	if len(host.todos) != 2 || host.todos[0].Content != "step one" || host.todos[0].Status != "pending" {
		t.Errorf("expected todos seeded from steps, got %+v", host.todos)
	}
}

func TestExitPlanModeSkipsEmptyStepsAfterTrim(t *testing.T) {
	host := &fakeHost{enabled: true}
	c := New(host)
	c.ExitPlanMode("plan", []string{"  ", "real step", ""})
	if len(host.todos) != 1 || host.todos[0].Content != "real step" {
		t.Errorf("blank steps should be dropped, got %+v", host.todos)
	}
}

func TestExitPlanModeWithNoStepsLeavesTodosUntouched(t *testing.T) {
	host := &fakeHost{enabled: true, todos: []todo.Item{{Content: "existing", Status: "pending"}}}
	c := New(host)
	c.ExitPlanMode("plan", nil)
	if len(host.todos) != 1 || host.todos[0].Content != "existing" {
		t.Errorf("no steps supplied should leave existing todos alone, got %+v", host.todos)
	}
}

func TestExitPlanModeNoopWhenDisabled(t *testing.T) {
	host := &fakeHost{enabled: false}
	c := New(host)
	c.ExitPlanMode("plan", []string{"step"})
	if len(host.snapshots) != 0 || len(host.todos) != 0 {
		t.Error("ExitPlanMode should not persist anything when the host has plan mode disabled")
	}
}

// Regression test: SubmitPlan's disabled-host fallback used to route through
// ExitPlanMode, which itself gates on the same "enabled" check that was just
// found false — so it silently did nothing, while the returned message
// claimed "Plan recorded. Proceed to implement it." Verify the plan and
// steps are actually persisted now.
func TestSubmitPlanWithDisabledHostActuallyPersists(t *testing.T) {
	host := &fakeHost{enabled: false}
	c := New(host)

	msg := c.SubmitPlan(context.Background(), "do the thing", []string{"step1", "step2"})

	if !strings.Contains(msg, "Plan recorded") {
		t.Errorf("unexpected message: %q", msg)
	}
	if len(host.snapshots) != 1 || host.snapshots[0].Content != "do the thing" {
		t.Errorf("the plan should have been persisted, got snapshots=%+v", host.snapshots)
	}
	if len(host.todos) != 2 {
		t.Errorf("the steps should have become the todo list, got todos=%+v", host.todos)
	}
	if host.writeStateCalls == 0 {
		t.Error("WriteRuntimeState should be called after persisting")
	}
}

func TestSubmitPlanWithNilHostDoesNotPanic(t *testing.T) {
	c := New(nil)
	msg := c.SubmitPlan(context.Background(), "plan", []string{"step"})
	if !strings.Contains(msg, "Plan recorded") {
		t.Errorf("unexpected message: %q", msg)
	}
}

func TestSubmitPlanApprovedExitsPlanModeAndSeedsTodos(t *testing.T) {
	host := &fakeHost{enabled: true}
	c := New(host)
	c.EnterPlanMode()
	c.SetDecisionCallback(func(plan string, steps []string) string { return "approve" })

	msg := c.SubmitPlan(context.Background(), "my plan", []string{"step1"})
	if c.IsPlanMode() {
		t.Error("approved plan should exit plan mode")
	}
	if !strings.Contains(msg, "approved") {
		t.Errorf("unexpected message: %q", msg)
	}
	if len(host.todos) != 1 {
		t.Errorf("expected todos seeded, got %+v", host.todos)
	}
}

func TestSubmitPlanApproveAutoAllowsToolsAlways(t *testing.T) {
	host := &fakeHost{enabled: true}
	c := New(host)
	c.EnterPlanMode()
	c.SetDecisionCallback(func(plan string, steps []string) string { return "approve_auto" })

	msg := c.SubmitPlan(context.Background(), "my plan", nil)
	if !strings.Contains(msg, "auto-accept") {
		t.Errorf("unexpected message: %q", msg)
	}
	for _, want := range []string{"write", "edit", "bash"} {
		found := false
		for _, tool := range host.allowedTools {
			if tool == want {
				found = true
			}
		}
		if !found {
			t.Errorf("expected %q to be allowed always, got %v", want, host.allowedTools)
		}
	}
}

func TestSubmitPlanRejectedStaysInPlanMode(t *testing.T) {
	host := &fakeHost{enabled: true}
	c := New(host)
	c.EnterPlanMode()
	c.SetDecisionCallback(func(plan string, steps []string) string { return "reject:needs more detail" })

	msg := c.SubmitPlan(context.Background(), "my plan", nil)
	if !c.IsPlanMode() {
		t.Error("rejected plan should remain in plan mode")
	}
	if !strings.Contains(msg, "REJECTED") || !strings.Contains(msg, "needs more detail") {
		t.Errorf("unexpected message: %q", msg)
	}
	if len(host.snapshots) != 0 {
		t.Error("a rejected plan should not be persisted")
	}
}

func TestSubmitPlanRejectedWithoutFeedback(t *testing.T) {
	host := &fakeHost{enabled: true}
	c := New(host)
	c.EnterPlanMode()
	c.SetDecisionCallback(func(plan string, steps []string) string { return "reject" })

	msg := c.SubmitPlan(context.Background(), "my plan", nil)
	if !strings.Contains(msg, "REJECTED") || !strings.Contains(msg, "Ask what they want changed") {
		t.Errorf("unexpected message: %q", msg)
	}
}

func TestSubmitPlanNoCallbackDefaultsToApprove(t *testing.T) {
	host := &fakeHost{enabled: true}
	c := New(host)
	c.EnterPlanMode()
	// No SetDecisionCallback call.
	msg := c.SubmitPlan(context.Background(), "my plan", nil)
	if !strings.Contains(msg, "approved") {
		t.Errorf("no callback should default to approve, got: %q", msg)
	}
}

func TestSubmitPlanContextCancelledRejects(t *testing.T) {
	host := &fakeHost{enabled: true}
	c := New(host)
	c.EnterPlanMode()
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	c.SetDecisionCallback(func(plan string, steps []string) string {
		<-block // never returns before the context is cancelled
		return "approve"
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	msg := c.SubmitPlan(ctx, "my plan", nil)
	if !strings.Contains(msg, "REJECTED") {
		t.Errorf("a cancelled context should reject, got: %q", msg)
	}
}

func TestSubmitPlanEmptyDecisionRejects(t *testing.T) {
	host := &fakeHost{enabled: true}
	c := New(host)
	c.EnterPlanMode()
	c.SetDecisionCallback(func(plan string, steps []string) string { return "   " })

	msg := c.SubmitPlan(context.Background(), "my plan", nil)
	if !strings.Contains(msg, "REJECTED") {
		t.Errorf("a blank decision should reject, got: %q", msg)
	}
}

func TestStatusReportsTodoCounters(t *testing.T) {
	host := &fakeHost{enabled: true, todos: []todo.Item{
		{Content: "a", Status: "pending"},
		{Content: "b", Status: "in_progress"},
		{Content: "c", Status: "completed"},
		{Content: "d", Status: "completed"},
	}}
	c := New(host)
	status := c.Status()
	if status.TodoTotal != 4 || status.TodoPending != 1 || status.TodoInProgress != 1 || status.TodoCompleted != 2 {
		t.Errorf("unexpected status: %+v", status)
	}
}

func TestStatusReflectsLatestPlan(t *testing.T) {
	host := &fakeHost{enabled: true}
	c := New(host)
	c.ExitPlanMode("first plan", nil)
	c.EnterPlanMode()
	c.ExitPlanMode("second plan", nil)

	status := c.Status()
	if !status.PlanExists || status.LatestPlan != "second plan" {
		t.Errorf("unexpected status: %+v", status)
	}
	if status.RevisionCount != 2 {
		t.Errorf("RevisionCount = %d, want 2", status.RevisionCount)
	}
}

func TestClearRemovesLatestPlanAndTodos(t *testing.T) {
	host := &fakeHost{enabled: true, todos: []todo.Item{{Content: "x", Status: "pending"}}}
	c := New(host)
	c.ExitPlanMode("a plan", nil)

	if err := c.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	status := c.Status()
	if status.PlanExists {
		t.Errorf("plan should no longer exist after Clear, got %+v", status)
	}
	if host.todos != nil {
		t.Errorf("todos should be cleared, got %+v", host.todos)
	}
}

func TestListRevisionsExcludesClearedAndEmpty(t *testing.T) {
	host := &fakeHost{enabled: true}
	c := New(host)
	c.ExitPlanMode("plan one", nil)
	time.Sleep(2 * time.Millisecond)
	c.ExitPlanMode("plan two", nil)
	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}

	revisions := c.ListRevisions()
	// Clear() marks the store cleared, so ListRevisions (unlike latestPlan)
	// still surfaces prior real revisions — only Cleared/empty entries
	// themselves are excluded.
	for _, r := range revisions {
		if strings.TrimSpace(r.Content) == "" {
			t.Errorf("revision list should never include an empty-content entry: %+v", r)
		}
	}
	if len(revisions) != 2 {
		t.Fatalf("expected 2 real revisions, got %d: %+v", len(revisions), revisions)
	}
	// Newest first.
	if revisions[0].Content != "plan two" {
		t.Errorf("revisions[0].Content = %q, want %q (newest first)", revisions[0].Content, "plan two")
	}
}

func TestLatestPlanReturnsFalseAfterClear(t *testing.T) {
	host := &fakeHost{enabled: true}
	c := New(host)
	c.ExitPlanMode("plan one", nil)
	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	status := c.Status()
	if status.PlanExists {
		t.Error("the most recent action was Clear, so no plan should be reported as existing")
	}
}

func TestLatestPlanResumesAfterClearThenNewPlan(t *testing.T) {
	host := &fakeHost{enabled: true}
	c := New(host)
	c.ExitPlanMode("plan one", nil)
	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	c.ExitPlanMode("plan two", nil)

	status := c.Status()
	if !status.PlanExists || status.LatestPlan != "plan two" {
		t.Errorf("a new plan after Clear should become the latest, got %+v", status)
	}
}
