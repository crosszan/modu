# modu_code

`modu_code` 是运行在终端中的 AI 编程助手，能在当前工作目录中读写文件、搜索代码并执行命令。

默认工具集还包含：

- `web_search`：搜索网页并返回标题、URL 和摘要；无额外配置时使用无需 Key 的 Bing RSS，也可通过 `[settings.webSearch]` 选择 Exa、Tavily、Brave、Firecrawl 或显式使用 DuckDuckGo。未指定 provider 时会优先按现有 API Key 自动选择，调用时可用 `allowed_domains` / `blocked_domains` 过滤来源。
- `web_fetch`：抓取 HTTP/HTTPS 页面并提取可读 Markdown；可通过 `[settings.webFetch]` 使用 Firecrawl Scrape。普通 HTTP 抓取拒绝跨 origin 重定向。
- `bash_output` / `kill_bash`：管理 `bash` 以 `run_in_background=true` 启动的任务；`bash` 返回稳定的 `bash_id`，日志按次增量读取。

网络工具与其他工具一样经过现有权限规则和交互审批。Provider、API Key 和自定义 Endpoint 的配置格式见[详细文档](../../docs/reference/coding-agent.md#配置)。

## Trust、Hooks 与回退

- `/trust status|on|off|once` 查看或设置当前目录信任状态。持久化决定写入 `~/.modu/trust.json`；`once` 只在本进程有效。可信项目可自动执行 `write`、`edit`、`kill_bash` 和普通 Bash，但危险 Bash 仍然要求确认，显式 deny 规则仍然优先。
- 配置中的 `PreToolUse`、`PostToolUse`、`UserPromptSubmit` shell hooks 只会在项目可信时执行。Hook 使用 JSON stdin/stdout 协议，能阻断、改写工具参数或 prompt、补充上下文。
- `/rewind` 列出本进程内的文件检查点，`/rewind N` 回退第 N 个检查点及其后的内置 `write/edit` 修改，同时把对话移动到该轮之前。Bash、MCP、网络及其他外部副作用不在回退范围内；文件被外部修改后，回退会拒绝覆盖。

## 临时旁路对话

在交互 TUI 中输入 `/btw <问题>`，可以基于当前主会话上下文开启一条临时旁路对话。之后直接输入可连续追问，`/exit` 或 `/quit` 返回主会话；单独输入 `/btw` 可恢复本进程最近一条旁路对话。

旁路消息不写入主会话、不参与主会话压缩，也不会保存到 session 文件。旁路对话仍可使用当前工具，文件、命令和网络调用产生的副作用不会随退出撤销。

## 安装与启动

需要 Go，具体版本见仓库根目录的 `go.mod`。

```bash
go run ./cmd/modu_code
```

也可以编译后运行：

```bash
go build -o modu_code ./cmd/modu_code
./modu_code
```

启动前至少配置一个模型。以下示例使用 DeepSeek：

```bash
export DEEPSEEK_API_KEY=sk-xxx
go run ./cmd/modu_code
```

没有 provider 时，交互 TUI 会打开配置引导；print、RPC 和 ACP 等非交互模式会直接退出。

## 常用参数

| 参数 | 用途 |
|---|---|
| `-p "<prompt>"` | 执行一次 prompt，输出结果后退出 |
| `--json` | 与 `-p` 配合，输出 NDJSON 事件流 |
| `--rpc` | 通过 stdin/stdout 使用 JSON-line RPC |
| `--acp` | 作为 ACP stdio server 运行 |
| `--no-approve` | 自动允许工具执行；仅在你信任输入和工作区时使用 |
| `--resume [id]` | 用完整 session id 或唯一前缀恢复会话；不带 id 时恢复当前目录最近的 session |
| `--worktree` | 在隔离的 Git worktree 中启动 |

一次性执行示例：

```bash
go run ./cmd/modu_code -p "总结 cmd/modu_code 的职责" --no-approve
```

`--no-approve` 会跳过工具审批，不适合处理不可信 prompt。

## 详细文档

模型配置、TUI 快捷键、斜杠命令、渠道、会话和扩展说明见 [`docs/guides/modu-code.md`](../../docs/guides/modu-code.md)。引擎内部机制见 [`pkg/coding_agent/README.md`](../../pkg/coding_agent/README.md)。
