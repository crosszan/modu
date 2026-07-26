package subagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openmodu/modu/pkg/coding_agent/plugins/extension"
	"github.com/openmodu/modu/pkg/types"
)

// A finished background child must reach the orchestrator on its own. Before
// this the only path was action=status, so a fire-and-forget dispatch went
// unnoticed whenever the model didn't think to poll.
func TestBackgroundCompletionPushesFollowUp(t *testing.T) {
	_, api := newExtensionWithProfiles(t, map[string]string{
		"explorer": frontmatterBody("explorer", "reads code"),
	})

	api.emit(extension.SubagentTaskDoneEvent, types.Event{
		Type:   types.EventType(extension.SubagentTaskDoneEvent),
		TaskID: "task-7",
		Reason: "completed",
		Args: extension.SubagentTaskDone{
			TaskID:     "task-7",
			Agent:      "explorer",
			Summary:    "map auth flow",
			Status:     "completed",
			Result:     "the login path starts in handler.go",
			Turns:      3,
			Tokens:     12400,
			DurationMs: 8200,
		},
	})

	followUps := api.followUpsSnapshot()
	if len(followUps) != 1 {
		t.Fatalf("got %d follow-up messages, want 1: %v", len(followUps), followUps)
	}
	notice := followUps[0]
	for _, want := range []string{
		`task_id="task-7"`,
		`agent="explorer"`,
		`status="completed"`,
		"map auth flow",
		"Done (3 turns · 12.4K tokens · 8s)",
		"the login path starts in handler.go",
	} {
		if !strings.Contains(notice, want) {
			t.Errorf("notice missing %q:\n%s", want, notice)
		}
	}
}

// Children of a batch stay quiet — the batch reports once when it finishes,
// and one notice per child would bury it.
func TestBatchChildCompletionStaysQuiet(t *testing.T) {
	_, api := newExtensionWithProfiles(t, map[string]string{
		"explorer": frontmatterBody("explorer", "reads code"),
	})

	api.emit(extension.SubagentTaskDoneEvent, types.Event{
		Type:   types.EventType(extension.SubagentTaskDoneEvent),
		TaskID: "task-8",
		Args: extension.SubagentTaskDone{
			TaskID:  "task-8",
			Agent:   "explorer",
			Status:  "completed",
			BatchID: "subagent-batch-1",
			Result:  "partial",
		},
	})

	if followUps := api.followUpsSnapshot(); len(followUps) != 0 {
		t.Fatalf("batch child should not notify on its own, got: %v", followUps)
	}
}

func TestTaskDoneNoticeTruncatesLongResults(t *testing.T) {
	notice := formatTaskDoneNotice(extension.SubagentTaskDone{
		TaskID: "task-9",
		Agent:  "explorer",
		Status: "completed",
		Result: strings.Repeat("x", notifyResultLimit+500),
	})
	if !strings.Contains(notice, "subagent action=status id=task-9") {
		t.Fatalf("truncated notice should point at the full record:\n%s", notice)
	}
	if len(notice) > notifyResultLimit+500 {
		t.Fatalf("notice not truncated: %d bytes", len(notice))
	}
}

// A live child takes the message straight into its next turn; anything the
// host can't reach falls back to the on-disk inbox.
func TestIntercomSendPrefersLiveDelivery(t *testing.T) {
	ext, api := newExtensionWithProfiles(t, map[string]string{
		"explorer": frontmatterBody("explorer", "reads code"),
	})
	api.agentDir = t.TempDir()
	api.sendToTaskFn = func(id, _ string) bool { return id == "task-3" }
	tool := registeredTool(t, api, "subagent_intercom_send")

	res, err := tool.Execute(context.Background(), "c1", map[string]any{
		"taskId":  "task-3",
		"message": "also check the refresh path",
		"from":    "orchestrator",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(textOf(res), "Delivered to running task task-3") {
		t.Fatalf("expected live delivery, got: %s", textOf(res))
	}
	if len(api.sentToTasks) != 1 || !strings.Contains(api.sentToTasks[0], "<intercom from=\"orchestrator\">") {
		t.Fatalf("message not framed for the child: %v", api.sentToTasks)
	}
	if _, err := os.Stat(intercomPath(ext, "task-3")); !os.IsNotExist(err) {
		t.Fatalf("live delivery should not touch the inbox file (err=%v)", err)
	}

	res, err = tool.Execute(context.Background(), "c2", map[string]any{
		"taskId":  "subagent-batch-1",
		"message": "status?",
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(textOf(res), "queued in its intercom inbox") {
		t.Fatalf("expected inbox fallback, got: %s", textOf(res))
	}
	if _, err := os.Stat(intercomPath(ext, "subagent-batch-1")); err != nil {
		t.Fatalf("fallback did not write the inbox file: %v", err)
	}
}

func intercomPath(ext *Extension, taskID string) string {
	return filepath.Join(ext.api.AgentDir(), "tool-results", projectKey(ext.api.Cwd()), "subagents", "intercom", taskID+".jsonl")
}

// The short description rides through to the host so the transcript and task
// list show a label instead of the raw task text.
func TestDescriptionReachesForkOptions(t *testing.T) {
	_, api := newExtensionWithProfiles(t, map[string]string{
		"explorer": frontmatterBody("explorer", "reads code"),
	})
	api.forkFn = func(_ context.Context, _ extension.ForkOptions) (string, error) { return "ok", nil }
	tool := toolOf(t, api)

	if _, err := tool.Execute(context.Background(), "d1", map[string]any{
		"agent":       "explorer",
		"task":        "Read every file under pkg/auth and summarise the login path",
		"description": "map auth flow",
	}, nil); err != nil {
		t.Fatalf("Execute single: %v", err)
	}
	if _, err := tool.Execute(context.Background(), "d2", map[string]any{
		"mode": "parallel",
		"parallel": []any{
			map[string]any{"agent": "explorer", "task": "read pkg/a", "description": "scan pkg/a"},
		},
	}, nil); err != nil {
		t.Fatalf("Execute parallel: %v", err)
	}

	api.mu.Lock()
	defer api.mu.Unlock()
	if len(api.forkCalls) != 2 {
		t.Fatalf("got %d fork calls, want 2", len(api.forkCalls))
	}
	if api.forkCalls[0].Summary != "map auth flow" {
		t.Errorf("single summary = %q, want \"map auth flow\"", api.forkCalls[0].Summary)
	}
	if api.forkCalls[1].Summary != "scan pkg/a" {
		t.Errorf("parallel item summary = %q, want \"scan pkg/a\"", api.forkCalls[1].Summary)
	}
}

// Profile editing lives on its own tool so the dispatch schema no longer
// offers "rewrite a profile" next to "delegate work".
func TestAdminToolOwnsProfileManagement(t *testing.T) {
	_, api := newExtensionWithProfiles(t, map[string]string{
		"explorer": frontmatterBody("explorer", "reads code"),
	})
	admin := registeredTool(t, api, "subagent_admin")

	res, err := admin.Execute(context.Background(), "a1", map[string]any{"action": "list"}, nil)
	if err != nil {
		t.Fatalf("Execute list: %v", err)
	}
	if !strings.Contains(textOf(res), "explorer") {
		t.Fatalf("list did not report the profile: %s", textOf(res))
	}

	// Omitting the action lists rather than erroring.
	if res, err = admin.Execute(context.Background(), "a2", map[string]any{}, nil); err != nil {
		t.Fatalf("Execute default: %v", err)
	} else if res.IsError {
		t.Fatalf("default action should not error: %s", textOf(res))
	}

	// Runtime actions stay on the dispatch tool.
	res, err = admin.Execute(context.Background(), "a3", map[string]any{"action": "status"}, nil)
	if err != nil {
		t.Fatalf("Execute status: %v", err)
	}
	if !res.IsError || !strings.Contains(textOf(res), "unknown action") {
		t.Fatalf("admin tool should reject runtime actions, got: %s", textOf(res))
	}

	// The dispatch tool no longer advertises profile management.
	desc := toolOf(t, api).Description()
	for _, gone := range []string{"create: create a new profile", "delete: remove a profile"} {
		if strings.Contains(desc, gone) {
			t.Errorf("dispatch description still advertises %q", gone)
		}
	}
	if !strings.Contains(desc, "subagent_admin") {
		t.Error("dispatch description should point at subagent_admin")
	}
}

// The doctor notice is rendered as Markdown by hosts, so a directory
// placeholder written with angle brackets disappears as an HTML tag — the
// line then names a path starting at the filesystem root.
func TestDoctorScanPathsSurviveMarkdown(t *testing.T) {
	ext, _ := newExtensionWithProfiles(t, map[string]string{
		"explorer": frontmatterBody("explorer", "reads code"),
	})
	ext.cfg.AgentsDir = ""

	doctor := formatDoctor(ext)
	line := ""
	for _, l := range strings.Split(doctor, "\n") {
		if strings.Contains(l, "scanned (later wins)") {
			line = l
		}
	}
	if line == "" {
		t.Fatalf("doctor did not list the scan paths:\n%s", doctor)
	}
	if strings.ContainsAny(line, "<>") {
		t.Errorf("scan-path line uses angle brackets, which Markdown eats: %q", line)
	}
	for _, want := range []string{"{agent_dir}/agents", "{cwd}/.coding_agent/agents"} {
		if !strings.Contains(line, want) {
			t.Errorf("scan-path line missing %q: %q", want, line)
		}
	}
	// Discovery is modu's own directories only.
	if strings.Contains(line, ".claude") {
		t.Errorf("scan-path line advertises another tool's directory: %q", line)
	}
}
