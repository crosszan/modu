# coding_agent

[English](README.md) | [中文](README_zh.md)

`coding_agent` turns the generic `pkg/agent` loop into a coding session with file and shell tools, persistent conversations, context compaction, extensions, and host-facing runtime state.

Use this package when you are building a coding-agent host. If you only need an LLM loop with custom tools, use `pkg/agent` directly; `coding_agent` also owns filesystem access, session files, configuration lookup, and extension lifecycle.

## Minimal session

```go
package main

import (
	"context"

	coding_agent "github.com/openmodu/modu/pkg/coding_agent"
	"github.com/openmodu/modu/pkg/providers"
	"github.com/openmodu/modu/pkg/providers/openai"
	"github.com/openmodu/modu/pkg/types"
)

func main() {
	providers.Register(openai.New(
		"ollama",
		openai.WithBaseURL("http://localhost:11434/v1"),
	))

	session, err := coding_agent.NewCodingSession(coding_agent.CodingSessionOptions{
		Cwd: "/path/to/project",
		Model: &types.Model{
			ID:            "qwen3-coder-next",
			ProviderID:    "ollama",
			ContextWindow: 32768,
			MaxTokens:     4096,
		},
		GetAPIKey: func(string) (string, error) { return "", nil },
	})
	if err != nil {
		panic(err)
	}

	if err := session.Prompt(context.Background(), "Explain main.go"); err != nil {
		panic(err)
	}
	session.WaitForIdle()
}
```

The caller must register the model provider before creating the session. `Cwd` determines which files tools can resolve and which project-level configuration and resources are discovered. Tool calls can modify that working tree, so the host must apply an approval policy appropriate to its environment.

The default SDK tool provider links only core coding tools. Network research is
kept in `pkg/coding_agent/tools/research` so embedded hosts do not pay for the
browser and HTML extraction stack unless they explicitly compose its provider.
The `modu_code` CLI includes that provider and continues to expose
`web_search` and `web_fetch`.

Multimodal hosts can call `PromptWithImages`. `ImageContent.Data` is base64 content, not a file path; the session persists it and the provider adapter converts it to the active protocol:

```go
err := session.PromptWithImages(context.Background(), "Explain this screenshot", []types.ImageContent{{
	Type:     "image",
	MimeType: "image/png",
	Data:     base64.StdEncoding.EncodeToString(pngBytes),
}})
```

Persisted tool results include parallel batch metadata, allowing resumed transcript UIs to restore grouped tool calls.
For timeline replay, use `GetSessionTranscript`; it returns persisted messages
and compaction markers from the current session path in causal order.
`GetMessages` instead returns the compacted context currently sent to the model.

Use `FollowUpWithImages` or `SteerWithImages` while a task is active. These methods reject images when the model explicitly lacks image input support or configuration enables `blockImages`.

Interactive hosts can build a non-persistent side conversation with `BeginSideThread`, `PromptSideThread`, and `GetSideThreadSnapshot`. The side thread inherits a copy of the current messages, model, thinking level, system prompt, and tools, but it has no session manager or context manager. Its messages therefore do not change the main transcript, session tree, token accounting, or compaction state. `AbortSideThread` cancels its active turn; `ClearSideThread` discards it. Tool side effects remain real.

Cross-process communication is opt-in through `CodingSessionOptions.EnableSessionIPC`. A single local app-server exposes one UDS, while `session_list` includes both live and persisted sessions. `session_send` resumes a `notLoaded` target, starts an idle target, or queues a follow-up for a busy target. Interactive `modu_code` starts and connects to the daemon automatically; print, RPC, ACP, and embedded sessions remain disabled by default. See [Session IPC](sessionipc/README.md) for lifecycle, protocol, and security details.

## Operational controls

- Background Bash calls return a `bash_id`; the default coding tool provider also registers `bash_output` and `kill_bash`.
- `ConfigureProjectTrust` and `GetProjectTrust` manage persistent or process-only cwd trust. Dangerous Bash and explicit deny rules still take precedence.
- Trusted configuration may define `PreToolUse`, `PostToolUse`, and `UserPromptSubmit` shell hooks.
- `GetRewindPoints` and `Rewind` restore in-process checkpoints produced by built-in `write` / `edit` turns. Bash, MCP, network, and external changes are outside that boundary.
- Prompt-template arguments support shell quoting, positional values, defaults, and slices while retaining legacy `{{input}}` / `{{args}}`.
- `/skill-creator` is bundled from Anthropic's official Skill without Modu-specific rewrites. Its full resource directory is materialized under `<agent-dir>/builtin-skills/<revision>/skill-creator`; project, user, and package skills with the same name override it. Claude Code-specific evaluation steps still require the `claude` CLI and their original Python dependencies.
- Memory above the configured threshold is summarized in the background without changing source notes; `OrganizeMemory` and `/memory` expose manual execution and status.
- Session IPC reaches live and persisted sessions owned by the same local user through one app-server. It has no persistent offline queue, broadcast, remote transport, or preemption.

## Documentation

- [Detailed reference](../../docs/reference/coding-agent.md) — features, tools, configuration, runtime files, and request flow. This document is currently maintained in Chinese.
- [Architecture](../../docs/architecture/coding-agent.md) — layer boundaries, dependency rules, and known violations.
- [Subagent parity](../../docs/reference/subagent-parity.md) — implemented, partial, and deferred `pi-subagents` compatibility.
