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
