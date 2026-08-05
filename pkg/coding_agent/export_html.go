package coding_agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openmodu/modu/pkg/coding_agent/services/session"
	"github.com/openmodu/modu/pkg/types"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

var exportHTMLMarkdown = goldmark.New(goldmark.WithExtensions(extension.GFM))

type exportHTMLToolResult struct {
	message   types.ToolResultMessage
	timestamp int64
}

type exportHTMLRenderState struct {
	toolCalls   map[string]struct{}
	toolResults map[string]exportHTMLToolResult
}

// ExportHTML writes the complete persisted session transcript as HTML.
func (s *CodingSession) ExportHTML(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var buf bytes.Buffer
	writeExportHTMLHeader(&buf)

	transcript := s.GetSessionTranscript()
	if len(transcript) == 0 {
		for _, msg := range s.agent.GetState().Messages {
			transcript = append(transcript, SessionTranscriptEntry{
				Type:      string(session.EntryTypeMessage),
				Message:   msg,
				Timestamp: agentMessageTimestamp(msg),
			})
		}
	}

	renderState := newExportHTMLRenderState(transcript)
	for _, entry := range transcript {
		switch entry.Type {
		case string(session.EntryTypeMessage):
			writeExportHTMLMessage(&buf, entry.Message, entry.Timestamp, renderState)
		case string(session.EntryTypeCompaction):
			writeExportHTMLCompaction(&buf, entry.Timestamp)
		}
	}

	buf.WriteString("</main></body></html>\n")
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func writeExportHTMLHeader(buf *bytes.Buffer) {
	buf.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Session Export</title>
<style>
:root { color-scheme: light dark; font-family: ui-sans-serif, system-ui, sans-serif; }
body { margin: 0; background: Canvas; color: CanvasText; }
main { box-sizing: border-box; max-width: 960px; margin: 0 auto; padding: 32px 20px 64px; }
h1 { margin: 0 0 24px; font-size: 24px; }
.message { margin: 0 0 16px; padding: 14px 16px; border: 1px solid color-mix(in srgb, CanvasText 18%, transparent); border-radius: 10px; }
.message > header { display: flex; align-items: baseline; justify-content: space-between; gap: 16px; margin-bottom: 10px; }
.message.user { border-left: 4px solid #2563eb; }
.message.assistant { border-left: 4px solid #16a34a; }
.message.tool { border-left: 4px solid #d97706; }
.message.tool > details { margin-top: 0; }
time { color: color-mix(in srgb, CanvasText 58%, transparent); font-size: 12px; white-space: nowrap; }
pre.raw { margin: 0; overflow-wrap: anywhere; white-space: pre-wrap; font: inherit; line-height: 1.55; }
details { margin-top: 10px; padding: 8px 10px; border-radius: 7px; background: color-mix(in srgb, CanvasText 6%, transparent); }
summary { cursor: pointer; font-weight: 600; overflow-wrap: anywhere; }
details pre.raw { font-family: ui-monospace, SFMono-Regular, Consolas, monospace; font-size: 13px; }
.tool-section { margin-top: 12px; }
.tool-section + .tool-section { padding-top: 12px; border-top: 1px solid color-mix(in srgb, CanvasText 14%, transparent); }
.tool-section > header { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; margin-bottom: 7px; }
.markdown { line-height: 1.6; overflow-wrap: anywhere; }
.markdown > :first-child { margin-top: 0; }
.markdown > :last-child { margin-bottom: 0; }
.markdown h1, .markdown h2, .markdown h3 { margin: 1em 0 .45em; line-height: 1.3; }
.markdown h1 { font-size: 1.45em; }
.markdown h2 { font-size: 1.25em; }
.markdown h3 { font-size: 1.1em; }
.markdown p, .markdown ul, .markdown ol, .markdown blockquote { margin: .65em 0; }
.markdown code { padding: .12em .3em; border-radius: 4px; background: color-mix(in srgb, CanvasText 8%, transparent); font-family: ui-monospace, SFMono-Regular, Consolas, monospace; font-size: .92em; }
.markdown pre { margin: .7em 0; padding: 10px 12px; overflow-x: auto; border-radius: 7px; background: color-mix(in srgb, CanvasText 7%, transparent); }
.markdown pre code { padding: 0; background: transparent; }
.markdown blockquote { padding-left: 12px; border-left: 3px solid color-mix(in srgb, CanvasText 22%, transparent); color: color-mix(in srgb, CanvasText 72%, transparent); }
.markdown table { display: block; max-width: 100%; overflow-x: auto; border-collapse: collapse; }
.markdown th, .markdown td { padding: 6px 10px; border: 1px solid color-mix(in srgb, CanvasText 18%, transparent); }
.error { color: #dc2626; }
.compaction { display: flex; align-items: center; gap: 12px; margin: 24px 0; color: color-mix(in srgb, CanvasText 58%, transparent); font-size: 13px; }
.compaction::before, .compaction::after { content: ""; flex: 1; border-top: 1px solid color-mix(in srgb, CanvasText 18%, transparent); }
</style>
</head>
<body><main>
<h1>Session Export</h1>
`)
}

func newExportHTMLRenderState(transcript []SessionTranscriptEntry) exportHTMLRenderState {
	state := exportHTMLRenderState{
		toolCalls:   make(map[string]struct{}),
		toolResults: make(map[string]exportHTMLToolResult),
	}
	for _, entry := range transcript {
		switch message := entry.Message.(type) {
		case types.AssistantMessage:
			collectExportHTMLToolCalls(state.toolCalls, message.Content)
		case *types.AssistantMessage:
			if message != nil {
				collectExportHTMLToolCalls(state.toolCalls, message.Content)
			}
		case types.ToolResultMessage:
			if message.ToolCallID != "" {
				state.toolResults[message.ToolCallID] = exportHTMLToolResult{message: message, timestamp: entry.Timestamp}
			}
		case *types.ToolResultMessage:
			if message != nil && message.ToolCallID != "" {
				state.toolResults[message.ToolCallID] = exportHTMLToolResult{message: *message, timestamp: entry.Timestamp}
			}
		}
	}
	return state
}

func collectExportHTMLToolCalls(calls map[string]struct{}, blocks []types.ContentBlock) {
	for _, block := range blocks {
		if call, ok := block.(*types.ToolCallContent); ok && call != nil && call.ID != "" {
			calls[call.ID] = struct{}{}
		}
	}
}

func writeExportHTMLMessage(buf *bytes.Buffer, msg types.AgentMessage, timestamp int64, state exportHTMLRenderState) {
	switch m := msg.(type) {
	case types.UserMessage:
		writeExportHTMLUser(buf, m.Content, timestamp)
	case *types.UserMessage:
		if m != nil {
			writeExportHTMLUser(buf, m.Content, timestamp)
		}
	case types.AssistantMessage:
		writeExportHTMLAssistant(buf, m.Content, timestamp, state)
	case *types.AssistantMessage:
		if m != nil {
			writeExportHTMLAssistant(buf, m.Content, timestamp, state)
		}
	case types.ToolResultMessage:
		if _, paired := state.toolCalls[m.ToolCallID]; !paired {
			writeExportHTMLToolResult(buf, m, timestamp)
		}
	case *types.ToolResultMessage:
		if m != nil {
			if _, paired := state.toolCalls[m.ToolCallID]; !paired {
				writeExportHTMLToolResult(buf, *m, timestamp)
			}
		}
	}
}

func writeExportHTMLUser(buf *bytes.Buffer, content any, timestamp int64) {
	writeExportHTMLMessageStart(buf, "user", "User", timestamp)
	writeExportHTMLMarkdown(buf, exportHTMLUserText(content))
	buf.WriteString("</article>\n")
}

func writeExportHTMLAssistant(buf *bytes.Buffer, blocks []types.ContentBlock, timestamp int64, state exportHTMLRenderState) {
	assistantOpen := false
	closeAssistant := func() {
		if assistantOpen {
			buf.WriteString("</article>\n")
			assistantOpen = false
		}
	}
	for _, block := range blocks {
		switch value := block.(type) {
		case *types.TextContent:
			if value != nil && value.Text != "" {
				if !assistantOpen {
					writeExportHTMLMessageStart(buf, "assistant", "Assistant", timestamp)
					assistantOpen = true
				}
				writeExportHTMLMarkdown(buf, value.Text)
			}
		case *types.ToolCallContent:
			if value != nil {
				closeAssistant()
				result, hasResult := state.toolResults[value.ID]
				writeExportHTMLToolCall(buf, value, timestamp, result, hasResult)
			}
		}
	}
	closeAssistant()
}

func writeExportHTMLToolCall(buf *bytes.Buffer, call *types.ToolCallContent, timestamp int64, result exportHTMLToolResult, hasResult bool) {
	writeExportHTMLMessageStart(buf, "tool", "Tool", timestamp)
	buf.WriteString("<details><summary>")
	buf.WriteString(html.EscapeString(call.Name))
	if hasResult && result.message.IsError {
		buf.WriteString(" <span class=\"error\">Failed</span>")
	}
	buf.WriteString("</summary><section class=\"tool-section\"><header><strong>Input</strong>")
	writeExportHTMLTime(buf, timestamp)
	buf.WriteString("</header>")
	writeExportHTMLPre(buf, exportHTMLJSON(call.Arguments))
	buf.WriteString("</section>")
	if hasResult {
		buf.WriteString("<section class=\"tool-section\"><header><strong>Output</strong>")
		writeExportHTMLTime(buf, result.timestamp)
		buf.WriteString("</header>")
		output := exportHTMLBlocksText(result.message.Content)
		if output == "" && result.message.Details != nil {
			output = exportHTMLJSON(result.message.Details)
		}
		writeExportHTMLPre(buf, output)
		buf.WriteString("</section>")
	}
	buf.WriteString("</details></article>\n")
}

func writeExportHTMLToolResult(buf *bytes.Buffer, result types.ToolResultMessage, timestamp int64) {
	writeExportHTMLMessageStart(buf, "tool", "Tool", timestamp)
	buf.WriteString("<details><summary>Tool result · ")
	buf.WriteString(html.EscapeString(result.ToolName))
	if result.IsError {
		buf.WriteString(" <span class=\"error\">Failed</span>")
	}
	buf.WriteString("</summary>")
	writeExportHTMLPre(buf, exportHTMLBlocksText(result.Content))
	buf.WriteString("</details></article>\n")
}

func writeExportHTMLCompaction(buf *bytes.Buffer, timestamp int64) {
	buf.WriteString("<div class=\"compaction\"><span>Context compacted")
	if timestamp > 0 {
		buf.WriteString(" · ")
		buf.WriteString(exportHTMLTime(timestamp))
	}
	buf.WriteString("</span></div>\n")
}

func writeExportHTMLMessageStart(buf *bytes.Buffer, class, label string, timestamp int64) {
	fmt.Fprintf(buf, "<article class=\"message %s\"><header><strong>%s</strong>", class, label)
	if timestamp > 0 {
		t := time.UnixMilli(timestamp)
		fmt.Fprintf(buf, "<time datetime=\"%s\">%s</time>",
			t.UTC().Format(time.RFC3339Nano), exportHTMLTime(timestamp))
	}
	buf.WriteString("</header>")
}

func writeExportHTMLPre(buf *bytes.Buffer, content string) {
	buf.WriteString("<pre class=\"raw\">")
	buf.WriteString(html.EscapeString(content))
	buf.WriteString("</pre>")
}

func writeExportHTMLMarkdown(buf *bytes.Buffer, content string) {
	var rendered bytes.Buffer
	if err := exportHTMLMarkdown.Convert([]byte(content), &rendered); err != nil {
		writeExportHTMLPre(buf, content)
		return
	}
	buf.WriteString("<div class=\"markdown\">")
	buf.Write(rendered.Bytes())
	buf.WriteString("</div>")
}

func exportHTMLUserText(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case []types.ContentBlock:
		return exportHTMLBlocksText(value)
	default:
		return exportHTMLJSON(value)
	}
}

func exportHTMLBlocksText(blocks []types.ContentBlock) string {
	var parts []string
	for _, block := range blocks {
		switch value := block.(type) {
		case *types.TextContent:
			if value != nil && value.Text != "" {
				parts = append(parts, value.Text)
			}
		case *types.ImageContent:
			if value != nil {
				parts = append(parts, "[Image: "+value.MimeType+"]")
			}
		}
	}
	return strings.Join(parts, "\n")
}

func exportHTMLJSON(value any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}

func exportHTMLTime(timestamp int64) string {
	return time.UnixMilli(timestamp).Local().Format("2006-01-02 15:04:05")
}

func writeExportHTMLTime(buf *bytes.Buffer, timestamp int64) {
	if timestamp <= 0 {
		return
	}
	t := time.UnixMilli(timestamp)
	fmt.Fprintf(buf, "<time datetime=\"%s\">%s</time>",
		t.UTC().Format(time.RFC3339Nano), exportHTMLTime(timestamp))
}

func agentMessageTimestamp(msg types.AgentMessage) int64 {
	switch value := msg.(type) {
	case types.UserMessage:
		return value.Timestamp
	case *types.UserMessage:
		if value != nil {
			return value.Timestamp
		}
	case types.AssistantMessage:
		return value.Timestamp
	case *types.AssistantMessage:
		if value != nil {
			return value.Timestamp
		}
	case types.ToolResultMessage:
		return value.Timestamp
	case *types.ToolResultMessage:
		if value != nil {
			return value.Timestamp
		}
	}
	return 0
}
