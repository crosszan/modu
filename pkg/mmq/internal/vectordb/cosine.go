// Package vectordb 提供向量数据库相关的底层实现
package vectordb

import (
	"fmt"
	"math"
)

// CosineDist 计算两个向量的余弦距离
// 返回值范围 [0, 2]，0表示完全相同，2表示完全相反
func CosineDist(a, b []float32) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("vector dimension mismatch: %d != %d", len(a), len(b))
	}

	if len(a) == 0 {
		return 0, fmt.Errorf("empty vectors")
	}

	var dotProduct, normA, normB float64

	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	// 处理零向量
	if normA == 0 || normB == 0 {
		return 1.0, nil // 返回中间距离
	}

	// 余弦相似度 = dotProduct / (||a|| * ||b||)
	similarity := dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))

	// 余弦距离 = 1 - 余弦相似度
	// 限制在 [0, 1] 范围内（由于浮点误差可能略微超出）
	distance := 1.0 - similarity
	if distance < 0 {
		distance = 0
	} else if distance > 2 {
		distance = 2
	}

	return distance, nil
}

// CosineSim 计算两个向量的余弦相似度
// 返回值范围 [-1, 1]，1表示完全相同，-1表示完全相反
func CosineSim(a, b []float32) (float64, error) {
	dist, err := CosineDist(a, b)
	if err != nil {
		return 0, err
	}
	return 1.0 - dist, nil
}

// BatchCosineDist 批量计算查询向量与候选向量的余弦距离
// 返回 (索引, 距离) 对的切片，按距离升序排列
func BatchCosineDist(query []float32, candidates [][]float32) ([]DistResult, error) {
	results := make([]DistResult, len(candidates))

	for i, candidate := range candidates {
		dist, err := CosineDist(query, candidate)
		if err != nil {
			return nil, fmt.Errorf("failed to compute distance for candidate %d: %w", i, err)
		}

		results[i] = DistResult{
			Index:    i,
			Distance: dist,
		}
	}

	return results, nil
}

// DistResult 距离计算结果
type DistResult struct {
	Index    int     // 候选向量索引
	Distance float64 // 余弦距离
}

// TopK 返回距离最小的k个结果
func TopK(results []DistResult, k int) []DistResult {
	if k <= 0 || len(results) == 0 {
		return nil
	}

	if k > len(results) {
		k = len(results)
	}

	// 简单的冒泡选择（对于小k值效率足够）
	// TODO: 对于大规模数据可以使用堆优化
	sorted := make([]DistResult, len(results))
	copy(sorted, results)

	for i := 0; i < k; i++ {
		minIdx := i
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Distance < sorted[minIdx].Distance {
				minIdx = j
			}
		}
		sorted[i], sorted[minIdx] = sorted[minIdx], sorted[i]
	}

	return sorted[:k]
}
