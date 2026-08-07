// Content for the generated per-topic docs pages.
//
// Kept apart from build-docs-pages.mjs so editing a page means editing prose,
// not a template. Sourced from docs/guides/modu-code.md and the provider
// configuration in pkg/provider — keep them in sync when those change.

const cliSectionsZh = [
  {
    id: 'install',
    title: '安装',
    blocks: [
      { p: '需要 Go（具体版本以仓库中的 <code>go.mod</code> 为准）。可以直接从源码运行：' },
      { code: 'go run ./cmd/modu_code', shell: true },
      { p: '也可以编译成单个二进制放进 <code>PATH</code>：' },
      { code: 'go build -o modu_code ./cmd/modu_code\n./modu_code', shell: true },
      { p: '下文的 <code>go run ./cmd/modu_code</code> 与 <code>modu_code</code> 两种写法等价。' }
    ]
  },
  {
    id: 'quickstart',
    title: '零配置起步',
    blocks: [
      {
        p: '最快的路径是设一个环境变量，不写任何配置文件。<code>modu_code</code> 按下面的顺序挑选 provider，第一个命中的生效：'
      },
      {
        list: [
          '<code>ANTHROPIC_API_KEY</code> — Anthropic（走 OpenAI 兼容端点）',
          '<code>OPENAI_API_KEY</code> — OpenAI Responses，模型取 <code>$OPENAI_MODEL</code>，默认 <code>gpt-4o</code>',
          '<code>DEEPSEEK_API_KEY</code> — DeepSeek，模型取 <code>$DEEPSEEK_MODEL</code>，默认 <code>deepseek-chat</code>',
          '<code>OLLAMA_HOST</code> — Ollama，模型取 <code>$OLLAMA_MODEL</code>（必填）'
        ]
      },
      { code: 'export DEEPSEEK_API_KEY=sk-xxx\ngo run ./cmd/modu_code', shell: true },
      {
        note:
          '一个 provider 都没配也能进 TUI：启动后会提示用 <code>/config</code> 现场配置 provider、API key 和模型。需要多模型、role、reasoning 这些，见<a href="/docs/models">模型与 Provider 配置</a>。'
      }
    ]
  },
  {
    id: 'modes',
    title: '运行模式与命令行参数',
    blocks: [
      {
        p:
          '不带 <code>-p</code>/<code>--rpc</code>/<code>--acp</code> 时是默认的交互 TUI，其余三种是非交互模式，供脚本或编辑器集成使用。'
      },
      {
        list: [
          '<code>（无）</code> — 交互 TUI（默认）',
          '<code>-p "&lt;prompt&gt;"</code> — print 模式：发送一条 prompt，把结果输出到 stdout 后退出',
          '<code>--json</code> — 配合 <code>-p</code>：输出 NDJSON 事件流而非纯文本',
          '<code>--rpc</code> — RPC 模式：stdin/stdout 上的 JSON-line 协议',
          '<code>--acp</code> — ACP stdio server：JSON-RPC 2.0 LDJSON，供 Zed 等 ACP 客户端接入',
          '<code>--no-approve</code> — 跳过工具执行的人工确认，自动放行全部工具',
          '<code>--resume &lt;id&gt;</code> — 恢复已保存的 session（完整 id 或唯一前缀均可）',
          '<code>--worktree</code> — 在隔离的 git worktree 中启动'
        ]
      },
      { code: 'go run ./cmd/modu_code -p "总结 cmd/modu_code 的职责" --no-approve', shell: true },
      {
        note:
          '非交互模式在没有配置 provider 时会直接报错退出，而不是进入配置引导——脚本场景需要先把模型配好。'
      }
    ]
  },
  {
    id: 'sessions',
    title: '会话与恢复',
    blocks: [
      {
        p:
          '默认每次启动都会创建新的 session id，不会自动带入同一路径上一次的对话上下文。退出时终端会打印当前 session 的 id 和恢复命令：'
      },
      { code: 'go run ./cmd/modu_code --resume <session-id>', shell: true },
      { p: 'id 支持唯一前缀，不必粘贴完整的 UUID。' }
    ]
  },
  {
    id: 'keys',
    title: '交互与快捷键',
    blocks: [
      {
        p: 'Agent 正在跑的时候，输入框里打字有两种送达方式，状态栏会实时提示：'
      },
      {
        list: [
          '<strong>Enter</strong> — 插话：消息在下一个工具边界加入当前这一轮，正在执行的工作不会被丢弃',
          '<strong>⇧Enter</strong> — 排队：消息等这一轮结束后再处理',
          '<strong>Esc</strong> — 中断当前任务',
          '<strong>Ctrl+V</strong> — 粘贴剪贴板里的图片（也可以把图片文件拖进终端）'
        ]
      },
      {
        note:
          '在 macOS 终端里请用 <strong>Ctrl+V</strong> 而不是 Cmd+V 粘贴图片：Cmd+V 会被终端自己拦截，程序收不到这个按键。'
      }
    ]
  }
];

const cliSectionsEn = [
  {
    id: 'install',
    title: 'Install',
    blocks: [
      { p: 'Requires Go (see <code>go.mod</code> in the repository for the exact version). Run straight from source:' },
      { code: 'go run ./cmd/modu_code', shell: true },
      { p: 'Or build a single binary and put it on your <code>PATH</code>:' },
      { code: 'go build -o modu_code ./cmd/modu_code\n./modu_code', shell: true },
      { p: 'Below, <code>go run ./cmd/modu_code</code> and <code>modu_code</code> are interchangeable.' }
    ]
  },
  {
    id: 'quickstart',
    title: 'Zero-config start',
    blocks: [
      {
        p:
          'The fastest path is one environment variable and no config file. <code>modu_code</code> picks the first provider that matches, in this order:'
      },
      {
        list: [
          '<code>ANTHROPIC_API_KEY</code> — Anthropic (over the OpenAI-compatible endpoint)',
          '<code>OPENAI_API_KEY</code> — OpenAI Responses; model from <code>$OPENAI_MODEL</code>, default <code>gpt-4o</code>',
          '<code>DEEPSEEK_API_KEY</code> — DeepSeek; model from <code>$DEEPSEEK_MODEL</code>, default <code>deepseek-chat</code>',
          '<code>OLLAMA_HOST</code> — Ollama; model from <code>$OLLAMA_MODEL</code> (required)'
        ]
      },
      { code: 'export DEEPSEEK_API_KEY=sk-xxx\ngo run ./cmd/modu_code', shell: true },
      {
        note:
          'You can enter the TUI with no provider at all: it offers <code>/config</code> to set up a provider, API key, and model on the spot. For multiple models, roles, and reasoning levels see <a href="/en/docs/models">Models &amp; providers</a>.'
      }
    ]
  },
  {
    id: 'modes',
    title: 'Run modes and flags',
    blocks: [
      {
        p:
          'Without <code>-p</code>/<code>--rpc</code>/<code>--acp</code> you get the interactive TUI. The other three are non-interactive modes for scripts and editor integrations.'
      },
      {
        list: [
          '<code>(none)</code> — interactive TUI (default)',
          '<code>-p "&lt;prompt&gt;"</code> — print mode: send one prompt, write the result to stdout, exit',
          '<code>--json</code> — with <code>-p</code>: emit an NDJSON event stream instead of plain text',
          '<code>--rpc</code> — RPC mode: JSON-line protocol over stdin/stdout',
          '<code>--acp</code> — ACP stdio server: JSON-RPC 2.0 LDJSON, for ACP clients such as Zed',
          '<code>--no-approve</code> — skip tool-execution approval and auto-allow every tool',
          '<code>--resume &lt;id&gt;</code> — resume a saved session (full id or a unique prefix)',
          '<code>--worktree</code> — start inside an isolated git worktree'
        ]
      },
      { code: 'go run ./cmd/modu_code -p "Summarize what cmd/modu_code does" --no-approve', shell: true },
      {
        note:
          'The non-interactive modes exit with an error when no provider is configured rather than opening the setup flow — scripted runs need the model configured up front.'
      }
    ]
  },
  {
    id: 'sessions',
    title: 'Sessions and resuming',
    blocks: [
      {
        p:
          'Every start creates a new session id; the previous conversation in the same directory is not picked up automatically. On exit the terminal prints the session id and the command to resume it:'
      },
      { code: 'go run ./cmd/modu_code --resume <session-id>', shell: true },
      { p: 'A unique prefix works, so you do not have to paste the whole UUID.' }
    ]
  },
  {
    id: 'keys',
    title: 'Keys while it runs',
    blocks: [
      { p: 'While the agent is working, what you type can be delivered two ways. The status line shows which is which:' },
      {
        list: [
          '<strong>Enter</strong> — interject: the message joins the current turn at its next tool boundary, and work in flight is not thrown away',
          '<strong>⇧Enter</strong> — queue: the message waits until the turn finishes',
          '<strong>Esc</strong> — interrupt the running task',
          '<strong>Ctrl+V</strong> — paste an image from the clipboard (dragging an image file into the terminal also works)'
        ]
      },
      {
        note:
          'On macOS use <strong>Ctrl+V</strong> rather than Cmd+V to paste an image: the terminal intercepts Cmd+V itself, so the program never sees the keypress.'
      }
    ]
  }
];

const modelSectionsZh = [
  {
    id: 'env',
    title: '最简单：环境变量',
    blocks: [
      { p: '只用一个模型时不需要配置文件，设一个环境变量即可，命中顺序见<a href="/docs/cli">命令行指南</a>。' },
      { code: 'export DEEPSEEK_API_KEY=sk-xxx\ngo run ./cmd/modu_code', shell: true },
      {
        p:
          '辅助变量：<code>OPENAI_BASE_URL</code> 覆盖 OpenAI Responses 的 base URL；<code>THINKING_LEVEL</code> 设推理档位（<code>off|low|medium|high</code>，默认 <code>off</code>）。'
      }
    ]
  },
  {
    id: 'config',
    title: '配置文件：多模型',
    blocks: [
      {
        p:
          '需要多个模型、模型切换或专用 role 时，写 <code>~/.modu/config.toml</code>。它的优先级高于环境变量。'
      },
      {
        caption: '~/.modu/config.toml',
        code: `version = 2
active = "local-qwen"
scopedModels = ["local-qwen", "deepseek"]

[roles]
summary = "local-qwen"
dispatcher = "deepseek"

[reasoning]
level = "off"

[providers.lmstudio]
type = "openai-compatible"
baseUrl = "http://127.0.0.1:1234/v1"
apiKey = "lm-studio"

[providers.deepseek]
type = "openai-compatible"
baseUrl = "https://api.deepseek.com/v1"
apiKeyEnv = "DEEPSEEK_API_KEY"

[[models]]
name = "local-qwen"
description = "local coding model"
provider = "lmstudio"
model = "qwen/qwen3.6-35b-a3b"
capabilities = ["tools"]
contextWindow = 262144

[[models]]
name = "deepseek"
description = "remote fallback model"
provider = "deepseek"
model = "deepseek-chat"
capabilities = ["tools"]
contextWindow = 1000000`
      }
    ]
  },
  {
    id: 'fields',
    title: '各字段的职责',
    blocks: [
      {
        list: [
          '<code>providers</code> — 只描述<strong>怎么连</strong>：base URL、API key。<code>apiKeyEnv</code> 让 key 留在环境变量里，不落到配置文件。',
          '<code>models</code> — 只描述<strong>有哪些模型可选</strong>，每个绑定一个 provider。',
          '<code>active</code> — 默认使用的模型。',
          '<code>scopedModels</code> — 模型循环切换的范围。',
          '<code>roles</code> — 给 summary、dispatcher 这类专用场景指定模型。',
          '<code>capabilities</code> — 模型支持的能力，例如 <code>tools</code>、<code>image</code>。带 <code>image</code> 才允许发图片。',
          '<code>contextWindow</code> — 显式覆盖上下文窗口；不写时内置厂商会按其最大窗口补默认值。'
        ]
      }
    ]
  },
  {
    id: 'responses',
    title: 'OpenAI Responses',
    blocks: [
      { p: 'OpenAI Responses 用的是独立的 provider 类型：' },
      {
        caption: '~/.modu/config.toml',
        code: `[providers.openai]
type = "openai-responses"
baseUrl = "https://api.openai.com/v1"
apiKeyEnv = "OPENAI_API_KEY"

[[models]]
name = "gpt-5"
provider = "openai"
model = "gpt-5"
capabilities = ["text", "image", "tools"]`
      }
    ]
  },
  {
    id: 'switching',
    title: '运行中切换模型',
    blocks: [
      {
        p:
          '在 TUI 里用 <code>/model</code> 切换，或用 <code>/config</code> 现场增删 provider 与模型，无需重启。切换范围由 <code>scopedModels</code> 决定。'
      }
    ]
  }
];

const modelSectionsEn = [
  {
    id: 'env',
    title: 'Simplest: environment variables',
    blocks: [
      {
        p:
          'With a single model you need no config file at all — just one environment variable. See the <a href="/en/docs/cli">CLI guide</a> for the resolution order.'
      },
      { code: 'export DEEPSEEK_API_KEY=sk-xxx\ngo run ./cmd/modu_code', shell: true },
      {
        p:
          'Helpers: <code>OPENAI_BASE_URL</code> overrides the OpenAI Responses base URL; <code>THINKING_LEVEL</code> sets the reasoning level (<code>off|low|medium|high</code>, default <code>off</code>).'
      }
    ]
  },
  {
    id: 'config',
    title: 'Config file: multiple models',
    blocks: [
      {
        p:
          'For several models, model switching, or dedicated roles, write <code>~/.modu/config.toml</code>. It takes precedence over the environment variables.'
      },
      {
        caption: '~/.modu/config.toml',
        code: `version = 2
active = "local-qwen"
scopedModels = ["local-qwen", "deepseek"]

[roles]
summary = "local-qwen"
dispatcher = "deepseek"

[reasoning]
level = "off"

[providers.lmstudio]
type = "openai-compatible"
baseUrl = "http://127.0.0.1:1234/v1"
apiKey = "lm-studio"

[providers.deepseek]
type = "openai-compatible"
baseUrl = "https://api.deepseek.com/v1"
apiKeyEnv = "DEEPSEEK_API_KEY"

[[models]]
name = "local-qwen"
description = "local coding model"
provider = "lmstudio"
model = "qwen/qwen3.6-35b-a3b"
capabilities = ["tools"]
contextWindow = 262144

[[models]]
name = "deepseek"
description = "remote fallback model"
provider = "deepseek"
model = "deepseek-chat"
capabilities = ["tools"]
contextWindow = 1000000`
      }
    ]
  },
  {
    id: 'fields',
    title: 'What each field is for',
    blocks: [
      {
        list: [
          '<code>providers</code> — describes <strong>how to connect</strong> only: base URL and API key. <code>apiKeyEnv</code> keeps the key in the environment instead of the file.',
          '<code>models</code> — describes <strong>which models are available</strong>, each bound to one provider.',
          '<code>active</code> — the model used by default.',
          '<code>scopedModels</code> — the set cycled through when switching models.',
          '<code>roles</code> — assigns models to dedicated jobs such as summary and dispatcher.',
          '<code>capabilities</code> — what the model supports, e.g. <code>tools</code>, <code>image</code>. Sending images requires <code>image</code>.',
          '<code>contextWindow</code> — overrides the context window; omitted, built-in vendors fall back to their largest.'
        ]
      }
    ]
  },
  {
    id: 'responses',
    title: 'OpenAI Responses',
    blocks: [
      { p: 'OpenAI Responses uses its own provider type:' },
      {
        caption: '~/.modu/config.toml',
        code: `[providers.openai]
type = "openai-responses"
baseUrl = "https://api.openai.com/v1"
apiKeyEnv = "OPENAI_API_KEY"

[[models]]
name = "gpt-5"
provider = "openai"
model = "gpt-5"
capabilities = ["text", "image", "tools"]`
      }
    ]
  },
  {
    id: 'switching',
    title: 'Switching models at runtime',
    blocks: [
      {
        p:
          'Use <code>/model</code> in the TUI to switch, or <code>/config</code> to add and remove providers and models on the spot — no restart. <code>scopedModels</code> defines what you cycle through.'
      }
    ]
  }
];

export const pages = [
  {
    lang: 'zh-CN',
    path: '/docs/cli',
    title: 'modu_code 命令行指南 · Modu 文档',
    heading: '在终端里使用 modu_code',
    crumb: '命令行 (modu_code)',
    eyebrow: 'CLI',
    description:
      'modu_code 命令行指南：安装、零配置起步、print/RPC/ACP 运行模式、会话恢复，以及运行中的插话与排队快捷键。',
    lede: 'modu_code 是 Modu 的终端编码 Agent。这一页覆盖安装、四种运行模式、会话恢复，以及 Agent 运行时的按键交互。',
    sections: cliSectionsZh
  },
  {
    lang: 'en',
    path: '/docs/cli',
    title: 'modu_code CLI guide · Modu Docs',
    heading: 'Using modu_code in the terminal',
    crumb: 'CLI (modu_code)',
    eyebrow: 'CLI',
    description:
      'The modu_code CLI guide: install, zero-config start, the print/RPC/ACP run modes, resuming sessions, and the keys for interjecting or queueing while it runs.',
    lede:
      'modu_code is Modu’s terminal coding agent. This page covers installation, the four run modes, resuming sessions, and what the keys do while the agent works.',
    sections: cliSectionsEn
  },
  {
    lang: 'zh-CN',
    path: '/docs/models',
    title: '模型与 Provider 配置 · Modu 文档',
    heading: '配置模型与 Provider',
    crumb: '模型与 Provider 配置',
    eyebrow: 'CONFIG',
    description:
      'Modu 的模型配置：环境变量快速起步，以及 ~/.modu/config.toml 中的多模型、provider、role、capabilities 与上下文窗口设置。',
    lede: '从一个环境变量起步，到用 config.toml 管理多个模型、provider 和专用 role。',
    sections: modelSectionsZh
  },
  {
    lang: 'en',
    path: '/docs/models',
    title: 'Models & providers · Modu Docs',
    heading: 'Configuring models and providers',
    crumb: 'Models & providers',
    eyebrow: 'CONFIG',
    description:
      'Model configuration for Modu: the environment-variable quick start, and multiple models, providers, roles, capabilities, and context windows in ~/.modu/config.toml.',
    lede:
      'From a single environment variable to managing several models, providers, and dedicated roles in config.toml.',
    sections: modelSectionsEn
  }
];
