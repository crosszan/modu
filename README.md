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

## 🚀 快速开始

```go
package main

import (
    "context"
    "fmt"

    genimagerepo "github.com/crosszan/modu/repos/gen_image_repo"
    genimagevo "github.com/crosszan/modu/vo/gen_image_vo"
)

func main() {
    repo := genimagerepo.NewGeminiImageImpl("http://127.0.0.1:8045", "your-api-key")

    result, err := repo.Generate(context.Background(), &genimagevo.GenImageRequest{
        UserPrompt: "a beautiful sunset",
    })
    if err != nil {
        panic(err)
    }

    // 保存到默认目录 ./images
    files, _ := genimagerepo.SaveAllImages(result)
    
    // 或指定目录
    // files, _ := genimagerepo.SaveAllImages(result, "./output")

    for _, f := range files {
        fmt.Printf("✓ 已保存: %s\n", f)
    }
}
```

## 🗂 项目结构

```
modu/
├── consts/                 # 常量定义
│   └── provider/           # Provider 类型与模型常量
├── repos/                  # 仓库层 (业务抽象)
│   └── gen_image_repo/     # 图片生成仓库
├── vo/                     # 值对象
│   └── gen_image_vo/       # 图片生成请求/响应
├── pkg/                    # 工具包
│   └── utils/              # 通用工具函数
└── examples/               # 使用示例
```

## 📚 模块

| 模块 | 描述 |
|------|------|
| `repos/gen_image_repo` | 图片生成仓库 (支持多 Provider) |
| `vo/gen_image_vo` | 图片生成值对象 |
| `pkg/utils` | 图片保存等工具函数 |
| `consts/provider` | Provider 类型常量 |

## 📄 License

MIT License
