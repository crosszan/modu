<p align="center">
  <img src="logo.png" alt="Modu Logo" width="200">
</p>

<h1 align="center">Modu 毛肚</h1>

<p align="center">
  <strong>🚀 快捷高效搭建 Agent 应用的 Go 基础设施工具库</strong>
</p>

<p align="center">
  <a href="#特性">特性</a> •
  <a href="#安装">安装</a> •
  <a href="#快速开始">快速开始</a> •
  <a href="#包列表">包列表</a> •
  <a href="#贡献">贡献</a>
</p>

---

## ✨ 特性

- 🔌 **模块化设计** - 每个包独立可用，按需引入
- 🛠 **开箱即用** - 提供生产级别的默认配置
- ⚡ **高性能** - 针对 Agent 应用场景优化
- 🎯 **类型安全** - 完整的 Go 类型定义
- 📦 **零依赖** - 核心包仅依赖标准库

## 📦 安装

```bash
go get github.com/crosszan/modu
```

## 🚀 快速开始

### 图片生成 (nano_banana_pro)

```go
package main

import (
    "fmt"
    "github.com/crosszan/modu/pkg/nano_banana_pro"
)

func main() {
    // 创建客户端
    client := nano_banana_pro.NewClient(
        "http://127.0.0.1:8045",
        "your-api-key",
    )

    // 生成图片
    result, err := client.GenerateImage("a beautiful sunset over mountains")
    if err != nil {
        panic(err)
    }

    // 保存所有图片
    files, _ := nano_banana_pro.SaveAllImages(result, "./output", "image")
    for _, f := range files {
        fmt.Printf("✓ 已保存: %s\n", f)
    }
}
```

## 📚 包列表

| 包名 | 描述 | 状态 |
|------|------|------|
| `pkg/nano_banana_pro` | Gemini 图片生成 API 封装 | ✅ 可用 |

## 🔧 配置选项

大多数包支持选项模式配置：

```go
client := nano_banana_pro.NewClient(baseURL, apiKey,
    nano_banana_pro.WithModel("gemini-3-pro-image"),
    nano_banana_pro.WithTimeout(180*time.Second),
)
```

## 🗂 项目结构

```
modu/
├── pkg/                    # 核心包
│   └── nano_banana_pro/    # 图片生成客户端
├── examples/               # 使用示例
│   └── image_gen/          # 图片生成示例
├── go.mod
└── README.md
```

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 License

MIT License

---

<p align="center">
  <sub>Made with ❤️ for Agent Developers</sub>
</p>
