# MMQ (Modu Memory & Query)

MMQ是一个Go语言实现的RAG（检索增强生成）和记忆管理系统，作为`modu`项目的核心组件。

## 功能特性

### Phase 1 已完成 ✅

- **文档存储**：基于SQLite的内容寻址存储（content-addressable）
- **全文搜索**：FTS5 BM25算法，支持多语言
- **文档分块**：智能分块算法，3200字符/块，15%重叠
- **集合管理**：支持多集合组织文档
- **搜索过滤**：按集合、分数阈值过滤

### Phase 2-4 规划中

- **向量搜索**：语义相似度检索
- **混合搜索**：BM25 + 向量 + LLM重排
- **RAG API**：上下文检索增强生成
- **记忆系统**：对话/事实/偏好/情景记忆

## 快速开始

### 安装

```go
import "github.com/crosszan/modu/pkg/mmq"
```

### 基础使用

```go
package main

import (
    "fmt"
    "time"

    "github.com/crosszan/modu/pkg/mmq"
)

func main() {
    // 初始化MMQ
    m, err := mmq.NewWithDB("~/.modu/memory.db")
    if err != nil {
        panic(err)
    }
    defer m.Close()

    // 索引文档
    doc := mmq.Document{
        Collection: "notes",
        Path:       "golang/intro.md",
        Title:      "Go语言入门",
        Content:    "Go是一门静态类型的编译型语言...",
        CreatedAt:  time.Now(),
        ModifiedAt: time.Now(),
    }

    err = m.IndexDocument(doc)
    if err != nil {
        panic(err)
    }

    // 搜索
    results, err := m.Search("Go语言", mmq.SearchOptions{
        Limit:      5,
        Collection: "notes",
    })
    if err != nil {
        panic(err)
    }

    // 显示结果
    for _, result := range results {
        fmt.Printf("[%.2f] %s: %s\n",
            result.Score, result.Title, result.Snippet)
    }

    // 查看状态
    status, _ := m.Status()
    fmt.Printf("总文档数: %d\n", status.TotalDocuments)
    fmt.Printf("集合: %v\n", status.Collections)
}
```

## API文档

### 初始化

```go
// 使用完整配置
func New(cfg Config) (*MMQ, error)

// 快速初始化（使用默认配置）
func NewWithDB(dbPath string) (*MMQ, error)

// 关闭
func (m *MMQ) Close() error
```

### 文档管理

```go
// 索引单个文档
func (m *MMQ) IndexDocument(doc Document) error

// 获取文档（支持path或hash）
func (m *MMQ) GetDocument(id string) (*Document, error)

// 删除文档（软删除）
func (m *MMQ) DeleteDocument(id string) error
```

### 搜索

```go
// BM25全文搜索
func (m *MMQ) Search(query string, opts SearchOptions) ([]SearchResult, error)

// 搜索选项
type SearchOptions struct {
    Limit      int     // 返回结果数量
    MinScore   float64 // 最小相关性分数
    Collection string  // 集合过滤
}
```

### 状态

```go
// 获取索引状态
func (m *MMQ) Status() (Status, error)

type Status struct {
    TotalDocuments int      // 文档总数
    NeedsEmbedding int      // 待嵌入文档数
    Collections    []string // 集合列表
    DBPath         string   // 数据库路径
    CacheDir       string   // 缓存目录
}
```

## 配置

### 默认配置

```go
cfg := mmq.DefaultConfig()
// DBPath:     ~/.modu/memory.db
// CacheDir:   ~/.cache/modu/models
// ChunkSize:  3200 (字符)
// ChunkOverlap: 480 (字符)
```

### 自定义配置

```go
cfg := mmq.Config{
    DBPath:         "/custom/path/memory.db",
    CacheDir:       "/custom/cache",
    EmbeddingModel: "embeddinggemma-300M-Q8_0",
    RerankModel:    "qwen3-reranker-0.6b-q8_0",
    ChunkSize:      3200,
    ChunkOverlap:   480,
}

m, err := mmq.New(cfg)
```

## 编译和测试

### 编译

```bash
# 必须使用fts5标签启用FTS5支持
go build -tags "fts5" ./pkg/mmq/...
```

### 测试

```bash
# 运行所有测试
go test -v -tags "fts5" ./pkg/mmq

# 性能基准测试
go test -tags "fts5" -bench=. -benchmem ./pkg/mmq
```

## 性能指标

基于Apple M1 Pro的测试结果：

| 操作 | 性能 | 内存 |
|------|------|------|
| BM25搜索 | ~0.12ms/次 | 13KB/操作 |
| 文档索引 | ~1ms/文档 | - |
| 分块处理 | ~0.1ms/KB | - |

**测试规模**：100文档

**性能目标**：
- ✅ BM25搜索 < 5ms（实际0.12ms）
- ⏳ 向量搜索 < 50ms（待实现）
- ⏳ 混合查询 < 2s（待实现）

## 架构设计

```
pkg/mmq/
├── mmq.go              # 公开API入口
├── types.go            # 数据类型定义
├── config.go           # 配置管理
├── store/              # 数据访问层
│   ├── database.go     # SQLite初始化
│   ├── document.go     # 文档CRUD
│   ├── search.go       # 搜索实现
│   ├── chunking.go     # 分块算法
│   └── types.go        # 内部类型
└── internal/
    └── vectordb/
        └── cosine.go   # 向量距离计算
```

## 数据库Schema

### 核心表

- **content**: 内容寻址存储（hash -> content）
- **documents**: 文档元数据（collection, path, title, hash）
- **documents_fts**: FTS5全文索引
- **content_vectors**: 向量嵌入（待Phase 2实现）
- **llm_cache**: LLM结果缓存（待Phase 2实现）

### 特性

- WAL模式，并发安全
- 外键约束
- 自动FTS同步（触发器）
- 内容去重（SHA256哈希）
- 软删除（active标志）

## 使用示例

### 示例1：索引本地文档

```go
m, _ := mmq.NewWithDB("./docs.db")
defer m.Close()

// 索引多个文档
docs := []mmq.Document{
    {
        Collection: "tech",
        Path:       "golang.md",
        Title:      "Go编程",
        Content:    "...",
    },
    {
        Collection: "tech",
        Path:       "python.md",
        Title:      "Python编程",
        Content:    "...",
    },
}

for _, doc := range docs {
    doc.CreatedAt = time.Now()
    doc.ModifiedAt = time.Now()
    m.IndexDocument(doc)
}
```

### 示例2：过滤搜索

```go
// 在特定集合中搜索
results, _ := m.Search("编程语言", mmq.SearchOptions{
    Limit:      10,
    Collection: "tech",
    MinScore:   0.5,
})

// 显示结果
for i, result := range results {
    fmt.Printf("%d. [%.2f] %s/%s\n",
        i+1, result.Score, result.Collection, result.Path)
    fmt.Printf("   %s\n\n", result.Snippet)
}
```

### 示例3：内容去重

```go
// 相同内容只存储一次
doc1 := mmq.Document{
    Collection: "notes",
    Path:       "v1.md",
    Content:    "相同的内容",
}

doc2 := mmq.Document{
    Collection: "notes",
    Path:       "v2.md",
    Content:    "相同的内容", // 相同内容
}

m.IndexDocument(doc1)
m.IndexDocument(doc2)

// 数据库中只存储一份内容，但有两个文档引用
status, _ := m.Status()
fmt.Printf("文档数: %d\n", status.TotalDocuments) // 2
// 内容表中只有1条记录
```

## 注意事项

1. **编译要求**：必须使用`-tags "fts5"`启用FTS5支持
2. **并发安全**：单个MMQ实例可以多goroutine并发访问
3. **文档ID**：自动生成，使用path或hash查询
4. **分块策略**：长文档自动分块，每块独立索引
5. **软删除**：删除文档不会立即删除内容，便于恢复

## 下一步计划

### Phase 2: LLM推理层（1周）

- [ ] go-llama.cpp集成
- [ ] 嵌入生成
- [ ] 重排序
- [ ] 模型下载器

### Phase 3: RAG API（1周）

- [ ] 检索器实现
- [ ] 上下文构建
- [ ] 混合检索策略

### Phase 4: Memory API（1周）

- [ ] 记忆管理器
- [ ] 对话/事实/偏好记忆
- [ ] 时间衰减算法

## License

与modu项目保持一致
