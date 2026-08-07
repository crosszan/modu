package memory

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/openmodu/modu/pkg/types"
)

func TestOrganizeWritesSummariesAndPreservesSources(t *testing.T) {
	store := New(t.TempDir(), t.TempDir())
	if err := store.WriteGlobalLongTerm("Prefer concise answers."); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteProjectLongTerm("Use go run for integration checks."); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendToday("Prompt templates now accept quoted arguments."); err != nil {
		t.Fatal(err)
	}

	calls := 0
	var prompt string
	streamFn := organizationTestStream(t, func(llmCtx *types.LLMContext) string {
		calls++
		prompt = llmCtx.Messages[0].(types.UserMessage).Content.(string)
		return `{"global_summary":"- Prefer concise answers.","project_summary":"- Use go run for integration checks.\n- Prompt templates accept quoted arguments."}`
	})
	opts := OrganizeOptions{
		Model:          &types.Model{ID: "test", ProviderID: "test"},
		StreamFn:       streamFn,
		ThresholdBytes: 1,
		RecentDays:     1,
		MinInterval:    time.Hour,
	}
	result, err := store.Organize(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Organized || result.State.Status != "succeeded" {
		t.Fatalf("unexpected result: %+v", result)
	}
	for _, want := range []string{"Prefer concise answers.", "Use go run", "quoted arguments"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("organizer prompt missing %q:\n%s", want, prompt)
		}
	}
	if got := store.ReadGlobalSummary(); got != "- Prefer concise answers." {
		t.Fatalf("global summary = %q", got)
	}
	if got := store.ReadProjectSummary(); !strings.Contains(got, "Prompt templates accept quoted arguments.") {
		t.Fatalf("project summary = %q", got)
	}
	if got := store.ReadGlobalLongTerm(); got != "Prefer concise answers." {
		t.Fatalf("global source was modified: %q", got)
	}
	if got := store.ReadProjectLongTerm(); got != "Use go run for integration checks." {
		t.Fatalf("project source was modified: %q", got)
	}
	if got := store.GetRecentDailyNotes(1); !strings.Contains(got, "Prompt templates now accept quoted arguments.") {
		t.Fatalf("daily source was modified: %q", got)
	}

	result, err = store.Organize(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if result.Organized || result.Reason != "unchanged" || calls != 1 {
		t.Fatalf("unchanged pass should skip model: result=%+v calls=%d", result, calls)
	}
}

func TestOrganizeSkipsBelowThreshold(t *testing.T) {
	store := New(t.TempDir(), t.TempDir())
	if err := store.WriteProjectLongTerm("small"); err != nil {
		t.Fatal(err)
	}
	called := false
	result, err := store.Organize(context.Background(), OrganizeOptions{
		Model:          &types.Model{ID: "test", ProviderID: "test"},
		StreamFn:       organizationTestStream(t, func(*types.LLMContext) string { called = true; return `{}` }),
		ThresholdBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Organized || result.Reason != "below_threshold" || called {
		t.Fatalf("below-threshold pass = %+v, called=%v", result, called)
	}
}

func TestOrganizeFailurePreservesExistingSummaryAndRecordsState(t *testing.T) {
	store := New(t.TempDir(), t.TempDir())
	if err := store.WriteProjectLongTerm("durable source"); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteProjectSummary("existing summary"); err != nil {
		t.Fatal(err)
	}
	_, err := store.Organize(context.Background(), OrganizeOptions{
		Model:    &types.Model{ID: "test", ProviderID: "test"},
		StreamFn: organizationTestStream(t, func(*types.LLMContext) string { return `not json` }),
		Force:    true,
	})
	if err == nil {
		t.Fatal("expected invalid organizer output to fail")
	}
	if got := store.ReadProjectSummary(); got != "existing summary" {
		t.Fatalf("failed run changed summary: %q", got)
	}
	status := store.OrganizationStatus()
	if status.Status != "failed" || status.LastError == "" || status.Running {
		t.Fatalf("unexpected failure status: %+v", status)
	}
}

func TestShouldOrganizeIgnoresStaleLock(t *testing.T) {
	store := New(t.TempDir(), t.TempDir())
	if err := store.WriteProjectLongTerm("durable source"); err != nil {
		t.Fatal(err)
	}
	lockPath := store.organizationLockPath()
	if err := os.WriteFile(lockPath, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-defaultLockStaleAfter - time.Minute)
	if err := os.Chtimes(lockPath, stale, stale); err != nil {
		t.Fatal(err)
	}
	should, reason := store.ShouldOrganize(OrganizeOptions{ThresholdBytes: 1})
	if !should || reason != "" {
		t.Fatalf("stale lock blocked automatic organization: should=%v reason=%q", should, reason)
	}
	if status := store.OrganizationStatus(); status.Running {
		t.Fatalf("stale lock reported as running: %+v", status)
	}
}

func organizationTestStream(t *testing.T, reply func(*types.LLMContext) string) types.StreamFn {
	t.Helper()
	return func(_ context.Context, model *types.Model, llmCtx *types.LLMContext, _ *types.SimpleStreamOptions) (types.EventStream, error) {
		stream := types.NewEventStream()
		go func() {
			defer stream.Close()
			message := &types.AssistantMessage{
				Role:       types.RoleAssistant,
				ProviderID: model.ProviderID,
				Model:      model.ID,
				Content:    []types.ContentBlock{&types.TextContent{Type: "text", Text: reply(llmCtx)}},
				StopReason: "stop",
			}
			stream.Push(types.StreamEvent{Type: types.EventDone, Message: message})
			stream.Resolve(message, nil)
		}()
		return stream, nil
	}
}

// organizationTestStreamWithUsage is organizationTestStream plus reported
// token usage, so the cost bookkeeping can be asserted.
func organizationTestStreamWithUsage(t *testing.T, reply string, usage types.AgentUsage) types.StreamFn {
	t.Helper()
	return func(_ context.Context, model *types.Model, _ *types.LLMContext, _ *types.SimpleStreamOptions) (types.EventStream, error) {
		stream := types.NewEventStream()
		go func() {
			defer stream.Close()
			message := &types.AssistantMessage{
				Role:       types.RoleAssistant,
				ProviderID: model.ProviderID,
				Model:      model.ID,
				Content:    []types.ContentBlock{&types.TextContent{Type: "text", Text: reply}},
				StopReason: "stop",
				Usage:      usage,
			}
			stream.Push(types.StreamEvent{Type: types.EventDone, Message: message})
			stream.Resolve(message, nil)
		}()
		return stream, nil
	}
}

func TestOrganizeRecordsWhatTheRunCost(t *testing.T) {
	store := New(t.TempDir(), t.TempDir())
	if err := store.WriteProjectLongTerm(strings.Repeat("project fact\n", 500)); err != nil {
		t.Fatal(err)
	}

	result, err := store.Organize(context.Background(), OrganizeOptions{
		Model: &types.Model{ID: "test", ProviderID: "test"},
		StreamFn: organizationTestStreamWithUsage(t,
			`{"global_summary":"","project_summary":"short summary"}`,
			types.AgentUsage{Input: 1200, Output: 80, TotalTokens: 1280}),
		ThresholdBytes: 1,
	})
	if err != nil {
		t.Fatalf("Organize: %v", err)
	}
	if !result.Organized {
		t.Fatalf("expected an organize run, got %#v", result)
	}

	state := result.State
	if state.LastInputTokens != 1200 || state.LastOutputTokens != 80 || state.LastTotalTokens != 1280 {
		t.Fatalf("token usage not recorded: %#v", state)
	}
	if state.TotalRuns != 1 || state.TotalTokens != 1280 {
		t.Fatalf("totals = runs %d tokens %d, want 1 / 1280", state.TotalRuns, state.TotalTokens)
	}
	if state.LastDurationMs < 0 {
		t.Fatalf("duration = %d", state.LastDurationMs)
	}

	// Reloading must not lose the numbers — they are the record over time.
	reloaded := store.OrganizationStatus()
	if reloaded.TotalTokens != 1280 || reloaded.TotalRuns != 1 {
		t.Fatalf("state did not persist: %#v", reloaded)
	}
}

func TestOrganizeTotalsAccumulateAcrossRuns(t *testing.T) {
	store := New(t.TempDir(), t.TempDir())
	run := func(text string, tokens int) OrganizationState {
		t.Helper()
		if err := store.WriteProjectLongTerm(text); err != nil {
			t.Fatal(err)
		}
		result, err := store.Organize(context.Background(), OrganizeOptions{
			Model: &types.Model{ID: "test", ProviderID: "test"},
			StreamFn: organizationTestStreamWithUsage(t,
				`{"global_summary":"","project_summary":"s"}`,
				types.AgentUsage{Input: tokens, TotalTokens: tokens}),
			ThresholdBytes: 1,
			Force:          true,
		})
		if err != nil {
			t.Fatal(err)
		}
		return result.State
	}

	run(strings.Repeat("a\n", 200), 500)
	second := run(strings.Repeat("b\n", 200), 700)

	// One cheap run says nothing about a session that reorganizes daily —
	// the cumulative figure is what shows whether upkeep is affordable.
	if second.TotalRuns != 2 || second.TotalTokens != 1200 {
		t.Fatalf("totals = runs %d tokens %d, want 2 / 1200", second.TotalRuns, second.TotalTokens)
	}
}

func TestOrganizeRecordsCostOfFailedRuns(t *testing.T) {
	store := New(t.TempDir(), t.TempDir())
	if err := store.WriteProjectLongTerm(strings.Repeat("fact\n", 300)); err != nil {
		t.Fatal(err)
	}

	_, err := store.Organize(context.Background(), OrganizeOptions{
		Model: &types.Model{ID: "test", ProviderID: "test"},
		StreamFn: organizationTestStreamWithUsage(t, "not json",
			types.AgentUsage{Input: 900, TotalTokens: 900}),
		ThresholdBytes: 1,
	})
	if err == nil {
		t.Fatal("expected the invalid JSON to fail the run")
	}

	// A model that keeps failing still spends the budget every interval; if
	// failures were free in the accounting that would be invisible.
	state := store.OrganizationStatus()
	if state.Status != "failed" {
		t.Fatalf("status = %q", state.Status)
	}
	if state.TotalRuns != 1 || state.TotalTokens != 900 {
		t.Fatalf("a failed run should still be counted: %#v", state)
	}
}

func TestCompressionRatioReportsTheSaving(t *testing.T) {
	if got := (OrganizationState{}).CompressionRatio(); got != 0 {
		t.Fatalf("ratio with no source = %v, want 0", got)
	}
	state := OrganizationState{SourceBytes: 1000, GlobalSummaryBytes: 100, ProjectSummaryBytes: 150}
	if got := state.CompressionRatio(); got != 0.25 {
		t.Fatalf("ratio = %v, want 0.25", got)
	}
}
