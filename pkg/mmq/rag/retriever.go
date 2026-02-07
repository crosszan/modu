package rag

import (
	"fmt"

	"github.com/crosszan/modu/pkg/mmq/llm"
	"github.com/crosszan/modu/pkg/mmq/store"
)

// Retriever RAG检索器
type Retriever struct {
	store     *store.Store
	llm       llm.LLM
	embedding *llm.EmbeddingGenerator
}

// NewRetriever 创建检索器
func NewRetriever(st *store.Store, llmImpl llm.LLM, embGen *llm.EmbeddingGenerator) *Retriever {
	return &Retriever{
		store:     st,
		llm:       llmImpl,
		embedding: embGen,
	}
}

// RetrievalStrategy 检索策略
type RetrievalStrategy string

const (
	StrategyFTS    RetrievalStrategy = "fts"    // 仅BM25全文搜索
	StrategyVector RetrievalStrategy = "vector" // 仅向量搜索
	StrategyHybrid RetrievalStrategy = "hybrid" // 混合搜索
)

// RetrieveOptions 检索选项
type RetrieveOptions struct {
	Limit      int               // 返回结果数量
	MinScore   float64           // 最小分数阈值
	Collection string            // 集合过滤
	Strategy   RetrievalStrategy // 检索策略
	Rerank     bool              // 是否重排序
	RRFWeights []float64         // RRF权重
	RRFK       int               // RRF参数K
}

// DefaultRetrieveOptions 默认检索选项
func DefaultRetrieveOptions() RetrieveOptions {
	return RetrieveOptions{
		Limit:      10,
		MinScore:   0.0,
		Strategy:   StrategyHybrid,
		Rerank:     false,
		RRFWeights: []float64{1.0, 1.0},
		RRFK:       60,
	}
}

// Context RAG上下文
type Context struct {
	Text      string                 // 文本内容
	Source    string                 // 来源文档
	Relevance float64                // 相关性分数
	Metadata  map[string]interface{} // 元数据
}

// Retrieve 执行检索
func (r *Retriever) Retrieve(query string, opts RetrieveOptions) ([]Context, error) {
	var results []store.SearchResult
	var err error

	switch opts.Strategy {
	case StrategyFTS:
		results, err = r.retrieveFTS(query, opts)
	case StrategyVector:
		results, err = r.retrieveVector(query, opts)
	case StrategyHybrid:
		results, err = r.retrieveHybrid(query, opts)
	default:
		return nil, fmt.Errorf("unknown strategy: %s", opts.Strategy)
	}

	if err != nil {
		return nil, err
	}

	// 过滤低分结果
	if opts.MinScore > 0 {
		filtered := make([]store.SearchResult, 0, len(results))
		for _, r := range results {
			if r.Score >= opts.MinScore {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	// 重排序
	if opts.Rerank && len(results) > 0 {
		results, err = r.rerank(query, results)
		if err != nil {
			return nil, fmt.Errorf("rerank failed: %w", err)
		}
	}

	// 限制结果数量
	if len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	// 转换为Context
	return r.toContexts(results), nil
}

// retrieveFTS BM25全文搜索
func (r *Retriever) retrieveFTS(query string, opts RetrieveOptions) ([]store.SearchResult, error) {
	return r.store.SearchFTS(query, opts.Limit*2, opts.Collection)
}

// retrieveVector 向量语义搜索
func (r *Retriever) retrieveVector(query string, opts RetrieveOptions) ([]store.SearchResult, error) {
	// 生成查询嵌入
	embedding, err := r.embedding.Generate(query, true)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	return r.store.SearchVector(query, embedding, opts.Limit*2, opts.Collection)
}

// retrieveHybrid 混合搜索
func (r *Retriever) retrieveHybrid(query string, opts RetrieveOptions) ([]store.SearchResult, error) {
	// 1. BM25搜索
	ftsResults, err := r.retrieveFTS(query, opts)
	if err != nil {
		return nil, fmt.Errorf("FTS search failed: %w", err)
	}

	// 2. 向量搜索
	vecResults, err := r.retrieveVector(query, opts)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	// 3. RRF融合
	resultLists := [][]store.SearchResult{ftsResults, vecResults}
	fused := store.ReciprocalRankFusion(resultLists, opts.RRFWeights, opts.RRFK)

	return fused, nil
}

// rerank 使用LLM重排序
func (r *Retriever) rerank(query string, results []store.SearchResult) ([]store.SearchResult, error) {
	if len(results) == 0 {
		return results, nil
	}

	// 转换为LLM文档格式
	docs := make([]llm.Document, len(results))
	for i, res := range results {
		docs[i] = llm.Document{
			ID:      res.ID,
			Content: res.Content,
			Title:   res.Title,
		}
	}

	// 调用LLM重排
	rerankResults, err := r.llm.Rerank(query, docs)
	if err != nil {
		return nil, err
	}

	// 创建索引映射
	indexMap := make(map[string]int)
	for i, res := range results {
		indexMap[res.ID] = i
	}

	// 根据重排结果重新排序
	reranked := make([]store.SearchResult, 0, len(rerankResults))
	for _, rr := range rerankResults {
		if idx, ok := indexMap[rr.ID]; ok {
			result := results[idx]
			result.Score = rr.Score
			result.Source = "rerank"
			reranked = append(reranked, result)
		}
	}

	return reranked, nil
}

// toContexts 转换为Context
func (r *Retriever) toContexts(results []store.SearchResult) []Context {
	contexts := make([]Context, len(results))

	for i, res := range results {
		contexts[i] = Context{
			Text:      res.Content,
			Source:    fmt.Sprintf("%s/%s", res.Collection, res.Path),
			Relevance: res.Score,
			Metadata: map[string]interface{}{
				"title":      res.Title,
				"collection": res.Collection,
				"path":       res.Path,
				"snippet":    res.Snippet,
				"source":     res.Source,
				"timestamp":  res.Timestamp,
			},
		}
	}

	return contexts
}

// AdaptiveRetrieve 自适应检索（根据查询类型选择策略）
func (r *Retriever) AdaptiveRetrieve(query string, opts RetrieveOptions) ([]Context, error) {
	// 检测查询类型
	queryType := detectQueryType(query)

	switch queryType {
	case QueryTypeKeyword:
		// 关键词查询 -> BM25最优
		opts.Strategy = StrategyFTS
	case QueryTypeSemantic:
		// 语义查询 -> 向量最优
		opts.Strategy = StrategyVector
	case QueryTypeComplex:
		// 复杂查询 -> 混合最优
		opts.Strategy = StrategyHybrid
	}

	return r.Retrieve(query, opts)
}

// QueryType 查询类型
type QueryType int

const (
	QueryTypeKeyword  QueryType = iota // 关键词查询
	QueryTypeSemantic                  // 语义查询
	QueryTypeComplex                   // 复杂查询
)

// detectQueryType 检测查询类型
func detectQueryType(query string) QueryType {
	// 简化实现：基于查询长度和复杂度
	words := len(splitWords(query))

	if words <= 3 {
		return QueryTypeKeyword
	} else if words <= 8 {
		return QueryTypeSemantic
	} else {
		return QueryTypeComplex
	}
}

// splitWords 简单分词
func splitWords(text string) []string {
	var words []string
	var word []rune

	for _, r := range text {
		if r == ' ' || r == '\n' || r == '\t' || r == ',' || r == '.' {
			if len(word) > 0 {
				words = append(words, string(word))
				word = nil
			}
		} else {
			word = append(word, r)
		}
	}

	if len(word) > 0 {
		words = append(words, string(word))
	}

	return words
}
