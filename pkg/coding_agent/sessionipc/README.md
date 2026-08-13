# Session IPC

`sessionipc` 是 Modu 的本地 app-server 控制面。一个长期运行的 `modu_code app-server` 监听一个 Unix domain socket；所有交互式 Coding Session 都作为客户端注册到该服务。app-server 同时读取持久化 Session 索引，因此 `session_list` 不只列出当前在线会话，也列出尚未加载的历史会话。

这套结构采用 Codex app-server 的核心进程模型：单服务、单 socket、WebSocket over UDS、双向 JSON-RPC、历史 thread 按需恢复。它不是 Codex 协议的兼容实现。

## 使用

交互式 `modu_code` 会自动确保 app-server 已启动，再连接 `<agent-dir>/ipc/ipc.sock`。默认 `agent-dir` 为 `~/.modu`。print、RPC、ACP 和直接嵌入的 `CodingSession` 默认不接入。

```text
modu_code app-server start
modu_code app-server status
modu_code app-server stop
modu_code app-server serve   # 前台运行，便于调试或由进程管理器托管
```

嵌入式宿主需要先启动 `sessionipc.Server`，再为会话设置：

```go
session, err := coding_agent.NewCodingSession(coding_agent.CodingSessionOptions{
    Cwd:                 cwd,
    AgentDir:            agentDir,
    Model:               model,
    EnableSessionIPC:    true,
    SessionIPCRuntimeDir: sessionipc.DefaultRuntimeDir(agentDir),
})
```

启用后注册两个工具：

- `session_list`：返回 Session ID、cwd、名称和 `notLoaded` / `idle` / `busy` 状态。
- `session_send`：向指定 Session 投递普通用户消息。`notLoaded` 目标先恢复，`idle` 目标启动新一轮，`busy` 目标加入 follow-up 队列。

消息会带有来源与 ID：`[Message from session <id>; id=<message-id>]`。它会进入正常 transcript，不是隐藏控制指令，也不是离线队列记录。

## 进程与文件

默认目录结构：

```text
~/.modu/ipc/
├── app-server.lock
├── app-server.pid
└── ipc.sock
```

`app-server.lock` 保证同一 `AgentDir` 只有一个服务实例。异常退出后，下一次启动会在确认旧 socket 已失活后回收它。交互式 `modu_code` 启动时会先探测服务；未运行时创建脱离终端的 daemon，并等待初始化握手成功。

macOS 的 UDS 路径长度很短。如果自定义 `AgentDir` 使 socket 路径接近上限，`DefaultRuntimeDir` 会退回到 `/tmp/modu-app-server-<uid>-<agent-dir-hash>`。

app-server 的 Session backend 使用 `session.ListAll` 枚举历史 JSONL。列表操作不会加载模型或 Session；向 `notLoaded` 目标发送消息时，backend 才按 Session ID 恢复它并启动 turn。由 daemon 恢复的 Session 会保持加载，直到 daemon 退出，或该 Session 空闲时被新启动的交互客户端接管。

后台恢复不会擅自扩大工具权限：只有历史 Session 已保存 `--no-approve` 时才沿用自动审批；否则需要审批的工具会被拒绝，因为当前版本没有把审批请求转发到交互式 UI。

## 线协议

客户端通过 UDS 完成 HTTP WebSocket upgrade，之后每个 text frame 是一个 JSON-RPC 风格对象。与 Codex app-server 一样，消息不带 `"jsonrpc":"2.0"` 字段。

连接必须先发送：

```json
{"id":1,"method":"initialize","params":{"clientInfo":{"name":"modu_code","version":"1"}}}
{"method":"initialized","params":null}
```

主要客户端请求：

```json
{"id":2,"method":"session/register","params":{"sessionId":"...","cwd":"/repo"}}
{"id":3,"method":"thread/list","params":{}}
{"id":4,"method":"turn/start","params":{"messageId":"...","threadId":"...","message":"检查 parser.go","sentAt":1786581000000}}
```

app-server 也会在同一连接上反向调用在线客户端：

- `thread/status`：读取实时 busy 状态。
- `turn/deliver`：向在线目标启动或追加消息。

投递结果的 `status` 为 `started`、`queued` 或 `duplicate`。服务保留最近 1024 个 `messageId` 做进程内去重；daemon 重启后该去重窗口不会恢复。

## 安全与边界

- runtime 目录权限为 `0700`，socket 与 PID 文件为 `0600`。
- macOS 使用 `LOCAL_PEERCRED`，Linux 使用 `SO_PEERCRED` 校验对端 UID。
- socket、目录和遗留路径必须属于当前用户；符号链接或不安全路径会被拒绝。
- WebSocket frame 和消息正文都有上限；正文最多 64 KiB，请求默认超时 5 秒。
- 不允许 Session 向自身发送消息。
- 当前只支持同机、同用户、点对点投递；不支持远程传输、广播、持久化离线队列、abort 或 steer。
