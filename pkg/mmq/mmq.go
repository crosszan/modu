// Package mmq (Modu Memory & Query) 提供RAG检索和记忆管理功能
package mmq

import (
	"fmt"

	"github.com/crosszan/modu/pkg/mmq/llm"
	"github.com/crosszan/modu/pkg/mmq/store"
)

// MMQ 核心实例
type MMQ struct {
	store     *store.Store
	llm       llm.LLM
	embedding *llm.EmbeddingGenerator
	cfg       Config
}

// New 创建新的MMQ实例
func New(cfg Config) (*MMQ, error) {
	// 验证配置
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// 初始化store
	st, err := store.New(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create store: %w", err)
	}

	// 初始化LLM
	// 默认使用MockLLM（用于测试和开发）
	// 生产环境需要使用真实的LlamaCpp实现
	var llmImpl llm.LLM
	llmImpl = llm.NewMockLLM(300) // 300维嵌入

	// 创建嵌入生成器
	embeddingGen := llm.NewEmbeddingGenerator(llmImpl, cfg.EmbeddingModel, 300)

	return &MMQ{
		store:     st,
		llm:       llmImpl,
		embedding: embeddingGen,
		cfg:       cfg,
	}, nil
}

// NewWithDB 使用指定数据库路径快速初始化
func NewWithDB(dbPath string) (*MMQ, error) {
	cfg := DefaultConfig()
	cfg.DBPath = dbPath
	return New(cfg)
}

// Close 关闭MMQ实例
func (m *MMQ) Close() error {
	// 关闭LLM
	if m.llm != nil {
		if err := m.llm.Close(); err != nil {
			return fmt.Errorf("failed to close LLM: %w", err)
		}
	}

	// 关闭store
	if m.store != nil {
		return m.store.Close()
	}
	return nil
}

// Status 返回索引状态
func (m *MMQ) Status() (Status, error) {
	storeStatus, err := m.store.GetStatus()
	if err != nil {
		return Status{}, err
	}

	status := Status{
		TotalDocuments: storeStatus.TotalDocuments,
		NeedsEmbedding: storeStatus.NeedsEmbedding,
		Collections:    storeStatus.Collections,
		DBPath:         storeStatus.DBPath,
		CacheDir:       m.cfg.CacheDir,
	}
	return status, nil
}

// 类型转换辅助函数

func convertSearchResults(storeResults []store.SearchResult) []SearchResult {
	results := make([]SearchResult, len(storeResults))
	for i, sr := range storeResults {
		results[i] = SearchResult{
			ID:         sr.ID,
			Score:      sr.Score,
			Title:      sr.Title,
			Content:    sr.Content,
			Snippet:    sr.Snippet,
			Source:     sr.Source,
			Collection: sr.Collection,
			Path:       sr.Path,
			Timestamp:  sr.Timestamp,
		}
	}
	return results
}

// --- RAG检索API（Phase 3实现）---

// RetrieveContext 检索相关上下文
func (m *MMQ) RetrieveContext(query string, opts RetrieveOptions) ([]Context, error) {
	// TODO: Phase 3实现
	return nil, fmt.Errorf("not implemented yet")
}

// Search 搜索文档
func (m *MMQ) Search(query string, opts SearchOptions) ([]SearchResult, error) {
	results, err := m.store.SearchFTS(query, opts.Limit, opts.Collection)
	if err != nil {
		return nil, err
	}

	// 转换类型
	return convertSearchResults(results), nil
}

// HybridSearch 混合搜索
func (m *MMQ) HybridSearch(query string, opts SearchOptions) ([]SearchResult, error) {
	// TODO: Phase 3实现（需要llm包）
	return nil, fmt.Errorf("not implemented yet")
}

// --- 记忆存储API（Phase 4实现）---

// StoreMemory 存储记忆
func (m *MMQ) StoreMemory(mem Memory) error {
	// TODO: Phase 4实现
	return fmt.Errorf("not implemented yet")
}

// RecallMemories 回忆记忆
func (m *MMQ) RecallMemories(query string, limit int) ([]Memory, error) {
	// TODO: Phase 4实现
	return nil, fmt.Errorf("not implemented yet")
}

// UpdateMemory 更新记忆
func (m *MMQ) UpdateMemory(id string, mem Memory) error {
	// TODO: Phase 4实现
	return fmt.Errorf("not implemented yet")
}

// DeleteMemory 删除记忆
func (m *MMQ) DeleteMemory(id string) error {
	// TODO: Phase 4实现
	return fmt.Errorf("not implemented yet")
}

// --- 文档管理API ---

// IndexDocument 索引单个文档
func (m *MMQ) IndexDocument(doc Document) error {
	storeDoc := store.Document{
		ID:         doc.ID,
		Collection: doc.Collection,
		Path:       doc.Path,
		Title:      doc.Title,
		Content:    doc.Content,
		CreatedAt:  doc.CreatedAt,
		ModifiedAt: doc.ModifiedAt,
	}
	return m.store.IndexDocument(storeDoc)
}

// IndexDirectory 索引目录
func (m *MMQ) IndexDirectory(path string, opts IndexOptions) error {
	// TODO: 实现目录遍历和批量索引
	return fmt.Errorf("not implemented yet")
}

// GetDocument 获取文档
func (m *MMQ) GetDocument(id string) (*Document, error) {
	storeDoc, err := m.store.GetDocument(id)
	if err != nil {
		return nil, err
	}

	doc := &Document{
		ID:         storeDoc.ID,
		Collection: storeDoc.Collection,
		Path:       storeDoc.Path,
		Title:      storeDoc.Title,
		Content:    storeDoc.Content,
		CreatedAt:  storeDoc.CreatedAt,
		ModifiedAt: storeDoc.ModifiedAt,
	}
	return doc, nil
}

// DeleteDocument 删除文档
func (m *MMQ) DeleteDocument(id string) error {
	return m.store.DeleteDocument(id)
}

// --- 嵌入管理（Phase 2实现）---

// GenerateEmbeddings 生成所有文档的嵌入
func (m *MMQ) GenerateEmbeddings() error {
	// 获取需要嵌入的文档
	docs, err := m.store.GetDocumentsNeedingEmbedding()
	if err != nil {
		return fmt.Errorf("failed to get documents: %w", err)
	}

	if len(docs) == 0 {
		return nil // 没有需要嵌入的文档
	}

	// 逐个文档生成嵌入
	for i, doc := range docs {
		// 分块
		chunks := store.ChunkDocument(doc.Content, m.cfg.ChunkSize, m.cfg.ChunkOverlap)

		// 为每个块生成嵌入
		for j, chunk := range chunks {
			embedding, err := m.embedding.Generate(chunk.Text, false)
			if err != nil {
				return fmt.Errorf("failed to generate embedding for doc %s chunk %d: %w",
					doc.Hash, j, err)
			}

			// 存储嵌入
			err = m.store.StoreEmbedding(doc.Hash, j, chunk.Pos, embedding, m.cfg.EmbeddingModel)
			if err != nil {
				return fmt.Errorf("failed to store embedding: %w", err)
			}
		}

		// 打印进度
		if (i+1)%10 == 0 || i == len(docs)-1 {
			fmt.Printf("Embedded %d/%d documents\n", i+1, len(docs))
		}
	}

	return nil
}

// EmbedText 对文本生成嵌入向量
func (m *MMQ) EmbedText(text string) ([]float32, error) {
	return m.embedding.Generate(text, true)
}
