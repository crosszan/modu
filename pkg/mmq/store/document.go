package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// IndexDocument 索引单个文档
func (s *Store) IndexDocument(doc Document) error {
	// 1. 计算内容哈希
	hash := computeHash(doc.Content)

	// 2. 检查内容是否已存在
	var exists bool
	err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM content WHERE hash = ?)", hash).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check content existence: %w", err)
	}

	// 3. 插入内容（如果不存在）
	if !exists {
		now := time.Now().UTC().Format(time.RFC3339)
		_, err = s.db.Exec(
			"INSERT OR IGNORE INTO content (hash, doc, created_at) VALUES (?, ?, ?)",
			hash, doc.Content, now,
		)
		if err != nil {
			return fmt.Errorf("failed to insert content: %w", err)
		}
	}

	// 4. 插入或更新文档记录
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = time.Now().UTC()
	}
	if doc.ModifiedAt.IsZero() {
		doc.ModifiedAt = time.Now().UTC()
	}

	// 使用REPLACE确保路径唯一性
	_, err = s.db.Exec(`
		INSERT INTO documents (collection, path, title, hash, created_at, modified_at, active)
		VALUES (?, ?, ?, ?, ?, ?, 1)
		ON CONFLICT(collection, path) DO UPDATE SET
			title = excluded.title,
			hash = excluded.hash,
			modified_at = excluded.modified_at,
			active = 1
	`, doc.Collection, doc.Path, doc.Title, hash,
	   doc.CreatedAt.Format(time.RFC3339),
	   doc.ModifiedAt.Format(time.RFC3339))

	if err != nil {
		return fmt.Errorf("failed to insert document: %w", err)
	}

	return nil
}

// GetDocument 获取文档
func (s *Store) GetDocument(id string) (*Document, error) {
	var doc Document
	var createdAt, modifiedAt string

	// 支持两种ID格式：数字ID或哈希
	query := `
		SELECT d.id, d.collection, d.path, d.title, c.doc, d.created_at, d.modified_at
		FROM documents d
		JOIN content c ON c.hash = d.hash
		WHERE (d.id = ? OR d.hash = ? OR d.path = ?) AND d.active = 1
		LIMIT 1
	`

	err := s.db.QueryRow(query, id, id, id).Scan(
		&doc.ID, &doc.Collection, &doc.Path, &doc.Title, &doc.Content,
		&createdAt, &modifiedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("document not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}

	// 解析时间
	doc.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	doc.ModifiedAt, _ = time.Parse(time.RFC3339, modifiedAt)

	return &doc, nil
}

// DeleteDocument 删除文档（软删除）
func (s *Store) DeleteDocument(id string) error {
	result, err := s.db.Exec(`
		UPDATE documents
		SET active = 0
		WHERE (id = ? OR hash = ? OR path = ?) AND active = 1
	`, id, id, id)

	if err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("document not found: %s", id)
	}

	return nil
}

// ListDocuments 列出文档
func (s *Store) ListDocuments(collection string, limit, offset int) ([]Document, error) {
	query := `
		SELECT d.id, d.collection, d.path, d.title, d.created_at, d.modified_at
		FROM documents d
		WHERE d.active = 1
	`

	args := []interface{}{}
	if collection != "" {
		query += " AND d.collection = ?"
		args = append(args, collection)
	}

	query += " ORDER BY d.modified_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list documents: %w", err)
	}
	defer rows.Close()

	var docs []Document
	for rows.Next() {
		var doc Document
		var createdAt, modifiedAt string

		err := rows.Scan(&doc.ID, &doc.Collection, &doc.Path, &doc.Title, &createdAt, &modifiedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan document: %w", err)
		}

		doc.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		doc.ModifiedAt, _ = time.Parse(time.RFC3339, modifiedAt)

		docs = append(docs, doc)
	}

	return docs, nil
}

// GetStatus 获取索引状态
func (s *Store) GetStatus() (Status, error) {
	var status Status
	status.DBPath = s.dbPath

	// 统计总文档数
	err := s.db.QueryRow("SELECT COUNT(*) FROM documents WHERE active = 1").Scan(&status.TotalDocuments)
	if err != nil {
		return status, fmt.Errorf("failed to count documents: %w", err)
	}

	// 统计需要嵌入的文档数
	err = s.db.QueryRow(`
		SELECT COUNT(DISTINCT d.hash)
		FROM documents d
		LEFT JOIN content_vectors v ON d.hash = v.hash AND v.seq = 0
		WHERE d.active = 1 AND v.hash IS NULL
	`).Scan(&status.NeedsEmbedding)
	if err != nil {
		return status, fmt.Errorf("failed to count documents needing embedding: %w", err)
	}

	// 获取集合列表
	rows, err := s.db.Query("SELECT DISTINCT collection FROM documents WHERE active = 1 ORDER BY collection")
	if err != nil {
		return status, fmt.Errorf("failed to list collections: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var collection string
		if err := rows.Scan(&collection); err != nil {
			return status, fmt.Errorf("failed to scan collection: %w", err)
		}
		status.Collections = append(status.Collections, collection)
	}

	return status, nil
}

// computeHash 计算内容的SHA256哈希
func computeHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

// getDocid 获取文档的短ID（哈希前6位）
func getDocid(hash string) string {
	if len(hash) >= 6 {
		return hash[:6]
	}
	return hash
}
