// Package mmq (Modu Memory & Query) 提供RAG检索和记忆管理功能
package mmq

import (
	"fmt"

	"github.com/crosszan/modu/pkg/mmq/llm"
	"github.com/crosszan/modu/pkg/mmq/memory"
	"github.com/crosszan/modu/pkg/mmq/rag"
	"github.com/crosszan/modu/pkg/mmq/store"
)

// MMQ 核心实例
type MMQ struct {
	store         *store.Store
	llm           llm.LLM
	embedding     *llm.EmbeddingGenerator
	retriever     *rag.Retriever
	memoryManager *memory.Manager
	cfg           Config
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

	// 创建RAG检索器
	retriever := rag.NewRetriever(st, llmImpl, embeddingGen)

	// 创建记忆管理器
	memoryMgr := memory.NewManager(st, embeddingGen)

	return &MMQ{
		store:         st,
		llm:           llmImpl,
		embedding:     embeddingGen,
		retriever:     retriever,
		memoryManager: memoryMgr,
		cfg:           cfg,
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
	// 转换为rag.RetrieveOptions
	ragOpts := rag.RetrieveOptions{
		Limit:      opts.Limit,
		MinScore:   opts.MinScore,
		Collection: opts.Collection,
		Strategy:   rag.RetrievalStrategy(opts.Strategy),
		Rerank:     opts.Rerank,
	}

	// 调用retriever
	ragContexts, err := m.retriever.Retrieve(query, ragOpts)
	if err != nil {
		return nil, err
	}

	// 转换类型
	return convertRagContexts(ragContexts), nil
}

// convertRagContexts 转换rag.Context到mmq.Context
func convertRagContexts(ragContexts []rag.Context) []Context {
	contexts := make([]Context, len(ragContexts))
	for i, rc := range ragContexts {
		contexts[i] = Context{
			Text:      rc.Text,
			Source:    rc.Source,
			Relevance: rc.Relevance,
			Metadata:  rc.Metadata,
		}
	}
	return contexts
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
	// 转换为rag.RetrieveOptions
	ragOpts := rag.RetrieveOptions{
		Limit:      opts.Limit,
		MinScore:   opts.MinScore,
		Collection: opts.Collection,
		Strategy:   rag.StrategyHybrid,
		Rerank:     false, // HybridSearch默认不重排
	}

	// 调用retriever获取上下文
	contexts, err := m.retriever.Retrieve(query, ragOpts)
	if err != nil {
		return nil, err
	}

	// 转换为SearchResult
	results := make([]SearchResult, len(contexts))
	for i, ctx := range contexts {
		results[i] = SearchResult{
			Score:      ctx.Relevance,
			Title:      getMetadataString(ctx.Metadata, "title"),
			Content:    ctx.Text,
			Snippet:    getMetadataString(ctx.Metadata, "snippet"),
			Source:     getMetadataString(ctx.Metadata, "source"),
			Collection: getMetadataString(ctx.Metadata, "collection"),
			Path:       getMetadataString(ctx.Metadata, "path"),
		}
	}

	return results, nil
}

// getMetadataString 从元数据中获取字符串值
func getMetadataString(metadata map[string]interface{}, key string) string {
	if val, ok := metadata[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// --- 记忆存储API（Phase 4实现）---

// StoreMemory 存储记忆
func (m *MMQ) StoreMemory(mem Memory) error {
	memoryMem := memory.Memory{
		ID:         mem.ID,
		Type:       memory.MemoryType(mem.Type),
		Content:    mem.Content,
		Metadata:   mem.Metadata,
		Tags:       mem.Tags,
		Timestamp:  mem.Timestamp,
		ExpiresAt:  mem.ExpiresAt,
		Importance: mem.Importance,
	}

	return m.memoryManager.Store(memoryMem)
}

// RecallMemories 回忆记忆
func (m *MMQ) RecallMemories(query string, opts RecallOptions) ([]Memory, error) {
	memOpts := memory.RecallOptions{
		Limit:              opts.Limit,
		MemoryTypes:        convertMemoryTypes(opts.MemoryTypes),
		ApplyDecay:         opts.ApplyDecay,
		DecayHalflife:      opts.DecayHalflife,
		WeightByImportance: opts.WeightByImportance,
		MinRelevance:       opts.MinRelevance,
	}

	memories, err := m.memoryManager.Recall(query, memOpts)
	if err != nil {
		return nil, err
	}

	return convertToMMQMemories(memories), nil
}

// UpdateMemory 更新记忆
func (m *MMQ) UpdateMemory(id string, mem Memory) error {
	memoryMem := memory.Memory{
		ID:         mem.ID,
		Type:       memory.MemoryType(mem.Type),
		Content:    mem.Content,
		Metadata:   mem.Metadata,
		Tags:       mem.Tags,
		Timestamp:  mem.Timestamp,
		ExpiresAt:  mem.ExpiresAt,
		Importance: mem.Importance,
	}

	return m.memoryManager.Update(id, memoryMem)
}

// DeleteMemory 删除记忆
func (m *MMQ) DeleteMemory(id string) error {
	return m.memoryManager.Delete(id)
}

// GetMemoryByID 根据ID获取记忆
func (m *MMQ) GetMemoryByID(id string) (*Memory, error) {
	mem, err := m.memoryManager.GetByID(id)
	if err != nil {
		return nil, err
	}

	return &Memory{
		ID:         mem.ID,
		Type:       MemoryType(mem.Type),
		Content:    mem.Content,
		Metadata:   mem.Metadata,
		Tags:       mem.Tags,
		Timestamp:  mem.Timestamp,
		ExpiresAt:  mem.ExpiresAt,
		Importance: mem.Importance,
	}, nil
}

// CleanupExpiredMemories 清理过期记忆
func (m *MMQ) CleanupExpiredMemories() (int, error) {
	return m.memoryManager.CleanupExpired()
}

// CountMemories 统计记忆数量
func (m *MMQ) CountMemories() (int, error) {
	return m.memoryManager.Count()
}

// convertMemoryTypes 转换记忆类型
func convertMemoryTypes(types []MemoryType) []memory.MemoryType {
	if types == nil {
		return nil
	}

	memTypes := make([]memory.MemoryType, len(types))
	for i, t := range types {
		memTypes[i] = memory.MemoryType(t)
	}
	return memTypes
}

// convertToMMQMemories 转换记忆到MMQ类型
func convertToMMQMemories(memories []memory.Memory) []Memory {
	mmqMemories := make([]Memory, len(memories))
	for i, mem := range memories {
		mmqMemories[i] = Memory{
			ID:         mem.ID,
			Type:       MemoryType(mem.Type),
			Content:    mem.Content,
			Metadata:   mem.Metadata,
			Tags:       mem.Tags,
			Timestamp:  mem.Timestamp,
			ExpiresAt:  mem.ExpiresAt,
			Importance: mem.Importance,
		}
	}
	return mmqMemories
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

// GetStore 获取Store实例（用于高级用法）
func (m *MMQ) GetStore() *store.Store {
	return m.store
}

// GetEmbedding 获取EmbeddingGenerator实例（用于高级用法）
func (m *MMQ) GetEmbedding() *llm.EmbeddingGenerator {
	return m.embedding
}

// GetMemoryManager 获取MemoryManager实例（用于高级用法）
func (m *MMQ) GetMemoryManager() *memory.Manager {
	return m.memoryManager
}
