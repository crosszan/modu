# Phase 5.1 完成：向量搜索补全

## 实施时间
2025-02-07

## 目标
补齐MMQ的向量搜索功能，完全对等QMD的vsearch命令。

## 问题分析

### 原有实现的缺陷
MMQ之前只有块级向量搜索（通过`RetrieveContext + StrategyVector`），返回文本片段而非完整文档：

```go
// 之前：只能获取文本片段
contexts, _ := m.RetrieveContext("Go并发", mmq.RetrieveOptions{
    Strategy: mmq.StrategyVector,
})
// contexts[0].Text = "...goroutine片段..."
```

### QMD的vsearch
```bash
qmd vsearch "Go并发" -n 5
# 返回完整文档 + 文档级相似度
```

**区别**：
- QMD: 文档级搜索，返回完整文档
- MMQ (旧): 块级检索，返回文本片段
- **不对等！**

## 实施内容

### 1. Store层：文档级向量搜索

**新文件**：`pkg/mmq/store/search_vector.go` (~130行)

```go
// SearchVectorDocuments 文档级向量搜索
func (s *Store) SearchVectorDocuments(
    query string,           // 查询文本（用于snippet）
    queryEmbed []float32,   // 查询向量
    limit int,              // 返回数量
    collection string       // 集合过滤
) ([]SearchResult, error)
```

**核心算法**：
1. 获取所有文档及其向量（可能多个chunk/文档）
2. 计算文档级相似度：取所有chunk的**最大相似度**
3. 按相似度排序
4. 返回完整文档

**相似度计算**：
```go
// 余弦相似度 = 1 - 余弦距离
distance := cosineDist(queryEmbed, docVector)
similarity := 1.0 - distance
```

### 2. MMQ层：公开API

**扩展**：`pkg/mmq/mmq.go`

```go
// VectorSearch 向量语义搜索（对标QMD的vsearch）
// 返回完整文档（文档级别），不是文本块
func (m *MMQ) VectorSearch(query string, opts SearchOptions) ([]SearchResult, error) {
    // 1. 生成查询向量
    queryEmbed, _ := m.embedding.Generate(query, true)

    // 2. 文档级向量搜索
    results, _ := m.store.SearchVectorDocuments(query, queryEmbed, opts.Limit, opts.Collection)

    return convertSearchResults(results), nil
}
```

### 3. 测试覆盖

**新文件**：`pkg/mmq/vector_search_test.go` (~220行)

**测试用例**：
- `TestVectorSearch/Basic_VectorSearch` - 基础向量搜索
- `TestVectorSearch/VectorSearch_with_Collection_Filter` - 集合过滤
- `TestVectorSearch/VectorSearch_Score_Ordering` - 分数排序
- `TestVectorSearchVsRetrieveContext` - 对比文档级 vs 块级
- `BenchmarkVectorSearch` - 性能基准

**测试结果**：✓ 全部通过

## 技术细节

### 时间解析问题修复
SQLite存储时间为RFC3339字符串，需要先扫描为string再解析：

```go
var createdAtStr, modifiedAtStr string
rows.Scan(..., &createdAtStr, &modifiedAtStr, ...)

doc.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
doc.ModifiedAt, _ = time.Parse(time.RFC3339, modifiedAtStr)
```

### 文档级相似度策略
当文档被分为多个chunk时，使用**最大相似度**（Max Pooling）：

```go
maxSimilarity := 0.0
for _, vec := range document.vectors {
    similarity := 1.0 - cosineDist(queryEmbed, vec)
    if similarity > maxSimilarity {
        maxSimilarity = similarity
    }
}
```

**原因**：最相关的chunk代表文档与查询的最佳匹配度。

### 代码复用
复用已有函数：
- `cosineDist()` from `memory.go` - 余弦距离
- `extractSnippet()` from `search.go` - 摘要提取

## API对比

### QMD
```typescript
// TypeScript
const results = await store.searchVector("Go并发", 5)
// 返回文档列表
```

### MMQ（现在）
```go
// Go
results, _ := m.VectorSearch("Go并发", mmq.SearchOptions{
    Limit: 5,
})
// 返回完整文档，功能对等！
```

## 测试验证

```bash
$ go test ./pkg/mmq -run TestVectorSearch -v -tags="fts5"

=== RUN   TestVectorSearch/Basic_VectorSearch
    VectorSearch returned 3 results
    [1] Score: 0.034, Title: Go Programming, Source: vector
    [2] Score: 0.000, Title: Python Programming, Source: vector
    [3] Score: 0.000, Title: RAG Systems, Source: vector
--- PASS: TestVectorSearch/Basic_VectorSearch

=== RUN   TestVectorSearch/VectorSearch_with_Collection_Filter
    Collection filter returned 2 results
--- PASS: TestVectorSearch/VectorSearch_with_Collection_Filter

=== RUN   TestVectorSearch/VectorSearch_Score_Ordering
    Score ordering verified, top score: 0.034
--- PASS: TestVectorSearch/VectorSearch_Score_Ordering

PASS
ok  	github.com/crosszan/modu/pkg/mmq	0.833s
```

## 文档级 vs 块级对比

| 特性 | VectorSearch（文档级） | RetrieveContext（块级） |
|------|----------------------|------------------------|
| 返回类型 | SearchResult（完整文档） | Context（文本片段） |
| 用途 | 文档搜索 | RAG上下文检索 |
| 相似度 | 文档级（最大chunk） | 块级 |
| 内容 | 完整文档内容 | 相关片段 |
| 对标QMD | ✓ vsearch | ✗ （QMD无此功能） |

## 功能对等检查表

| 功能 | QMD | MMQ (之前) | MMQ (现在) |
|------|-----|-----------|-----------|
| BM25搜索 | ✓ search | ✓ Search | ✓ Search |
| 向量搜索 | ✓ vsearch | ✗ | ✓ **VectorSearch** |
| 混合搜索 | ✓ query | ✓ HybridSearch | ✓ HybridSearch |
| 文档级结果 | ✓ | ✗ | ✓ |
| 集合过滤 | ✓ | ✓ | ✓ |
| 完整文档内容 | ✓ | ✗ | ✓ |

## 性能

```bash
BenchmarkVectorSearch-10    2000    550000 ns/op
```

- 单次向量搜索：~0.55ms（20个文档）
- 主要开销：向量加载 + 余弦距离计算

## 下一步：Phase 5.2

**Collection管理** - 实现QMD的collection命令：
- `collection add <path> --name <name>`
- `collection list`
- `collection remove <name>`
- `collection rename <old> <new>`

## 总结

✅ **Phase 5.1 完成**：向量搜索功能现已完全对等QMD！

**核心价值**：
- MMQ现在有独立的`VectorSearch()` API
- 返回完整文档而非文本片段
- 支持集合过滤和相似度排序
- 测试覆盖完整

**对等进度**：
- ✓ Phase 1-4: 核心引擎（RAG + Memory）
- ✓ Phase 5.1: 向量搜索补全
- ⏳ Phase 5.2-5.6: Collection管理、Context管理、CLI工具...
