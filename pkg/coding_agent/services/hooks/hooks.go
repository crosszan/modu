// Package hooks runs trusted, configuration-defined lifecycle commands.
package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/openmodu/modu/pkg/coding_agent/foundation/config"
	"github.com/openmodu/modu/pkg/types"
)

const (
	defaultTimeout = 10 * time.Second
	maxTimeout     = 60 * time.Second
	maxHookOutput  = 1024 * 1024
)

type Options struct {
	Cwd       func() string
	SessionID func() string
	Enabled   func() bool
}

type command struct {
	config config.CommandHookConfig
	match  *regexp.Regexp
}

type Runner struct {
	preTool   []command
	postTool  []command
	userInput []command
	options   Options
}

type Input struct {
	Event      string         `json:"event"`
	SessionID  string         `json:"session_id,omitempty"`
	Cwd        string         `json:"cwd,omitempty"`
	ToolName   string         `json:"tool_name,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolInput  map[string]any `json:"tool_input,omitempty"`
	ToolResult any            `json:"tool_result,omitempty"`
	Prompt     string         `json:"prompt,omitempty"`
}

type Output struct {
	Decision          string         `json:"decision,omitempty"`
	Reason            string         `json:"reason,omitempty"`
	UpdatedInput      map[string]any `json:"updatedInput,omitempty"`
	UpdatedPrompt     string         `json:"updatedPrompt,omitempty"`
	AdditionalContext string         `json:"additionalContext,omitempty"`
}

type BlockedError struct {
	Event  string
	Reason string
}

func (e *BlockedError) Error() string {
	if strings.TrimSpace(e.Reason) == "" {
		return e.Event + " blocked by hook"
	}
	return e.Event + " blocked by hook: " + e.Reason
}

func New(cfg config.HookConfig, options Options) (*Runner, error) {
	pre, err := compile(cfg.PreToolUse)
	if err != nil {
		return nil, fmt.Errorf("preToolUse hooks: %w", err)
	}
	post, err := compile(cfg.PostToolUse)
	if err != nil {
		return nil, fmt.Errorf("postToolUse hooks: %w", err)
	}
	user, err := compile(cfg.UserPromptSubmit)
	if err != nil {
		return nil, fmt.Errorf("userPromptSubmit hooks: %w", err)
	}
	return &Runner{preTool: pre, postTool: post, userInput: user, options: options}, nil
}

func compile(configs []config.CommandHookConfig) ([]command, error) {
	result := make([]command, 0, len(configs))
	for i, item := range configs {
		item.Command = strings.TrimSpace(item.Command)
		if item.Command == "" {
			return nil, fmt.Errorf("hook %d command is required", i+1)
		}
		matcher := strings.TrimSpace(item.Matcher)
		if matcher == "" || matcher == "*" {
			result = append(result, command{config: item})
			continue
		}
		compiled, err := regexp.Compile("^(?:" + matcher + ")$")
		if err != nil {
			return nil, fmt.Errorf("hook %d matcher: %w", i+1, err)
		}
		result = append(result, command{config: item, match: compiled})
	}
	return result, nil
}

func (r *Runner) Enabled() bool {
	return r != nil && (r.options.Enabled == nil || r.options.Enabled())
}

func (r *Runner) RunTool(
	ctx context.Context,
	tool types.Tool,
	toolCallID string,
	args map[string]any,
	onUpdate types.ToolUpdateCallback,
) (types.ToolResult, error) {
	if r == nil || !r.Enabled() {
		return tool.Execute(ctx, toolCallID, args, onUpdate)
	}
	input := Input{
		Event:      "PreToolUse",
		SessionID:  r.sessionID(),
		Cwd:        r.cwd(),
		ToolName:   tool.Name(),
		ToolCallID: toolCallID,
		ToolInput:  cloneMap(args),
	}
	current := cloneMap(args)
	var annotations []string
	for _, hook := range r.matching(r.preTool, tool.Name()) {
		input.ToolInput = current
		output, warning, err := r.run(ctx, hook, input)
		if warning != "" {
			annotations = append(annotations, warning)
		}
		if err != nil {
			return blockedToolResult(tool.Name(), err.Error()), nil
		}
		if output.UpdatedInput != nil {
			current = cloneMap(output.UpdatedInput)
		}
		if text := strings.TrimSpace(output.AdditionalContext); text != "" {
			annotations = append(annotations, text)
		}
	}

	result, err := tool.Execute(ctx, toolCallID, current, onUpdate)
	if err != nil {
		return result, err
	}
	postInput := Input{
		Event:      "PostToolUse",
		SessionID:  r.sessionID(),
		Cwd:        r.cwd(),
		ToolName:   tool.Name(),
		ToolCallID: toolCallID,
		ToolInput:  current,
		ToolResult: result,
	}
	for _, hook := range r.matching(r.postTool, tool.Name()) {
		output, warning, _ := r.run(ctx, hook, postInput)
		if warning != "" {
			annotations = append(annotations, warning)
		}
		if text := strings.TrimSpace(output.AdditionalContext); text != "" {
			annotations = append(annotations, text)
		}
	}
	return appendAnnotations(result, annotations), nil
}

func (r *Runner) UserPromptSubmit(ctx context.Context, prompt string) (string, error) {
	if r == nil || !r.Enabled() {
		return prompt, nil
	}
	current := prompt
	for _, hook := range r.userInput {
		output, _, err := r.run(ctx, hook, Input{
			Event:     "UserPromptSubmit",
			SessionID: r.sessionID(),
			Cwd:       r.cwd(),
			Prompt:    current,
		})
		if err != nil {
			return "", err
		}
		if output.UpdatedPrompt != "" {
			current = output.UpdatedPrompt
		}
		if extra := strings.TrimSpace(output.AdditionalContext); extra != "" {
			current += "\n\n[Hook context]\n" + extra
		}
	}
	return current, nil
}

func (r *Runner) matching(commands []command, toolName string) []command {
	result := make([]command, 0, len(commands))
	for _, hook := range commands {
		if hook.match == nil || hook.match.MatchString(toolName) {
			result = append(result, hook)
		}
	}
	return result
}

func (r *Runner) run(parent context.Context, hook command, input Input) (Output, string, error) {
	timeout := time.Duration(hook.config.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if timeout > maxTimeout {
		timeout = maxTimeout
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	payload, err := json.Marshal(input)
	if err != nil {
		return Output{}, "hook input encoding failed: " + err.Error(), nil
	}
	cmd := exec.CommandContext(ctx, "bash", "-c", hook.config.Command)
	cmd.Dir = r.cwd()
	cmd.Env = os.Environ()
	cmd.Stdin = bytes.NewReader(payload)
	stdout := &limitedBuffer{limit: maxHookOutput}
	stderr := &limitedBuffer{limit: maxHookOutput}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Run()

	var output Output
	stdoutText := strings.TrimSpace(stdout.String())
	if stdoutText != "" {
		if decodeErr := json.Unmarshal([]byte(stdoutText), &output); decodeErr != nil {
			if err == nil {
				return Output{}, "hook returned invalid JSON: " + decodeErr.Error(), nil
			}
		}
	}
	if strings.EqualFold(strings.TrimSpace(output.Decision), "block") {
		return output, "", &BlockedError{Event: input.Event, Reason: output.Reason}
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 2 {
		reason := strings.TrimSpace(output.Reason)
		if reason == "" {
			reason = strings.TrimSpace(stderr.String())
		}
		return output, "", &BlockedError{Event: input.Event, Reason: reason}
	}
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return output, fmt.Sprintf("hook timed out after %s", timeout), nil
		}
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return output, "hook failed open: " + message, nil
	}
	return output, "", nil
}

func (r *Runner) cwd() string {
	if r != nil && r.options.Cwd != nil {
		return r.options.Cwd()
	}
	return ""
}

func (r *Runner) sessionID() string {
	if r != nil && r.options.SessionID != nil {
		return r.options.SessionID()
	}
	return ""
}

func blockedToolResult(toolName, reason string) types.ToolResult {
	return types.ToolResult{
		Content: []types.ContentBlock{&types.TextContent{
			Type: "text",
			Text: fmt.Sprintf("%s blocked by hook: %s", toolName, reason),
		}},
		Details: map[string]any{"isError": true, "blockedBy": "hook"},
	}
}

func appendAnnotations(result types.ToolResult, annotations []string) types.ToolResult {
	var clean []string
	for _, annotation := range annotations {
		if annotation = strings.TrimSpace(annotation); annotation != "" {
			clean = append(clean, annotation)
		}
	}
	if len(clean) == 0 {
		return result
	}
	result.Content = append(result.Content, &types.TextContent{
		Type: "text",
		Text: "\n[Hook context]\n" + strings.Join(clean, "\n"),
	})
	return result
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	result := make(map[string]any, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

type limitedBuffer struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	original := len(p)
	remaining := b.limit - b.buffer.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
			b.truncated = true
		}
		_, _ = b.buffer.Write(p)
	} else {
		b.truncated = true
	}
	return original, nil
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}
