package subagent

import (
	"context"
	"fmt"

	"github.com/openmodu/modu/pkg/types"
)

// adminTool carries the profile-management half of the old mega-tool:
// listing, inspecting, and editing agent profiles, plus setup diagnostics.
//
// Keeping it separate from the dispatch tool means the model is not offered
// "rewrite a profile" in the same schema it uses to delegate work, and the
// dispatch tool's description stays short enough to read.
type adminTool struct {
	ext *Extension
}

func newAdminTool(ext *Extension) *adminTool { return &adminTool{ext: ext} }

func (t *adminTool) Name() string   { return "subagent_admin" }
func (t *adminTool) Label() string  { return "Subagent Admin" }
func (t *adminTool) Parallel() bool { return true }

func (t *adminTool) Description() string {
	return `Inspect and edit subagent profiles. Does not run anything — use the
subagent tool to delegate work.

Actions:
  - list: show discovered subagent profiles.
  - get: show one profile's full detail; requires "agent".
  - create: create a new profile; requires "config" object with name plus
    optional description / systemPrompt / tools / model / scope etc.
  - update: merge updates into an existing profile; requires "agent" and "config".
  - delete: remove a profile; requires "agent".
  - doctor: show read-only setup diagnostics.

Profiles are Markdown files with YAML frontmatter. Editing one changes how
every future run of that agent behaves, so only create or update a profile
when the user asked for it.`
}

func (t *adminTool) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"list", "get", "create", "update", "delete", "doctor"},
				"description": "Management action. Defaults to list.",
			},
			"agent": map[string]any{
				"type":        "string",
				"description": "Profile name for action=get|update|delete.",
			},
			"config": map[string]any{
				"type":        "object",
				"description": "Profile config for action=create|update. Recognised keys: name, description, systemPrompt, scope, model, tools, disallowed_tools, skills, memory, permission_mode, background, effort, isolation, default_context, thinking, max_turns, default_reads, default_progress, harness_block_tools.",
			},
			"agentScope": map[string]any{
				"type":        "string",
				"enum":        []string{"user", "project", "both"},
				"description": "Filter discovered agents by source for action=list|get. Default 'both'.",
			},
		},
	}
}

func (t *adminTool) Execute(ctx context.Context, _ string, args map[string]any, _ types.ToolUpdateCallback) (types.ToolResult, error) {
	action, _ := args["action"].(string)
	if action == "" {
		action = "list"
	}
	switch action {
	case "list", "get", "create", "update", "delete", "doctor":
	default:
		return errResult(fmt.Sprintf("subagent_admin: unknown action %q (expected list|get|create|update|delete|doctor)", action)), nil
	}
	text, err := runAction(ctx, t.ext, action, args)
	if err != nil {
		return errResult(fmt.Sprintf("subagent_admin: %v", err)), nil
	}
	return okResult(text, nil), nil
}
