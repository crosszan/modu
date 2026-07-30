package main

import (
	codetui "github.com/openmodu/modu/cmd/modu_code/internal/tui"
	coding_agent "github.com/openmodu/modu/pkg/coding_agent"
	modutui "github.com/openmodu/modu/pkg/modu-tui"
	"github.com/openmodu/modu/pkg/types"
)

func newModuTUIEventPresenter() codetui.EventPresenter {
	return codetui.NewEventPresenter(moduTUIToolPresenter{}, moduTUIContextCompactDivider)
}

func moduTUITranscriptEntries(session *coding_agent.CodingSession, presenter codetui.EventPresenter) []modutui.Entry {
	if session == nil {
		return nil
	}
	state := session.GetAgent().GetState()
	messages := moduTUIRestoreToolBatchMetadata(session.GetMessages(), state.Tools)
	nodes := session.GetSessionTreeNodes()
	if len(nodes) == 0 {
		return presenter.AgentMessages(messages, session.Cwd())
	}
	out := make([]modutui.Entry, 0, len(messages))
	messageIndex := 0
	for _, node := range nodes {
		if !node.InCurrentPath {
			continue
		}
		switch node.Type {
		case "message":
			if messageIndex >= len(messages) {
				continue
			}
			out = append(out, presenter.AgentMessage(messages[messageIndex], session.Cwd())...)
			messageIndex++
		case "compaction":
			out = append(out, presenter.ContextCompactEntry())
		}
	}
	for messageIndex < len(messages) {
		out = append(out, presenter.AgentMessage(messages[messageIndex], session.Cwd())...)
		messageIndex++
	}
	return out
}

type moduTUIToolBatchMetadata struct {
	size int
	id   string
}

// moduTUIRestoreToolBatchMetadata preserves persisted batch metadata and
// reconstructs it for sessions written before tool results carried BatchID.
// The grouping mirrors agent.DefaultTools: consecutive parallel-capable calls
// in one assistant message share a deterministic id based on the first call.
func moduTUIRestoreToolBatchMetadata(messages []types.AgentMessage, tools []types.Tool) []types.AgentMessage {
	parallelTools := make(map[string]bool, len(tools))
	for _, tool := range tools {
		parallel, ok := tool.(types.ParallelTool)
		if ok && parallel.Parallel() {
			parallelTools[tool.Name()] = true
		}
	}

	batches := make(map[string]moduTUIToolBatchMetadata)
	out := make([]types.AgentMessage, 0, len(messages))
	for _, message := range messages {
		switch typed := message.(type) {
		case types.AssistantMessage:
			out = append(out, moduTUIRestoreAssistantToolBatches(typed, parallelTools, batches))
		case *types.AssistantMessage:
			if typed == nil {
				out = append(out, message)
				continue
			}
			restored := moduTUIRestoreAssistantToolBatches(*typed, parallelTools, batches)
			out = append(out, &restored)
		case types.ToolResultMessage:
			if batch, ok := batches[typed.ToolCallID]; ok && typed.BatchID == "" {
				typed.BatchSize = batch.size
				typed.BatchID = batch.id
			}
			out = append(out, typed)
		case *types.ToolResultMessage:
			if typed == nil {
				out = append(out, message)
				continue
			}
			restored := *typed
			if batch, ok := batches[restored.ToolCallID]; ok && restored.BatchID == "" {
				restored.BatchSize = batch.size
				restored.BatchID = batch.id
			}
			out = append(out, &restored)
		default:
			out = append(out, message)
		}
	}
	return out
}

func moduTUIRestoreAssistantToolBatches(
	message types.AssistantMessage,
	parallelTools map[string]bool,
	batches map[string]moduTUIToolBatchMetadata,
) types.AssistantMessage {
	calls := make([]*types.ToolCallContent, 0)
	for _, block := range message.Content {
		if call, ok := block.(*types.ToolCallContent); ok && call != nil {
			calls = append(calls, call)
		}
	}
	for start := 0; start < len(calls); {
		if !parallelTools[calls[start].Name] {
			start++
			continue
		}
		stop := start + 1
		for stop < len(calls) && parallelTools[calls[stop].Name] {
			stop++
		}
		if stop-start > 1 {
			batch := moduTUIToolBatchMetadata{
				size: stop - start,
				id:   "batch-" + calls[start].ID,
			}
			for _, call := range calls[start:stop] {
				batches[call.ID] = batch
			}
		}
		start = stop
	}

	content := append([]types.ContentBlock(nil), message.Content...)
	for i, block := range content {
		call, ok := block.(*types.ToolCallContent)
		if !ok || call == nil {
			continue
		}
		batch, ok := batches[call.ID]
		if !ok {
			continue
		}
		restored := *call
		restored.BatchSize = batch.size
		restored.BatchID = batch.id
		content[i] = &restored
	}
	message.Content = content
	return message
}
