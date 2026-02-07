package store

import (
	"fmt"
	"sort"
	"time"
)

// SearchVectorDocuments 文档级向量搜索（对标QMD的vsearch）
// 返回完整文档，而非文本块
func (s *Store) SearchVectorDocuments(query string, queryEmbed []float32, limit int, collection string) ([]SearchResult, error) {
	// 1. 获取所有文档的向量
	sql := `
		SELECT DISTINCT
			d.id,
			d.collection,
			d.path,
			d.title,
			d.hash,
			d.created_at,
			d.modified_at,
			c.doc as content
		FROM documents d
		JOIN content c ON c.hash = d.hash
		WHERE d.active = 1
	`

	args := []interface{}{}
	if collection != "" {
		sql += " AND d.collection = ?"
		args = append(args, collection)
	}

	rows, err := s.db.Query(sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query documents: %w", err)
	}
	defer rows.Close()

	type docWithVectors struct {
		doc     Document
		vectors [][]float32
	}

	// 2. 收集文档和它们的向量
	var docs []docWithVectors
	for rows.Next() {
		var doc Document
		var createdAtStr, modifiedAtStr string

		err := rows.Scan(
			&doc.ID,
			&doc.Collection,
			&doc.Path,
			&doc.Title,
			&doc.Hash,
			&createdAtStr,
			&modifiedAtStr,
			&doc.Content,
		)
		if err != nil {
			continue
		}

		// 解析时间
		doc.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		doc.ModifiedAt, _ = time.Parse(time.RFC3339, modifiedAtStr)

		// 获取该文档的所有向量
		vectors, err := s.GetAllEmbeddings(doc.Hash)
		if err != nil || len(vectors) == 0 {
			continue // 跳过没有向量的文档
		}

		docs = append(docs, docWithVectors{
			doc:     doc,
			vectors: vectors,
		})
	}

	if len(docs) == 0 {
		return []SearchResult{}, nil
	}

	// 3. 计算每个文档的相似度
	type scoredDoc struct {
		doc        Document
		similarity float64 // 余弦相似度 (0-1)
	}

	var scored []scoredDoc
	for _, dv := range docs {
		// 计算文档级相似度：使用最大相似度（最相关的chunk）
		maxSimilarity := 0.0
		for _, vec := range dv.vectors {
			distance := cosineDist(queryEmbed, vec)
			similarity := 1.0 - distance // 转换为相似度
			if similarity > maxSimilarity {
				maxSimilarity = similarity
			}
		}

		scored = append(scored, scoredDoc{
			doc:        dv.doc,
			similarity: maxSimilarity,
		})
	}

	// 4. 按相似度排序
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].similarity > scored[j].similarity
	})

	// 5. 取TopK
	if len(scored) > limit {
		scored = scored[:limit]
	}

	// 6. 转换为SearchResult
	results := make([]SearchResult, len(scored))
	for i, sd := range scored {
		results[i] = SearchResult{
			ID:         sd.doc.ID,
			Title:      sd.doc.Title,
			Content:    sd.doc.Content,
			Snippet:    extractSnippet(sd.doc.Content, query, 200),
			Score:      sd.similarity,
			Source:     "vector",
			Collection: sd.doc.Collection,
			Path:       sd.doc.Path,
			Timestamp:  sd.doc.ModifiedAt,
		}
	}

	return results, nil
}
