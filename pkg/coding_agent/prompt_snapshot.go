package coding_agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/openmodu/modu/pkg/coding_agent/services/session"
)

// recordPromptSnapshot persists the model-visible instruction state — the
// assembled system prompt and the tool catalog — whenever it differs from what
// was last written.
//
// It is deliberately change-triggered rather than per-turn: a system prompt
// plus every tool's schema runs to tens of kilobytes, and appending that on
// every request would dwarf the conversation it is meant to explain. Writing
// only on change also makes the record useful on its own — each entry marks a
// point where the model's instructions actually moved.
//
// The entry is a sidecar, so it never joins the conversational branch.
func (s *engine) recordPromptSnapshot() {
	if s == nil || s.sessionManager == nil || s.agent == nil {
		return
	}
	state := s.agent.GetState()
	tools := make([]session.PromptToolData, 0, len(state.Tools))
	for _, tool := range state.Tools {
		if tool == nil {
			continue
		}
		entry := session.PromptToolData{
			Name:        tool.Name(),
			Label:       tool.Label(),
			Description: tool.Description(),
		}
		if schema, err := json.MarshalIndent(tool.Parameters(), "", "  "); err == nil {
			entry.Schema = string(schema)
		}
		tools = append(tools, entry)
	}
	if state.SystemPrompt == "" && len(tools) == 0 {
		return
	}

	systemDigest := digest(state.SystemPrompt)
	toolsDigest := digest(toolCatalogKey(tools))
	previous := s.promptDigest
	change := session.PromptChangeInitial
	if previous.recorded {
		systemMoved := previous.system != systemDigest
		toolsMoved := previous.tools != toolsDigest
		switch {
		case systemMoved && toolsMoved:
			change = session.PromptChangeSystemAndTools
		case systemMoved:
			change = session.PromptChangeSystem
		case toolsMoved:
			change = session.PromptChangeTools
		default:
			return
		}
	}

	entry := session.NewEntry(session.EntryTypePromptSnapshot, "", session.PromptSnapshotData{
		System: state.SystemPrompt,
		Tools:  tools,
		Change: change,
	})
	if err := s.sessionManager.AppendSidecar(entry); err != nil {
		return
	}
	s.promptDigest = promptDigest{system: systemDigest, tools: toolsDigest, recorded: true}
}

// promptDigest fingerprints the last persisted snapshot so an unchanged prompt
// costs one hash per turn instead of another copy on disk.
type promptDigest struct {
	system   string
	tools    string
	recorded bool
}

func toolCatalogKey(tools []session.PromptToolData) string {
	var builder strings.Builder
	for _, tool := range tools {
		builder.WriteString(tool.Name)
		builder.WriteByte(0)
		builder.WriteString(tool.Description)
		builder.WriteByte(0)
		builder.WriteString(tool.Schema)
		builder.WriteByte(0x1f)
	}
	return builder.String()
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
