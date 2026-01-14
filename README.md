<p align="center">
  <img src="logo.png" alt="Modu Logo" width="200">
</p>

<h1 align="center">modu, 中文名"毛肚"</h1>

<p align="center">
  <strong>🚀 快捷高效搭建 Agent 应用的 Go 基础设施工具库</strong>
</p>

---

## 📦 安装

```bash
go get github.com/crosszan/modu
```

## 🗂 项目结构

```
modu/
├── repos/                  # 仓库层 (业务抽象)
│   ├── gen_image_repo/     # 图片生成
│   ├── notebooklm/         # 基于Playwright 封装的Google NotebookLM
│   └── scraper/            # 网页爬虫
├── pkg/                    # 工具包
│   ├── env/                # 环境变量加载
│   ├── playwright/         # Playwright 封装
│   └── utils/              # 通用工具函数
├── vo/                     # 值对象
├── consts/                 # 常量定义
└── examples/               # 使用示例
```

## 📚 模块列表

### repos/ - 业务仓库

| 模块 | 描述 |
|------|------|
| [`repos/notebooklm`](repos/notebooklm/README.md) | Google NotebookLM 非官方 SDK，支持 Notebook/Source/Artifact/Chat |
| [`repos/gen_image_repo`](repos/gen_image_repo/README.md) | 图片生成抽象层，支持 Gemini 等 Provider |
| `repos/scraper` | 网页爬虫，支持 Hacker News 等 |

### pkg/ - 工具包

| 模块 | 描述 |
|------|------|
| [`pkg/env`](pkg/env/README.md) | 环境变量加载库，支持 `.env` 文件 |
| `pkg/playwright` | Playwright 浏览器自动化封装 |
| `pkg/utils` | 图片保存等工具函数 |

## 🚀 快速开始

### 环境变量加载

```go
import "github.com/crosszan/modu/pkg/env"

env.Load()                              // 加载 .env
env.Load(env.WithFile(".env.local"))    // 加载指定文件
env.Load(env.WithOverride())            // 覆盖已有变量

apiKey := env.GetDefault("API_KEY", "default")
```

### NotebookLM

```go
import "github.com/crosszan/modu/repos/notebooklm"

// 登录
notebooklm.Login()

// 创建客户端
client, _ := notebooklm.NewClientFromStorage("")

// 列出 Notebook
notebooks, _ := client.ListNotebooks(ctx)

// 生成音频
client.GenerateAudio(ctx, notebookID, vo.AudioFormatDeepDive, vo.AudioLengthDefault)

// 提问
result, _ := client.Ask(ctx, notebookID, "总结内容", nil)
```

### 图片生成

```go
import genimagerepo "github.com/crosszan/modu/repos/gen_image_repo"

repo := genimagerepo.NewGeminiImageImpl("https://generativelanguage.googleapis.com", "api-key")

result, _ := repo.Generate(ctx, &genimagevo.GenImageRequest{
    UserPrompt: "一只可爱的猫咪",
})

genimagerepo.SaveAllImages(result, "./output")
```

## 📄 License

MIT License

