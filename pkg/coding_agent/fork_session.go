package coding_agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openmodu/modu/pkg/coding_agent/plugins/extension"
	"github.com/openmodu/modu/pkg/coding_agent/plugins/subagent"
	toolpkg "github.com/openmodu/modu/pkg/coding_agent/tools"
	"github.com/openmodu/modu/pkg/types"
)

// forkSession is the host-side implementation of ExtensionAPI.ForkSession.
//
// It mirrors the old spawn_subagent dispatch tree so extension callers get
// the same capabilities:
//
//   - opts.Background: schedule the child on a goroutine, return a task-id
//     reference, surface completion through the session's taskManager.
//   - opts.Isolation == "worktree": create a fresh git worktree, rebind
//     file/shell tools to it, run the child there, and remove the worktree
//     on return.
//   - default: run the child synchronously in the caller's cwd.
//
// The synchronous and worktree paths both flow through subagent.Run after
// prepareSubagentDefinition has layered in skills / memory / harness-block
// directives — extension callers therefore get the same system-prompt
// augmentation spawn_subagent gives.
func (cs *engine) forkSession(ctx context.Context, opts extension.ForkOptions) (string, error) {
	childCwd := cs.resolveChildCwd(opts.Cwd)
	def := &subagent.SubagentDefinition{
		Name:            forkName(opts),
		SystemPrompt:    opts.SystemPrompt,
		Tools:           append([]string(nil), opts.AllowedTools...),
		DisallowedTools: append([]string(nil), opts.DisallowedTools...),
		Skills:          append([]string(nil), opts.Skills...),
		MemoryScope:     opts.MemoryScope,
		Model:           opts.Model,
		ThinkingLevel:   types.ThinkingLevel(opts.ThinkingLevel),
		PermissionMode:  opts.PermissionMode,
		MaxTurns:        opts.MaxTurns,
		Background:      opts.Background,
		Isolation:       opts.Isolation,
	}
	memoryEnabled := memoryFeatureEnabled(cs.config)
	def = prepareSubagentDefinition(def, cs.skillManager, cs.memoryStore, memoryEnabled)

	initialMessages, err := cs.initialMessagesForFork(opts.Context, opts.ParentTaskID)
	if err != nil {
		return "", err
	}

	if opts.Background {
		return cs.forkInBackground(ctx, def, childCwd, initialMessages, opts, cs.resolveForkSessionDir(opts.SessionDir))
	}
	// Synchronous runs have no host task to key on, so they get a synthetic
	// run id — the UI needs a stable handle to update one run's progress
	// block, and the extension bus still only sees BubbleTaskID.
	runID := "run-" + uuid.NewString()
	run := HarnessSubagentRun{Name: def.Name, Task: opts.Task, Label: opts.Summary, TaskID: runID}
	cs.OnSubagentStart(run)
	observe := cs.childObserver(runID, def.Name, opts.BubbleTaskID)
	var (
		result        string
		childMessages []types.AgentMessage
	)
	startedAt := time.Now()
	if strings.EqualFold(opts.Isolation, "worktree") {
		result, childMessages, err = cs.forkInWorktree(ctx, def, initialMessages, opts.Task, observe)
	} else {
		tools := cs.toolsForFork(childCwd, def.Tools)
		runResult, runErr := subagent.RunWithMessagesObserved(
			ctx,
			subagent.WithWorkingDirectory(def, childCwd),
			initialMessages,
			opts.Task,
			tools,
			cs.model,
			cs.getAPIKey,
			cs.streamFn,
			observe,
		)
		result, childMessages, err = runResult.Text, runResult.Messages, runErr
	}
	cs.OnSubagentStop(run, result, err, subagentRunStats(childMessages, startedAt))
	cs.emitSubagentChildUsage(opts.BubbleTaskID, childMessages)
	cs.persistSyncSubagentRun(opts.CallID, def.Name, childMessages)
	return result, err
}

// persistSyncSubagentRun writes a synchronous child's conversation where the
// parent's trajectory can find it again.
//
// A background run gets this for free: it registers a task, and the task holds
// its session file. A synchronous run registers nothing, so its work used to
// vanish the moment it returned — leaving one opaque tool call in the parent's
// ledger where a large share of a turn's time often went. The tool call id is
// the only handle the parent has, and one call can fork several children in
// parallel and chain runs, so the file is named for the call and a sequence
// number within it.
func (cs *engine) persistSyncSubagentRun(callID, agentName string, messages []types.AgentMessage) {
	if strings.TrimSpace(callID) == "" || len(messages) == 0 {
		return
	}
	dir := filepath.Join(cs.RuntimePaths().SubagentRunsDir, cs.GetSessionID())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	name := syncSubagentRunFile(dir, callID)
	if name == "" {
		return
	}
	id := strings.TrimSuffix(filepath.Base(name), ".jsonl")
	// Best effort: a child that ran is worth more than a parent turn that
	// fails because its transcript could not be filed.
	_ = writeSubagentSessionFile(name, cs.cwd, cs.GetSessionID(), id, agentName, messages)
}

// syncSubagentRunFile picks the next free slot for a call, so the children of
// one parallel call sit side by side instead of overwriting each other.
func syncSubagentRunFile(dir, callID string) string {
	safe := sanitizeRunID(callID)
	for i := range 100 {
		candidate := filepath.Join(dir, fmt.Sprintf("%s-%d.jsonl", safe, i))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return ""
}

func sanitizeRunID(id string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, id)
	if cleaned == "" {
		return "run"
	}
	return cleaned
}

// subagentRunStats tallies one child run into the closing figures host UIs
// render: one turn per assistant message, tokens as fresh input+output, and
// the wall-clock time since the run started.
func subagentRunStats(messages []types.AgentMessage, startedAt time.Time) SubagentRunStats {
	stats := SubagentRunStats{DurationMs: time.Since(startedAt).Milliseconds()}
	for _, msg := range messages {
		var assistant types.AssistantMessage
		switch m := msg.(type) {
		case types.AssistantMessage:
			assistant = m
		case *types.AssistantMessage:
			if m == nil {
				continue
			}
			assistant = *m
		default:
			continue
		}
		stats.Turns++
		stats.Tokens += assistant.Usage.Input + assistant.Usage.Output
	}
	return stats
}

// childObserver returns an event observer for one child run. It feeds two
// independent consumers:
//
//   - extensions, via "subagent_child_event" under bubbleID — unchanged
//     behaviour, still limited to callers that asked for it (batch dispatch
//     and background children);
//   - host UIs, via SessionEventSubagentProgress under runID, so a live run
//     can show what it is doing. Every child reports here, synchronous ones
//     included: a foreground delegation is exactly the case where the user is
//     sitting there watching.
func (cs *engine) childObserver(runID, agentName, bubbleID string) func(types.Event) {
	return func(ev types.Event) {
		cs.emitSubagentChildEvent(bubbleID, ev)
		cs.emitSubagentProgress(runID, agentName, ev)
	}
}

// emitSubagentProgress translates one child event into the single line a UI
// shows for it. Only tool completions and turn boundaries are reported —
// streaming deltas would drown the parent's event stream for no gain.
func (cs *engine) emitSubagentProgress(runID, agentName string, ev types.Event) {
	if runID == "" {
		return
	}
	switch ev.Type {
	case types.EventTypeToolExecutionEnd:
		errMessage := ""
		if ev.IsError {
			errMessage = "failed"
		}
		cs.onSubagentProgress(runID, agentName, "tool", ev.ToolName, toolCallDetail(ev.Args), errMessage, 0)
	case types.EventTypeTurnEnd:
		cs.onSubagentProgress(runID, agentName, "turn", "", "", "", turnTokens(ev.Message))
	}
}

// toolCallDetail picks the one argument worth showing beside a tool name —
// the path, command, or pattern that says what the call was about. Falls back
// to the first short string argument, and to "" when nothing fits.
func toolCallDetail(args any) string {
	m, ok := args.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range []string{"file_path", "path", "command", "pattern", "query", "url", "task", "agent"} {
		if s, ok := m[key].(string); ok && strings.TrimSpace(s) != "" {
			return firstLineOf(s, 60)
		}
	}
	return ""
}

func firstLineOf(text string, limit int) string {
	text = strings.TrimSpace(text)
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		text = text[:idx]
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}

// turnTokens returns the fresh input+output tokens a turn_end event carries.
func turnTokens(msg types.AgentMessage) int {
	switch m := msg.(type) {
	case types.AssistantMessage:
		return m.Usage.Input + m.Usage.Output
	case *types.AssistantMessage:
		if m != nil {
			return m.Usage.Input + m.Usage.Output
		}
	}
	return 0
}

// emitSubagentChildUsage broadcasts a child agent's token usage to
// extensions via the shared event bus. The child transcript carries
// per-assistant-message Usage, so consumers (e.g. the goal extension)
// can fold subagent token spend into their own accounting instead of
// silently undercounting it. No-op when there is nothing to report.
func (cs *engine) emitSubagentChildUsage(taskID string, messages []types.AgentMessage) {
	if cs.extensions == nil || len(messages) == 0 {
		return
	}
	cs.extensions.EmitEvent(types.Event{
		Type:     types.EventType("subagent_child_usage"),
		TaskID:   taskID,
		Messages: messages,
	})
}

// emitSubagentChildEvent re-emits a background child agent's lifecycle events
// to extensions, tagged with the child's task id, so an extension (e.g.
// subagent control) can track a running child's turn count, failed-tool
// count, and token usage in flight rather than only at completion. Only the
// coarse lifecycle events useful for control are forwarded, to keep the bus
// quiet. The original child event type travels in Reason.
func (cs *engine) emitSubagentChildEvent(taskID string, ev types.Event) {
	if cs.extensions == nil || taskID == "" {
		return
	}
	switch ev.Type {
	case types.EventTypeTurnEnd, types.EventTypeToolExecutionEnd, types.EventTypeAgentEnd:
	default:
		return
	}
	cs.extensions.EmitEvent(types.Event{
		Type:     types.EventType("subagent_child_event"),
		TaskID:   taskID,
		Reason:   string(ev.Type),
		ToolName: ev.ToolName,
		Args:     ev.Args,
		Result:   ev.Result,
		IsError:  ev.IsError,
		Message:  ev.Message, // carries per-turn Usage on turn_end
	})
}

// forkInBackground launches the child on its own goroutine. Returns a
// short string the model can pass to task_output to follow up. If the
// session has no task manager, surfaces a clear error instead of silently
// dropping the request. When sessionDirOverride is non-empty the task's
// session.jsonl/status.json land under that parent dir; otherwise the
// task manager picks its default run root.
func (cs *engine) forkInBackground(ctx context.Context, def *subagent.SubagentDefinition, childCwd string, initialMessages []types.AgentMessage, opts extension.ForkOptions, sessionDirOverride string) (string, error) {
	if cs.taskManager == nil {
		return "", fmt.Errorf("background fork requested but task manager is not configured")
	}
	name := "extension-fork"
	if def != nil && strings.TrimSpace(def.Name) != "" {
		name = def.Name
	}
	task := opts.Task
	outputPath := cs.resolveForkOutputPath(opts.OutputPath)
	summary := strings.TrimSpace(opts.Summary)
	if summary == "" {
		summary = task
	}
	taskID := cs.taskManager.CreateWithMetadataInDir("subagent", fmt.Sprintf("%s: %s", name, summary), name, task, opts.ParentTaskID, outputPath, sessionDirOverride)
	// Events bubble under the caller-supplied id when set (batch children all
	// share the batch id) so a batch's control counters aggregate across its
	// children; otherwise under this background task's own id.
	bubbleID := taskID
	if opts.BubbleTaskID != "" {
		bubbleID = opts.BubbleTaskID
	}
	run := HarnessSubagentRun{Name: name, Task: task, Background: true, Label: strings.TrimSpace(opts.Summary), TaskID: taskID}
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	cs.taskManager.RegisterCancel(taskID, cancel)
	go func() {
		defer cs.taskManager.UnregisterCancel(taskID)
		if def != nil {
			cs.OnSubagentStart(run)
		}
		tools := cs.toolsForFork(childCwd, def.Tools)
		startedAt := time.Now()
		result, err := subagent.RunWithHooks(
			runCtx,
			subagent.WithWorkingDirectory(def, childCwd),
			initialMessages,
			task,
			tools,
			cs.model,
			cs.getAPIKey,
			cs.streamFn,
			subagent.RunHooks{
				Observe: cs.childObserver(taskID, name, bubbleID),
				// Registering the live handle here — rather than before the
				// goroutine starts — keeps the steer map in step with the
				// child that actually exists.
				OnStart: func(h subagent.Handle) {
					cs.taskManager.RegisterSteer(taskID, func(text string) {
						h.Steer(types.UserMessage{Role: types.RoleUser, Content: text})
					})
				},
			},
		)
		text := result.Text
		stats := subagentRunStats(result.Messages, startedAt)
		cs.emitSubagentChildUsage(bubbleID, result.Messages)
		if taskRecord, ok := cs.taskManager.Get(taskID); ok {
			if writeErr := writeSubagentSessionFile(taskRecord.SessionFile, childCwd, cs.GetSessionID(), taskID, def.Name, result.Messages); writeErr != nil && err == nil {
				err = writeErr
			}
		}
		if err == nil && strings.TrimSpace(outputPath) != "" {
			savedText, saveErr := saveForkOutput(outputPath, opts.OutputMode, text)
			if saveErr != nil {
				err = saveErr
			} else {
				text = savedText
			}
		}
		if def != nil {
			cs.OnSubagentStop(run, text, err, stats)
		}
		if err != nil {
			cs.taskManager.Fail(taskID, err.Error())
			cs.emitSubagentTaskDone(taskID, name, summary, "failed", err.Error(), opts.BubbleTaskID, stats)
			return
		}
		cs.taskManager.Complete(taskID, text)
		cs.emitSubagentTaskDone(taskID, name, summary, "completed", text, opts.BubbleTaskID, stats)
	}()
	return fmt.Sprintf("Started extension-fork in background. Use task_output with task_id=%s to inspect the result.", taskID), nil
}

// emitSubagentTaskDone tells extensions that a background child finished, so
// the extension that dispatched it can push a completion notice back into the
// parent conversation instead of waiting to be polled. Carries the same
// figures the host UI renders, plus the child's final text.
func (cs *engine) emitSubagentTaskDone(taskID, agentName, summary, status, result, batchID string, stats SubagentRunStats) {
	if cs.extensions == nil || taskID == "" {
		return
	}
	cs.extensions.EmitEvent(types.Event{
		Type:    types.EventType(extension.SubagentTaskDoneEvent),
		TaskID:  taskID,
		Reason:  status,
		IsError: status == "failed",
		Args: extension.SubagentTaskDone{
			TaskID:     taskID,
			Agent:      agentName,
			Summary:    summary,
			Status:     status,
			BatchID:    batchID,
			Result:     result,
			Turns:      stats.Turns,
			Tokens:     stats.Tokens,
			DurationMs: stats.DurationMs,
		},
	})
}

func (cs *engine) resolveChildCwd(cwd string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return cs.cwd
	}
	if filepath.IsAbs(cwd) {
		return filepath.Clean(cwd)
	}
	return filepath.Clean(filepath.Join(cs.cwd, cwd))
}

func (cs *engine) resolveForkOutputPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(cs.RuntimePaths().ToolResultsDir, "subagents", path)
}

// resolveForkSessionDir turns a caller-supplied session dir override into
// an absolute path. Empty input passes through so the host's default run
// root is used. Relative input resolves against the parent session's cwd
// to match how Cwd/OutputPath are treated.
func (cs *engine) resolveForkSessionDir(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Clean(filepath.Join(cs.cwd, path))
}

func saveForkOutput(path, mode, text string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		return "", err
	}
	ref := fmt.Sprintf("Output saved to: %s (%d bytes, %d lines).", path, len([]byte(text)), countLines(text))
	if strings.EqualFold(strings.TrimSpace(mode), "file-only") {
		return ref, nil
	}
	return strings.TrimSpace(text) + "\n\n" + ref, nil
}

func countLines(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

func (cs *engine) loadSubagentParentMessages(parentID string) ([]types.AgentMessage, error) {
	if strings.TrimSpace(parentID) == "" || cs.taskManager == nil {
		return nil, nil
	}
	parent, ok := cs.taskManager.Get(parentID)
	if !ok || strings.TrimSpace(parent.SessionFile) == "" {
		return nil, nil
	}
	return loadSubagentSessionMessages(parent.SessionFile)
}

func (cs *engine) initialMessagesForFork(mode, parentID string) ([]types.AgentMessage, error) {
	if strings.TrimSpace(parentID) != "" {
		return cs.loadSubagentParentMessages(parentID)
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "fresh":
		return nil, nil
	case "fork":
		if cs == nil || cs.agent == nil {
			return nil, nil
		}
		return append([]types.AgentMessage(nil), cs.agent.GetState().Messages...), nil
	default:
		return nil, fmt.Errorf("unknown fork context %q (expected fresh|fork)", mode)
	}
}

// forkInWorktree creates a detached git worktree, rebinds file/shell
// tools to that path, runs the child, and removes the worktree on exit.
// Mirrors the legacy spawn_subagent worktree behavior closely.
func (cs *engine) forkInWorktree(ctx context.Context, def *subagent.SubagentDefinition, initialMessages []types.AgentMessage, task string, observe func(types.Event)) (string, []types.AgentMessage, error) {
	root, err := gitTopLevelDir(cs.cwd)
	if err != nil {
		return "", nil, fmt.Errorf("worktree isolation requires a git repository: %w", err)
	}
	baseDir := filepath.Join(cs.agentDir, "worktrees")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", nil, err
	}
	path := filepath.Join(baseDir, uuid.NewString(), filepath.Base(root))
	if _, err := runGitCommand(root, "worktree", "add", "--detach", path, "HEAD"); err != nil {
		return "", nil, err
	}
	defer func() {
		// Best-effort cleanup: if the worktree leaks the user can prune
		// it manually via `git worktree prune`.
		_, _ = runGitCommand(root, "worktree", "remove", "--force", path)
		removeEmptyWorktreeParents(path, baseDir)
	}()
	rebound := cs.toolsForFork(path, def.Tools)
	result, err := subagent.RunWithMessagesObserved(
		ctx,
		subagent.WithWorkingDirectory(def, path),
		initialMessages,
		task,
		rebound,
		cs.model,
		cs.getAPIKey,
		cs.streamFn,
		observe,
	)
	if err != nil {
		return "", result.Messages, err
	}
	return result.Text, result.Messages, nil
}

func forkName(opts extension.ForkOptions) string {
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		return "extension-fork"
	}
	return name
}

func (cs *engine) toolsForFork(cwd string, requested []string) []types.Tool {
	tools := cs.activeTools
	if len(requested) == 0 && cs.agent != nil {
		tools = cs.agent.GetState().Tools
	}
	if cwd != cs.cwd {
		tools = cs.rebindToolsToCwd(cwd, tools)
	}
	return cs.ensureRequestedReadOnlyTools(tools, requested, cwd)
}

func ensureRequestedReadOnlyTools(active []types.Tool, requested []string, cwd string) []types.Tool {
	return ensureRequestedReadOnlyToolsWithContext(active, requested, cwd, types.ToolContext{Cwd: cwd})
}

func (cs *engine) ensureRequestedReadOnlyTools(active []types.Tool, requested []string, cwd string) []types.Tool {
	return ensureRequestedReadOnlyToolsWithContext(active, requested, cwd, cs.toolContext(cwd))
}

func ensureRequestedReadOnlyToolsWithContext(active []types.Tool, requested []string, cwd string, ctx types.ToolContext) []types.Tool {
	if len(requested) == 0 {
		return active
	}
	have := make(map[string]bool, len(active))
	for _, tool := range active {
		have[tool.Name()] = true
	}
	want := make(map[string]bool, len(requested))
	for _, name := range requested {
		name = strings.TrimSpace(name)
		if name != "" {
			want[name] = true
		}
	}
	out := append([]types.Tool(nil), active...)
	for _, tool := range toolpkg.ReadOnlyTools(cwd) {
		name := tool.Name()
		if name == "read" {
			continue
		}
		if want[name] && !have[name] {
			out = append(out, tool)
			have[name] = true
		}
	}
	return out
}

// rebindToolsToCwd returns a copy of tools where cwd-bound tools point at the
// given path. Unknown tools pass through unchanged.
func (cs *engine) rebindToolsToCwd(cwd string, in []types.Tool) []types.Tool {
	out := make([]types.Tool, 0, len(in))
	for _, tool := range in {
		if rebound, ok := cs.toolProvider.Rebind(tool, cs.toolContext(cwd)); ok {
			out = append(out, rebound)
			continue
		}
		if rebindable, ok := tool.(interface{ WithCwd(string) types.Tool }); ok {
			out = append(out, rebindable.WithCwd(cwd))
		} else {
			out = append(out, tool)
		}
	}
	return out
}

// gitTopLevelDir returns the repo root that contains dir, trimmed of
// trailing whitespace. Useful to translate any cwd inside the repo back
// to the canonical worktree root that `git worktree add` accepts.
func gitTopLevelDir(dir string) (string, error) {
	out, err := runGitCommand(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func runGitCommand(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func removeEmptyWorktreeParents(path, base string) {
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return
	}
	for parent := filepath.Dir(path); parent != baseAbs && parent != "." && parent != string(os.PathSeparator); parent = filepath.Dir(parent) {
		parentAbs, err := filepath.Abs(parent)
		if err != nil || parentAbs == baseAbs {
			return
		}
		if err := os.Remove(parent); err != nil {
			return
		}
	}
}
