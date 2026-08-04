package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openmodu/modu/pkg/providers"
	"github.com/openmodu/modu/pkg/types"
)

func TestResponsesBuildBodyUsesItemsAndStatelessReasoning(t *testing.T) {
	replayed := json.RawMessage(`[
		{"id":"rs_1","type":"reasoning","encrypted_content":"encrypted","summary":[]},
		{"id":"fc_1","type":"function_call","call_id":"call_1","name":"weather","arguments":"{\"city\":\"上海\"}","status":"completed"}
	]`)
	temperature := 0.2
	maxTokens := 2048
	p := &responsesProvider{id: "openai", config: Config{}}

	body, err := p.buildBody(&providers.ChatRequest{
		Model:       "gpt-5",
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		Reasoning:   types.ThinkingLevelHigh,
		Messages: []providers.Message{
			{Role: providers.RoleSystem, Content: "Be concise."},
			{
				Role: providers.RoleUser,
				Content: []any{
					map[string]any{"type": "text", "text": "What is shown?"},
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,AAAA"}},
				},
			},
			{
				Role: providers.RoleAssistant,
				ToolCalls: []providers.ToolCall{{
					ID:       "call_1",
					Type:     "function",
					Function: providers.FuncCall{Name: "weather", Arguments: `{"city":"上海"}`},
				}},
				ProviderMetadata: map[string]json.RawMessage{responsesOutputMetadataKey: replayed},
			},
			{Role: providers.RoleTool, ToolCallID: "call_1", Content: "晴，28°C"},
		},
		Tools: []providers.Tool{{
			Type: "function",
			Function: providers.FuncDef{
				Name:        "weather",
				Description: "Get weather",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{"city": map[string]any{"type": "string"}},
				},
			},
		}},
	}, true)
	if err != nil {
		t.Fatalf("buildBody: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload["model"] != "gpt-5" || payload["instructions"] != "Be concise." {
		t.Fatalf("unexpected model/instructions: %#v", payload)
	}
	if payload["stream"] != true || payload["store"] != false {
		t.Fatalf("expected stream=true and store=false, got %#v", payload)
	}
	if payload["max_output_tokens"] != float64(maxTokens) || payload["temperature"] != temperature {
		t.Fatalf("unexpected generation settings: %#v", payload)
	}

	reasoning, ok := payload["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "high" || reasoning["summary"] != "auto" {
		t.Fatalf("unexpected reasoning settings: %#v", payload["reasoning"])
	}
	include, ok := payload["include"].([]any)
	if !ok || len(include) != 1 || include[0] != "reasoning.encrypted_content" {
		t.Fatalf("unexpected include: %#v", payload["include"])
	}

	tools, ok := payload["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("unexpected tools: %#v", payload["tools"])
	}
	tool := tools[0].(map[string]any)
	if tool["type"] != "function" || tool["name"] != "weather" || tool["description"] != "Get weather" {
		t.Fatalf("tool was not flattened for Responses: %#v", tool)
	}
	if _, nested := tool["function"]; nested {
		t.Fatalf("Responses tool must not contain a nested function object: %#v", tool)
	}

	input, ok := payload["input"].([]any)
	if !ok || len(input) != 4 {
		t.Fatalf("expected user + two replayed items + tool output, got %#v", payload["input"])
	}
	user := input[0].(map[string]any)
	content := user["content"].([]any)
	if content[0].(map[string]any)["type"] != "input_text" {
		t.Fatalf("text input was not converted: %#v", content)
	}
	image := content[1].(map[string]any)
	if image["type"] != "input_image" || image["image_url"] != "data:image/png;base64,AAAA" {
		t.Fatalf("image input was not converted: %#v", image)
	}
	if input[1].(map[string]any)["type"] != "reasoning" || input[2].(map[string]any)["type"] != "function_call" {
		t.Fatalf("provider output items were not replayed: %#v", input)
	}
	result := input[3].(map[string]any)
	if result["type"] != "function_call_output" || result["call_id"] != "call_1" || result["output"] != "晴，28°C" {
		t.Fatalf("tool result was not converted: %#v", result)
	}
}

func TestResponsesStreamMapsTypedEventsAndPersistsOutputItems(t *testing.T) {
	output := `[{"id":"rs_1","type":"reasoning","encrypted_content":"encrypted","summary":[{"type":"summary_text","text":"Check the tool."}]},{"id":"msg_1","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Checking.","annotations":[]}]},{"id":"fc_1","type":"function_call","call_id":"call_1","name":"weather","arguments":"{\"city\":\"上海\"}","status":"completed"}]`
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("unexpected authorization: %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"type":"response.created","response":{"model":"gpt-5"}}

data: {"type":"response.reasoning_summary_text.delta","output_index":0,"summary_index":0,"delta":"Check "}

data: {"type":"response.reasoning_summary_text.delta","output_index":0,"summary_index":0,"delta":"the tool."}

data: {"type":"response.output_text.delta","output_index":1,"content_index":0,"delta":"Checking."}

data: {"type":"response.output_item.added","output_index":2,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"weather","arguments":"","status":"in_progress"}}

data: {"type":"response.function_call_arguments.delta","output_index":2,"delta":"{\"city\":"}

data: {"type":"response.function_call_arguments.delta","output_index":2,"delta":"\"上海\"}"}

data: {"type":"response.output_item.done","output_index":0,"item":{"id":"rs_1","type":"reasoning","encrypted_content":"encrypted","summary":[{"type":"summary_text","text":"Check the tool."}]}}

data: {"type":"response.output_item.done","output_index":1,"item":{"id":"msg_1","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Checking.","annotations":[]}]}}

data: {"type":"response.output_item.done","output_index":2,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"weather","arguments":"{\"city\":\"上海\"}","status":"completed"}}

data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5","status":"completed","output":`+output+`,"usage":{"input_tokens":100,"input_tokens_details":{"cached_tokens":40},"output_tokens":20,"output_tokens_details":{"reasoning_tokens":8},"total_tokens":120}}}

`)
	}))
	defer server.Close()

	p := NewResponses(
		"openai",
		WithBaseURL(server.URL+"/v1"),
		WithAPIKey("secret"),
	)
	stream, err := p.Stream(context.Background(), &providers.ChatRequest{
		Model:     "gpt-5",
		Reasoning: types.ThinkingLevelHigh,
		Messages:  []providers.Message{{Role: providers.RoleUser, Content: "Weather?"}},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var sawThinkingStart, sawThinkingDelta, sawTextDelta, sawToolStart, sawToolDelta, sawToolEnd, sawDone bool
	for event := range stream.Events() {
		switch event.Type {
		case types.EventThinkingStart:
			sawThinkingStart = true
		case types.EventThinkingDelta:
			sawThinkingDelta = true
		case types.EventTextDelta:
			sawTextDelta = true
		case types.EventToolCallStart:
			sawToolStart = true
		case types.EventToolCallDelta:
			sawToolDelta = true
		case types.EventToolCallEnd:
			sawToolEnd = true
		case types.EventDone:
			sawDone = true
		case types.EventError:
			t.Fatalf("unexpected stream error: %v", event.Error)
		}
	}
	if !sawThinkingStart || !sawThinkingDelta || !sawTextDelta || !sawToolStart || !sawToolDelta || !sawToolEnd || !sawDone {
		t.Fatalf("missing mapped stream events: thinkingStart=%v thinkingDelta=%v text=%v toolStart=%v toolDelta=%v toolEnd=%v done=%v",
			sawThinkingStart, sawThinkingDelta, sawTextDelta, sawToolStart, sawToolDelta, sawToolEnd, sawDone)
	}

	msg, err := stream.Result()
	if err != nil {
		t.Fatalf("Result: %v", err)
	}
	if msg.Model != "gpt-5" || msg.StopReason != "toolUse" {
		t.Fatalf("unexpected response metadata: %#v", msg)
	}
	if msg.Usage.Input != 60 || msg.Usage.CacheRead != 40 || msg.Usage.Output != 20 || msg.Usage.TotalTokens != 120 {
		t.Fatalf("unexpected usage: %#v", msg.Usage)
	}
	if got := thinkingText(msg); got != "Check the tool." {
		t.Fatalf("thinking summary = %q", got)
	}
	if got := responseText(msg); got != "Checking." {
		t.Fatalf("response text = %q", got)
	}
	call := responseToolCall(msg)
	if call == nil || call.ID != "call_1" || call.Name != "weather" || call.Arguments["city"] != "上海" {
		t.Fatalf("unexpected tool call: %#v", call)
	}
	raw := msg.ProviderMetadata[responsesOutputMetadataKey]
	if len(raw) == 0 {
		t.Fatal("expected output Items in provider metadata")
	}
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil || len(items) != 3 || items[0]["type"] != "reasoning" {
		t.Fatalf("unexpected persisted output Items: %#v err=%v", items, err)
	}
	if requestBody["store"] != false {
		t.Fatalf("request did not default to stateless storage: %#v", requestBody)
	}
}

func TestResponsesChatMapsTextToolsUsageAndMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{
			"id":"resp_1",
			"model":"gpt-5",
			"status":"completed",
			"output":[
				{"id":"msg_1","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Use the tool.","annotations":[]}]},
				{"id":"fc_1","type":"function_call","call_id":"call_1","name":"read","arguments":"{\"path\":\"README.md\"}","status":"completed"}
			],
			"usage":{"input_tokens":50,"input_tokens_details":{"cached_tokens":10},"output_tokens":12,"total_tokens":62}
		}`)
	}))
	defer server.Close()

	p := NewResponses("openai", WithBaseURL(server.URL))
	resp, err := p.Chat(context.Background(), &providers.ChatRequest{
		Model:    "gpt-5",
		Messages: []providers.Message{{Role: providers.RoleUser, Content: "Read it"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Message.Content != "Use the tool." || resp.FinishReason != "toolUse" {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID != "call_1" || resp.ToolCalls[0].Function.Name != "read" {
		t.Fatalf("unexpected tool calls: %#v", resp.ToolCalls)
	}
	if resp.Usage.PromptTokens != 50 || resp.Usage.CacheReadTokens != 10 || resp.Usage.CompletionTokens != 12 {
		t.Fatalf("unexpected usage: %#v", resp.Usage)
	}
	if len(resp.Message.ProviderMetadata[responsesOutputMetadataKey]) == 0 {
		t.Fatal("expected provider output metadata")
	}
}

func TestResponsesErrorsIncludeProviderAndAPIMessage(t *testing.T) {
	t.Run("http status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"unsupported model"}}`)
		}))
		defer server.Close()

		p := NewResponses("custom", WithBaseURL(server.URL))
		_, err := p.Chat(context.Background(), &providers.ChatRequest{Model: "bad"})
		if err == nil || !strings.Contains(err.Error(), "custom") || !strings.Contains(err.Error(), "unsupported model") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("stream event", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, `data: {"type":"error","code":"server_error","message":"stream failed"}

`)
		}))
		defer server.Close()

		p := NewResponses("custom", WithBaseURL(server.URL))
		stream, err := p.Stream(context.Background(), &providers.ChatRequest{Model: "gpt-5"})
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}
		for range stream.Events() {
		}
		_, err = stream.Result()
		if err == nil || !strings.Contains(err.Error(), "stream failed") {
			t.Fatalf("unexpected stream result: %v", err)
		}
	})
}

func thinkingText(msg *types.AssistantMessage) string {
	var out strings.Builder
	for _, block := range msg.Content {
		if thinking, ok := block.(*types.ThinkingContent); ok {
			out.WriteString(thinking.Thinking)
		}
	}
	return out.String()
}

func responseText(msg *types.AssistantMessage) string {
	var out strings.Builder
	for _, block := range msg.Content {
		if text, ok := block.(*types.TextContent); ok {
			out.WriteString(text.Text)
		}
	}
	return out.String()
}

func responseToolCall(msg *types.AssistantMessage) *types.ToolCallContent {
	for _, block := range msg.Content {
		if call, ok := block.(*types.ToolCallContent); ok {
			return call
		}
	}
	return nil
}
