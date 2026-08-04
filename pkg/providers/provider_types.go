package providers

import (
	"encoding/json"

	"github.com/openmodu/modu/pkg/types"
)

// Role 消息角色
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is the provider-neutral message representation used by protocol
// adapters. Content is either a string or []any for multimodal input.
// ProviderMetadata carries opaque output Items that a protocol must replay.
type Message struct {
	Role             Role                       `json:"role"`
	Content          any                        `json:"content,omitempty"`
	ReasoningContent string                     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall                 `json:"tool_calls,omitempty"`
	ToolCallID       string                     `json:"tool_call_id,omitempty"`
	Name             string                     `json:"name,omitempty"`
	ProviderMetadata map[string]json.RawMessage `json:"-"`
}

// ToolCall 工具调用
type ToolCall struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Function FuncCall `json:"function"`
}

// FuncCall 函数调用信息
type FuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool 工具定义
type Tool struct {
	Type     string  `json:"type"`
	Function FuncDef `json:"function"`
}

// FuncDef 函数定义
type FuncDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters,omitempty"`
}

// ChatRequest is the common request passed to provider protocol adapters.
type ChatRequest struct {
	Model       string              `json:"model"`
	Messages    []Message           `json:"messages"`
	Tools       []Tool              `json:"tools,omitempty"`
	Temperature *float64            `json:"temperature,omitempty"`
	MaxTokens   *int                `json:"max_tokens,omitempty"`
	Reasoning   types.ThinkingLevel `json:"-"`
}

// Usage token 用量
type Usage struct {
	// PromptTokens is the raw input count as reported by the API and, like the
	// OpenAI/Anthropic/Gemini originals, still includes any cache-hit tokens.
	// CacheReadTokens / CacheWriteTokens are the cached subset broken out for
	// consumers that want to separate fresh input from reuse.
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
}

// ChatResponse 最终响应（非流式直接返回 / 流式 Result() 返回）
type ChatResponse struct {
	ID           string     `json:"id"`
	Model        string     `json:"model"`
	Message      Message    `json:"message"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	Usage        Usage      `json:"usage"`
	FinishReason string     `json:"finish_reason"`
	ErrorMessage string     `json:"error_message,omitempty"`
}
