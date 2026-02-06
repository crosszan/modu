package llm

import (
	"fmt"
)

// EmbeddingGenerator 嵌入生成器
type EmbeddingGenerator struct {
	llm  LLM
	info EmbeddingInfo
}

// NewEmbeddingGenerator 创建嵌入生成器
func NewEmbeddingGenerator(llm LLM, modelName string, dimensions int) *EmbeddingGenerator {
	return &EmbeddingGenerator{
		llm: llm,
		info: EmbeddingInfo{
			Dimensions: dimensions,
			Model:      modelName,
			MaxTokens:  512,
		},
	}
}

// Generate 生成单个嵌入
func (e *EmbeddingGenerator) Generate(text string, isQuery bool) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("empty text")
	}

	// 截断过长的文本
	text = truncateText(text, e.info.MaxTokens)

	// 生成嵌入
	embedding, err := e.llm.Embed(text, isQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	// 验证维度
	if len(embedding) != e.info.Dimensions && e.info.Dimensions > 0 {
		return nil, fmt.Errorf("unexpected embedding dimension: got %d, expected %d",
			len(embedding), e.info.Dimensions)
	}

	// 归一化
	embedding = normalizeVector(embedding)

	return embedding, nil
}

// GenerateBatch 批量生成嵌入
func (e *EmbeddingGenerator) GenerateBatch(texts []string, isQuery bool) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("empty texts")
	}

	// 截断过长的文本
	truncated := make([]string, len(texts))
	for i, text := range texts {
		truncated[i] = truncateText(text, e.info.MaxTokens)
	}

	// 批量生成
	embeddings, err := e.llm.EmbedBatch(truncated, isQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to generate batch embeddings: %w", err)
	}

	// 归一化所有向量
	for i := range embeddings {
		embeddings[i] = normalizeVector(embeddings[i])
	}

	return embeddings, nil
}

// GetInfo 获取嵌入信息
func (e *EmbeddingGenerator) GetInfo() EmbeddingInfo {
	return e.info
}

// truncateText 截断文本到指定token数
func truncateText(text string, maxTokens int) string {
	// 简化实现：使用字符数估算
	// 实际应该使用tokenizer
	maxChars := maxTokens * 4
	if len(text) <= maxChars {
		return text
	}
	return text[:maxChars]
}

// normalizeVector 归一化向量
func normalizeVector(vec []float32) []float32 {
	var sumSquares float32
	for _, v := range vec {
		sumSquares += v * v
	}

	if sumSquares == 0 {
		return vec
	}

	norm := float32(1.0 / sqrtFloat32(sumSquares))
	result := make([]float32, len(vec))
	for i, v := range vec {
		result[i] = v * norm
	}

	return result
}

// sqrtFloat32 计算平方根
func sqrtFloat32(x float32) float32 {
	// 使用牛顿迭代法
	if x == 0 {
		return 0
	}

	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

// ChunkTextForEmbedding 将长文本分块用于嵌入
func ChunkTextForEmbedding(text string, chunkSize, overlap int) []string {
	if len(text) <= chunkSize {
		return []string{text}
	}

	var chunks []string
	start := 0

	for start < len(text) {
		end := start + chunkSize
		if end > len(text) {
			end = len(text)
		}

		// 寻找单词边界
		if end < len(text) {
			for i := end; i > start && i > end-100; i-- {
				if text[i] == ' ' || text[i] == '\n' {
					end = i
					break
				}
			}
		}

		chunks = append(chunks, text[start:end])

		// 移动到下一个块，考虑重叠
		start = end - overlap
		if start < 0 {
			start = 0
		}

		if end >= len(text) {
			break
		}
	}

	return chunks
}

// AverageEmbeddings 平均多个嵌入向量
func AverageEmbeddings(embeddings [][]float32) []float32 {
	if len(embeddings) == 0 {
		return nil
	}

	if len(embeddings) == 1 {
		return embeddings[0]
	}

	dim := len(embeddings[0])
	result := make([]float32, dim)

	for _, emb := range embeddings {
		if len(emb) != dim {
			continue
		}
		for i, v := range emb {
			result[i] += v
		}
	}

	// 平均
	count := float32(len(embeddings))
	for i := range result {
		result[i] /= count
	}

	// 归一化
	return normalizeVector(result)
}
