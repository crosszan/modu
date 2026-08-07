package memory

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	memsvc "github.com/openmodu/modu/pkg/coding_agent/services/memory"
	"github.com/openmodu/modu/pkg/types"
)

// MemoryStore defines the interface for the backend storage used by the memory tool.
type MemoryStore interface {
	ReadLongTerm() string
	WriteLongTerm(content string) error
	ReadGlobalLongTerm() string
	WriteGlobalLongTerm(content string) error
	WriteProjectSummary(content string) error
	WriteGlobalSummary(content string) error
	AppendToday(content string) error
	List(scope, path string, maxResults int) ([]memsvc.Entry, bool, error)
	Read(scope, path string, lineOffset, maxLines int) (string, bool, error)
	Search(scope, query, path string, contextLines, maxResults int) ([]memsvc.SearchMatch, bool, error)
	LongTermEntries(scope string) []string
	UpdateLongTerm(scope, match, content string) error
	DeleteLongTerm(scope, match string) error
}

// MemoryTool allows the model to write data to long-term memory or daily notes.
type MemoryTool struct {
	store MemoryStore
}

// NewMemoryTool creates a new memory tool instance.
func NewMemoryTool(store MemoryStore) types.Tool {
	return &MemoryTool{store: store}
}

func (t *MemoryTool) Name() string {
	return "memo"
}

func (t *MemoryTool) Label() string {
	return "Memory"
}

func (t *MemoryTool) Description() string {
	return `Read and write persistent memory.
Use this tool proactively to record architectural choices, project rules, or recurring tasks
so that you can remember them across server restarts and context compactions. Use list/read/search
to inspect detailed memory when the prompt only includes a summary.

Memory is meant to stay correct, not just grow: when you learn that something
you recorded is wrong or no longer true, fix or remove that entry instead of
appending a contradiction and leaving both for a reader to reconcile.

Operations:
- 'record_long_term': Appends critical facts to MEMORY.md. Use scope 'global' for cross-project facts (user preferences, personal rules) or 'project' (default) for project-specific facts.
- 'update_long_term': Replaces one existing entry. Give 'match' (text that appears in exactly one entry) and the corrected 'content'.
- 'forget_long_term': Removes one existing entry, identified the same way by 'match'. Use it when a fact stopped being true, not merely because it is old.
- 'record_daily': Appends a scratchpad note or daily log to today's date (project scope).
- 'write_summary': Overwrites memory_summary.md for the selected scope with a concise bounded summary of important memory.
- 'list': Lists memory files under the selected scope.
- 'read': Reads a memory file by relative path.
- 'search': Searches memory files for a substring.`
}

func (t *MemoryTool) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type":        "string",
				"description": "Must be 'record_long_term', 'update_long_term', 'forget_long_term', 'record_daily', 'write_summary', 'list', 'read', or 'search'",
				"enum":        []string{"record_long_term", "update_long_term", "forget_long_term", "record_daily", "write_summary", "list", "read", "search"},
			},
			"content": map[string]any{
				"type":        "string",
				"description": "The markdown content to remember for record or write_summary operations.",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Relative memory path for list/read/search. Omit or use empty string for the memory root.",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "Substring query for search.",
			},
			"match": map[string]any{
				"type":        "string",
				"description": "Text identifying which long-term entry to update or forget. Must appear in exactly one entry; add surrounding words if it is ambiguous.",
			},
			"line_offset": map[string]any{
				"type":        "integer",
				"description": "1-indexed starting line for read.",
			},
			"max_lines": map[string]any{
				"type":        "integer",
				"description": "Maximum lines returned for read.",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Maximum entries or matches returned for list/search.",
			},
			"context_lines": map[string]any{
				"type":        "integer",
				"description": "Surrounding lines returned with each search match.",
			},
			"scope": map[string]any{
				"type":        "string",
				"description": "Memory scope: 'project' (default, current project only) or 'global' (shared across projects).",
				"enum":        []string{"project", "global", "user"},
			},
		},
		"required": []string{"operation"},
	}
}

func (t *MemoryTool) Execute(ctx context.Context, toolCallID string, args map[string]any, onUpdate types.ToolUpdateCallback) (types.ToolResult, error) {
	if t.store == nil {
		return textResult("Error: memory store is not configured for this session"), nil
	}

	operation, _ := args["operation"].(string)
	content, _ := args["content"].(string)
	scope, _ := args["scope"].(string)
	if scope == "" {
		scope = "project"
	}

	switch operation {
	case "record_long_term":
		if content == "" {
			return textResult("Error: content cannot be empty"), nil
		}
		var (
			existing string
			writeErr error
		)
		if isGlobalScope(scope) {
			existing = t.store.ReadGlobalLongTerm()
		} else {
			existing = t.store.ReadLongTerm()
		}
		var newContent string
		if existing == "" {
			newContent = content
		} else {
			newContent = existing + "\n\n" + content
		}
		if isGlobalScope(scope) {
			writeErr = t.store.WriteGlobalLongTerm(newContent)
		} else {
			writeErr = t.store.WriteLongTerm(newContent)
		}
		if writeErr != nil {
			return textResult(fmt.Sprintf("Failed to write to long-term memory: %v", writeErr)), nil
		}
		return textResult(fmt.Sprintf("Successfully recorded to %s long-term memory.", scope)), nil

	case "update_long_term", "forget_long_term":
		match, _ := args["match"].(string)
		if strings.TrimSpace(match) == "" {
			return textResult("Error: match is required and identifies which entry to change"), nil
		}
		forget := operation == "forget_long_term"
		if !forget && content == "" {
			return textResult("Error: content cannot be empty"), nil
		}

		var err error
		if forget {
			err = t.store.DeleteLongTerm(scope, match)
		} else {
			err = t.store.UpdateLongTerm(scope, match, content)
		}
		switch {
		case errors.Is(err, memsvc.ErrMemoryEntryNotFound):
			// Report what is actually there: the model is working from a
			// remembered snapshot and the file may have moved on.
			return textResult(fmt.Sprintf(
				"No %s long-term entry contains %q. Current entries:\n%s",
				scope, match, formatEntryList(t.store.LongTermEntries(scope)))), nil
		case errors.Is(err, memsvc.ErrMemoryEntryAmbiguous):
			return textResult(fmt.Sprintf(
				"%v for %q — nothing was changed. Add surrounding words so it identifies one entry.", err, match)), nil
		case err != nil:
			return textResult(fmt.Sprintf("Failed to change long-term memory: %v", err)), nil
		}
		if forget {
			return textResult(fmt.Sprintf("Removed the matching entry from %s long-term memory.", scope)), nil
		}
		return textResult(fmt.Sprintf("Updated the matching entry in %s long-term memory.", scope)), nil

	case "record_daily":
		if content == "" {
			return textResult("Error: content cannot be empty"), nil
		}
		err := t.store.AppendToday(content)
		if err != nil {
			return textResult(fmt.Sprintf("Failed to append to daily notes: %v", err)), nil
		}
		return textResult("Successfully appended to today's daily notes."), nil

	case "write_summary":
		if content == "" {
			return textResult("Error: content cannot be empty"), nil
		}
		var err error
		if isGlobalScope(scope) {
			err = t.store.WriteGlobalSummary(content)
		} else {
			err = t.store.WriteProjectSummary(content)
		}
		if err != nil {
			return textResult(fmt.Sprintf("Failed to write memory summary: %v", err)), nil
		}
		return textResult(fmt.Sprintf("Successfully wrote %s memory summary.", scope)), nil

	case "list":
		path, _ := args["path"].(string)
		entries, truncated, err := t.store.List(scope, path, intArg(args, "max_results"))
		if err != nil {
			return textResult(fmt.Sprintf("Failed to list memory: %v", err)), nil
		}
		return memoryResult(formatList(entries, truncated), map[string]any{
			"operation": "list",
			"scope":     normalizedScope(scope),
			"path":      path,
			"entries":   entries,
			"truncated": truncated,
		}), nil

	case "read":
		path, _ := args["path"].(string)
		if strings.TrimSpace(path) == "" {
			return textResult("Error: path is required for read"), nil
		}
		content, truncated, err := t.store.Read(scope, path, intArg(args, "line_offset"), intArg(args, "max_lines"))
		if err != nil {
			return textResult(fmt.Sprintf("Failed to read memory: %v", err)), nil
		}
		if truncated {
			content += "\n\n...[truncated]"
		}
		return memoryResult(content, map[string]any{
			"operation":   "read",
			"scope":       normalizedScope(scope),
			"path":        path,
			"line_offset": intArg(args, "line_offset"),
			"max_lines":   intArg(args, "max_lines"),
			"content":     content,
			"truncated":   truncated,
		}), nil

	case "search":
		query, _ := args["query"].(string)
		path, _ := args["path"].(string)
		matches, truncated, err := t.store.Search(scope, query, path, intArg(args, "context_lines"), intArg(args, "max_results"))
		if err != nil {
			return textResult(fmt.Sprintf("Failed to search memory: %v", err)), nil
		}
		return memoryResult(formatSearch(matches, truncated), map[string]any{
			"operation": "search",
			"scope":     normalizedScope(scope),
			"path":      path,
			"query":     query,
			"matches":   matches,
			"truncated": truncated,
		}), nil

	default:
		return textResult(fmt.Sprintf("Unknown operation: %s", operation)), nil
	}
}

func intArg(args map[string]any, name string) int {
	switch v := args[name].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return n
	default:
		return 0
	}
}

func formatList(entries []memsvc.Entry, truncated bool) string {
	if len(entries) == 0 {
		return "No memory entries found."
	}
	var lines []string
	for _, entry := range entries {
		suffix := ""
		if entry.IsDir {
			suffix = "/"
		}
		lines = append(lines, "- "+entry.Path+suffix)
	}
	if truncated {
		lines = append(lines, "...[truncated]")
	}
	return strings.Join(lines, "\n")
}

func formatSearch(matches []memsvc.SearchMatch, truncated bool) string {
	if len(matches) == 0 {
		return "No memory matches found."
	}
	var lines []string
	for _, match := range matches {
		lines = append(lines, fmt.Sprintf("## %s:%d\n%s", match.Path, match.Line, match.Content))
	}
	if truncated {
		lines = append(lines, "...[truncated]")
	}
	return strings.Join(lines, "\n")
}

func isGlobalScope(scope string) bool {
	return strings.EqualFold(scope, "global") || strings.EqualFold(scope, "user")
}

func normalizedScope(scope string) string {
	if isGlobalScope(scope) {
		return "global"
	}
	return "project"
}

// textResult creates a simple text ToolResult.
func textResult(text string) types.ToolResult {
	return memoryResult(text, map[string]any{"result": text})
}

func memoryResult(text string, details map[string]any) types.ToolResult {
	if details == nil {
		details = map[string]any{}
	}
	details["result"] = text
	return types.ToolResult{
		Content: []types.ContentBlock{&types.TextContent{Type: "text", Text: text}},
		Details: details,
	}
}

// formatEntryList renders the current entries compactly so a failed match can
// be retried against what is actually stored, rather than guessing.
func formatEntryList(entries []string) string {
	if len(entries) == 0 {
		return "(none)"
	}
	var b strings.Builder
	for i, entry := range entries {
		first := entry
		if idx := strings.IndexByte(first, '\n'); idx >= 0 {
			first = first[:idx]
		}
		if len([]rune(first)) > 100 {
			first = string([]rune(first)[:100]) + "…"
		}
		fmt.Fprintf(&b, "%d. %s\n", i+1, first)
	}
	return strings.TrimRight(b.String(), "\n")
}
