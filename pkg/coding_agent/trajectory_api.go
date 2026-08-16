package coding_agent

import (
	"encoding/json"
	"os"

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
