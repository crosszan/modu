package store

import (
	"fmt"
	"time"
)

// GetDocumentsNeedingEmbedding 获取需要生成嵌入的文档
func (s *Store) GetDocumentsNeedingEmbedding() ([]Document, error) {
	query := `
		SELECT DISTINCT d.hash, c.doc
		FROM documents d
		JOIN content c ON c.hash = d.hash
		LEFT JOIN content_vectors v ON d.hash = v.hash AND v.seq = 0
		WHERE d.active = 1 AND v.hash IS NULL
		ORDER BY d.modified_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query documents: %w", err)
	}
	defer rows.Close()

	var docs []Document
	for rows.Next() {
		var doc Document
		err := rows.Scan(&doc.Hash, &doc.Content)
		if err != nil {
			return nil, fmt.Errorf("failed to scan document: %w", err)
		}
		docs = append(docs, doc)
	}

	return docs, nil
}

// StoreEmbedding 存储嵌入向量
func (s *Store) StoreEmbedding(hash string, seq int, pos int, embedding []float32, model string) error {
	// 将float32数组转换为blob
	blob := float32ToBlob(embedding)

	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO content_vectors (hash, seq, pos, embedding, model, embedded_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, hash, seq, pos, blob, model, now)

	if err != nil {
		return fmt.Errorf("failed to store embedding: %w", err)
	}

	return nil
}

// GetEmbedding 获取嵌入向量
func (s *Store) GetEmbedding(hash string, seq int) ([]float32, error) {
	var blob []byte

	err := s.db.QueryRow(`
		SELECT embedding FROM content_vectors
		WHERE hash = ? AND seq = ?
	`, hash, seq).Scan(&blob)

	if err != nil {
		return nil, fmt.Errorf("failed to get embedding: %w", err)
	}

	return blobToFloat32(blob), nil
}

// GetAllEmbeddings 获取文档的所有嵌入向量
func (s *Store) GetAllEmbeddings(hash string) ([][]float32, error) {
	rows, err := s.db.Query(`
		SELECT seq, embedding FROM content_vectors
		WHERE hash = ?
		ORDER BY seq
	`, hash)

	if err != nil {
		return nil, fmt.Errorf("failed to query embeddings: %w", err)
	}
	defer rows.Close()

	var embeddings [][]float32
	for rows.Next() {
		var seq int
		var blob []byte

		err := rows.Scan(&seq, &blob)
		if err != nil {
			return nil, fmt.Errorf("failed to scan embedding: %w", err)
		}

		embeddings = append(embeddings, blobToFloat32(blob))
	}

	return embeddings, nil
}

// DeleteEmbeddings 删除文档的所有嵌入
func (s *Store) DeleteEmbeddings(hash string) error {
	_, err := s.db.Exec("DELETE FROM content_vectors WHERE hash = ?", hash)
	if err != nil {
		return fmt.Errorf("failed to delete embeddings: %w", err)
	}
	return nil
}

// CountEmbeddedDocuments 统计已嵌入的文档数
func (s *Store) CountEmbeddedDocuments() (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(DISTINCT hash) FROM content_vectors
	`).Scan(&count)

	if err != nil {
		return 0, fmt.Errorf("failed to count embedded documents: %w", err)
	}

	return count, nil
}
