package coding_agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/openmodu/modu/pkg/coding_agent/trajectory"
)

// Trajectory projects the current session's log into a turn-aware event
// ledger. It reads the persisted session file, which the session manager keeps
// current on every append, so a running session projects up to its last event.
//
// A session that has not persisted anything yet projects as empty rather than
// failing: "nothing has happened" is an answer, not an error.
func (s *CodingSession) Trajectory(opts trajectory.Options) (trajectory.Trajectory, error) {
	path := s.GetSessionFile()
	result, err := trajectory.Project(path, opts)
	if os.IsNotExist(err) {
		empty := trajectory.Trajectory{
			SchemaVersion: trajectory.SchemaVersion,
			Session:       trajectory.Session{ID: s.GetSessionID(), Title: "(no prompt)", Cwd: s.cwd},
		}
		empty.Session.Prompt = s.promptSnapshot()
		return empty, nil
	}
	if err != nil {
		return trajectory.Trajectory{}, err
	}
	result.Session.Prompt = s.promptSnapshot()
	s.resolveSubagentRuns(&result)
	return result, nil
}

// ExportTrajectoryHTML writes the session's trajectory as a self-contained
// interactive page. Detail is full: the export lands on the user's own disk,
// matching what ExportHTML already writes.
func (s *CodingSession) ExportTrajectoryHTML(path string) error {
	result, err := s.Trajectory(trajectory.Options{
		Detail:     trajectory.DetailFull,
		MaxRecords: trajectory.AllRecords,
	})
	if err != nil {
		return err
	}
	return trajectory.WriteHTML(result, path)
}

// promptSnapshot captures the model-visible instruction state: the assembled
// system prompt and the tool catalog with each tool's call-time schema.
//
// None of this is persisted in the session log — it is rebuilt on every run
// from config, skills, and context files — so it can only be reported for the
// live session, never reconstructed from a session file after the fact.
func (s *CodingSession) promptSnapshot() *trajectory.Prompt {
	if s == nil || s.agent == nil {
		return nil
	}
	state := s.agent.GetState()
	snapshot := &trajectory.Prompt{
		System: state.SystemPrompt,
		Bytes:  len(state.SystemPrompt),
		Tools:  make([]trajectory.Tool, 0, len(state.Tools)),
	}
	for _, tool := range state.Tools {
		if tool == nil {
			continue
		}
		entry := trajectory.Tool{
			Name:        tool.Name(),
			Label:       tool.Label(),
			Description: tool.Description(),
		}
		if schema, err := json.MarshalIndent(tool.Parameters(), "", "  "); err == nil {
			entry.Schema = string(schema)
		}
		snapshot.Tools = append(snapshot.Tools, entry)
	}
	if snapshot.System == "" && len(snapshot.Tools) == 0 {
		return nil
	}
	return snapshot
}

// resolveSubagentRuns fills in each subagent record with statistics from the
// session its child agent actually ran in.
//
// A subagent is a separate session: the parent's log records that the tool was
// called and nothing about what happened inside. The background task registry
// is the only link, holding the child's session file against the task id the
// tool reported. Only asynchronous runs register a task and write that file,
// so a synchronous subagent resolves to unavailable rather than to zeros.
func (s *CodingSession) resolveSubagentRuns(result *trajectory.Trajectory) {
	for i := range result.Records {
		record := &result.Records[i]
		if record.Kind != trajectory.KindSubagent {
			continue
		}
		if record.Subagent != nil && record.Subagent.RunID != "" {
			s.resolveAsyncSubagentRun(record.Subagent)
			continue
		}
		// No task id means the run was synchronous. Its transcript is filed
		// under the tool call that requested it, and one call can have forked
		// several children.
		for _, path := range s.syncSubagentSessionFiles(record.CallID) {
			run := trajectory.SubagentRun{
				RunID: strings.TrimSuffix(filepath.Base(path), ".jsonl"),
			}
			if !fillSubagentRun(&run, path) {
				run.Reason = "child session could not be read"
			}
			record.Subagents = append(record.Subagents, run)
		}
		if len(record.Subagents) == 1 && record.Subagent == nil {
			record.Subagent = &record.Subagents[0]
			record.Subagents = nil
		}
	}
}

func (s *CodingSession) resolveAsyncSubagentRun(run *trajectory.SubagentRun) {
	path, reason := s.subagentSessionFile(run.RunID)
	if path == "" {
		run.Reason = reason
		return
	}
	if !fillSubagentRun(run, path) {
		run.Reason = "child session could not be read"
	}
}

// fillSubagentRun projects a child session into a run summary.
func fillSubagentRun(run *trajectory.SubagentRun, path string) bool {
	child, err := trajectory.Project(path, trajectory.Options{MaxRecords: trajectory.AllRecords})
	if err != nil {
		return false
	}
	run.Available = true
	if run.Agent == "" {
		run.Agent = child.Session.Name
	}
	run.Turns = child.Stats.Turns
	run.Steps = child.Stats.Steps
	run.Records = child.Stats.Records
	run.ToolCalls = child.Stats.ToolCalls
	run.Failures = child.Stats.ToolFailures
	run.ActiveMs = child.Stats.ActiveMs
	run.Tokens = child.Stats.Tokens
	run.Tools = child.Stats.Tools
	return true
}

// syncSubagentSessionByID resolves a synchronous run's transcript by its id,
// which is the transcript's own file name.
func (s *CodingSession) syncSubagentSessionByID(runID string) string {
	runID = strings.TrimSpace(runID)
	if runID == "" || strings.ContainsAny(runID, `/\`) {
		return ""
	}
	path := filepath.Join(s.RuntimePaths().SubagentRunsDir, s.GetSessionID(), runID+".jsonl")
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

// syncSubagentSessionFiles lists the transcripts a synchronous run left under
// one tool call, in the order they were filed.
func (s *CodingSession) syncSubagentSessionFiles(callID string) []string {
	if strings.TrimSpace(callID) == "" {
		return nil
	}
	dir := filepath.Join(s.RuntimePaths().SubagentRunsDir, s.GetSessionID())
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	prefix := sanitizeRunID(callID) + "-"
	var paths []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		paths = append(paths, filepath.Join(dir, name))
	}
	sort.Strings(paths)
	return paths
}

// subagentSessionFile locates a child session by background task id, returning
// the reason when it cannot.
func (s *CodingSession) subagentSessionFile(taskID string) (string, string) {
	// A synchronous run has no task; it is addressed by the transcript filed
	// under its tool call, so fall through to that before giving up.
	if path := s.syncSubagentSessionByID(taskID); path != "" {
		return path, ""
	}
	if s.taskManager == nil {
		return "", "background tasks are not available in this session"
	}
	task, ok := s.taskManager.Get(taskID)
	if !ok {
		return "", "no background task is registered for this run"
	}
	if strings.TrimSpace(task.SessionFile) == "" {
		return "", "this run recorded no session file"
	}
	if _, err := os.Stat(task.SessionFile); err != nil {
		// A run still in flight has not written its session file yet.
		return "", "the child session has not been written yet"
	}
	return task.SessionFile, ""
}

// SubagentTrajectory projects the session a subagent ran in, addressed by the
// background task id its tool call reported.
//
// The child is an ordinary session file in the same format, so it projects
// through the same code path; only locating it differs.
func (s *CodingSession) SubagentTrajectory(taskID string, opts trajectory.Options) (trajectory.Trajectory, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return trajectory.Trajectory{}, fmt.Errorf("a task id is required")
	}
	path, reason := s.subagentSessionFile(taskID)
	if path == "" {
		return trajectory.Trajectory{}, fmt.Errorf("subagent %s: %s", taskID, reason)
	}
	result, err := trajectory.Project(path, opts)
	if err != nil {
		return trajectory.Trajectory{}, fmt.Errorf("subagent %s: %w", taskID, err)
	}
	if task, ok := s.taskManager.Get(taskID); ok && task.Agent != "" {
		result.Session.Name = task.Agent
	}
	// A subagent nests, so resolve the runs it started in turn.
	s.resolveSubagentRuns(&result)
	return result, nil
}

// ExportSubagentTrajectoryHTML writes a subagent's own trajectory page.
func (s *CodingSession) ExportSubagentTrajectoryHTML(taskID, path string) error {
	result, err := s.SubagentTrajectory(taskID, trajectory.Options{
		Detail:     trajectory.DetailFull,
		MaxRecords: trajectory.AllRecords,
	})
	if err != nil {
		return err
	}
	return trajectory.WriteHTML(result, path)
}

// SubagentRunIDs lists the addressable ids of subagent runs on the current
// branch, in the order they were started.
func (s *CodingSession) SubagentRunIDs() []string {
	result, err := s.Trajectory(trajectory.Options{MaxRecords: trajectory.AllRecords})
	if err != nil {
		return nil
	}
	var ids []string
	seen := make(map[string]bool)
	for _, record := range result.Records {
		run := record.Subagent
		if run == nil || run.RunID == "" || seen[run.RunID] {
			continue
		}
		seen[run.RunID] = true
		ids = append(ids, run.RunID)
	}
	return ids
}
