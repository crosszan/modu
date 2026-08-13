# coding_agent

[English](README.md) | [中文](README_zh.md)

`coding_agent` 把通用的 `pkg/agent` 循环组装成编码会话，负责文件与 Shell 工具、会话持久化、上下文压缩、扩展系统，以及供宿主读取的运行时状态。

需要开发编码 Agent 宿主时使用这个包；如果只需要带自定义工具的 LLM 循环，直接使用 `pkg/agent`。`coding_agent` 还会接管文件访问、会话文件、配置发现和扩展生命周期，这些行为不属于通用 Agent 内核。

## 最短示例

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

	if err := session.Prompt(context.Background(), "解释 main.go"); err != nil {
		panic(err)
	}
	session.WaitForIdle()
}
```

创建会话前必须注册模型 Provider。`Cwd` 决定工具解析文件的基准目录，也决定项目配置和资源的发现范围。工具可能修改该工作区，宿主必须根据运行环境配置审批策略。

多模态宿主可以调用 `PromptWithImages`。`ImageContent.Data` 使用 base64，不是文件路径；会话会保存图片内容，provider adapter 再转换成目标协议：

```go
err := session.PromptWithImages(context.Background(), "解释这张截图", []types.ImageContent{{
	Type:     "image",
	MimeType: "image/png",
	Data:     base64.StdEncoding.EncodeToString(pngBytes),
}})
```

持久化的工具结果包含并行批次元数据，恢复会话后可继续按批次展示工具调用。

任务运行中使用 `FollowUpWithImages` 或 `SteerWithImages`。模型显式声明不支持图片，或配置启用 `blockImages` 时，这三个入口会返回错误。

交互式宿主可通过 `BeginSideThread`、`PromptSideThread` 和 `GetSideThreadSnapshot` 实现不持久化的旁路对话。旁路会复制当前消息、模型、推理档位、system prompt 和工具，但不挂载 session manager 与 context manager，因此不会改变主 transcript、session tree、token 统计和压缩状态。`AbortSideThread` 取消当前旁路回合，`ClearSideThread` 丢弃旁路历史；工具产生的真实副作用不在丢弃范围内。

跨进程会话通信可通过 `CodingSessionOptions.EnableSessionIPC` 显式启用。单一本地 app-server 只暴露一个 UDS；`session_list` 同时发现在线和持久化 Session，`session_send` 会恢复 `notLoaded` 目标、启动空闲目标，或给忙碌目标追加 follow-up。交互式 `modu_code` 会自动启动并连接 daemon；print、RPC、ACP 和嵌入式会话默认关闭。生命周期、协议与安全边界见 [Session IPC](sessionipc/README.md)。

## 运行控制

- 后台 Bash 返回 `bash_id`；默认编码工具集同时注册 `bash_output` 和 `kill_bash`。
- `ConfigureProjectTrust` / `GetProjectTrust` 管理 cwd 的持久或进程内信任。危险 Bash 和显式 deny 规则仍然优先。
- 已信任项目可配置 `PreToolUse`、`PostToolUse`、`UserPromptSubmit` shell hooks。
- `GetRewindPoints` / `Rewind` 恢复本进程内由内置 `write` / `edit` 轮次生成的检查点；Bash、MCP、网络和外部修改不在回退边界内。
- Prompt template 参数支持 shell 引号、位置参数、默认值和切片；旧 `{{input}}` / `{{args}}` 继续可用。
- `/skill-creator` 原样内置 Anthropic 官方 Skill，完整资源目录在运行时落到 `<agent-dir>/builtin-skills/<revision>/skill-creator`。项目、用户和资源包中的同名 Skill 可以覆盖内置版本；其中面向 Claude Code 的评测步骤仍依赖 `claude` CLI 及原有 Python 依赖。
- Memory 达到配置阈值后会后台更新 bounded summary，原始记忆不变；`OrganizeMemory` 和 `/memory` 提供手动入口与状态。
- Session IPC 通过一个 app-server 连接同机、同用户的在线与持久化会话；不提供持久化离线队列、广播、远程传输或抢占当前任务。

## 文档

- [详细参考](../../docs/reference/coding-agent.md)：功能、工具、配置、运行时文件和请求流程。
- [架构说明](../../docs/architecture/coding-agent.md)：分层边界、依赖规则和已知违规。
- [Subagent 兼容进度](../../docs/reference/subagent-parity.md)：与 `pi-subagents` 对齐的已实现、部分实现和暂缓项。
