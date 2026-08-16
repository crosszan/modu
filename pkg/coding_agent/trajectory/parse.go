package trajectory

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// scanBufferBytes matches the session manager's own limit so a line it could
// write is a line we can read.
const scanBufferBytes = 10 * 1024 * 1024

// sidecarSuffixes gate out entries that are appended without moving the
// conversational leaf (runtime and plan snapshots). They can never sit on the
// root-to-leaf path, and they dominate a long session by volume — in a
// long-running session they are the large majority of all lines — so we match
// the line edge and skip them instead of decoding their payloads.
//
// SessionEntry marshals as a map with sorted keys, which puts "type" last on
// every entry line; see session.SessionEntry.MarshalJSON. The header line is a
// struct marshal and instead starts with its type.
var (
	sidecarSuffixes = [][]byte{
		[]byte(`"type":"runtime_state"}`),
		[]byte(`"type":"plan_snapshot"}`),
	}
	promptSuffix = []byte(`"type":"prompt_snapshot"}`)
	headerPrefix = []byte(`{"type":"session",`)
)

// header is the first line of a session file.
type header struct {
	Type      string `json:"type"`
	Version   int    `json:"version"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Cwd       string `json:"cwd"`
}

// entry is one decoded non-sidecar session line.
type entry struct {
	ID       string
	ParentID string
	Type     string
	TimeMs   int64

	Message  *wireMessage
	Name     string // session_info
	Provider string // model_change
	ModelID  string // model_change
	Prompt   *wirePrompt

	// Compaction fields.
	OriginalCount int
	NewCount      int
	TokensBefore  int
}

// wirePrompt is a persisted prompt snapshot: the system prompt and tool
// catalog in force at that point, with what changed since the previous one.
type wirePrompt struct {
	System string           `json:"system"`
	Tools  []wirePromptTool `json:"tools"`
	Change string           `json:"change"`
}

type wirePromptTool struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Schema      string `json:"schema"`
}

type wireMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCallID string          `json:"toolCallId"`
	ToolName   string          `json:"toolName"`
	IsError    bool            `json:"isError"`
	Model      string          `json:"model"`
	Provider   string          `json:"provider"`
	Usage      wireUsage       `json:"usage"`
	Timing     *wireTiming     `json:"timing"`
}

// wireTiming is the model call's recorded clock, present on sessions written
// after timing was persisted. Older sessions have none, and the projection
// falls back to deriving the start from the previous event.
type wireTiming struct {
	RequestStartMs int64 `json:"requestStartMs"`
	FirstTokenMs   int64 `json:"firstTokenMs"`
	CompletedMs    int64 `json:"completedMs"`
}

type wireUsage struct {
	Input       int `json:"input"`
	Output      int `json:"output"`
	CacheRead   int `json:"cacheRead"`
	CacheWrite  int `json:"cacheWrite"`
	TotalTokens int `json:"totalTokens"`
	Cost        struct {
		Total float64 `json:"total"`
	} `json:"cost"`
}

func (u wireUsage) toUsage() Usage {
	return Usage{
		Input:      u.Input,
		Output:     u.Output,
		CacheRead:  u.CacheRead,
		CacheWrite: u.CacheWrite,
		Cost:       u.Cost.Total,
	}
}

func (u wireUsage) empty() bool {
	return u.Input == 0 && u.Output == 0 && u.TotalTokens == 0 && u.CacheRead == 0 && u.CacheWrite == 0
}

type wireBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	MimeType  string          `json:"mimeType"`
}

// readSession streams one session file and returns its header plus the
// entries on the current root-to-leaf branch, in causal order.
func readSession(path string) (header, []entry, []entry, []Warning, error) {
	file, err := os.Open(path)
	if err != nil {
		return header{}, nil, nil, nil, err
	}
	defer file.Close()

	var (
		head     header
		entries  []entry
		prompts  []entry
		warnings []Warning
	)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), scanBufferBytes)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 || isSidecar(line) {
			continue
		}
		if head.ID == "" && bytes.HasPrefix(line, headerPrefix) {
			if err := json.Unmarshal(line, &head); err != nil {
				warnings = append(warnings, Warning{Line: lineNo, Message: "unreadable session header: " + err.Error()})
			}
			continue
		}
		decoded, err := decodeEntry(line)
		if err != nil {
			warnings = append(warnings, Warning{Line: lineNo, Message: err.Error()})
			continue
		}
		// A prompt snapshot is a sidecar too: it must not enter the branch
		// walk, but unlike runtime state it belongs in the ledger, merged back
		// by timestamp.
		if bytes.HasSuffix(line, promptSuffix) {
			prompts = append(prompts, decoded)
			continue
		}
		entries = append(entries, decoded)
	}
	if err := scanner.Err(); err != nil {
		return head, nil, nil, warnings, fmt.Errorf("read session: %w", err)
	}
	return head, currentBranch(entries), prompts, warnings, nil
}

func isSidecar(line []byte) bool {
	for _, suffix := range sidecarSuffixes {
		if bytes.HasSuffix(line, suffix) {
			return true
		}
	}
	return false
}

// currentBranch walks parent links back from the last entry, mirroring how the
// session manager resolves its leaf: entries are appended in causal order, and
// every entry that can move the leaf is retained here, so the last one read is
// the leaf. A session that has been forked keeps abandoned branches in the same
// file; walking the parent chain is what excludes them.
func currentBranch(entries []entry) []entry {
	if len(entries) == 0 {
		return nil
	}
	byID := make(map[string]entry, len(entries))
	for _, e := range entries {
		byID[e.ID] = e
	}
	var path []entry
	seen := make(map[string]bool, len(entries))
	current, ok := entries[len(entries)-1], true
	for ok && !seen[current.ID] {
		seen[current.ID] = true
		path = append(path, current)
		current, ok = byID[current.ParentID]
	}
	for left, right := 0, len(path)-1; left < right; left, right = left+1, right-1 {
		path[left], path[right] = path[right], path[left]
	}
	return path
}

func decodeEntry(line []byte) (entry, error) {
	var wire struct {
		ID        string          `json:"id"`
		ParentID  *string         `json:"parentId"`
		Type      string          `json:"type"`
		Timestamp json.RawMessage `json:"timestamp"`
		Message   *wireMessage    `json:"message"`
		Name      string          `json:"name"`
		Provider  string          `json:"provider"`
		ModelID   string          `json:"modelId"`
		Prompt    *wirePrompt     `json:"prompt"`

		OriginalCount int `json:"originalCount"`
		NewCount      int `json:"newCount"`
		TokensBefore  int `json:"tokensBefore"`
	}
	if err := json.Unmarshal(line, &wire); err != nil {
		return entry{}, fmt.Errorf("malformed session entry: %w", err)
	}
	result := entry{
		ID:            wire.ID,
		Type:          wire.Type,
		TimeMs:        parseTimestamp(wire.Timestamp),
		Message:       wire.Message,
		Name:          wire.Name,
		Provider:      wire.Provider,
		ModelID:       wire.ModelID,
		Prompt:        wire.Prompt,
		OriginalCount: wire.OriginalCount,
		NewCount:      wire.NewCount,
		TokensBefore:  wire.TokensBefore,
	}
	if wire.ParentID != nil {
		result.ParentID = *wire.ParentID
	}
	return result, nil
}

// parseTimestamp accepts both shapes the session format has used: an RFC3339
// string (what entries are written as today) and epoch milliseconds.
func parseTimestamp(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil && text != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, text); err == nil {
			return parsed.UnixMilli()
		}
		return 0
	}
	var number int64
	_ = json.Unmarshal(raw, &number)
	return number
}

// blocks decodes a message's content blocks. User content is sometimes a bare
// string rather than a block list.
func (m *wireMessage) blocks() []wireBlock {
	if m == nil || len(m.Content) == 0 {
		return nil
	}
	var list []wireBlock
	if err := json.Unmarshal(m.Content, &list); err == nil {
		return list
	}
	var text string
	if err := json.Unmarshal(m.Content, &text); err == nil {
		return []wireBlock{{Type: "text", Text: text}}
	}
	return nil
}

// text joins the readable text of a message's content blocks.
func (m *wireMessage) text() string {
	var builder []byte
	for _, block := range m.blocks() {
		switch block.Type {
		case "text":
			builder = appendLine(builder, block.Text)
		case "thinking":
			builder = appendLine(builder, block.Thinking)
		case "image":
			builder = appendLine(builder, "[image: "+block.MimeType+"]")
		}
	}
	return string(builder)
}

func appendLine(builder []byte, text string) []byte {
	if text == "" {
		return builder
	}
	if len(builder) > 0 {
		builder = append(builder, '\n')
	}
	return append(builder, text...)
}

func isoTime(milliseconds int64) string {
	if milliseconds <= 0 {
		return ""
	}
	return time.UnixMilli(milliseconds).UTC().Format(time.RFC3339Nano)
}
