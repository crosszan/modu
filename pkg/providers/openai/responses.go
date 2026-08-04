package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/openmodu/modu/pkg/providers"
	"github.com/openmodu/modu/pkg/types"
)

const responsesOutputMetadataKey = "openai.responses.output"

type responsesProvider struct {
	id     string
	config Config
}

// NewResponses creates a provider that speaks the OpenAI Responses protocol.
func NewResponses(id string, opts ...Option) providers.Provider {
	var cfg Config
	for _, option := range opts {
		option(&cfg)
	}
	return &responsesProvider{id: id, config: cfg}
}

func (p *responsesProvider) ID() string { return p.id }

type responsesUsage struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	TotalTokens        int `json:"total_tokens"`
	InputTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

type responsesError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Param   string `json:"param"`
	Type    string `json:"type"`
}

type responsesObject struct {
	ID                string            `json:"id"`
	Model             string            `json:"model"`
	Status            string            `json:"status"`
	Output            []json.RawMessage `json:"output"`
	Usage             *responsesUsage   `json:"usage"`
	Error             *responsesError   `json:"error"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
}

func (p *responsesProvider) Chat(ctx context.Context, req *providers.ChatRequest) (*providers.ChatResponse, error) {
	body, err := p.buildBody(req, false)
	if err != nil {
		return nil, err
	}
	resp, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, p.httpError(resp, data)
	}

	var raw responsesObject
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: decode Responses response: %w", p.id, err)
	}
	if raw.Error != nil {
		return nil, p.apiError(raw.Error)
	}

	message, toolCalls := responseProviderMessage(raw.Output)
	out := &providers.ChatResponse{
		ID:           raw.ID,
		Model:        raw.Model,
		Message:      message,
		ToolCalls:    toolCalls,
		FinishReason: responsesStopReason(raw.Status, raw.IncompleteDetails, len(toolCalls) > 0),
	}
	setProviderMessageOutput(&out.Message, raw.Output)
	if raw.Usage != nil {
		out.Usage = providers.Usage{
			PromptTokens:     raw.Usage.InputTokens,
			CompletionTokens: raw.Usage.OutputTokens,
			TotalTokens:      raw.Usage.TotalTokens,
			CacheReadTokens:  raw.Usage.InputTokensDetails.CachedTokens,
		}
	}
	return out, nil
}

func (p *responsesProvider) Stream(ctx context.Context, req *providers.ChatRequest) (types.EventStream, error) {
	body, err := p.buildBody(req, true)
	if err != nil {
		return nil, err
	}
	resp, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, p.httpError(resp, data)
	}

	stream := types.NewEventStream()
	go p.readResponsesSSE(resp.Body, stream)
	return stream, nil
}

func (p *responsesProvider) buildBody(req *providers.ChatRequest, stream bool) ([]byte, error) {
	if req == nil || strings.TrimSpace(req.Model) == "" {
		return nil, fmt.Errorf("%s: model must be specified in the request", p.id)
	}

	input, instructions := responseInput(req.Messages)
	payload := map[string]any{
		"model":  req.Model,
		"input":  input,
		"stream": stream,
		"store":  false,
	}
	if instructions != "" {
		payload["instructions"] = instructions
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, tool := range req.Tools {
			entry := map[string]any{
				"type":        "function",
				"name":        tool.Function.Name,
				"description": tool.Function.Description,
			}
			if tool.Function.Parameters != nil {
				entry["parameters"] = tool.Function.Parameters
			}
			tools = append(tools, entry)
		}
		payload["tools"] = tools
	}
	if req.Temperature != nil {
		payload["temperature"] = *req.Temperature
	}
	if req.MaxTokens != nil {
		payload["max_output_tokens"] = *req.MaxTokens
	}
	if req.Reasoning != "" && req.Reasoning != types.ThinkingLevelOff {
		payload["reasoning"] = map[string]any{
			"effort":  req.Reasoning,
			"summary": "auto",
		}
		payload["include"] = []string{"reasoning.encrypted_content"}
	}

	for key, value := range p.config.extraBody {
		switch key {
		case "model", "input", "instructions", "stream":
			continue
		default:
			payload[key] = value
		}
	}
	return json.Marshal(payload)
}

func responseInput(messages []providers.Message) ([]any, string) {
	input := make([]any, 0, len(messages))
	var instructions []string
	for _, message := range messages {
		switch message.Role {
		case providers.RoleSystem:
			if text := responseTextValue(message.Content); text != "" {
				instructions = append(instructions, text)
			}
		case providers.RoleUser:
			if content := responseUserContent(message.Content); content != nil {
				input = append(input, map[string]any{
					"role":    "user",
					"content": content,
				})
			}
		case providers.RoleAssistant:
			if items := responseMetadataItems(message.ProviderMetadata); len(items) > 0 {
				for _, item := range items {
					var decoded any
					if json.Unmarshal(item, &decoded) == nil {
						input = append(input, decoded)
					}
				}
				continue
			}
			if text := responseTextValue(message.Content); text != "" {
				input = append(input, map[string]any{
					"role":    "assistant",
					"content": text,
				})
			}
			for _, call := range message.ToolCalls {
				input = append(input, map[string]any{
					"type":      "function_call",
					"call_id":   call.ID,
					"name":      call.Function.Name,
					"arguments": call.Function.Arguments,
				})
			}
		case providers.RoleTool:
			if message.ToolCallID == "" {
				continue
			}
			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": message.ToolCallID,
				"output":  responseTextValue(message.Content),
			})
		}
	}
	return input, strings.Join(instructions, "\n\n")
}

func responseUserContent(content any) any {
	switch value := content.(type) {
	case string:
		if value == "" {
			return nil
		}
		return value
	case []any:
		parts := make([]any, 0, len(value))
		for _, rawPart := range value {
			part, ok := rawPart.(map[string]any)
			if !ok {
				continue
			}
			switch part["type"] {
			case "text":
				if text, _ := part["text"].(string); text != "" {
					parts = append(parts, map[string]any{"type": "input_text", "text": text})
				}
			case "image_url":
				imageURL := ""
				switch image := part["image_url"].(type) {
				case string:
					imageURL = image
				case map[string]any:
					imageURL, _ = image["url"].(string)
				}
				if imageURL != "" {
					parts = append(parts, map[string]any{"type": "input_image", "image_url": imageURL})
				}
			}
		}
		if len(parts) > 0 {
			return parts
		}
	}
	return nil
}

func responseTextValue(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case []any:
		var text strings.Builder
		for _, rawPart := range value {
			part, ok := rawPart.(map[string]any)
			if !ok {
				continue
			}
			if value, _ := part["text"].(string); value != "" {
				text.WriteString(value)
			}
		}
		return text.String()
	default:
		return ""
	}
}

func responseMetadataItems(metadata map[string]json.RawMessage) []json.RawMessage {
	if len(metadata) == 0 {
		return nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(metadata[responsesOutputMetadataKey], &items); err != nil {
		return nil
	}
	return items
}

func (p *responsesProvider) doRequest(ctx context.Context, body []byte) (*http.Response, error) {
	if strings.TrimSpace(p.config.baseURL) == "" {
		return nil, fmt.Errorf("%s: BaseURL must be set", p.id)
	}
	url := strings.TrimRight(p.config.baseURL, "/") + "/responses"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	if p.config.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+p.config.apiKey)
	}
	for key, value := range p.config.headers {
		request.Header.Set(key, value)
	}
	return (&http.Client{Timeout: 10 * time.Minute}).Do(request)
}

func (p *responsesProvider) httpError(resp *http.Response, data []byte) error {
	var payload struct {
		Error *responsesError `json:"error"`
	}
	if json.Unmarshal(data, &payload) == nil && payload.Error != nil && payload.Error.Message != "" {
		return fmt.Errorf("%s: %s: %s", p.id, resp.Status, payload.Error.Message)
	}
	return fmt.Errorf("%s: %s: %s", p.id, resp.Status, strings.TrimSpace(string(data)))
}

func (p *responsesProvider) apiError(apiErr *responsesError) error {
	if apiErr == nil {
		return fmt.Errorf("%s: Responses API request failed", p.id)
	}
	if apiErr.Code != "" {
		return fmt.Errorf("%s: %s: %s", p.id, apiErr.Code, apiErr.Message)
	}
	return fmt.Errorf("%s: %s", p.id, apiErr.Message)
}

type responseOutputItem struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Summary []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"summary"`
}

func responseProviderMessage(output []json.RawMessage) (providers.Message, []providers.ToolCall) {
	message := providers.Message{Role: providers.RoleAssistant}
	var text strings.Builder
	var reasoning strings.Builder
	var toolCalls []providers.ToolCall
	for _, rawItem := range output {
		var item responseOutputItem
		if json.Unmarshal(rawItem, &item) != nil {
			continue
		}
		switch item.Type {
		case "message":
			for _, part := range item.Content {
				if part.Type == "output_text" {
					text.WriteString(part.Text)
				}
			}
		case "reasoning":
			for _, part := range item.Summary {
				if part.Type == "summary_text" {
					reasoning.WriteString(part.Text)
				}
			}
		case "function_call":
			toolCalls = append(toolCalls, providers.ToolCall{
				ID:   item.CallID,
				Type: "function",
				Function: providers.FuncCall{
					Name:      item.Name,
					Arguments: item.Arguments,
				},
			})
		}
	}
	if text.Len() > 0 {
		message.Content = text.String()
	}
	message.ReasoningContent = reasoning.String()
	message.ToolCalls = toolCalls
	return message, toolCalls
}

func setProviderMessageOutput(message *providers.Message, output []json.RawMessage) {
	if message == nil || len(output) == 0 {
		return
	}
	raw, err := json.Marshal(output)
	if err != nil {
		return
	}
	message.ProviderMetadata = map[string]json.RawMessage{
		responsesOutputMetadataKey: raw,
	}
}

type responsesStreamEvent struct {
	Type         string           `json:"type"`
	Code         string           `json:"code"`
	Message      string           `json:"message"`
	Delta        string           `json:"delta"`
	Arguments    string           `json:"arguments"`
	OutputIndex  int              `json:"output_index"`
	ContentIndex int              `json:"content_index"`
	SummaryIndex int              `json:"summary_index"`
	Item         json.RawMessage  `json:"item"`
	Response     *responsesObject `json:"response"`
	Error        *responsesError  `json:"error"`
}

type responseToolAccumulator struct {
	contentIndex int
	callID       string
	name         string
	arguments    strings.Builder
	doneArgs     string
}

func (p *responsesProvider) readResponsesSSE(body io.ReadCloser, stream *types.EventStreamImpl) {
	defer body.Close()
	defer stream.Close()

	partial := &types.AssistantMessage{
		Role:       types.RoleAssistant,
		Content:    []types.ContentBlock{},
		ProviderID: p.id,
	}
	push := func(event types.StreamEvent) {
		if event.Partial == partial {
			event.Partial = responsesMessageSnapshot(partial)
		}
		if event.Message == partial {
			event.Message = responsesMessageSnapshot(partial)
		}
		if event.ErrorMessage == partial {
			event.ErrorMessage = responsesMessageSnapshot(partial)
		}
		stream.Push(event)
	}
	push(types.StreamEvent{Type: types.EventStart, Partial: partial})

	nextContentIndex := 0
	thinkingIndex := -1
	textIndex := -1
	toolAccumulators := map[int]*responseToolAccumulator{}
	var outputItems []json.RawMessage
	var finalResponse *responsesObject
	failed := false

	fail := func(err error) {
		if failed {
			return
		}
		failed = true
		partial.StopReason = "error"
		partial.ErrorMessage = err.Error()
		push(types.StreamEvent{
			Type:         types.EventError,
			Reason:       "error",
			ErrorMessage: partial,
			Error:        err,
		})
		stream.Resolve(partial, err)
	}

	startThinking := func() {
		if thinkingIndex >= 0 {
			return
		}
		thinkingIndex = nextContentIndex
		nextContentIndex++
		partial.Content = append(partial.Content, &types.ThinkingContent{Type: "thinking"})
		push(types.StreamEvent{
			Type:         types.EventThinkingStart,
			ContentIndex: thinkingIndex,
			Partial:      partial,
		})
	}
	startText := func() {
		if textIndex >= 0 {
			return
		}
		textIndex = nextContentIndex
		nextContentIndex++
		partial.Content = append(partial.Content, &types.TextContent{Type: "text"})
		push(types.StreamEvent{
			Type:         types.EventTextStart,
			ContentIndex: textIndex,
			Partial:      partial,
		})
	}
	startTool := func(outputIndex int, item responseOutputItem) *responseToolAccumulator {
		if accumulator := toolAccumulators[outputIndex]; accumulator != nil {
			if item.CallID != "" {
				accumulator.callID = item.CallID
			}
			if item.Name != "" {
				accumulator.name = item.Name
			}
			return accumulator
		}
		accumulator := &responseToolAccumulator{
			contentIndex: nextContentIndex,
			callID:       item.CallID,
			name:         item.Name,
		}
		nextContentIndex++
		toolAccumulators[outputIndex] = accumulator
		partial.Content = append(partial.Content, &types.ToolCallContent{
			Type:      "toolCall",
			ID:        item.CallID,
			Name:      item.Name,
			Arguments: map[string]any{},
		})
		push(types.StreamEvent{
			Type:         types.EventToolCallStart,
			ContentIndex: accumulator.contentIndex,
			Partial:      partial,
		})
		return accumulator
	}

	scanErr := providers.ScanSSEData(body, func(data string) bool {
		var event responsesStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			fail(fmt.Errorf("%s: decode Responses stream event: %w", p.id, err))
			return false
		}

		switch event.Type {
		case "response.created", "response.in_progress":
			if event.Response != nil && event.Response.Model != "" {
				partial.Model = event.Response.Model
			}
		case "response.reasoning_summary_text.delta":
			if event.Delta == "" {
				break
			}
			startThinking()
			if thinking, ok := getThinkingAt(partial, thinkingIndex); ok {
				thinking.Thinking += event.Delta
			}
			push(types.StreamEvent{
				Type:         types.EventThinkingDelta,
				ContentIndex: thinkingIndex,
				Delta:        event.Delta,
				Partial:      partial,
			})
		case "response.output_text.delta":
			if event.Delta == "" {
				break
			}
			startText()
			if text, ok := getTextAt(partial, textIndex); ok {
				text.Text += event.Delta
			}
			push(types.StreamEvent{
				Type:         types.EventTextDelta,
				ContentIndex: textIndex,
				Delta:        event.Delta,
				Partial:      partial,
			})
		case "response.output_item.added":
			var item responseOutputItem
			if json.Unmarshal(event.Item, &item) == nil && item.Type == "function_call" {
				startTool(event.OutputIndex, item)
			}
		case "response.function_call_arguments.delta":
			accumulator := startTool(event.OutputIndex, responseOutputItem{})
			accumulator.arguments.WriteString(event.Delta)
			push(types.StreamEvent{
				Type:         types.EventToolCallDelta,
				ContentIndex: accumulator.contentIndex,
				Delta:        event.Delta,
				Partial:      partial,
			})
		case "response.function_call_arguments.done":
			accumulator := startTool(event.OutputIndex, responseOutputItem{})
			accumulator.doneArgs = event.Arguments
		case "response.output_item.done":
			if len(event.Item) > 0 {
				outputItems = append(outputItems, append(json.RawMessage(nil), event.Item...))
			}
			var item responseOutputItem
			if json.Unmarshal(event.Item, &item) == nil && item.Type == "function_call" {
				accumulator := startTool(event.OutputIndex, item)
				accumulator.doneArgs = item.Arguments
			}
		case "response.completed", "response.incomplete":
			finalResponse = event.Response
			if finalResponse != nil {
				if finalResponse.Model != "" {
					partial.Model = finalResponse.Model
				}
				if len(finalResponse.Output) > 0 {
					outputItems = finalResponse.Output
				}
			}
			return false
		case "response.failed":
			if event.Response != nil && event.Response.Error != nil {
				fail(p.apiError(event.Response.Error))
			} else {
				fail(fmt.Errorf("%s: Responses API response failed", p.id))
			}
			return false
		case "error":
			message := event.Message
			if event.Error != nil && event.Error.Message != "" {
				message = event.Error.Message
			}
			if message == "" {
				message = "Responses API stream failed"
			}
			fail(fmt.Errorf("%s: %s", p.id, message))
			return false
		}
		return true
	})
	if failed {
		return
	}
	if scanErr != nil {
		fail(fmt.Errorf("%s: read Responses stream: %w", p.id, scanErr))
		return
	}

	if thinkingIndex >= 0 {
		content := ""
		if thinking, ok := getThinkingAt(partial, thinkingIndex); ok {
			content = thinking.Thinking
		}
		push(types.StreamEvent{
			Type:         types.EventThinkingEnd,
			ContentIndex: thinkingIndex,
			Content:      content,
			Partial:      partial,
		})
	}
	if textIndex >= 0 {
		content := ""
		if text, ok := getTextAt(partial, textIndex); ok {
			content = text.Text
		}
		push(types.StreamEvent{
			Type:         types.EventTextEnd,
			ContentIndex: textIndex,
			Content:      content,
			Partial:      partial,
		})
	}

	toolOutputIndexes := make([]int, 0, len(toolAccumulators))
	for outputIndex := range toolAccumulators {
		toolOutputIndexes = append(toolOutputIndexes, outputIndex)
	}
	sort.Ints(toolOutputIndexes)
	for _, outputIndex := range toolOutputIndexes {
		accumulator := toolAccumulators[outputIndex]
		argumentsJSON := accumulator.doneArgs
		if argumentsJSON == "" {
			argumentsJSON = accumulator.arguments.String()
		}
		arguments := map[string]any{}
		if argumentsJSON != "" {
			_ = json.Unmarshal([]byte(argumentsJSON), &arguments)
		}
		call := &types.ToolCallContent{
			Type:      "toolCall",
			ID:        accumulator.callID,
			Name:      accumulator.name,
			Arguments: arguments,
		}
		partial.Content[accumulator.contentIndex] = call
		push(types.StreamEvent{
			Type:         types.EventToolCallEnd,
			ContentIndex: accumulator.contentIndex,
			ToolCall:     call,
			Partial:      partial,
		})
	}

	if finalResponse != nil {
		applyResponsesUsage(partial, finalResponse.Usage)
		partial.StopReason = responsesStopReason(finalResponse.Status, finalResponse.IncompleteDetails, len(toolAccumulators) > 0)
	}
	if partial.StopReason == "" {
		if len(toolAccumulators) > 0 {
			partial.StopReason = "toolUse"
		} else {
			partial.StopReason = "stop"
		}
	}
	setAssistantOutput(partial, outputItems)
	push(types.StreamEvent{
		Type:    types.EventDone,
		Reason:  partial.StopReason,
		Message: partial,
	})
	stream.Resolve(partial, nil)
}

func applyResponsesUsage(message *types.AssistantMessage, usage *responsesUsage) {
	if message == nil || usage == nil {
		return
	}
	cached := usage.InputTokensDetails.CachedTokens
	message.Usage = types.AgentUsage{
		Input:       max(usage.InputTokens-cached, 0),
		Output:      usage.OutputTokens,
		CacheRead:   cached,
		TotalTokens: usage.TotalTokens,
	}
	if message.Usage.TotalTokens == 0 {
		message.Usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
}

func responsesStopReason(status string, incompleteDetails *struct {
	Reason string `json:"reason"`
}, hasToolCalls bool) string {
	if hasToolCalls {
		return "toolUse"
	}
	if status == "incomplete" {
		if incompleteDetails != nil && incompleteDetails.Reason == "max_output_tokens" {
			return "length"
		}
		return "incomplete"
	}
	return "stop"
}

func setAssistantOutput(message *types.AssistantMessage, output []json.RawMessage) {
	if message == nil || len(output) == 0 {
		return
	}
	raw, err := json.Marshal(output)
	if err != nil {
		return
	}
	message.ProviderMetadata = map[string]json.RawMessage{
		responsesOutputMetadataKey: raw,
	}
}

func responsesMessageSnapshot(message *types.AssistantMessage) *types.AssistantMessage {
	if message == nil {
		return nil
	}
	snapshot := *message
	snapshot.Content = make([]types.ContentBlock, len(message.Content))
	for index, block := range message.Content {
		switch value := block.(type) {
		case *types.TextContent:
			if value != nil {
				copied := *value
				snapshot.Content[index] = &copied
			}
		case *types.ThinkingContent:
			if value != nil {
				copied := *value
				snapshot.Content[index] = &copied
			}
		case *types.ImageContent:
			if value != nil {
				copied := *value
				snapshot.Content[index] = &copied
			}
		case *types.ToolCallContent:
			if value != nil {
				copied := *value
				if value.Arguments != nil {
					copied.Arguments = make(map[string]any, len(value.Arguments))
					for key, argument := range value.Arguments {
						copied.Arguments[key] = argument
					}
				}
				snapshot.Content[index] = &copied
			}
		default:
			snapshot.Content[index] = block
		}
	}
	if message.ProviderMetadata != nil {
		snapshot.ProviderMetadata = make(map[string]json.RawMessage, len(message.ProviderMetadata))
		for key, value := range message.ProviderMetadata {
			snapshot.ProviderMetadata[key] = append(json.RawMessage(nil), value...)
		}
	}
	return &snapshot
}
