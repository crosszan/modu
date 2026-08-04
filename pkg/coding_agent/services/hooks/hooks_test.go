package hooks

import (
	"context"
	"strings"
	"testing"

	"github.com/openmodu/modu/pkg/coding_agent/foundation/config"
	"github.com/openmodu/modu/pkg/types"
)

type captureTool struct {
	args map[string]any
}

func (t *captureTool) Name() string        { return "capture" }
func (t *captureTool) Label() string       { return "Capture" }
func (t *captureTool) Description() string { return "Capture arguments" }
func (t *captureTool) Parameters() any     { return map[string]any{"type": "object"} }
func (t *captureTool) Execute(ctx context.Context, id string, args map[string]any, update types.ToolUpdateCallback) (types.ToolResult, error) {
	t.args = cloneMap(args)
	return types.ToolResult{
		Content: []types.ContentBlock{&types.TextContent{Type: "text", Text: "ran"}},
	}, nil
}

func TestRunToolUpdatesInputAndAddsPostContext(t *testing.T) {
	runner, err := New(config.HookConfig{
		PreToolUse: []config.CommandHookConfig{{
			Matcher: "capture",
			Command: `printf '%s' '{"updatedInput":{"value":"changed"}}'`,
		}},
		PostToolUse: []config.CommandHookConfig{{
			Matcher: "capture",
			Command: `printf '%s' '{"additionalContext":"post checked"}'`,
		}},
	}, Options{Cwd: t.TempDir, Enabled: func() bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	tool := &captureTool{}
	result, err := runner.RunTool(context.Background(), tool, "call-1", map[string]any{"value": "original"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if tool.args["value"] != "changed" {
		t.Fatalf("tool args = %#v", tool.args)
	}
	if text := hookText(result); !strings.Contains(text, "post checked") {
		t.Fatalf("post context missing: %s", text)
	}
}

func TestRunToolBlocksOnExitCodeTwo(t *testing.T) {
	runner, err := New(config.HookConfig{
		PreToolUse: []config.CommandHookConfig{{
			Matcher: "capture",
			Command: `printf 'policy denied' >&2; exit 2`,
		}},
	}, Options{Cwd: t.TempDir, Enabled: func() bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	tool := &captureTool{}
	result, err := runner.RunTool(context.Background(), tool, "call-1", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if tool.args != nil {
		t.Fatalf("blocked tool executed with %#v", tool.args)
	}
	if text := hookText(result); !strings.Contains(text, "policy denied") {
		t.Fatalf("block reason missing: %s", text)
	}
}

func TestUserPromptSubmitUpdatesAndBlocks(t *testing.T) {
	updated, err := New(config.HookConfig{
		UserPromptSubmit: []config.CommandHookConfig{{
			Command: `printf '%s' '{"updatedPrompt":"rewritten","additionalContext":"project rule"}'`,
		}},
	}, Options{Cwd: t.TempDir, Enabled: func() bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := updated.UserPromptSubmit(context.Background(), "original")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"rewritten", "project rule"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("missing %q in prompt %q", want, prompt)
		}
	}

	blocked, err := New(config.HookConfig{
		UserPromptSubmit: []config.CommandHookConfig{{
			Command: `printf '%s' '{"decision":"block","reason":"prompt denied"}'`,
		}},
	}, Options{Cwd: t.TempDir, Enabled: func() bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := blocked.UserPromptSubmit(context.Background(), "original"); err == nil || !strings.Contains(err.Error(), "prompt denied") {
		t.Fatalf("block error = %v", err)
	}
}

func TestHooksAreDisabledForUntrustedProject(t *testing.T) {
	runner, err := New(config.HookConfig{
		PreToolUse: []config.CommandHookConfig{{
			Matcher: "capture",
			Command: `exit 2`,
		}},
	}, Options{Cwd: t.TempDir, Enabled: func() bool { return false }})
	if err != nil {
		t.Fatal(err)
	}
	tool := &captureTool{}
	if _, err := runner.RunTool(context.Background(), tool, "call-1", map[string]any{"value": "ran"}, nil); err != nil {
		t.Fatal(err)
	}
	if tool.args["value"] != "ran" {
		t.Fatalf("disabled hook affected tool: %#v", tool.args)
	}
}

func TestHookFailureIsFailOpenAndVisible(t *testing.T) {
	runner, err := New(config.HookConfig{
		PreToolUse: []config.CommandHookConfig{{
			Matcher: "capture",
			Command: `printf 'broken hook' >&2; exit 3`,
		}},
	}, Options{Cwd: t.TempDir, Enabled: func() bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	tool := &captureTool{}
	result, err := runner.RunTool(context.Background(), tool, "call-1", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if text := hookText(result); !strings.Contains(text, "hook failed open: broken hook") {
		t.Fatalf("warning missing: %s", text)
	}
}

func hookText(result types.ToolResult) string {
	var builder strings.Builder
	for _, block := range result.Content {
		if text, ok := block.(*types.TextContent); ok {
			builder.WriteString(text.Text)
		}
	}
	return builder.String()
}
