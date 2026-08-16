package coding_agent

import (
	"encoding/json"
	"os"
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
		run := result.Records[i].Subagent
		if run == nil || run.TaskID == "" {
			continue
		}
		path, reason := s.subagentSessionFile(run.TaskID)
		if path == "" {
			run.Reason = reason
			continue
		}
		child, err := trajectory.Project(path, trajectory.Options{MaxRecords: trajectory.AllRecords})
		if err != nil {
			run.Reason = "child session could not be read: " + err.Error()
			continue
		}
		run.Available = true
		run.Turns = child.Stats.Turns
		run.Steps = child.Stats.Steps
		run.Records = child.Stats.Records
		run.ToolCalls = child.Stats.ToolCalls
		run.Failures = child.Stats.ToolFailures
		run.ActiveMs = child.Stats.ActiveMs
		run.Tokens = child.Stats.Tokens
		run.Tools = child.Stats.Tools
	}
}

// subagentSessionFile locates a child session by background task id, returning
// the reason when it cannot.
func (s *CodingSession) subagentSessionFile(taskID string) (string, string) {
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
